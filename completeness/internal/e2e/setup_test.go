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

	"github.com/RachidChabane/musthold/completeness/internal/testkit"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest/claudecode"
)

const (
	repositoryDir  = "../../.."
	moduleDir      = "completeness"
	memGuardScript = "script/mem-guard"
	binaryName     = "musthold-completeness"
	pluginName     = "musthold-completeness"
	niceLevel      = "10"
	buildTimeout   = 5 * time.Minute
	commandTimeout = 5 * time.Minute
	directoryMode  = 0o755

	exitClean    = 0
	exitAwaiting = 3
	maxWaves     = 12

	shallnotVariable = "MUSTHOLD_SHALLNOT"
	reportVariable   = "SHALLNOT_RECORDED_REPORT"
	stateVariable    = "XDG_STATE_HOME"
	cacheVariable    = "XDG_CACHE_HOME"
	tmpVariable      = "TMPDIR"
	projectVariable  = "CLAUDE_PROJECT_DIR"
	procsVariable    = "GOMAXPROCS=2"

	stateDirectory = "musthold"
	moduleState    = "completeness"
	hostDirectory  = "host"
	journalName    = "journal.jsonl"
	cacheRelative  = ".musthold/cache/completeness"
	gateConfig     = "shallnot.yaml"
	requireKey     = "require:"
)

var (
	buildOnce  sync.Once
	binaryPath string
	buildErr   error
)

// identity is the plugin of the module and its executor agents, from which the
// Claude harness spells the names of the executor tools.
var identity = harness.Identity{Plugin: pluginName, Agents: []hostproto.Agent{hostproto.AgentDecider}}

// builtBinary builds the module's binary once, throttled: one compiler job at a
// time under nice.
func builtBinary(t *testing.T) string {
	t.Helper()
	buildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "musthold-completeness-bin-")
		if err != nil {
			buildErr = err
			return
		}
		binaryPath = filepath.Join(dir, binaryName)
		root, err := filepath.Abs(filepath.Join(repositoryDir, moduleDir))
		if err != nil {
			buildErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), buildTimeout)
		defer cancel()
		command := exec.CommandContext(ctx, "nice", "-n", niceLevel, "go", "build", "-C", root, "-p", "1", "-o", binaryPath, "./cmd/"+binaryName)
		command.Env = append(os.Environ(), procsVariable)
		if output, err := command.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("%w\n%s", err, output)
		}
	})
	if buildErr != nil {
		t.Fatalf("the module cannot be built: %v", buildErr)
	}
	return binaryPath
}

// requireMemory skips the test, saying why, when the memory guard refuses a run.
func requireMemory(t *testing.T) {
	t.Helper()
	root, err := filepath.Abs(repositoryDir)
	if err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("bash", filepath.Join(root, memGuardScript), "check").CombinedOutput(); err != nil {
		t.Skipf("the memory guard refused the run: %s", strings.TrimSpace(string(output)))
	}
}

// result is what one run of the binary left behind.
type result struct {
	code           int
	stdout, stderr string
}

func (r result) output() string { return r.stdout + r.stderr }

// project is a fixture project in its own repository, with the built binary, the
// recorded Shallnot and the state and cache of a user's machine around it.
type project struct {
	t        *testing.T
	root     string
	binary   string
	shallnot string
	report   string
	state    string
	cache    string
	tmp      string
}

func newProject(t *testing.T) *project {
	t.Helper()
	requireMemory(t)
	binary := builtBinary(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	shallnot, err := filepath.Abs(filepath.Join(testkit.FixturesDir(), testkit.RecordedShallnot))
	if err != nil {
		t.Fatal(err)
	}
	p := &project{
		t: t, root: testkit.Materialize(t, testkit.GoLedger), binary: binary, shallnot: shallnot,
		report: testkit.ReportPath(testkit.GoLedger),
		state:  filepath.Join(base, "state"), cache: filepath.Join(base, "cache"), tmp: filepath.Join(base, "tmp"),
	}
	for _, dir := range []string{p.state, p.cache, p.tmp} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			t.Fatal(err)
		}
	}
	return p
}

// environment is what a process of the user's machine runs with: a state and a
// cache of their own, Shallnot and the project directory the harness exports to
// its hooks.
func (p *project) environment() []string {
	return append(os.Environ(),
		stateVariable+"="+p.state, cacheVariable+"="+p.cache, tmpVariable+"="+p.tmp,
		shallnotVariable+"="+p.shallnot, reportVariable+"="+p.report, projectVariable+"="+p.root, procsVariable,
	)
}

func (p *project) command(name string, args ...string) *exec.Cmd {
	command := exec.Command("nice", append([]string{"-n", niceLevel, name}, args...)...)
	command.Dir, command.Env = p.root, p.environment()
	return command
}

// run runs the module's binary in the project.
func (p *project) run(stdin string, args ...string) result {
	p.t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", niceLevel, p.binary}, args...)...)
	command.Dir, command.Env = p.root, p.environment()
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
		p.t.Fatalf("%s %v: %v", binaryName, args, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

// mustRun runs the binary and fails unless it exits cleanly.
func (p *project) mustRun(args ...string) result {
	p.t.Helper()
	r := p.run("", args...)
	if r.code != exitClean {
		p.t.Fatalf("%v exited %d:\n%s", args, r.code, r.output())
	}
	return r
}

// Run implements hostprototest.ModuleRunner: it fails when the command does.
func (p *project) Run(args []string, stdin []byte) ([]byte, error) {
	r := p.run(string(stdin), args...)
	if r.code != exitClean {
		return []byte(r.stdout), fmt.Errorf("exit %d: %s", r.code, strings.TrimSpace(r.stderr))
	}
	return []byte(r.stdout), nil
}

// moduleOf is the module under test as the simulated Claude Code runs it: its
// binary in the project, its executor server and the tool names of the Claude
// harness.
type moduleOf struct{ p *project }

func (m moduleOf) Run(stdin string, args ...string) claudecode.Result {
	r := m.p.run(stdin, args...)
	return claudecode.Result{Code: r.code, Stdout: r.stdout, Stderr: r.stderr}
}

func (m moduleOf) Server() *exec.Cmd {
	return m.p.command(m.p.binary, "mcp", "--role", "executor", "--harness", "claude")
}

func (m moduleOf) ToolName(tool string) string { return harness.Claude(identity).ToolName(tool) }

// claudeHost starts the executor server and returns the simulated Claude Code
// that serves it.
func (p *project) claudeHost() *claudecode.Host {
	return claudecode.NewHost(p.t, moduleOf{p}, p.tmp)
}

// pending lists the tickets that wait for an answer.
func (p *project) pending() hostproto.Listing {
	p.t.Helper()
	r := p.mustRun(hostproto.ListArgs(hostproto.SessionProve)...)
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(r.stdout), &listing); err != nil {
		p.t.Fatal(err)
	}
	return listing
}

// journal reads the events of the host journal of the project.
func (p *project) journal() []hostproto.Event {
	p.t.Helper()
	matches, err := filepath.Glob(filepath.Join(p.state, stateDirectory, moduleState, "*", hostDirectory, journalName))
	if err != nil || len(matches) != 1 {
		p.t.Fatalf("the journal of the module: %v, %v", matches, err)
	}
	content, err := os.ReadFile(matches[0])
	if err != nil {
		p.t.Fatal(err)
	}
	var events []hostproto.Event
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var record hostproto.Record
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			p.t.Fatalf("a journal record: %v", err)
		}
		events = append(events, record.Event)
	}
	return events
}

// statements lists the Statement files the module wrote below its proof cache.
func (p *project) statements() []string {
	p.t.Helper()
	var found []string
	root := filepath.Join(p.root, filepath.FromSlash(cacheRelative))
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			found = append(found, path)
		}
		return err
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		p.t.Fatal(err)
	}
	return found
}

func countKind(events []hostproto.Event, kind hostproto.EventKind) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func mustJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
