package app_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

const (
	repoFromHere = "../../../../.."
	testToken    = "sk-ant-oat01-test-token-that-must-stay-in-the-session"
	testPin      = "2.1.295"
	shimVerb     = "shim"
)

// TestMain lets the test binary stand in for the runner when a shim calls it.
func TestMain(m *testing.M) {
	if len(os.Args) > 1 && os.Args[1] == shimVerb {
		var real, record string
		args := os.Args[2:]
		for len(args) >= 2 && strings.HasPrefix(args[0], "--") && args[0] != "--" {
			switch args[0] {
			case launch.FlagReal:
				real = args[1]
			case launch.FlagRecord:
				record = args[1]
			}
			args = args[2:]
		}
		if len(args) > 0 && args[0] == "--" {
			args = args[1:]
		}
		os.Exit(adapters.RunShim(real, record, args, os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(m.Run())
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

var (
	night = time.Date(2026, time.October, 25, 22, 0, 0, 0, time.Local)
	noon  = time.Date(2026, time.October, 25, 12, 0, 0, 0, time.Local)
)

// fakeCredential is a credential source that counts the reads of its value.
type fakeCredential struct {
	present bool
	reads   *int
}

func (f fakeCredential) Source() string                        { return canary.SourceKeychain }
func (f fakeCredential) Present(context.Context) (bool, error) { return f.present, nil }
func (f fakeCredential) Token(context.Context) (string, error) { *f.reads++; return testToken, nil }

// supportProcesses answers the checks a run makes before it prepares anything and
// runs every other command for real.
type supportProcesses struct {
	python  string
	version string
	mutex   sync.Mutex
	calls   [][]string
}

func (s *supportProcesses) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	s.mutex.Lock()
	s.calls = append(s.calls, spec.Argv)
	s.mutex.Unlock()
	switch {
	case strings.HasSuffix(spec.Argv[0], plan.MemGuardRelative):
		return canary.Output{Stdout: "mem-guard: ok"}, nil
	case len(spec.Argv) == 2 && spec.Argv[1] == "--version":
		return canary.Output{Stdout: s.version + " (Claude Code)"}, nil
	case strings.HasSuffix(spec.Argv[0], "script/resolve-tool-dirs"):
		return canary.Output{Stdout: "python3\t" + s.python + "\n"}, nil
	}
	return adapters.Exec{}.Run(ctx, spec)
}

// recordedSession stands in for the Claude Code session: it writes a recorded
// stream, and runs the end-of-turn hook through the shim on the session's PATH,
// as Claude Code does when the agent ends its turn.
type recordedSession struct {
	t     *testing.T
	specs []canary.SessionSpec
}

func (r *recordedSession) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	r.specs = append(r.specs, spec)
	for source, target := range map[string]string{record.StreamFile: spec.StdoutPath, record.TimesFile: spec.TimesPath} {
		content, err := os.ReadFile(filepath.Join(recording, source))
		if err != nil {
			return canary.SessionOutcome{}, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return canary.SessionOutcome{}, err
		}
	}
	hook := exec.Command("shallnot", "hook", "claude-stop")
	hook.Dir, hook.Env = spec.Dir, spec.Env
	hook.Stdin = strings.NewReader(`{"hook_event_name":"Stop","session_id":"recorded-session","stop_hook_active":false}`)
	var stderr bytes.Buffer
	hook.Stderr = &stderr
	for _, pair := range spec.Env {
		if value, found := strings.CutPrefix(pair, "PATH="); found {
			path, err := lookPath("shallnot", value)
			if err != nil {
				return canary.SessionOutcome{}, err
			}
			hook.Path = path
		}
	}
	if err := hook.Run(); err != nil && hook.ProcessState == nil {
		return canary.SessionOutcome{}, err
	}
	return canary.SessionOutcome{ExitCode: 0}, os.WriteFile(spec.StderrPath, nil, 0o644)
}

func lookPath(name, path string) (string, error) {
	for _, dir := range filepath.SplitList(path) {
		candidate := filepath.Join(dir, name)
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", exec.ErrNotFound
}

type noLock struct{}

func (noLock) Acquire(string) (canary.Lock, error) { return noLock{}, nil }
func (noLock) Release()                            {}

type world struct {
	runner   app.Runner
	options  app.Options
	process  *supportProcesses
	sessions *recordedSession
	reads    *int
	stdout   *bytes.Buffer
}

func newWorld(t *testing.T, at time.Time, present bool) world {
	t.Helper()
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "demo/loop/demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Caps = config.Caps{AssistantMessages: 12, Seconds: 90, BudgetUSD: "1.00"}
	out, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	python, _ := exec.LookPath("python3")
	reads := 0
	w := world{process: &supportProcesses{python: filepath.Dir(python), version: testPin}, sessions: &recordedSession{t: t}, reads: &reads, stdout: &bytes.Buffer{}}
	w.runner = app.Runner{
		Clock: fixedClock{at}, Window: window.Nightly(), Processes: w.process, Sessions: w.sessions,
		Files: adapters.OSFiles{}, Locker: noLock{}, Credentials: []canary.ClaudeCredential{fakeCredential{present: present, reads: &reads}},
		Env: append(os.Environ(), launch.OAuthTokenVariable+"="+testToken), Stdout: w.stdout,
	}
	w.options = app.Options{
		RepoRoot: repo, Config: parsed, OutDir: out, Executable: os.Args[0],
		Claude: filepath.Join(repo, ".tools/claude-code", testPin, "bin", "claude"), ClaudePin: testPin,
	}
	return w
}

func requireProduct(t *testing.T, repo string) {
	t.Helper()
	for _, binary := range []string{"shallnot", "spec-proof-dp", "spec-proof-alignment", "spec-proof-completeness"} {
		if _, err := os.Stat(filepath.Join(repo, plan.ToolsBinRelative, binary)); err != nil {
			t.Skipf("%s is not built: run script/product-build", binary)
		}
	}
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 with pytest is required")
	}
}

func TestLiveRun(t *testing.T) {
	w := newWorld(t, night, true)
	requireProduct(t, w.options.RepoRoot)
	dirs, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || len(w.sessions.specs) != 1 {
		t.Fatalf("dirs %v, sessions %d", dirs, len(w.sessions.specs))
	}
	l := plan.Layout{Repo: w.options.RepoRoot, Run: dirs[0]}
	spec := w.sessions.specs[0]

	t.Run("the fixture is copied, committed, equipped the documented way, bound and committed, and every step is recorded [verifies SP-2401~1]", func(t *testing.T) {
		content, err := os.ReadFile(l.Record(record.StepsFile))
		if err != nil {
			t.Fatal(err)
		}
		steps, err := record.DecodeSteps(content)
		if err != nil {
			t.Fatal(err)
		}
		var commands []string
		for _, step := range steps {
			if step.ExitCode != 0 && !strings.Contains(step.Purpose, "gate") {
				t.Errorf("step %v exited %d: %s", step.Argv, step.ExitCode, step.Stderr)
			}
			commands = append(commands, filepath.Base(step.Argv[0])+" "+strings.Join(step.Argv[1:min(2, len(step.Argv))], " "))
		}
		want := []string{"git init", "git add", "git commit", "shallnot init", "spec-proof-dp init", "spec-proof-dp enable", "spec-proof-alignment init", "spec-proof-alignment enable",
			"spec-proof-completeness init", "spec-proof-completeness enable", "git add", "git commit", "shallnot gate", "git add", "git diff", "shallnot gate"}
		if !slices.Equal(commands, want) {
			t.Errorf("steps = %v\nwant    %v", commands, want)
		}
		bound, err := os.ReadFile(filepath.Join(l.Project(), plan.SpecProofConfig))
		if err != nil || !strings.Contains(string(bound), "operations:\n  ticket_price:\n    python: \"boxoffice.prices:ticket_price\"\n") {
			t.Errorf("config.yaml = %s (%v)", bound, err)
		}
		for _, name := range []string{record.InitialGateFile, record.FinalGateFile} {
			if report, err := os.ReadFile(l.Record(name)); err != nil || !strings.Contains(string(report), `"schema_version"`) {
				t.Errorf("%s: %v", name, err)
			}
		}
	})

	t.Run("the session runs the pinned CLI with the token, the caps and the times file, and its PATH leads with the shims [verifies SP-2402~1]", func(t *testing.T) {
		if spec.Argv[0] != w.options.Claude || spec.Dir != l.Project() || spec.Cap != 12 || spec.Timeout != 90*time.Second || spec.TimesPath != l.Record(record.TimesFile) {
			t.Errorf("spec = %+v", spec)
		}
		tokens := 0
		for _, pair := range spec.Env {
			if pair == launch.OAuthTokenVariable+"="+testToken {
				tokens++
			}
			if strings.HasPrefix(pair, "PATH=") && !strings.HasPrefix(pair, "PATH="+l.Path(plan.ShimDir)+":"+filepath.Join(l.Repo, plan.ToolsBinRelative)+":") {
				t.Errorf("PATH = %s", pair)
			}
		}
		if tokens != 1 || *w.reads != 1 {
			t.Errorf("the token is in the environment %d times and was read %d times", tokens, *w.reads)
		}
		trusted, err := os.ReadFile(l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile))
		if err != nil || !strings.Contains(string(trusted), l.Project()) {
			t.Errorf("trust = %s (%v)", trusted, err)
		}
		settings, err := os.ReadFile(l.Path(plan.SessionSettings))
		if err != nil || !strings.Contains(string(settings), `"Bash(spec-proof-dp *)"`) {
			t.Errorf("settings = %s (%v)", settings, err)
		}
	})

	t.Run("the records hold the stream, its times, the hook the shim saw with the gate's message, the reports and the transcript [verifies SP-2404~1]", func(t *testing.T) {
		stream, _ := os.ReadFile(l.Record(record.StreamFile))
		recorded, _ := os.ReadFile(filepath.Join(recording, record.StreamFile))
		if !bytes.Equal(stream, recorded) {
			t.Error("the stream is not the one the session printed")
		}
		hooks, err := os.ReadFile(l.Record(record.HooksFile))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(hooks), `"module":"shallnot","args":["hook","claude-stop"]`) || !strings.Contains(string(hooks), `"exitCode":2`) || !strings.Contains(string(hooks), "uncovered_requirement") {
			t.Errorf("hooks = %s", hooks)
		}
		transcript, err := os.ReadFile(l.Record(record.TranscriptMarkdown))
		if err != nil || !strings.Contains(string(transcript), "· Gate: blocked") || !strings.Contains(string(transcript), "requirement BOX-4~1 has no bound test") {
			t.Errorf("transcript: %v\n%s", err, transcript)
		}
		if _, err := os.Stat(l.Record(record.ChangeFile)); err != nil {
			t.Error(err)
		}
		if !strings.Contains(w.stdout.String(), l.Record(record.TranscriptMarkdown)) {
			t.Errorf("stdout = %s", w.stdout.String())
		}
	})

	t.Run("no file of the run holds the token [verifies SP-2404~1]", func(t *testing.T) {
		err := filepath.WalkDir(dirs[0], func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil || entry.IsDir() {
				return walkErr
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			if bytes.Contains(content, []byte(testToken)) {
				t.Errorf("%s holds the token", path)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestLiveRefusals(t *testing.T) {
	t.Run("a run outside the window, or with no credential, starts nothing [verifies SP-2403~1]", func(t *testing.T) {
		for name, w := range map[string]world{"outside the window": newWorld(t, noon, true), "no credential": newWorld(t, night, false)} {
			_, err := w.runner.Run(context.Background(), w.options)
			want := app.ErrOutsideWindow
			if name == "no credential" {
				want = app.ErrNoCredential
			}
			if !errors.Is(err, want) || len(w.process.calls) != 0 || len(w.sessions.specs) != 0 || *w.reads != 0 {
				t.Errorf("%s: error %v, %d commands, %d sessions, %d reads", name, err, len(w.process.calls), len(w.sessions.specs), *w.reads)
			}
		}
	})

	t.Run("a forced window lets a run start, and a CLI that is not the pin stops it before any preparation [verifies SP-2403~1]", func(t *testing.T) {
		w := newWorld(t, noon, true)
		w.options.ForceWindow = true
		w.process.version = "2.1.999"
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrPinMismatch) || len(w.sessions.specs) != 0 || len(w.process.calls) != 2 {
			t.Errorf("error %v, calls %v", err, w.process.calls)
		}
	})

	t.Run("a dry run prints every command and the credential source and starts nothing [verifies SP-2403~1]", func(t *testing.T) {
		w := newWorld(t, noon, true)
		w.options.DryRun = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		printed := w.stdout.String()
		for _, wanted := range []string{
			"window 21:00-07:30 local time: closed (--force-window needed)",
			"credentials: source keychain (CLAUDE_CODE_OAUTH_TOKEN=<from keychain>, given to the session only)",
			"run 1: boxoffice-four-proofs-1", "shallnot init", "spec-proof-alignment enable", "python: \"boxoffice.prices:ticket_price\"",
			"--include-hook-events", "--max-budget-usd 1.00", "caps: 12 assistant messages", "CLAUDE_CODE_OAUTH_TOKEN=<from the credential source>",
		} {
			if !strings.Contains(printed, wanted) {
				t.Errorf("the plan lacks %q:\n%s", wanted, printed)
			}
		}
		if strings.Contains(printed, testToken) || len(w.process.calls) != 0 || len(w.sessions.specs) != 0 || *w.reads != 0 {
			t.Errorf("the dry run read or started something: %d commands, %d sessions, %d reads", len(w.process.calls), len(w.sessions.specs), *w.reads)
		}
	})
}
