package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest/claudecode"
)

// Locations and names of the project end-to-end tests.
const (
	parcelFixture      = "discpower/fixtures/prove/parcel"
	shallnotScript     = "script/shallnot-build"
	memGuardScript     = "script/mem-guard"
	shallnotRelative   = ".tools/bin/shallnot"
	dpBinary           = "musthold-dp"
	alignmentBinary    = "musthold-alignment"
	completenessBinary = "musthold-completeness"
	s1Binary           = "musthold-s1"
	dispatcherBinary   = "musthold"
	alignmentTags      = "grammar_subset,grammar_subset_python,grammar_subset_javascript,grammar_subset_typescript,grammar_subset_tsx,grammar_subset_java,grammar_subset_kotlin"
	buildTimeout       = 5 * time.Minute
	commandTimeout     = 8 * time.Minute
	niceLevel          = "10"
	shallnotVariable   = "MUSTHOLD_SHALLNOT"
	reportFile         = "report.json"
	verifiedPath       = ".musthold/verified"
	cachePath          = ".musthold/cache"
	verifiesTag        = "verifies"
)

// dpIdentity is the plugin of the discriminating-power module and its executor
// agents, from which the harness spells the names of the executor tools.
var dpIdentity = harness.Identity{Plugin: "musthold-dp", Agents: hostproto.Agents()}

// alignmentIdentity is the plugin of the alignment module and its executor agents.
var alignmentIdentity = harness.Identity{Plugin: "musthold-alignment", Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}}

var (
	productOnce sync.Once
	productDir  string
	productErr  error
)

// productBinaries builds the binary of every module that ships one, once,
// throttled: one compiler job at a time under nice.
func productBinaries(t *testing.T) string {
	t.Helper()
	productOnce.Do(func() {
		productDir, productErr = os.MkdirTemp("", "musthold-chain-bin-")
		if productErr != nil {
			return
		}
		builds := []struct{ module, binary, tags string }{
			{"discpower", dpBinary, ""}, {"alignment", alignmentBinary, alignmentTags}, {"completeness", completenessBinary, ""},
			{"verify", verifyBinary, ""}, {"ledger", ledgerBinary, ""}, {"s1relay", s1Binary, ""},
			{"umbrella", dispatcherBinary, ""},
		}
		for _, build := range builds {
			args := []string{"-n", niceLevel, "go", "build", "-C", filepath.Join(repositoryDir, build.module), "-p", "1", "-o", filepath.Join(productDir, build.binary)}
			if build.tags != "" {
				args = append(args, "-tags", build.tags)
			}
			ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
			command := exec.CommandContext(ctx, "nice", append(args, "./cmd/"+build.binary)...)
			command.Env = append(os.Environ(), "GOMAXPROCS=2")
			output, err := command.CombinedOutput()
			cancel()
			if err != nil {
				productErr = errors.New(build.module + ": " + err.Error() + "\n" + string(output))
				return
			}
		}
	})
	if productErr != nil {
		t.Fatalf("build the binaries: %v", productErr)
	}
	return productDir
}

// requireMachine skips the test, with the reason, when the machine is short of free
// memory, and when the pinned Shallnot cannot be built. The skip is recorded under
// one subtest per requirement the test verifies, tagged as the requirement is, so
// that the gate reports each requirement as skipped and never drops it. It returns
// the Shallnot.
func requireMachine(t *testing.T, requirements []string) string {
	t.Helper()
	root := requireMemory(t, requirements)
	command := exec.Command("bash", filepath.Join(root, shallnotScript))
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		skipRequirements(t, requirements, fmt.Sprintf("the pinned Shallnot cannot be built from $SHALLNOT_SRC (%v): %s", err, strings.TrimSpace(string(output))))
	}
	return filepath.Join(root, filepath.FromSlash(shallnotRelative))
}

// requireMemory skips the test, as requireMachine does, when the memory guard
// refuses a heavy run. It returns the absolute root of the repository.
func requireMemory(t *testing.T, requirements []string) string {
	t.Helper()
	root, err := filepath.Abs(repositoryDir)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("bash", filepath.Join(root, memGuardScript), "check").CombinedOutput(); err != nil {
		skipRequirements(t, requirements, "the memory guard refused the run: "+strings.TrimSpace(string(output)))
	}
	return root
}

// skipRequirements skips the test with the reason, recording the skip under one
// subtest per requirement so that the gate reports each as skipped.
func skipRequirements(t *testing.T, requirements []string, reason string) {
	t.Helper()
	for _, requirement := range requirements {
		t.Run("is not checked ["+verifiesTag+" "+requirement+"]", func(t *testing.T) { t.Skip(reason) })
	}
	t.Skip(reason)
}

// result is what a command left behind.
type result struct {
	code           int
	stdout, stderr string
}

func (r result) output() string { return r.stdout + r.stderr }

// parcelProject is a copy of the parcel pytest project with the binaries of the
// modules, the verifier, Shallnot and the state of a user's machine around it.
type parcelProject struct {
	t        *testing.T
	root     string
	bin      string
	shallnot string
	state    string
	cache    string
	tmp      string
}

func newParcelProject(t *testing.T, requirements []string) *parcelProject {
	t.Helper()
	shallnot := requireMachine(t, requirements)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p := &parcelProject{
		t: t, root: filepath.Join(base, "parcel"), bin: productBinaries(t), shallnot: shallnot,
		state: filepath.Join(base, "state"), cache: filepath.Join(base, "cache"), tmp: filepath.Join(base, "tmp"),
	}
	for _, dir := range []string{p.state, p.cache, p.tmp} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			t.Fatal(err)
		}
	}
	copyProject(t, filepath.Join(repositoryDir, filepath.FromSlash(parcelFixture)), p.root)
	return p
}

// copyAs copies the project as it stands, state of the host included, to a new
// project, so that two hosts can answer the same proofs differently.
func (p *parcelProject) copyAs(name string) *parcelProject {
	p.t.Helper()
	base, err := filepath.EvalSymlinks(p.t.TempDir())
	if err != nil {
		p.t.Fatal(err)
	}
	other := &parcelProject{
		t: p.t, root: filepath.Join(base, "parcel"), bin: p.bin, shallnot: p.shallnot,
		state: filepath.Join(base, "state"), cache: filepath.Join(base, "cache"), tmp: filepath.Join(base, "tmp"),
	}
	for _, dir := range []string{other.state, other.cache, other.tmp} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			p.t.Fatal(err)
		}
	}
	copyProject(p.t, p.root, other.root)
	copyProject(p.t, p.state, other.state)
	return other
}

func copyProject(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			if entry.Name() == "__pycache__" || entry.Name() == ".pytest_cache" {
				return fs.SkipDir
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

// environment is what a process of the user's machine runs with: the binaries and
// Shallnot on the PATH, a state and cache of their own and the project directory the
// harness exports to its hooks.
func (p *parcelProject) environment() []string {
	path := p.bin + string(os.PathListSeparator) + filepath.Dir(p.shallnot) + string(os.PathListSeparator) + os.Getenv("PATH")
	return append(os.Environ(),
		"PATH="+path, "XDG_STATE_HOME="+p.state, "XDG_CACHE_HOME="+p.cache, "TMPDIR="+p.tmp,
		shallnotVariable+"="+p.shallnot, "CLAUDE_PROJECT_DIR="+p.root, "GOMAXPROCS=2",
	)
}

func (p *parcelProject) run(stdin, name string, args ...string) result {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", niceLevel, name}, args...)...)
	p.withEnvironment(command).Stdin = strings.NewReader(stdin)
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

// dp runs the discriminating-power binary, alignment the alignment binary, verify
// the verifier and sn Shallnot, in the project.
func (p *parcelProject) dp(stdin string, args ...string) result {
	p.t.Helper()
	return p.run(stdin, filepath.Join(p.bin, dpBinary), args...)
}

func (p *parcelProject) alignment(stdin string, args ...string) result {
	p.t.Helper()
	return p.run(stdin, filepath.Join(p.bin, alignmentBinary), args...)
}

func (p *parcelProject) verify(args ...string) result {
	p.t.Helper()
	return p.run("", filepath.Join(p.bin, verifyBinary), args...)
}

func (p *parcelProject) sn(stdin string, args ...string) result {
	p.t.Helper()
	return p.run(stdin, p.shallnot, args...)
}

func (p *parcelProject) read(relative string) string {
	p.t.Helper()
	content, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(relative)))
	if errors.Is(err, fs.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		p.t.Fatal(err)
	}
	return string(content)
}

func (p *parcelProject) write(relative, content string) {
	p.t.Helper()
	target := filepath.Join(p.root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
		p.t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), fileMode); err != nil {
		p.t.Fatal(err)
	}
}

// dpModule is the discriminating-power module as the simulated Claude Code runs it.
type dpModule struct{ p *parcelProject }

func (m dpModule) Run(stdin string, args ...string) claudecode.Result {
	r := m.p.dp(stdin, args...)
	return claudecode.Result{Code: r.code, Stdout: r.stdout, Stderr: r.stderr}
}

func (m dpModule) Server() *exec.Cmd {
	return m.p.withEnvironment(exec.Command(filepath.Join(m.p.bin, dpBinary), "mcp", "--role", "executor", "--harness", "claude"))
}

func (m dpModule) ToolName(tool string) string { return harness.Claude(dpIdentity).ToolName(tool) }

// alignmentModule is the alignment module as the simulated Claude Code runs it.
type alignmentModule struct{ p *parcelProject }

func (m alignmentModule) Run(stdin string, args ...string) claudecode.Result {
	r := m.p.alignment(stdin, args...)
	return claudecode.Result{Code: r.code, Stdout: r.stdout, Stderr: r.stderr}
}

func (m alignmentModule) Server() *exec.Cmd {
	return m.p.withEnvironment(exec.Command(filepath.Join(m.p.bin, alignmentBinary), "mcp", "--role", "executor", "--harness", "claude"))
}

func (m alignmentModule) ToolName(tool string) string {
	return harness.Claude(alignmentIdentity).ToolName(tool)
}

// withEnvironment makes a command run in the project's environment.
func (p *parcelProject) withEnvironment(command *exec.Cmd) *exec.Cmd {
	command.Dir, command.Env = p.root, p.environment()
	return command
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// claudeHostOf starts the executor server of the discriminating-power module and
// returns the simulated Claude Code that serves it.
func claudeHostOf(p *parcelProject) *claudecode.Host {
	return claudecode.NewHost(p.t, dpModule{p}, p.tmp)
}

// claudeAlignmentHostOf starts the executor server of the alignment module and
// returns the simulated Claude Code that serves it.
func claudeAlignmentHostOf(p *parcelProject) *claudecode.Host {
	return claudecode.NewHost(p.t, alignmentModule{p}, p.tmp)
}

func (p *parcelProject) readBytes(relative string) []byte {
	p.t.Helper()
	content, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(relative)))
	if err != nil {
		p.t.Fatal(err)
	}
	return content
}
