// Package e2e drives the shipped binary and a real Shallnot, built from the pinned
// commit, over a copy of the parcel project: init, enable, an edit, the end-of-turn
// hook, a simulated Claude Code host answering through the real MCP server and
// hooks, and the verdict of `shallnot check`.
package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/nodeinstall"
)

// Locations and names the end-to-end tests use.
const (
	repositoryRoot   = "../../.."
	discpowerDir     = "../.."
	shallnotRelative = ".tools/bin/shallnot"
	buildScript      = "script/shallnot-build"
	memGuardScript   = "script/mem-guard"
	fixtureProject   = "fixtures/prove/parcel"
	binaryName       = "spec-proof-dp"
	buildTimeout     = 5 * time.Minute
	commandTimeout   = 8 * time.Minute
	directoryMode    = 0o755
	fileMode         = 0o644
	shallnotVariable = "SPEC_PROOF_SHALLNOT"
)

var (
	buildOnce   sync.Once
	builtBinary string
	buildError  error
)

// abs resolves a path relative to this package's directory.
func abs(t *testing.T, relative string) string {
	t.Helper()
	absolute, err := filepath.Abs(relative)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}

// guardMemory refuses to run the heavy end-to-end work when the machine is short
// of free memory, the way every heavy batch of the repository does.
func guardMemory(t *testing.T) {
	t.Helper()
	script := filepath.Join(abs(t, repositoryRoot), memGuardScript)
	if output, err := exec.Command("bash", script, "check").CombinedOutput(); err != nil {
		t.Skipf("the memory guard refused the end-to-end run: %s", strings.TrimSpace(string(output)))
	}
}

// shallnotBinary returns the Shallnot built at the pinned commit. The build script
// is a no-op while the binary matches the pin, so it always runs: a binary of
// another commit is never used. The test is skipped, with the reason, when the
// pinned source is unavailable.
func shallnotBinary(t *testing.T) string {
	t.Helper()
	root := abs(t, repositoryRoot)
	command := exec.Command("bash", filepath.Join(root, buildScript))
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Skipf("the pinned Shallnot is not available (%v): %s", err, strings.TrimSpace(string(output)))
	}
	return filepath.Join(root, filepath.FromSlash(shallnotRelative))
}

// productBinary builds the binary the tests drive, once, under nice and with
// bounded parallelism.
func productBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "spec-proof-e2e-bin-")
		if err != nil {
			buildError = err
			return
		}
		builtBinary = filepath.Join(dir, binaryName)
		ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
		defer cancel()
		command := exec.CommandContext(ctx, "nice", "-n", "10", "go", "build", "-p", "1", "-o", builtBinary, "./cmd/"+binaryName)
		command.Dir = abs(t, discpowerDir)
		command.Env = append(os.Environ(), "GOMAXPROCS=2")
		if output, err := command.CombinedOutput(); err != nil {
			buildError = errors.New(err.Error() + ": " + string(output))
		}
	})
	if buildError != nil {
		t.Fatalf("build the binary: %v", buildError)
	}
	return builtBinary
}

// project is a copy of the parcel project with the binary, Shallnot and the state
// of a user's machine around it.
type project struct {
	t        *testing.T
	root     string
	binary   string
	shallnot string
	state    string
	cache    string
	tmp      string
}

func newProject(t *testing.T) *project {
	t.Helper()
	guardMemory(t)
	shallnot := shallnotBinary(t)
	binary := productBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &project{
		t: t, root: filepath.Join(base, "parcel"), binary: binary, shallnot: shallnot,
		state: filepath.Join(base, "state"), cache: filepath.Join(base, "cache"), tmp: filepath.Join(base, "tmp"),
	}
	for _, dir := range []string{p.state, p.cache, p.tmp} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			t.Fatal(err)
		}
	}
	copyTree(t, filepath.Join(abs(t, discpowerDir), fixtureProject), p.root)
	return p
}

// The Node projects the end-to-end tests copy, whose dependencies are those
// of an installed ecosystem fixture of their runner.
const (
	nodeManifest      = "package.json"
	nodeModulesDir    = "node_modules"
	ecosystemFixtures = "fixtures/ecosystems"
)

// nodeRunners are the runners a Node project's dev dependencies name, each
// with the ecosystem fixture whose installed dependencies it shares.
var nodeRunners = []string{"jest", "vitest"}

// linkNodeModules links the node_modules of a copied Node project to those
// installed for the ecosystem fixture of its runner; a tree with no
// package.json, or none that names a runner, is left as it is.
func linkNodeModules(t *testing.T, root string) {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, nodeManifest))
	if errors.Is(err, os.ErrNotExist) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		DevDependencies map[string]string `json:"devDependencies"`
	}
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	for _, runner := range nodeRunners {
		if _, depends := manifest.DevDependencies[runner]; !depends {
			continue
		}
		installed := nodeinstall.Installed(t, filepath.Join(abs(t, discpowerDir), ecosystemFixtures, runner))
		if err := os.Symlink(filepath.Join(installed, nodeModulesDir), filepath.Join(root, nodeModulesDir)); err != nil {
			t.Fatal(err)
		}
		return
	}
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.Walk(from, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if info.IsDir() {
			if info.Name() == "__pycache__" {
				return filepath.SkipDir
			}
			return os.MkdirAll(target, directoryMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, fileMode)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// environment is what a process of the user's machine runs with: the product
// binary and Shallnot on the PATH, a state and cache of their own and the project
// directory the harness exports to its hooks.
func (p *project) environment(extra ...string) []string {
	path := filepath.Dir(p.binary) + string(os.PathListSeparator) + filepath.Dir(p.shallnot) + string(os.PathListSeparator) + os.Getenv("PATH")
	env := append(os.Environ(),
		"PATH="+path, "XDG_STATE_HOME="+p.state, "XDG_CACHE_HOME="+p.cache, "TMPDIR="+p.tmp,
		shallnotVariable+"="+p.shallnot, "CLAUDE_PROJECT_DIR="+p.root, "GOMAXPROCS=2",
	)
	return append(env, extra...)
}

// result is what a command left behind.
type result struct {
	code   int
	stdout string
	stderr string
}

func (r result) output() string { return r.stdout + r.stderr }

// run runs a program in the project with the standard input and returns how it ended.
func (p *project) run(stdin string, name string, args ...string) result {
	p.t.Helper()
	return p.runWith(nil, stdin, name, args...)
}

func (p *project) runWith(extra []string, stdin string, name string, args ...string) result {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", "10", name}, args...)...)
	command.Dir = p.root
	command.Env = p.environment(extra...)
	command.Stdin = strings.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		p.t.Fatalf("%s %v: %v", name, args, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// dp runs the product binary in the project.
func (p *project) dp(stdin string, args ...string) result {
	p.t.Helper()
	return p.run(stdin, p.binary, args...)
}

// sn runs the pinned Shallnot in the project.
func (p *project) sn(stdin string, args ...string) result {
	p.t.Helper()
	return p.run(stdin, p.shallnot, args...)
}

func (p *project) read(relative string) string {
	p.t.Helper()
	content, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(relative)))
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		p.t.Fatal(err)
	}
	return string(content)
}

func (p *project) write(relative, content string) {
	p.t.Helper()
	target := filepath.Join(p.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), fileMode); err != nil {
		p.t.Fatal(err)
	}
}
