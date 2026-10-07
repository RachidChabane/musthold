package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

const (
	ticketA = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"
	ticketB = "88ec618dd2f5f5a73120577c8324765babf222a1fe52b144337a61bc9116d709.2.1"
	decider = "spec-proof-dp:decider"
	judge   = "spec-proof-dp:judge"
	gen     = "spec-proof-dp:generator"

	fixturesRoot = "../domain/checks/testdata"
)

func listing(items ...[2]string) string {
	var entries []string
	for _, item := range items {
		entries = append(entries, `{"ticket":{"requestId":"r","vote":0,"emission":1,"nonce":"n"},"role":"`+item[1]+`","view":{"ticket":"`+item[0]+`","nonce":"n","system":"s","user":"u","responseSchema":{}}}`)
	}
	return `{"items":[` + strings.Join(entries, ",") + `]}`
}

func TestSelectTickets(t *testing.T) {
	t.Run("the executor is a decider when one is pending, and the probe takes another ticket [verifies SP-1507~1]", func(t *testing.T) {
		selection, err := app.SelectTickets([]byte(listing([2]string{ticketA, gen}, [2]string{ticketA, decider}, [2]string{ticketB, decider})))
		if err != nil {
			t.Fatal(err)
		}
		if selection.Role != decider || selection.Ticket != ticketA || selection.ProbeTicket != ticketB {
			t.Fatalf("selection = %+v", selection)
		}
	})

	t.Run("a judge comes before a generator, which asks the most of an executor [verifies SP-1507~1]", func(t *testing.T) {
		selection, err := app.SelectTickets([]byte(listing([2]string{ticketA, gen}, [2]string{ticketB, judge})))
		if err != nil || selection.Role != judge {
			t.Fatalf("selection = %+v, %v", selection, err)
		}
		only, err := app.SelectTickets([]byte(listing([2]string{ticketA, gen})))
		if err != nil || only.Role != gen || only.ProbeTicket != only.Ticket {
			t.Fatalf("selection = %+v, %v", only, err)
		}
	})

	t.Run("a listing with no ticket of the module, or no JSON, is an error [verifies SP-1507~1]", func(t *testing.T) {
		if _, err := app.SelectTickets([]byte(listing([2]string{ticketA, "spec-proof-alignment:judge"}))); !errors.Is(err, app.ErrNoTicket) {
			t.Errorf("error = %v", err)
		}
		if _, err := app.SelectTickets([]byte(`{"items":[]}`)); !errors.Is(err, app.ErrNoTicket) {
			t.Errorf("error = %v", err)
		}
		if _, err := app.SelectTickets([]byte("nope")); err == nil {
			t.Error("a listing that is not JSON was read")
		}
	})
}

// recordingProcesses answers commands from a script and remembers them.
type recordingProcesses struct {
	calls  [][]string
	answer func(argv []string) app.Output
}

func (p *recordingProcesses) Run(_ context.Context, spec app.CommandSpec) (app.Output, error) {
	p.calls = append(p.calls, spec.Argv)
	return p.answer(spec.Argv), nil
}

func (p *recordingProcesses) ran(fragment string) bool {
	return slices.ContainsFunc(p.calls, func(argv []string) bool { return strings.Contains(strings.Join(argv, " "), fragment) })
}

// fakeSession plays the harness CLI: it writes the recorded files of a fixture
// session where the CLI and the module would have left them.
type fakeSession struct {
	// afterRun lets a test play what else the CLI wrote while it ran.
	afterRun func(spec app.SessionSpec)
	t        *testing.T
	fixture  string
	runs     []app.SessionSpec
	outcome  app.SessionOutcome
	editFile string
}

func (s *fakeSession) Run(spec app.SessionSpec) (app.SessionOutcome, error) {
	s.runs = append(s.runs, spec)
	records := filepath.Dir(spec.StdoutPath)
	copyFile := func(name, to string) {
		data, err := os.ReadFile(filepath.Join(fixturesRoot, s.fixture, name))
		if err != nil {
			s.t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(to), 0o755); err != nil {
			s.t.Fatal(err)
		}
		if err := os.WriteFile(to, data, 0o644); err != nil {
			s.t.Fatal(err)
		}
	}
	copyFile(evidence.StreamFile, spec.StdoutPath)
	copyFile(evidence.HooksFile, filepath.Join(records, evidence.HooksFile))
	var state string
	for _, pair := range spec.Env {
		if value, found := strings.CutPrefix(pair, "XDG_STATE_HOME="); found {
			state = value
		}
	}
	copyFile(evidence.JournalFile, filepath.Join(state, "spec-proof", "discpower", "digest", "host", evidence.JournalFile))
	edit := filepath.Join(spec.Dir, launch.EditFile)
	content, err := os.ReadFile(edit)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := os.WriteFile(edit, bytes.ReplaceAll(content, []byte(launch.EditFrom), []byte(launch.EditTo)), 0o644); err != nil {
		s.t.Fatal(err)
	}
	if s.afterRun != nil {
		s.afterRun(spec)
	}
	return s.outcome, nil
}

type fakeProber struct {
	result     evidence.CapProbe
	specs      []app.ServerSpec
	handshakes []app.ServerSpec
	// startFails names the binary whose server does not start.
	startFails string
}

func (p *fakeProber) Handshake(_ context.Context, spec app.ServerSpec) evidence.ServerStart {
	p.handshakes = append(p.handshakes, spec)
	return evidence.ServerStart{Binary: filepath.Base(spec.Binary), Started: p.startFails != filepath.Base(spec.Binary), Detail: "fetch_request, submit_response"}
}

func (p *fakeProber) Probe(_ context.Context, spec app.ServerSpec, _ string) evidence.CapProbe {
	p.specs = append(p.specs, spec)
	return p.result
}

type fakeLocker struct {
	err      error
	acquired []string
	released int
}

func (l *fakeLocker) Acquire(path string) (app.Lock, error) {
	if l.err != nil {
		return nil, l.err
	}
	l.acquired = append(l.acquired, path)
	return releaser{l}, nil
}

type releaser struct{ locker *fakeLocker }

func (r releaser) Release() { r.locker.released++ }

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

// world is a canary wired to fakes, over a repository that holds the fixture
// project and the binaries a run needs.
type world struct {
	t         *testing.T
	repo      string
	out       string
	runner    app.Runner
	processes *recordingProcesses
	session   *fakeSession
	prober    *fakeProber
	locker    *fakeLocker
	stdout    *bytes.Buffer
	options   app.Options
}

func newWorld(t *testing.T, flag string, now time.Time) *world {
	t.Helper()
	repo := t.TempDir()
	files := adapters.OSFiles{}
	for path, content := range map[string]string{
		"discpower/fixtures/prove/parcel/parcel/rates.py":    "FREE_SHIPPING_FROM = 50\nSHIPPING_FEE = 4.5\nMAX_PERCENT = 100\n",
		"discpower/fixtures/prove/parcel/.spec-proof/c.yaml": "proofs: {}\n",
		"script/shallnot.pin":                                "831c5454a564b8cf4e1e010780f35543831efe7e\n",
	} {
		if err := files.Write(filepath.Join(repo, path), []byte(content), false); err != nil {
			t.Fatal(err)
		}
	}
	binaries := []string{"shallnot"}
	for _, module := range modules.All() {
		binaries = append(binaries, module.Binary)
	}
	for _, binary := range binaries {
		if err := files.Write(filepath.Join(repo, ".tools", "bin", binary), []byte("#!/bin/sh\n"), true); err != nil {
			t.Fatal(err)
		}
	}
	w := &world{t: t, repo: repo, out: t.TempDir(), stdout: &bytes.Buffer{}}
	w.processes = &recordingProcesses{answer: func(argv []string) app.Output {
		joined := strings.Join(argv, " ")
		switch {
		case strings.HasSuffix(joined, "mem-guard check"):
			return app.Output{}
		case slices.Contains(argv, "--version") && argv[0] == "claude":
			return app.Output{Stdout: "2.1.292 (Claude Code)\n"}
		case slices.Contains(argv, "--version") && argv[0] == "codex":
			return app.Output{Stdout: "codex-cli 0.155.1\n"}
		case strings.HasSuffix(joined, "prove --all"):
			return app.Output{Stdout: "awaiting_host 9 tickets\n", ExitCode: 3}
		case strings.HasSuffix(joined, "request list --json"):
			return app.Output{Stdout: listing([2]string{ticketA, decider}, [2]string{ticketB, decider})}
		case len(argv) > 1 && argv[1] == "version":
			return app.Output{Stdout: argv[0] + " 0.1.0\n"}
		default:
			return app.Output{}
		}
	}}
	fixture := "claude-session"
	if flag == "codex" {
		fixture = "codex-session"
	}
	w.session = &fakeSession{t: t, fixture: fixture}
	w.prober = &fakeProber{result: evidence.CapProbe{Attempted: true, Responded: true, Refused: flag == "claude", ServedTicket: flag == "codex", Detail: "probe"}}
	w.locker = &fakeLocker{}
	w.runner = app.Runner{
		Clock: fixedClock{now}, Window: window.Nightly(), Processes: w.processes, Sessions: w.session, Prober: w.prober,
		Locker: w.locker, Files: files, Env: []string{"HOME=/home/u", "PATH=/usr/bin", "CLAUDE_CODE_OAUTH_TOKEN=secret"},
		Stdout: w.stdout, Stderr: &bytes.Buffer{},
	}
	w.options = app.Options{Harness: flag, RepoRoot: repo, Canary: "/bin/canary", OutDir: w.out}
	return w
}

var (
	night = time.Date(2026, time.October, 7, 22, 30, 0, 0, time.Local)
	noon  = time.Date(2026, time.October, 7, 12, 0, 0, 0, time.Local)
)

func TestRefusals(t *testing.T) {
	t.Run("outside the nightly window nothing is started unless the window is forced [verifies SP-1500~1]", func(t *testing.T) {
		w := newWorld(t, "claude", noon)
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrOutsideWindow) || !strings.Contains(err.Error(), "--force-window") {
			t.Fatalf("error = %v", err)
		}
		if len(w.processes.calls) != 0 || len(w.session.runs) != 0 || len(w.locker.acquired) != 0 {
			t.Fatalf("something ran: %v", w.processes.calls)
		}
		w.options.ForceWindow = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatalf("a forced run was refused: %v", err)
		}
	})

	t.Run("a second session is refused before anything is prepared [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.locker.err = adapters.ErrSessionRunning
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, adapters.ErrSessionRunning) || len(w.processes.calls) != 0 {
			t.Fatalf("error %v, calls %v", err, w.processes.calls)
		}
	})

	t.Run("a machine short of memory is refused before the project is prepared [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		inner := w.processes.answer
		w.processes.answer = func(argv []string) app.Output {
			if strings.HasSuffix(strings.Join(argv, " "), "mem-guard check") {
				return app.Output{ExitCode: 4, Stderr: "mem-guard: refused\n"}
			}
			return inner(argv)
		}
		_, err := w.runner.Run(context.Background(), w.options)
		if err == nil || !strings.Contains(err.Error(), "memory guard") || len(w.processes.calls) != 1 || len(w.session.runs) != 0 {
			t.Fatalf("error %v, calls %v", err, w.processes.calls)
		}
		if w.locker.released != 1 {
			t.Errorf("the lock was released %d times", w.locker.released)
		}
	})

	t.Run("Claude Code with no isolated credential is refused unless the shared home is chosen [verifies SP-1504~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Env = []string{"HOME=/home/u", "PATH=/usr/bin"}
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrNoClaudeAuth) || !strings.Contains(err.Error(), "--shared-home") || len(w.processes.calls) != 0 {
			t.Fatalf("error %v, calls %v", err, w.processes.calls)
		}
		w.options.SharedHome = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatalf("the shared home was refused: %v", err)
		}
		if got := w.session.runs[0].Env; slices.ContainsFunc(got, func(pair string) bool { return strings.HasPrefix(pair, "CLAUDE_CONFIG_DIR=") }) {
			t.Error("the shared home session sets a configuration directory")
		}
	})

	t.Run("a binary that was not built is named, with the script that builds it [verifies SP-1504~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		if err := os.Remove(filepath.Join(w.repo, ".tools", "bin", "spec-proof-alignment")); err != nil {
			t.Fatal(err)
		}
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrMissingBinary) || !strings.Contains(err.Error(), "script/canary-build") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a harness the canary has no checks for is refused [verifies SP-1504~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.options.Harness = "gemini"
		if _, err := w.runner.Run(context.Background(), w.options); err == nil {
			t.Fatal("gemini was run")
		}
	})
}

func TestDryRun(t *testing.T) {
	t.Run("a dry run prints the exact command lines and starts nothing, in or out of the window [verifies SP-1504~1]", func(t *testing.T) {
		for _, flag := range []string{"claude", "codex"} {
			w := newWorld(t, flag, noon)
			w.options.DryRun = true
			w.options.SharedHome = true
			if _, err := w.runner.Run(context.Background(), w.options); err != nil {
				t.Fatal(err)
			}
			if len(w.processes.calls) != 0 || len(w.session.runs) != 0 || len(w.locker.acquired) != 0 {
				t.Fatalf("%s: a dry run started something: %v", flag, w.processes.calls)
			}
			out := w.stdout.String()
			for _, want := range []string{"closed (--force-window needed)", "mem-guard check", "prove --all", "request list --json", flag + " --version", "<ticket>", "<run-dir>", "outside the run directory the run writes:"} {
				if !strings.Contains(out, want) {
					t.Errorf("%s: the plan lacks %q:\n%s", flag, want, out)
				}
			}
		}
	})

	t.Run("the plan of Claude Code writes the settings and validates the plugins, and that of Codex runs each module's init [verifies SP-1504~1]", func(t *testing.T) {
		claude, err := app.Plan("claude", "/repo", false, "")
		if err != nil {
			t.Fatal(err)
		}
		codex, err := app.Plan("codex", "/repo", false, "/dedicated")
		if err != nil {
			t.Fatal(err)
		}
		joinedClaude, joinedCodex := strings.Join(claude, "\n"), strings.Join(codex, "\n")
		for _, want := range []string{".claude/settings.json", "plugin validate /repo/discpower/plugin/claude", "claude -p"} {
			if !strings.Contains(joinedClaude, want) {
				t.Errorf("the Claude plan lacks %q", want)
			}
		}
		for _, want := range []string{"/repo/.tools/bin/spec-proof-completeness init --hooks codex", "CODEX_HOME=/dedicated", "codex exec -m gpt-6-luna"} {
			if !strings.Contains(joinedCodex, want) {
				t.Errorf("the Codex plan lacks %q", want)
			}
		}
		if strings.Contains(joinedClaude, "init --hooks") {
			t.Error("the Claude plan runs an init, which would enable plugins in the project settings")
		}
	})
}

func TestDryRunNamesWhatTheRunTouches(t *testing.T) {
	t.Run("the plan says what each mode writes in the user's home, which the session's fingerprints then confirm [verifies SP-1504~1]", func(t *testing.T) {
		for _, tc := range []struct {
			flag               string
			shared             bool
			codexHome          string
			wants, doesNotWant []string
		}{
			{"claude", false, "", []string{"nothing: Claude Code runs with CLAUDE_CONFIG_DIR inside the run directory"}, []string{"~/.claude.json"}},
			{"claude", true, "", []string{"~/.claude/projects/", "~/.claude.json", "--setting-sources project"}, nil},
			{"codex", false, "", []string{"~/.codex/sessions", "--ignore-user-config"}, nil},
			{"codex", false, "/dedicated", []string{"nothing outside /dedicated"}, []string{"~/.codex/sessions"}},
		} {
			w := newWorld(t, tc.flag, noon)
			w.options.DryRun, w.options.SharedHome, w.options.CodexHome = true, tc.shared, tc.codexHome
			if _, err := w.runner.Run(context.Background(), w.options); err != nil {
				t.Fatal(err)
			}
			out := w.stdout.String()
			for _, want := range tc.wants {
				if !strings.Contains(out, want) {
					t.Errorf("%s shared=%v: the plan lacks %q:\n%s", tc.flag, tc.shared, want, out)
				}
			}
			for _, unwanted := range tc.doesNotWant {
				if strings.Contains(out, unwanted) {
					t.Errorf("%s shared=%v: the plan mentions %q", tc.flag, tc.shared, unwanted)
				}
			}
		}
	})
}

func TestPrepareOnly(t *testing.T) {
	t.Run("the project is prepared and the no-call probes run, but no session starts [verifies SP-1504~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.options.PrepareOnly = true
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil || len(w.session.runs) != 0 || outcome.RunDir == "" {
			t.Fatalf("outcome %+v, error %v, sessions %d", outcome, err, len(w.session.runs))
		}
		out := w.stdout.String()
		for _, want := range []string{"ticket: " + ticketA, "cap_without_hooks", "PASS", "the session would run"} {
			if !strings.Contains(out, want) {
				t.Errorf("the report lacks %q:\n%s", want, out)
			}
		}
		for _, path := range []string{"project/shallnot.yaml", "project/.claude/settings.json", "project/parcel/rates.py", "shim/spec-proof-dp", "shim/spec-proof-alignment", "shim/spec-proof-completeness", "records/nocall.json"} {
			if _, err := os.Stat(filepath.Join(outcome.RunDir, path)); err != nil {
				t.Errorf("the run lacks %s", path)
			}
		}
		shim, _ := os.ReadFile(filepath.Join(outcome.RunDir, "shim", "spec-proof-dp"))
		if !strings.Contains(string(shim), "/bin/canary shim --real "+filepath.Join(w.repo, ".tools/bin/spec-proof-dp")) {
			t.Errorf("shim = %s", shim)
		}
		for _, export := range []string{"export XDG_STATE_HOME=" + outcome.RunDir, "export XDG_CACHE_HOME=", "export TMPDIR=", "export SPEC_PROOF_SHALLNOT=", "export CLAUDE_PROJECT_DIR=" + outcome.RunDir} {
			if !strings.Contains(string(shim), export) {
				t.Errorf("the shim does not set %s:\n%s", export, shim)
			}
		}
	})

	t.Run("a module that asks the host for nothing stops the preparation [verifies SP-1507~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		inner := w.processes.answer
		w.processes.answer = func(argv []string) app.Output {
			if strings.HasSuffix(strings.Join(argv, " "), "request list --json") {
				return app.Output{Stdout: `{"items":[]}`}
			}
			return inner(argv)
		}
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrPrepare) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a proof that does not stop awaiting the host stops the preparation [verifies SP-1507~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		inner := w.processes.answer
		w.processes.answer = func(argv []string) app.Output {
			if strings.HasSuffix(strings.Join(argv, " "), "prove --all") {
				return app.Output{ExitCode: 2, Stderr: "load the criteria failed"}
			}
			return inner(argv)
		}
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrPrepare) || !strings.Contains(err.Error(), "load the criteria") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSyntheticSessionOfClaudeCode(t *testing.T) {
	w := newWorld(t, "claude", night)
	outcome, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a run prepares first, takes the lock, runs one session and evaluates what it recorded [verifies SP-1508~1]", func(t *testing.T) {
		if len(w.session.runs) != 1 || len(w.locker.acquired) != 1 || w.locker.released != 1 {
			t.Fatalf("sessions %d, locks %v, released %d", len(w.session.runs), w.locker.acquired, w.locker.released)
		}
		if !strings.HasSuffix(strings.Join(w.processes.calls[0], " "), "mem-guard check") {
			t.Errorf("the first command is %v", w.processes.calls[0])
		}
		if outcome.Report.ExitCode() != verdict.ExitAllPass {
			for _, v := range outcome.Report.Verdicts {
				t.Logf("%s %s %s", v.Status, v.Check, v.Summary)
			}
			t.Fatalf("exit code %d", outcome.Report.ExitCode())
		}
		if len(outcome.Report.Verdicts) == 0 {
			t.Fatal("no verdict")
		}
	})

	t.Run("the session runs the built command in the project with the shims first on the PATH and the state in the run [verifies SP-1508~1]", func(t *testing.T) {
		spec := w.session.runs[0]
		want, _ := launch.Build("claude", launch.Input{RepoRoot: w.repo, RunDir: outcome.RunDir, Ticket: ticketA, Role: decider})
		if !slices.Equal(spec.Argv, want.Argv) || spec.Dir != filepath.Join(outcome.RunDir, "project") {
			t.Fatalf("argv %v in %s", spec.Argv, spec.Dir)
		}
		environment := map[string]string{}
		for _, pair := range spec.Env {
			name, value, _ := strings.Cut(pair, "=")
			environment[name] = value
		}
		if !strings.HasPrefix(environment["PATH"], filepath.Join(outcome.RunDir, "shim")+":"+filepath.Join(w.repo, ".tools/bin")) {
			t.Errorf("PATH = %s", environment["PATH"])
		}
		for variable, prefix := range map[string]string{"XDG_STATE_HOME": outcome.RunDir, "XDG_CACHE_HOME": outcome.RunDir, "TMPDIR": outcome.RunDir} {
			if !strings.HasPrefix(environment[variable], prefix) {
				t.Errorf("%s = %s, want it inside the run", variable, environment[variable])
			}
		}
		if environment["CLAUDE_CONFIG_DIR"] != filepath.Join(outcome.RunDir, "claude-config") || environment["CLAUDE_CODE_OAUTH_TOKEN"] != "secret" {
			t.Errorf("config dir %q, token %q", environment["CLAUDE_CONFIG_DIR"], environment["CLAUDE_CODE_OAUTH_TOKEN"])
		}
		if environment["SPEC_PROOF_SHALLNOT"] != filepath.Join(w.repo, ".tools/bin/shallnot") {
			t.Errorf("SPEC_PROOF_SHALLNOT = %s", environment["SPEC_PROOF_SHALLNOT"])
		}
		if spec.Cap != launch.ClaudeEventCap || spec.Timeout != launch.TimeoutSeconds*time.Second {
			t.Errorf("cap %d, timeout %s", spec.Cap, spec.Timeout)
		}
	})

	t.Run("the preparation commands run before the session and without the shims, so the hook log holds the session only [verifies SP-1506~1]", func(t *testing.T) {
		proveAt := slices.IndexFunc(w.processes.calls, func(argv []string) bool { return slices.Contains(argv, "prove") })
		if proveAt < 0 || !w.processes.ran("request list --json") || !w.processes.ran("plugin validate") {
			t.Fatalf("calls = %v", w.processes.calls)
		}
		if len(w.prober.specs) != 1 || strings.Contains(strings.Join(w.prober.specs[0].Env, "\n"), "/shim") {
			t.Errorf("the cap probe ran through the shims: %+v", w.prober.specs)
		}
	})

	t.Run("the records hold the session, the journal, the edit, the no-call results and the report, with the versions of what ran [verifies SP-1505~1]", func(t *testing.T) {
		records := filepath.Join(outcome.RunDir, "records")
		for _, name := range []string{"session.json", "stream.jsonl", "hooks.jsonl", "journal.jsonl", "nocall.json", "edited.txt", "report.json"} {
			if _, err := os.Stat(filepath.Join(records, name)); err != nil {
				t.Errorf("the records lack %s", name)
			}
		}
		raw, _ := os.ReadFile(filepath.Join(records, "session.json"))
		var recorded evidence.Session
		if err := json.Unmarshal(raw, &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Versions["claude"] != "2.1.292 (Claude Code)" || recorded.Versions["spec-proof-dp"] == "" || recorded.Versions["shallnot"] != "831c5454a564b8cf4e1e010780f35543831efe7e" {
			t.Errorf("versions = %v", recorded.Versions)
		}
		if recorded.Ticket != ticketA || recorded.Role != decider || recorded.Model != "claude-sonnet-5-5" || recorded.EventCap != launch.ClaudeEventCap {
			t.Errorf("session = %+v", recorded)
		}
		if !recorded.EndedAt.After(recorded.StartedAt) && !recorded.EndedAt.Equal(recorded.StartedAt) {
			t.Errorf("times %s %s", recorded.StartedAt, recorded.EndedAt)
		}
	})

	t.Run("a recorded run can be evaluated again without starting anything [verifies SP-1508~1]", func(t *testing.T) {
		before := len(w.processes.calls)
		report, err := w.runner.Evaluate(filepath.Join(outcome.RunDir, "records"), "claude")
		if err != nil || report.ExitCode() != verdict.ExitAllPass || len(w.processes.calls) != before {
			t.Fatalf("report %+v, error %v", report, err)
		}
	})

	t.Run("the report prints one JSON verdict per line and a summary [verifies SP-1501~1]", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		if err := app.WriteReport(outcome.Report, &stdout, &stderr); err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
		if len(lines) != len(outcome.Report.Verdicts) {
			t.Fatalf("%d lines for %d verdicts", len(lines), len(outcome.Report.Verdicts))
		}
		for _, line := range lines {
			var decoded verdict.Verdict
			if err := json.Unmarshal([]byte(line), &decoded); err != nil || decoded.Check == "" || decoded.Status == "" {
				t.Errorf("line %q: %v", line, err)
			}
		}
		if !strings.Contains(stderr.String(), fmt.Sprintf("claude: %d PASS, 0 FAIL, 0 INCONCLUSIVE", len(outcome.Report.Verdicts))) {
			t.Errorf("summary = %q", stderr.String())
		}
	})
}

func TestSyntheticSessionThatTheRunnerHadToStop(t *testing.T) {
	t.Run("a session killed at the cap is recorded as killed and fails the cap check [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.session.outcome = app.SessionOutcome{ExitCode: -1, Killed: evidence.KilledTurnCap}
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		var capCheck verdict.Verdict
		for _, v := range outcome.Report.Verdicts {
			if v.Check == "session_within_caps" {
				capCheck = v
			}
		}
		if capCheck.Status != verdict.Fail || outcome.Report.ExitCode() != verdict.ExitSomeFailed {
			t.Fatalf("cap check %+v, exit %d", capCheck, outcome.Report.ExitCode())
		}
	})
}

func TestSyntheticSessionOfCodex(t *testing.T) {
	w := newWorld(t, "codex", night)
	w.options.CodexHome = t.TempDir()
	outcome, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("each module is initialised for Codex, the probes are merged in and the sandbox probe is recorded [verifies SP-1508~1]", func(t *testing.T) {
		for _, module := range modules.All() {
			if !w.processes.ran(module.Binary + " init --hooks codex") {
				t.Errorf("%s was not initialised", module.Binary)
			}
		}
		hooks, err := os.ReadFile(filepath.Join(outcome.RunDir, "project", ".codex", "hooks.json"))
		if err != nil || !strings.Contains(string(hooks), "PreToolUse-probe") || !strings.Contains(string(hooks), "Stop-probe") {
			t.Fatalf("hooks = %s, %v", hooks, err)
		}
		probe, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", "sandbox-probe.json"))
		if err != nil || !strings.Contains(string(probe), `"exists": false`) {
			t.Fatalf("sandbox probe = %s, %v", probe, err)
		}
	})

	t.Run("the Codex session passes every check on the expected recordings [verifies SP-1508~1]", func(t *testing.T) {
		if outcome.Report.ExitCode() != verdict.ExitAllPass {
			for _, v := range outcome.Report.Verdicts {
				t.Logf("%s %s %s", v.Status, v.Check, v.Summary)
			}
			t.Fatalf("exit code %d", outcome.Report.ExitCode())
		}
		if w.session.runs[0].Cap != launch.CodexEventCap || w.session.runs[0].Argv[0] != "codex" {
			t.Errorf("session = %+v", w.session.runs[0])
		}
	})
}

func TestSessionAgainstTheUsersConfiguration(t *testing.T) {
	write := func(t *testing.T, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	verdictOf := func(report verdict.Report, name string) verdict.Verdict {
		for _, v := range report.Verdicts {
			if v.Check == name {
				return v
			}
		}
		t.Fatalf("no verdict for %s", name)
		return verdict.Verdict{}
	}

	t.Run("a session with its own configuration that writes an entry naming its run fails, and the changes of others are only counted [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "HOME="+home)
		write(t, filepath.Join(home, ".claude", "history.jsonl"))
		w.session.afterRun = func(spec app.SessionSpec) {
			run := filepath.Base(filepath.Dir(spec.Dir))
			write(t, filepath.Join(home, ".claude", "projects", "-private-"+run+"-project", "session.jsonl"))
			write(t, filepath.Join(home, ".claude", "stats-cache.json"))
		}
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		v := verdictOf(outcome.Report, "user_config_untouched")
		if v.Status != verdict.Fail || !strings.Contains(v.Summary, "-project") {
			t.Fatalf("verdict = %+v", v)
		}
		raw, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", "touched.json"))
		if err != nil {
			t.Fatal(err)
		}
		var touched evidence.Touched
		if err := json.Unmarshal(raw, &touched); err != nil {
			t.Fatal(err)
		}
		if len(touched.Attributed) == 0 || !slices.ContainsFunc(touched.Unattributed, func(path string) bool { return strings.HasSuffix(path, "stats-cache.json") }) {
			t.Fatalf("touched = %+v", touched)
		}
		if slices.ContainsFunc(touched.Unattributed, func(path string) bool { return strings.HasSuffix(path, "history.jsonl") }) {
			t.Error("a file that was there before and did not change was reported")
		}
		if want := []string{filepath.Join(home, ".claude.json"), filepath.Join(home, ".claude")}; !slices.Equal(touched.Roots, want) {
			t.Errorf("roots = %v, want %v", touched.Roots, want)
		}
	})

	t.Run("a session with its own configuration that leaves the user's alone passes [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "HOME="+home)
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if v := verdictOf(outcome.Report, "user_config_untouched"); v.Status != verdict.Pass {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("the records say when the session shared the user's directories, and the check leaves it open [verifies SP-1505~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.runner.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin"}
		w.options.SharedHome = true
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if v := verdictOf(outcome.Report, "user_config_untouched"); v.Status != verdict.Inconclusive {
			t.Fatalf("verdict = %+v", v)
		}
		raw, _ := os.ReadFile(filepath.Join(outcome.RunDir, "records", "session.json"))
		if !strings.Contains(string(raw), `"configMode": "shared"`) {
			t.Errorf("session = %s", raw)
		}
	})

	t.Run("each module's executor server is started in the prepared project and a server that does not start is recorded [verifies SP-1502~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.prober.startFails = "spec-proof-completeness"
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if len(w.prober.handshakes) != len(modules.All()) {
			t.Fatalf("%d handshakes", len(w.prober.handshakes))
		}
		if v := verdictOf(outcome.Report, "executor_servers_start"); v.Status != verdict.Fail || !strings.Contains(v.Summary, "spec-proof-completeness") {
			t.Fatalf("verdict = %+v", v)
		}
	})
}
