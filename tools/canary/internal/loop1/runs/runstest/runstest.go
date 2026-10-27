// Package runstest holds the stand-ins the tests of the run and canary verbs share:
// the study's engine, which runs git and the shell of each step on the host and
// plays the product's binaries; the session, which writes a recorded stream; and a
// world that wires them to the attempts of the run verb with the study's
// configuration and an inputs freeze computed as resolve computes it. Nothing in it
// calls a model or the container engine.
package runstest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Values the stand-ins answer with.
const (
	Token       = "sk-ant-oat01-test-token-that-must-stay-in-the-session"
	Pin         = "2.1.295"
	DockerFront = "/repo/experiments/loop-1/script/docker"
	Image       = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
	Claude      = "/usr/local/bin/claude"
	Model       = "claude-sonnet-5-5"
	AllowedHost = "api.anthropic.com"
	recording   = "demo/loop/recording"
	repoMarker  = "experiments/loop-1/loop-1.json"
)

// Start is when every world's clock starts.
var Start = time.Date(2026, time.November, 2, 22, 0, 0, 0, time.UTC)

// RepoRoot is the repository root, found above the test's directory.
func RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for ; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, repoMarker)); err == nil {
			return dir
		}
	}
	t.Fatal("no repository root above the test's directory")
	return ""
}

// Engine stands for the study's engine: it records every docker command, runs
// the git and shell steps on the host, plays the product's binaries, and answers
// the engine's own queries.
type Engine struct {
	mutex sync.Mutex
	calls [][]string
	// Version is what the CLI reports; State what inspecting the session's
	// container answers; FailInit fails `shallnot init`; ProxyLog the entries the
	// proxy logs, one per line.
	Version  string
	State    string
	FailInit bool
	ProxyLog []string
	// Down makes every docker command fail as a stopped engine does; GateFails
	// makes the acceptance gate's step fail to run, as a step past its limit does.
	Down      bool
	GateFails bool
}

// ErrStepLimit is the error of a step the engine ran past its limit.
var ErrStepLimit = errors.New("the step ran past its limit")

// NewEngine is an engine whose CLI is the pin and whose proxy forwards one
// CONNECT to the model's endpoint.
func NewEngine() *Engine {
	return &Engine{Version: Pin, State: "0 false", ProxyLog: []string{ProxyEntry(AllowedHost+":443", "allowed")}}
}

// ProxyEntry is one line of the proxy's log.
func ProxyEntry(target, decision string) string {
	return fmt.Sprintf(`{"time":"2026-11-02T22:00:00Z","method":"CONNECT","target":%q,"decision":%q}`, target, decision)
}

// Argvs are the commands the engine received, in order.
func (e *Engine) Argvs() [][]string {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	return slices.Clone(e.calls)
}

// Forget drops the commands received so far.
func (e *Engine) Forget() {
	e.mutex.Lock()
	defer e.mutex.Unlock()
	e.calls = nil
}

// Run implements canary.Processes: a docker command, or a command the box port
// runs in the image.
func (e *Engine) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	e.mutex.Lock()
	e.calls = append(e.calls, spec.Argv)
	e.mutex.Unlock()
	argv := spec.Argv
	switch {
	case strings.HasSuffix(argv[0], "script/mem-guard"):
		return canary.Output{}, nil
	case argv[0] != DockerFront:
		return e.play(ctx, spec.Dir, hostPath(spec.Env), argv)
	case e.Down:
		return canary.Output{ExitCode: 1, Stderr: "Cannot connect to the Docker daemon"}, nil
	case argv[1] == "version":
		return canary.Output{Stdout: "28.5.1\n"}, nil
	case slices.Equal(argv[1:3], []string{"image", "inspect"}):
		return canary.Output{Stdout: Image + "\n"}, nil
	case argv[1] == "inspect":
		return canary.Output{Stdout: e.State + "\n"}, nil
	case argv[1] == "run" && slices.Contains(argv, "--detach"):
		return canary.Output{}, e.writeProxyLog(argv)
	case argv[1] == "run" && slices.Contains(argv, "--rm"):
		return e.step(ctx, argv)
	}
	return canary.Output{}, nil
}

func (e *Engine) writeProxyLog(argv []string) error {
	index := slices.Index(argv, "--log")
	return os.WriteFile(argv[index+1], []byte(strings.Join(e.ProxyLog, "\n")+"\n"), 0o644)
}

var variablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

// step runs the command of a step container: what follows `env -i` and its
// variables, in the container's working directory.
func (e *Engine) step(ctx context.Context, argv []string) (canary.Output, error) {
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
func (e *Engine) play(ctx context.Context, dir string, env, argv []string) (canary.Output, error) {
	name := filepath.Base(argv[0])
	switch {
	case name == "git" || name == "sh":
		return host(ctx, dir, env, argv)
	case name == "claude":
		return canary.Output{Stdout: e.Version + " (Claude Code)\n"}, nil
	case name == "sha256sum":
		return canary.Output{Stdout: strings.Repeat("c", 64) + "  " + argv[1] + "\n"}, nil
	case name == "python3":
		return canary.Output{Stdout: "Python 3.13.14\n"}, nil
	case name == "pytest":
		return canary.Output{Stdout: "pytest 8.4.2\n"}, nil
	case name == "uv":
		return canary.Output{Stdout: "uv 0.11.23\n"}, nil
	case name == "node":
		return canary.Output{Stdout: "v24.11.0\n"}, nil
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

func (e *Engine) shallnot(dir string, args []string) (canary.Output, error) {
	switch args[0] {
	case "init":
		if e.FailInit {
			return canary.Output{ExitCode: 1, Stderr: "init failed"}, nil
		}
		settings := fmt.Sprintf(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"shallnot hook claude-stop","cwd":%q}]}]}}`, dir)
		return canary.Output{}, WriteAll(dir, map[string]string{"shallnot.yaml": initialShallnot, "AGENTS.md": agentsBlock, ".claude/settings.json": settings})
	case "--version":
		return canary.Output{Stdout: "shallnot 0.5.1\n"}, nil
	case "gate":
		if e.GateFails && filepath.Base(args[2]) == record.FinalGateFile {
			return canary.Output{}, ErrStepLimit
		}
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

// WriteAll writes files below a directory, creating their directories.
func WriteAll(dir string, files map[string]string) error {
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

// Session stands for the session: it writes the stream it is given line by line
// up to the first line the spec stops at, edits the project as the agent would,
// writes what the container's probes print, and, when asked, prints the token to
// its error stream.
type Session struct {
	Specs  []canary.SessionSpec
	Stream []byte
	Edit   func(project string) error
	Probes string
	Leak   bool
	Killed string
	Err    error
}

// Run implements canary.Sessions.
func (s *Session) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	s.Specs = append(s.Specs, spec)
	if s.Err != nil {
		return canary.SessionOutcome{}, s.Err
	}
	outcome := canary.SessionOutcome{Killed: s.Killed}
	var written, times []byte
	for number, line := range bytes.SplitAfter(s.Stream, []byte("\n")) {
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
	stderr := "session ended\n"
	if s.Leak {
		stderr = "env: " + launch.OAuthTokenVariable + "=" + string(spec.Stdin)
	}
	files := map[string][]byte{spec.StdoutPath: written, spec.TimesPath: times, spec.StderrPath: []byte(stderr)}
	if out := probesFile(spec.Argv); out != "" {
		files[out] = []byte(s.Probes)
	}
	for path, content := range files {
		if err := os.WriteFile(path, content, 0o644); err != nil {
			return outcome, err
		}
	}
	if s.Edit != nil {
		return outcome, s.Edit(ProjectOf(spec))
	}
	return outcome, nil
}

// probesFile is where the session's container writes what its probes print, when
// it runs any.
func probesFile(argv []string) string {
	for _, word := range argv {
		if strings.HasSuffix(word, string(filepath.Separator)+app.ProbesFile) {
			return word
		}
	}
	return ""
}

// ProjectOf is the session's working directory, from its docker command.
func ProjectOf(spec canary.SessionSpec) string {
	return spec.Argv[slices.Index(spec.Argv, "--workdir")+1]
}

// Clock gives each reading a second after the one before.
type Clock struct {
	mutex sync.Mutex
	at    time.Time
}

// Now implements canary.Clock.
func (c *Clock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := c.at
	c.at = c.at.Add(time.Second)
	return now
}

// Set moves the clock.
func (c *Clock) Set(at time.Time) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.at = at
}

// Credential is a keychain source that counts the reads of its value.
type Credential struct{ Reads *int }

func (Credential) Source() string                        { return canary.SourceKeychain }
func (Credential) Present(context.Context) (bool, error) { return true, nil }
func (c Credential) Token(context.Context) (string, error) {
	*c.Reads++
	return Token, nil
}

type noLock struct{}

func (noLock) Acquire(string) (canary.Lock, error) { return noLock{}, nil }
func (noLock) Release()                            {}

// Stamps stands for the timestamp authority: it records the entry files it is
// asked to stamp, each holding one line of the deviation log.
type Stamps struct{ Lines []string }

// Stamp implements deviation.Stamping.
func (s *Stamps) Stamp(_ context.Context, file string) (deviation.Token, error) {
	content, err := os.ReadFile(file)
	s.Lines = append(s.Lines, strings.TrimSpace(string(content)))
	return deviation.Token{}, err
}

// World is a work directory, the study's configuration and inputs, and the
// stand-ins of the engine and the session.
type World struct {
	Repo, Work, Product string
	Engine              *Engine
	Session             *Session
	Clock               *Clock
	Stamps              *Stamps
	Reads               *int
	Stdout              *bytes.Buffer
	Runner              runs.Runner
	Config              config.Config
	Inputs              study.Inputs
	Options             app.Options
}

// NewWorld wires the stand-ins to the study's configuration, with one
// repetition, and freezes the inputs as resolve would.
func NewWorld(t *testing.T) *World {
	t.Helper()
	repo := RepoRoot(t)
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
	w := &World{
		Repo: repo, Work: TempDir(t), Product: TempDir(t), Engine: NewEngine(), Session: &Session{Stream: ShortStream(t, ResultLine("success", false, "done"))},
		Clock: &Clock{at: Start}, Stamps: &Stamps{}, Reads: &reads, Stdout: &bytes.Buffer{}, Config: parsed,
	}
	appRunner := app.Runner{
		Clock: w.Clock, Window: window.Nightly(), Processes: w.Engine, Sessions: w.Session, Files: adapters.OSFiles{}, Locker: noLock{},
		Credentials: []canary.ClaudeCredential{Credential{Reads: &reads}},
		Env:         []string{"HOME=/Users/someone", "PATH=/usr/bin", launch.OAuthTokenVariable + "=" + Token}, Stdout: w.Stdout,
	}
	w.Runner = runs.Runner{App: appRunner, Tree: runs.OSTree{},
		Deviations: deviation.NewAppender(study.Layout{Root: w.Work}, w.Stamps, w.Clock.Now)}
	w.Options = app.Options{
		RepoRoot: repo, Config: parsed, Work: w.Work, Product: w.Product, Image: Image, Docker: []string{DockerFront},
		UID: 501, GID: 20, ClaudePin: Pin, Profile: runs.StudyProfile(Claude),
	}
	w.Inputs = w.freeze(t)
	w.Engine.Forget()
	return w
}

// freeze computes the inputs freeze as resolve would: each task's start commits
// in each arm, prepared in a directory of its own.
func (w *World) freeze(t *testing.T) study.Inputs {
	t.Helper()
	inputs := study.Inputs{
		Order: runs.Order(w.Config), EgressAllowlist: []string{AllowedHost}, QuotaPatterns: runs.QuotaPatterns(),
		ClaudeCode: study.ClaudeCode{Version: Pin}, Images: study.Images{Built: Image},
	}
	equipper := runs.Equipper{Options: w.Options, Processes: w.Engine, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	for _, task := range w.Config.Tasks {
		entry := study.TaskInputs{Name: task.Name, EquippedTree: map[string]string{}}
		for _, arm := range w.Config.Arms {
			equipped, err := equipper.Equip(context.Background(), filepath.Join(TempDir(t), "resolve-"+string(arm)), config.Run{Task: task, Arm: arm, Repetition: 1})
			if err != nil {
				t.Fatal(err)
			}
			entry.BaseCommit, entry.EquippedTree[string(arm)] = equipped.BaseCommit, equipped.Tree
		}
		inputs.Tasks = append(inputs.Tasks, entry)
	}
	return inputs
}

// Plan is the plan of the world's attempts.
func (w *World) Plan() runs.Plan {
	return runs.Plan{Options: w.Options, Inputs: w.Inputs, ImageID: Image, Source: Credential{Reads: w.Reads}}
}

// Invocation is an invocation of the run verb on the world.
func (w *World) Invocation() runs.Invocation {
	return runs.Invocation{Plan: runs.Plan{Options: w.Options, Inputs: w.Inputs}, Config: w.Config, Layout: study.Layout{Root: w.Work}}
}

// Batch runs the world's order.
func (w *World) Batch() runs.Batch { return runs.Batch{Runner: w.Runner, Stdout: w.Stdout} }

// First is the first run of the order.
func (w *World) First() config.Run { return w.Config.Runs()[0] }

// RunOnce runs the first attempt of the order's first run and returns it with its
// directory.
func (w *World) RunOnce(t *testing.T) (runs.Result, string) {
	t.Helper()
	configured := w.First()
	expect, _ := w.Inputs.Task(configured.Task.Name)
	dir := study.Layout{Root: w.Work}.RunDir(configured.Name(), 1)
	result, err := w.Runner.Attempt(context.Background(), w.Plan(),
		runs.Spec{Run: configured, Dir: dir, Attempt: 1, Expect: &expect, Egress: container.HostEndpoints(w.Inputs.EgressAllowlist)})
	if err != nil {
		t.Fatal(err)
	}
	return result, dir
}

// Attempts reads the attempts of every run of the world's order.
func (w *World) Attempts(t *testing.T) map[string][]phase.Attempt {
	t.Helper()
	attempts, err := phase.Load(study.Layout{Root: w.Work}, w.Inputs.Order)
	if err != nil {
		t.Fatal(err)
	}
	return attempts
}

// State is a run's state from its attempts, the calendar cap reached or not.
func (w *World) State(t *testing.T, name string, capped bool) study.Outcome {
	t.Helper()
	return phase.StateOf(name, w.Attempts(t)[name], capped).State
}

// Deviations reads the world's deviation log.
func (w *World) Deviations(t *testing.T) []study.Deviation {
	t.Helper()
	content, _ := os.ReadFile(filepath.Join(w.Work, study.DeviationsFile))
	return study.ParseDeviations(string(content))
}

// TempDir is a fresh directory whose path holds no character a mount refuses,
// with symbolic links resolved.
func TempDir(t *testing.T) string {
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

// RecordedStream is the stream of the demonstration's committed recording.
func RecordedStream(t *testing.T) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(RepoRoot(t), recording, record.StreamFile))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

// InitLine is the init event of the recorded stream, which lists every plugin of
// a session equipped with the modules.
func InitLine(t *testing.T) string {
	t.Helper()
	for _, line := range strings.Split(string(RecordedStream(t)), "\n") {
		if strings.Contains(line, `"subtype":"init"`) {
			return line + "\n"
		}
	}
	t.Fatal("the recorded stream holds no init event")
	return ""
}

// ResultLine is a result event whose usage names the models given, the session
// model when none is.
func ResultLine(subtype string, isError bool, text string, models ...string) string {
	if len(models) == 0 {
		models = []string{Model}
	}
	usage := map[string]any{}
	for _, model := range models {
		usage[model] = map[string]any{"outputTokens": 1}
	}
	encoded, _ := json.Marshal(map[string]any{"type": "result", "subtype": subtype, "is_error": isError, "result": text, "modelUsage": usage})
	return string(encoded)
}

// ShortStream is the recorded init event, one assistant message and the result
// event given.
func ShortStream(t *testing.T, result string) []byte {
	t.Helper()
	assistant := `{"type":"assistant","message":{"model":"` + Model + `","content":[{"type":"text","text":"done"}]}}` + "\n"
	return []byte(InitLine(t) + assistant + result + "\n")
}

// Read reads a file the test needs.
func Read(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
