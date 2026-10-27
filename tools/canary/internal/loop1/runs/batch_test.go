package runs_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs/runstest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func kinds(deviations []study.Deviation) []study.Kind {
	var found []study.Kind
	for _, deviation := range deviations {
		found = append(found, deviation.Kind)
	}
	return found
}

func TestOutcomePaths(t *testing.T) {
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, w *runstest.World)
		want   study.Outcome
		reason string
	}{
		{"the CLI exits by itself", func(t *testing.T, w *runstest.World) {
			w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", false, "done"))
		}, study.OutcomeCompleted, ""},
		{"the runner ends the session at its message cap", func(t *testing.T, w *runstest.World) {
			w.Session.Stream, w.Session.Killed = runstest.ShortStream(t, runstest.ResultLine("success", false, "done")), evidence.KilledTurnCap
		}, study.OutcomeCappedMessages, ""},
		{"the runner ends the session at its time cap", func(t *testing.T, w *runstest.World) {
			w.Session.Stream, w.Session.Killed = runstest.ShortStream(t, runstest.ResultLine("success", false, "done")), evidence.KilledTimeout
		}, study.OutcomeCappedTime, ""},
		{"the result event reports the spend cap", func(t *testing.T, w *runstest.World) {
			w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("error_max_budget_usd", true, "budget"))
		}, study.OutcomeCappedBudget, ""},
		{"the container is killed for memory after the session started", func(t *testing.T, w *runstest.World) {
			w.Session.Stream, w.Engine.State = runstest.ShortStream(t, runstest.ResultLine("success", false, "done")), "137 true"
		}, study.OutcomeCappedMemory, ""},
		{"the container is killed for memory before the session started", func(t *testing.T, w *runstest.World) {
			w.Session.Stream, w.Engine.State = nil, "137 true"
		}, study.OutcomeHarnessError, runs.ReasonMemoryEarly},
		{"system/init names another model", func(t *testing.T, w *runstest.World) {
			w.Session.Stream = []byte(strings.Replace(string(runstest.ShortStream(t, runstest.ResultLine("success", false, "done"))), `"model":"`+runstest.Model+`","permissionMode"`, `"model":"claude-other-1","permissionMode"`, 1))
		}, study.OutcomeHarnessError, runs.ReasonModelMismatch},
		{"the base commit differs from the inputs freeze", func(t *testing.T, w *runstest.World) {
			w.Inputs.Tasks[0].BaseCommit = strings.Repeat("0", 40)
		}, study.OutcomeHarnessError, "the base commit"},
		{"the equipped tree differs from the inputs freeze", func(t *testing.T, w *runstest.World) {
			for arm := range w.Inputs.Tasks[0].EquippedTree {
				w.Inputs.Tasks[0].EquippedTree[arm] = strings.Repeat("0", 64)
			}
		}, study.OutcomeHarnessError, "equipped tree"},
		{"the acceptance gate cannot run", func(t *testing.T, w *runstest.World) {
			w.Engine.GateFails = true
		}, study.OutcomeHarnessError, "past its limit"},
		{"the session cannot run", func(t *testing.T, w *runstest.World) {
			w.Session.Err = errors.New("the engine refused the container")
		}, study.OutcomeHarnessError, "the engine refused"},
	} {
		t.Run(tc.name+" [verifies SP-2524~1] [verifies SP-2526~1]", func(t *testing.T) {
			w := runstest.NewWorld(t)
			tc.setup(t, w)
			result, _ := w.RunOnce(t)
			if result.Run.Outcome != tc.want || !strings.Contains(result.Run.Reason, tc.reason) {
				t.Errorf("outcome %s (%q), want %s (%q)", result.Run.Outcome, result.Run.Reason, tc.want, tc.reason)
			}
			if strings.Contains(tc.reason, "frozen") || strings.Contains(tc.name, "inputs freeze") {
				if len(w.Session.Specs) != 0 {
					t.Error("a session started on a project that differs from the freeze")
				}
			}
		})
	}
}

func TestQuotaStop(t *testing.T) {
	w := runstest.NewWorld(t)
	reset := runstest.Start.Add(3 * time.Hour)
	w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", true, "Claude AI usage limit reached|"+itoa(reset.Unix())))
	if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
		t.Fatal(err)
	}
	first := w.Inputs.Order[0]

	t.Run("a quota stop records its reset time, is logged, and ends the invocation without starting another attempt [verifies SP-2525~1] [verifies SP-2528~1]", func(t *testing.T) {
		attempts := w.Attempts(t)
		if len(w.Session.Specs) != 1 || len(attempts[first]) != 1 || len(attempts[w.Inputs.Order[1]]) != 0 {
			t.Fatalf("%d sessions, attempts %+v", len(w.Session.Specs), attempts)
		}
		run := attempts[first][0].Run
		if run.Outcome != study.OutcomeQuotaStop || run.QuotaReset == nil || !run.QuotaReset.Equal(reset) {
			t.Errorf("run %+v", run)
		}
		if got := kinds(w.Deviations(t)); !slices.Equal(got, []study.Kind{study.KindQuotaStop}) {
			t.Errorf("deviations %v", got)
		}
	})

	t.Run("the next invocation refuses to start before the reset [verifies SP-2528~1]", func(t *testing.T) {
		w.Clock.Set(reset.Add(-time.Minute))
		err := w.Batch().Run(context.Background(), w.Invocation())
		if !errors.Is(err, runs.ErrBeforeReset) || len(w.Session.Specs) != 1 {
			t.Errorf("error %v, %d sessions", err, len(w.Session.Specs))
		}
	})

	t.Run("after the reset it resumes the order at that run in a fresh attempt directory, the quota stop not counted against it [verifies SP-2528~1] [verifies SP-2530~1]", func(t *testing.T) {
		w.Clock.Set(reset.Add(time.Minute))
		w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", false, "done"))
		if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
			t.Fatal(err)
		}
		attempts := w.Attempts(t)
		if len(attempts[first]) != 2 || attempts[first][1].Number != 2 || attempts[first][1].Run.Outcome != study.OutcomeCompleted {
			t.Errorf("first run %+v", attempts[first])
		}
		if _, err := os.Stat(study.Layout{Root: w.Work}.Records(first, 2)); err != nil {
			t.Error(err)
		}
		for _, name := range w.Inputs.Order {
			if !w.State(t, name, false).Counted() {
				t.Errorf("%s is not counted", name)
			}
		}
		if order := sessionsOrder(w); !slices.Equal(order, w.Inputs.Order) {
			t.Errorf("the attempts ran in the order %v, want %v", order, w.Inputs.Order)
		}
	})
}

// sessionsOrder lists the runs whose sessions ran, in order, the repeated
// attempts of one run counted once.
func sessionsOrder(w *runstest.World) []string {
	var names []string
	for _, spec := range w.Session.Specs {
		dir := filepath.Base(filepath.Dir(runstest.ProjectOf(spec)))
		name := dir[:strings.LastIndex(dir, "-")]
		if len(names) == 0 || names[len(names)-1] != name {
			names = append(names, name)
		}
	}
	return names
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestRateLimitedRetry(t *testing.T) {
	w := runstest.NewWorld(t)
	retry := `{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"error_status":429,"error":"rate_limit"}`
	w.Session.Stream = []byte(runstest.InitLine(t) + retry + "\n" + runstest.ResultLine("success", false, "done") + "\n")
	result, dir := w.RunOnce(t)

	t.Run("an API error with HTTP 429 is a quota stop and the session is ended at it [verifies SP-2525~1]", func(t *testing.T) {
		stream := runstest.Read(t, filepath.Join(dir, plan.RecordsDir, record.StreamFile))
		if result.Run.Outcome != study.OutcomeQuotaStop || strings.Contains(stream, `"type":"result"`) || result.Run.Killed != runs.ReasonQuota {
			t.Errorf("outcome %s, killed %q, stream:\n%s", result.Run.Outcome, result.Run.Killed, stream)
		}
	})
}

func TestHarnessBlocked(t *testing.T) {
	w := runstest.NewWorld(t)
	w.Engine.FailInit = true
	if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("three harness errors block a run, each logged, the block logged once, and the order moves on [verifies SP-2527~1] [verifies SP-2531~1]", func(t *testing.T) {
		attempts := w.Attempts(t)
		for _, name := range w.Inputs.Order {
			if len(attempts[name]) != study.MaxAttempts || w.State(t, name, false) != study.OutcomeHarnessBlocked {
				t.Errorf("%s: %d attempts, state %s", name, len(attempts[name]), w.State(t, name, false))
			}
		}
		if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
			t.Fatal(err)
		}
		count := map[study.Kind]int{}
		for _, deviation := range w.Deviations(t) {
			count[deviation.Kind]++
			if deviation.Section == "" || len(deviation.Scope) != 1 || !slices.Contains(w.Inputs.Order, deviation.Scope[0]) {
				t.Errorf("entry %+v", deviation)
			}
		}
		if count[study.KindHarnessError] != 2*study.MaxAttempts || count[study.KindHarnessBlocked] != 2 {
			t.Errorf("deviations %v", count)
		}
	})
}

func TestInterruptedAttempt(t *testing.T) {
	w := runstest.NewWorld(t)
	w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", false, "done"))
	layout := study.Layout{Root: w.Work}
	if err := os.MkdirAll(layout.Records(w.Inputs.Order[0], 1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("an attempt directory without run.json is recorded and logged as a harness error, and the next attempt takes the next number [verifies SP-2527~1]", func(t *testing.T) {
		attempts := w.Attempts(t)[w.Inputs.Order[0]]
		if len(attempts) != 2 || attempts[0].Run.Outcome != study.OutcomeHarnessError || attempts[0].Run.Reason != runs.ReasonInterruptedAttempt || attempts[1].Number != 2 || !attempts[1].Run.Outcome.Counted() {
			t.Errorf("attempts %+v", attempts)
		}
		if got := kinds(w.Deviations(t)); !slices.Equal(got, []study.Kind{study.KindHarnessError}) {
			t.Errorf("deviations %v", got)
		}
	})
}

func TestCalendarCap(t *testing.T) {
	w := runstest.NewWorld(t)
	layout := study.Layout{Root: w.Work}
	first := w.Inputs.Order[0]
	stop := study.Run{Outcome: study.OutcomeQuotaStop, Attempt: 1}
	stop.StartedAt = runstest.Start
	encoded, _ := json.Marshal(stop)
	if err := runstest.WriteAll(layout.Records(first, 1), map[string]string{record.MetaFile: string(encoded)}); err != nil {
		t.Fatal(err)
	}
	w.Clock.Set(runstest.Start.Add(phase.CalendarCap + time.Hour))
	if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("28 days after the first start no attempt starts, and each run takes its state at the cap [verifies SP-2529~1]", func(t *testing.T) {
		if len(w.Session.Specs) != 0 {
			t.Errorf("%d sessions after the cap", len(w.Session.Specs))
		}
		if got := []study.Outcome{w.State(t, first, true), w.State(t, w.Inputs.Order[1], true)}; !slices.Equal(got, []study.Outcome{study.OutcomeQuotaBlocked, study.OutcomeNotRun}) {
			t.Errorf("states %v", got)
		}
		if got := kinds(w.Deviations(t)); !slices.Equal(got, []study.Kind{study.KindQuotaBlocked}) {
			t.Errorf("deviations %v", got)
		}
		printed := w.Stdout.String()
		if !strings.Contains(printed, "calendar cap") || !strings.Contains(printed, string(study.OutcomeNotRun)) {
			t.Errorf("printed:\n%s", printed)
		}
	})
}

func TestProvenanceDeviation(t *testing.T) {
	w := runstest.NewWorld(t)
	w.Inputs.Toolchains = map[string]string{freeze.ToolchainPython: "3.13.13", freeze.ToolchainUV: "0.11.23"}
	w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", false, "done"))
	if err := w.Batch().Run(context.Background(), w.Invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("a toolchain version that differs from the freeze is a provenance deviation and the runs stay counted [verifies SP-2526~1] [verifies SP-2531~1]", func(t *testing.T) {
		got := kinds(w.Deviations(t))
		if !slices.Equal(got, []study.Kind{study.KindProvenance, study.KindProvenance}) {
			t.Errorf("deviations %v", got)
		}
		for _, name := range w.Inputs.Order {
			if !w.State(t, name, false).Counted() {
				t.Errorf("%s is not counted", name)
			}
		}
	})
}

func TestRefusals(t *testing.T) {
	t.Run("an order that differs from the freeze is refused before anything runs [verifies SP-2530~1]", func(t *testing.T) {
		w := runstest.NewWorld(t)
		slices.Reverse(w.Inputs.Order)
		err := w.Batch().Run(context.Background(), w.Invocation())
		if !errors.Is(err, runs.ErrOrder) || len(w.Engine.Argvs()) != 0 {
			t.Errorf("error %v, %d commands", err, len(w.Engine.Argvs()))
		}
	})

	t.Run("a CLI whose version does not begin with the pin is refused before any attempt [verifies SP-2513~1]", func(t *testing.T) {
		w := runstest.NewWorld(t)
		w.Engine.Version = "2.1.296"
		err := w.Batch().Run(context.Background(), w.Invocation())
		if !errors.Is(err, app.ErrPinMismatch) || len(w.Session.Specs) != 0 {
			t.Errorf("error %v, %d sessions", err, len(w.Session.Specs))
		}
		if _, err := os.Stat(filepath.Join(w.Work, study.RunsDir)); !errors.Is(err, os.ErrNotExist) {
			t.Error("an attempt directory was created")
		}
		var asked bool
		for _, argv := range w.Engine.Argvs() {
			asked = asked || (slices.Contains(argv, runstest.Claude) && slices.Contains(argv, "--version"))
		}
		if !asked {
			t.Error("the image's CLI was not asked its version")
		}
	})

	t.Run("a freeze without an allowlist, quota patterns, or a task's equipped tree in an arm is refused [verifies SP-2530~1]", func(t *testing.T) {
		w := runstest.NewWorld(t)
		w.Inputs.QuotaPatterns = nil
		if err := w.Batch().Run(context.Background(), w.Invocation()); !errors.Is(err, runs.ErrInputs) {
			t.Errorf("error %v", err)
		}
		incomplete := runstest.NewWorld(t)
		delete(incomplete.Inputs.Tasks[0].EquippedTree, string(incomplete.Config.Arms[1]))
		if err := incomplete.Batch().Run(context.Background(), incomplete.Invocation()); !errors.Is(err, runs.ErrInputs) || len(incomplete.Engine.Argvs()) != 0 {
			t.Errorf("error %v", err)
		}
	})

	t.Run("an engine whose image is not the frozen one is refused before any attempt [verifies SP-2526~1]", func(t *testing.T) {
		w := runstest.NewWorld(t)
		w.Inputs.Images.Built = "sha256:" + strings.Repeat("2", 64)
		err := w.Batch().Run(context.Background(), w.Invocation())
		if !errors.Is(err, runs.ErrImage) || len(w.Session.Specs) != 0 {
			t.Errorf("error %v, %d sessions", err, len(w.Session.Specs))
		}
		if _, err := os.Stat(filepath.Join(w.Work, study.RunsDir)); !errors.Is(err, os.ErrNotExist) {
			t.Error("an attempt directory was created")
		}
	})

	t.Run("an engine that stops answering ends the invocation after one harness error instead of blocking the runs [verifies SP-2527~1] [verifies SP-2530~1]", func(t *testing.T) {
		w := runstest.NewWorld(t)
		w.Session.Edit = func(string) error { w.Engine.Down = true; return nil }
		if err := w.Batch().Run(context.Background(), w.Invocation()); err == nil {
			t.Fatal("the invocation went on with a stopped engine")
		}
		attempts := w.Attempts(t)
		if len(attempts[w.Inputs.Order[0]]) != 1 || len(attempts[w.Inputs.Order[1]]) != 0 || attempts[w.Inputs.Order[0]][0].Run.Outcome != study.OutcomeHarnessError {
			t.Errorf("attempts %+v", attempts)
		}
	})
}

func TestDryRun(t *testing.T) {
	w := runstest.NewWorld(t)
	inv := w.Invocation()
	inv.DryRun = true
	if err := w.Batch().Run(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	printed := w.Stdout.String()

	t.Run("a dry run prints the order and every command of each attempt to come, and starts nothing [verifies SP-2532~1]", func(t *testing.T) {
		for _, wanted := range append([]string{
			"(standard input: CLAUDE_CODE_OAUTH_TOKEN=<stdin>)", runstest.DockerFront + " network create --internal", "--allow " + runstest.AllowedHost + ":443",
			"permissions.deny: WebFetch, WebSearch", "check --json-out", "inspect --format", "restore shallnot.yaml", runstest.Claude + " --version",
		}, w.Inputs.Order...) {
			if !strings.Contains(printed, wanted) {
				t.Errorf("the plan lacks %q", wanted)
			}
		}
		if len(w.Engine.Argvs()) != 0 || len(w.Session.Specs) != 0 || *w.Reads != 0 || strings.Contains(printed, runstest.Token) {
			t.Errorf("the dry run started or read something: %d commands, %d sessions, %d reads", len(w.Engine.Argvs()), len(w.Session.Specs), *w.Reads)
		}
	})
}

func TestRunMain(t *testing.T) {
	repo := runstest.RepoRoot(t)
	work := runstest.TempDir(t)
	main := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := runs.Command{Environ: []string{"HOME=" + work, "LOOP1_WORK=" + work}}.Main(context.Background(), args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	t.Run("a dry run reads the repository's configuration and exits 0, an unknown flag exits 2, and an absent freeze exits 1 [verifies SP-2534~1]", func(t *testing.T) {
		code, stdout, stderr := main("--repo", repo, "--dry-run", "--inputs", filepath.Join(work, "absent.json"))
		if code != runs.ExitOK || !strings.Contains(stdout, "boxoffice-") || !strings.Contains(stdout, "work: "+work) {
			t.Errorf("dry run: %d\n%s\n%s", code, stdout, stderr)
		}
		if code, _, _ := main("--no-such-flag"); code != runs.ExitUsage {
			t.Errorf("unknown flag: %d", code)
		}
		if code, _, stderr := main("--repo", repo, "--inputs", filepath.Join(work, "absent.json")); code != runs.ExitFailure || !strings.Contains(stderr, "absent") {
			t.Errorf("absent freeze: %d %s", code, stderr)
		}
	})
}
