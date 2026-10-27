package runs_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	repoFromHere = "../../../../.."
	testToken    = "sk-ant-oat01-test-token-that-must-stay-in-the-session"
	testPin      = "2.1.295"
	dockerFront  = "/repo/experiments/loop-1/script/docker"
	testImage    = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	testClaude   = "/usr/local/bin/claude"
	testModel    = "claude-sonnet-5-5"
	recording    = "../../../../../demo/loop/recording"
	allowedHost  = "api.anthropic.com"
)

// engine stands for the study's engine: it records every docker command, runs
// the git and shell steps on the host, plays the product's binaries, and answers
// the engine's own queries.
type engine struct {
	mutex sync.Mutex
	calls [][]string
	// version is what the CLI reports; state what inspecting the session's
	// container answers; failInit fails `shallnot init`.
	version  string
	state    string
	failInit bool
}

func newEngine() *engine { return &engine{version: testPin, state: "0 false"} }

func (e *engine) argvs() [][]string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return slices.Clone(e.calls)
}

func (e *engine) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	e.mutex.Lock()
	e.calls = append(e.calls, spec.Argv)
	e.mutex.Unlock()
	argv := spec.Argv
	switch {
	case strings.HasSuffix(argv[0], "script/mem-guard"):
		return canary.Output{}, nil
	case argv[0] != dockerFront:
		return e.play(ctx, spec.Dir, hostPath(spec.Env), argv)
	case slices.Equal(argv[1:3], []string{"image", "inspect"}):
		return canary.Output{Stdout: testImage + "\n"}, nil
	case argv[1] == "inspect":
		return canary.Output{Stdout: e.state + "\n"}, nil
	case argv[1] == "run" && slices.Contains(argv, "--detach"):
		return canary.Output{}, writeProxyLog(argv)
	case argv[1] == "run" && slices.Contains(argv, "--rm"):
		return e.step(ctx, argv)
	}
	return canary.Output{}, nil
}

// writeProxyLog writes the log the proxy would keep: one allowed CONNECT.
func writeProxyLog(argv []string) error {
	index := slices.Index(argv, "--log")
	line := fmt.Sprintf(`{"time":"2026-10-28T22:00:00Z","method":"CONNECT","target":"%s:443","decision":"allowed"}`+"\n", allowedHost)
	return os.WriteFile(argv[index+1], []byte(line), 0o644)
}

var variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// step runs the command of a step container: what follows `env -i` and its
// variables, in the container's working directory.
func (e *engine) step(ctx context.Context, argv []string) (canary.Output, error) {
	dir := argv[slices.Index(argv, "--workdir")+1]
	at := slices.Index(argv, "/usr/bin/env") + 2
	var env []string
	for ; at < len(argv) && variablePattern.MatchString(argv[at]); at++ {
		env = append(env, argv[at])
	}
	return e.play(ctx, dir, hostPath(env), argv[at:])
}

// hostPath gives a command's environment the host's PATH in place of the image's.
func hostPath(env []string) []string {
	kept := slices.DeleteFunc(slices.Clone(env), func(pair string) bool { return strings.HasPrefix(pair, "PATH=") })
	return append(kept, "PATH="+os.Getenv("PATH"))
}

// play runs git and the shell for real and plays the product's binaries.
func (e *engine) play(ctx context.Context, dir string, env, argv []string) (canary.Output, error) {
	name := filepath.Base(argv[0])
	switch {
	case name == "git" || name == "sh":
		return host(ctx, dir, env, argv)
	case name == "claude":
		return canary.Output{Stdout: e.version + " (Claude Code)\n"}, nil
	case name == "sha256sum":
		return canary.Output{Stdout: strings.Repeat("c", 64) + "  " + argv[1] + "\n"}, nil
	case name == "python3":
		return canary.Output{Stdout: "Python 3.13.14\n"}, nil
	case name == "shallnot":
		return e.shallnot(dir, argv[1:])
	case strings.HasPrefix(name, "musthold-"):
		return module(dir, name, argv[1:])
	}
	return canary.Output{ExitCode: 127, Stderr: name + ": not found"}, nil
}

func host(ctx context.Context, dir string, env, argv []string) (canary.Output, error) {
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Dir, command.Env = dir, env
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	output := canary.Output{Stdout: stdout.String(), Stderr: stderr.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		output.ExitCode, err = exit.ExitCode(), nil
	}
	return output, err
}

// The files the played `shallnot init` writes; the hook settings name the
// project's absolute path, as an equipped project's files may.
const (
	initialShallnot = "version: 1\nspecs:\n  - specs\nresults:\n  - test-results/*.xml\n"
	agentsBlock     = "# Agents\n\nRun the gate before you end a turn.\n"
)

func (e *engine) shallnot(dir string, args []string) (canary.Output, error) {
	switch args[0] {
	case "init":
		if e.failInit {
			return canary.Output{ExitCode: 1, Stderr: "init failed"}, nil
		}
		settings := fmt.Sprintf(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"shallnot hook claude-stop","cwd":%q}]}]}}`, dir)
		return canary.Output{}, writeAll(dir, map[string]string{"shallnot.yaml": initialShallnot, "AGENTS.md": agentsBlock, ".claude/settings.json": settings})
	case "--version":
		return canary.Output{Stdout: "shallnot 0.5.1\n"}, nil
	case "gate":
		files, _ := os.ReadFile(filepath.Join(dir, "shallnot.yaml"))
		report := fmt.Sprintf(`{"verdict":"pass","exit_code":0,"findings":[],"config":%q}`, files)
		return canary.Output{}, os.WriteFile(args[2], []byte(report), 0o644)
	}
	return canary.Output{}, nil
}

// module plays `init` and `enable` of a module: enable adds a required proof's
// kind to proofs.require as the shared installer does.
func module(dir, name string, args []string) (canary.Output, error) {
	if args[0] != "enable" {
		return canary.Output{Stdout: name + " 0.1.1\n"}, nil
	}
	path := filepath.Join(dir, "shallnot.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		return canary.Output{}, err
	}
	text := string(content)
	switch name {
	case "musthold-dp":
		text += "proofs:\n  require:\n    - discriminating-power\n  trust: local\n  code:\n    - boxoffice\n"
	case "musthold-alignment":
		text = strings.Replace(text, "    - discriminating-power\n", "    - discriminating-power\n    - alignment\n", 1)
	}
	return canary.Output{}, os.WriteFile(path, []byte(text), 0o644)
}

func writeAll(dir string, files map[string]string) error {
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// session stands for the session: it writes the stream it is given line by line
// up to the first line the spec stops at, edits the project as the agent would,
// and, when asked, prints the token to its error stream.
type session struct {
	specs  []canary.SessionSpec
	stream []byte
	edit   func(project string) error
	leak   bool
	killed string
	err    error
}

func (s *session) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	s.specs = append(s.specs, spec)
	if s.err != nil {
		return canary.SessionOutcome{}, s.err
	}
	outcome := canary.SessionOutcome{Killed: s.killed}
	var written, times []byte
	for number, line := range bytes.SplitAfter(s.stream, []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		written = append(written, line...)
		times = append(times, fmt.Sprintf(`{"line":%d,"ms":%d}`+"\n", number+1, (number+1)*10)...)
		if why := spec.Stops(bytes.TrimSpace(line)); why != "" {
			outcome.Killed = why
			break
		}
	}
	for path, content := range map[string][]byte{spec.StdoutPath: written, spec.TimesPath: times} {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return outcome, err
		}
	}
	stderr := "session ended\n"
	if s.leak {
		stderr = "env: " + launch.OAuthTokenVariable + "=" + string(spec.Stdin)
	}
	if err := os.WriteFile(spec.StderrPath, []byte(stderr), 0o644); err != nil {
		return outcome, err
	}
	if s.edit != nil {
		return outcome, s.edit(projectOf(spec))
	}
	return outcome, nil
}

// projectOf is the session's working directory, from its docker command.
func projectOf(spec canary.SessionSpec) string {
	return spec.Argv[slices.Index(spec.Argv, "--workdir")+1]
}

// tickingClock gives each reading a second after the one before.
type tickingClock struct {
	mutex sync.Mutex
	at    time.Time
}

func (c *tickingClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := c.at
	c.at = c.at.Add(time.Second)
	return now
}

func (c *tickingClock) set(at time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.at = at
}

var start = time.Date(2026, time.November, 2, 22, 0, 0, 0, time.UTC)

type credential struct{ reads *int }

func (credential) Source() string                        { return canary.SourceKeychain }
func (credential) Present(context.Context) (bool, error) { return true, nil }
func (c credential) Token(context.Context) (string, error) {
	*c.reads++
	return testToken, nil
}

type noLock struct{}

func (noLock) Acquire(string) (canary.Lock, error) { return noLock{}, nil }
func (noLock) Release()                            {}

// stamps records the entries handed to the timestamping writer.
type stamps struct{ lines []string }

func (s *stamps) Stamp(_ context.Context, _ study.Deviation, line string) error {
	s.lines = append(s.lines, line)
	return nil
}

// world is a work directory, the study's configuration and inputs, and the
// stand-ins of the engine and the session.
type world struct {
	repo, work, product string
	engine              *engine
	session             *session
	clock               *tickingClock
	stamps              *stamps
	reads               *int
	stdout              *bytes.Buffer
	runner              runs.Runner
	config              config.Config
	inputs              study.Inputs
	options             app.Options
}

func newWorld(t *testing.T) *world {
	t.Helper()
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, runs.DefaultConfig))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Repetitions = 1
	reads := 0
	w := &world{
		repo: repo, work: tempDir(t), product: tempDir(t), engine: newEngine(), session: &session{stream: shortStream(t, resultLine("success", false, "done"))},
		clock: &tickingClock{at: start}, stamps: &stamps{}, reads: &reads, stdout: &bytes.Buffer{}, config: parsed,
	}
	appRunner := app.Runner{
		Clock: w.clock, Window: window.Nightly(), Processes: w.engine, Sessions: w.session, Files: adapters.OSFiles{}, Locker: noLock{},
		Credentials: []canary.ClaudeCredential{credential{reads: &reads}},
		Env:         []string{"HOME=/Users/someone", "PATH=/usr/bin", launch.OAuthTokenVariable + "=" + testToken}, Stdout: w.stdout,
	}
	w.runner = runs.Runner{App: appRunner, Tree: runs.OSTree{},
		Deviations: runs.DeviationLog{Files: adapters.OSFiles{}, Path: filepath.Join(w.work, study.DeviationsFile), Stamper: w.stamps}}
	w.options = app.Options{
		RepoRoot: repo, Config: parsed, Work: w.work, Product: w.product, Image: testImage, Docker: []string{dockerFront},
		UID: 501, GID: 20, ClaudePin: testPin, Profile: runs.StudyProfile(testClaude),
	}
	w.inputs = w.freeze(t)
	w.engine.calls = nil
	return w
}

// freeze computes the inputs freeze as resolve would: each task's start commits
// in each arm, prepared in a directory of its own.
func (w *world) freeze(t *testing.T) study.Inputs {
	t.Helper()
	inputs := study.Inputs{
		Order: runs.Order(w.config), EgressAllowlist: []string{allowedHost}, QuotaPatterns: runs.QuotaPatterns(),
		ClaudeCode: study.ClaudeCode{Version: testPin}, Images: study.Images{Built: testImage},
	}
	equipper := runs.Equipper{Options: w.options, Processes: w.engine, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	for _, task := range w.config.Tasks {
		entry := study.TaskInputs{Name: task.Name, EquippedTree: map[string]string{}}
		for _, arm := range w.config.Arms {
			equipped, err := equipper.Equip(context.Background(), filepath.Join(tempDir(t), "resolve-"+string(arm)), config.Run{Task: task, Arm: arm, Repetition: 1})
			if err != nil {
				t.Fatal(err)
			}
			entry.BaseCommit, entry.EquippedTree[string(arm)] = equipped.BaseCommit, equipped.Tree
		}
		inputs.Tasks = append(inputs.Tasks, entry)
	}
	return inputs
}

func (w *world) invocation() runs.Invocation {
	return runs.Invocation{Plan: runs.Plan{Options: w.options, Inputs: w.inputs}, Config: w.config, Layout: study.Layout{Root: w.work}}
}

func (w *world) batch() runs.Batch { return runs.Batch{Runner: w.runner, Stdout: w.stdout} }

// first is the first run of the order.
func (w *world) first() config.Run { return w.config.Runs()[0] }

// tempDir is a fresh directory whose path holds no character a mount refuses,
// with symbolic links resolved.
func tempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "loop1-runs-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return resolved
}

func recordedStream(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(recording, record.StreamFile))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// initLine is the init event of the recorded stream.
func initLine(t *testing.T) string {
	t.Helper()
	first, _, _ := strings.Cut(string(recordedStream(t)), "\n")
	for _, line := range strings.Split(string(recordedStream(t)), "\n") {
		if strings.Contains(line, `"subtype":"init"`) {
			return line + "\n"
		}
	}
	return first + "\n"
}

// shortStream is an init event, one assistant message and the result event given.
func shortStream(t *testing.T, result string) []byte {
	t.Helper()
	assistant := `{"type":"assistant","message":{"model":"` + testModel + `","content":[{"type":"text","text":"done"}]}}` + "\n"
	return []byte(initLine(t) + assistant + result + "\n")
}

func read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
