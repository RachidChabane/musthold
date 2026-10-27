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

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// resultLine is a result event of the session model.
func resultLine(subtype string, isError bool, text string) string {
	encoded, _ := json.Marshal(map[string]any{
		"type": "result", "subtype": subtype, "is_error": isError, "result": text,
		"modelUsage": map[string]any{testModel: map[string]any{"outputTokens": 1}},
	})
	return string(encoded)
}

// ledgers reads the attempts of every run of the world's order.
func (w *world) ledgers(t *testing.T) []runs.Ledger {
	t.Helper()
	ledgers, err := runs.ReadLedgers(adapters.OSFiles{}, study.Layout{Root: w.work}, w.inputs.Order)
	if err != nil {
		t.Fatal(err)
	}
	return ledgers
}

func (w *world) deviations(t *testing.T) []study.Deviation {
	t.Helper()
	content, _ := os.ReadFile(filepath.Join(w.work, study.DeviationsFile))
	return study.ParseDeviations(string(content))
}

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
		setup  func(t *testing.T, w *world)
		want   study.Outcome
		reason string
	}{
		{"the CLI exits by itself", func(t *testing.T, w *world) {
			w.session.stream = shortStream(t, resultLine("success", false, "done"))
		}, study.OutcomeCompleted, ""},
		{"the runner ends the session at its message cap", func(t *testing.T, w *world) {
			w.session.stream, w.session.killed = shortStream(t, resultLine("success", false, "done")), evidence.KilledTurnCap
		}, study.OutcomeCappedMessages, ""},
		{"the runner ends the session at its time cap", func(t *testing.T, w *world) {
			w.session.stream, w.session.killed = shortStream(t, resultLine("success", false, "done")), evidence.KilledTimeout
		}, study.OutcomeCappedTime, ""},
		{"the result event reports the spend cap", func(t *testing.T, w *world) {
			w.session.stream = shortStream(t, resultLine("error_max_budget_usd", true, "budget"))
		}, study.OutcomeCappedBudget, ""},
		{"the container is killed for memory after the session started", func(t *testing.T, w *world) {
			w.session.stream, w.engine.state = shortStream(t, resultLine("success", false, "done")), "137 true"
		}, study.OutcomeCappedMemory, ""},
		{"the container is killed for memory before the session started", func(t *testing.T, w *world) {
			w.session.stream, w.engine.state = nil, "137 true"
		}, study.OutcomeHarnessError, runs.ReasonMemoryEarly},
		{"system/init names another model", func(t *testing.T, w *world) {
			w.session.stream = []byte(strings.Replace(string(shortStream(t, resultLine("success", false, "done"))), `"model":"`+testModel+`","permissionMode"`, `"model":"claude-other-1","permissionMode"`, 1))
		}, study.OutcomeHarnessError, runs.ReasonModelMismatch},
		{"the base commit differs from the inputs freeze", func(t *testing.T, w *world) {
			w.inputs.Tasks[0].BaseCommit = strings.Repeat("0", 40)
		}, study.OutcomeHarnessError, "the base commit"},
		{"the equipped tree differs from the inputs freeze", func(t *testing.T, w *world) {
			for arm := range w.inputs.Tasks[0].EquippedTree {
				w.inputs.Tasks[0].EquippedTree[arm] = strings.Repeat("0", 64)
			}
		}, study.OutcomeHarnessError, "equipped tree"},
		{"the image is not the frozen one", func(t *testing.T, w *world) {
			w.inputs.Images.Built = "sha256:" + strings.Repeat("2", 64)
		}, study.OutcomeHarnessError, "the image"},
		{"the session cannot run", func(t *testing.T, w *world) {
			w.session.err = errors.New("the engine refused the container")
		}, study.OutcomeHarnessError, "the engine refused"},
	} {
		t.Run(tc.name+" [verifies SP-2524~1] [verifies SP-2526~1]", func(t *testing.T) {
			w := newWorld(t)
			tc.setup(t, w)
			result, _ := w.runOnce(t)
			if result.Run.Outcome != tc.want || !strings.Contains(result.Run.Reason, tc.reason) {
				t.Errorf("outcome %s (%q), want %s (%q)", result.Run.Outcome, result.Run.Reason, tc.want, tc.reason)
			}
			if strings.Contains(tc.reason, "frozen") || strings.Contains(tc.name, "inputs freeze") {
				if len(w.session.specs) != 0 {
					t.Error("a session started on a project that differs from the freeze")
				}
			}
		})
	}
}

func TestQuotaStop(t *testing.T) {
	w := newWorld(t)
	reset := start.Add(3 * time.Hour)
	w.session.stream = shortStream(t, resultLine("success", true, "Claude AI usage limit reached|"+itoa(reset.Unix())))
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}
	first := w.inputs.Order[0]

	t.Run("a quota stop records its reset time, is logged, and ends the invocation without starting another attempt [verifies SP-2525~1] [verifies SP-2528~1]", func(t *testing.T) {
		ledgers := w.ledgers(t)
		if len(w.session.specs) != 1 || len(ledgers[0].Attempts) != 1 || len(ledgers[1].Attempts) != 0 {
			t.Fatalf("%d sessions, ledgers %+v", len(w.session.specs), ledgers)
		}
		run := ledgers[0].Attempts[0].Run
		if run.Outcome != study.OutcomeQuotaStop || run.QuotaReset == nil || !run.QuotaReset.Equal(reset) {
			t.Errorf("run %+v", run)
		}
		if got := kinds(w.deviations(t)); !slices.Equal(got, []study.Kind{study.KindQuotaStop}) {
			t.Errorf("deviations %v", got)
		}
	})

	t.Run("the next invocation refuses to start before the reset [verifies SP-2528~1]", func(t *testing.T) {
		w.clock.set(reset.Add(-time.Minute))
		err := w.batch().Run(context.Background(), w.invocation())
		if !errors.Is(err, runs.ErrBeforeReset) || len(w.session.specs) != 1 {
			t.Errorf("error %v, %d sessions", err, len(w.session.specs))
		}
	})

	t.Run("after the reset it resumes the order at that run in a fresh attempt directory, the quota stop not counted against it [verifies SP-2528~1] [verifies SP-2530~1]", func(t *testing.T) {
		w.clock.set(reset.Add(time.Minute))
		w.session.stream = shortStream(t, resultLine("success", false, "done"))
		if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
			t.Fatal(err)
		}
		ledgers := w.ledgers(t)
		if len(ledgers[0].Attempts) != 2 || ledgers[0].Attempts[1].Number != 2 || ledgers[0].Attempts[1].Run.Outcome != study.OutcomeCompleted {
			t.Errorf("first run %+v", ledgers[0].Attempts)
		}
		if _, err := os.Stat(study.Layout{Root: w.work}.Records(first, 2)); err != nil {
			t.Error(err)
		}
		for _, ledger := range ledgers {
			if !ledger.Done() {
				t.Errorf("%s is not done", ledger.Name)
			}
		}
		if order := sessionsOrder(w); !slices.Equal(order, w.inputs.Order) {
			t.Errorf("the attempts ran in the order %v, want %v", order, w.inputs.Order)
		}
	})
}

// sessionsOrder lists the runs whose sessions ran, in order, the repeated
// attempts of one run counted once.
func sessionsOrder(w *world) []string {
	var names []string
	for _, spec := range w.session.specs {
		dir := filepath.Base(filepath.Dir(projectOf(spec)))
		name := dir[:strings.LastIndex(dir, "-")]
		if len(names) == 0 || names[len(names)-1] != name {
			names = append(names, name)
		}
	}
	return names
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestRateLimitedRetry(t *testing.T) {
	w := newWorld(t)
	retry := `{"type":"system","subtype":"api_retry","attempt":1,"max_retries":10,"error_status":429,"error":"rate_limit"}`
	w.session.stream = []byte(initLine(t) + retry + "\n" + resultLine("success", false, "done") + "\n")
	result, dir := w.runOnce(t)

	t.Run("an API error with HTTP 429 is a quota stop and the session is ended at it [verifies SP-2525~1]", func(t *testing.T) {
		stream := read(t, filepath.Join(dir, plan.RecordsDir, record.StreamFile))
		if result.Run.Outcome != study.OutcomeQuotaStop || strings.Contains(stream, `"type":"result"`) || result.Run.Killed != runs.ReasonQuota {
			t.Errorf("outcome %s, killed %q, stream:\n%s", result.Run.Outcome, result.Run.Killed, stream)
		}
	})
}

func TestHarnessBlocked(t *testing.T) {
	w := newWorld(t)
	w.engine.failInit = true
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("three harness errors block a run, each logged, the block logged once, and the order moves on [verifies SP-2527~1] [verifies SP-2531~1]", func(t *testing.T) {
		for _, ledger := range w.ledgers(t) {
			if len(ledger.Attempts) != study.MaxAttempts || ledger.State(false) != study.OutcomeHarnessBlocked {
				t.Errorf("%s: %d attempts, state %s", ledger.Name, len(ledger.Attempts), ledger.State(false))
			}
		}
		if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
			t.Fatal(err)
		}
		count := map[study.Kind]int{}
		for _, deviation := range w.deviations(t) {
			count[deviation.Kind]++
			if deviation.Section == "" || len(deviation.Scope) != 1 || !slices.Contains(w.inputs.Order, deviation.Scope[0]) {
				t.Errorf("entry %+v", deviation)
			}
		}
		if count[study.KindHarnessError] != 2*study.MaxAttempts || count[study.KindHarnessBlocked] != 2 {
			t.Errorf("deviations %v", count)
		}
	})
}

func TestInterruptedAttempt(t *testing.T) {
	w := newWorld(t)
	w.session.stream = shortStream(t, resultLine("success", false, "done"))
	layout := study.Layout{Root: w.work}
	if err := os.MkdirAll(layout.Records(w.inputs.Order[0], 1), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("an attempt directory without run.json counts as a harness error and the next attempt takes the next number [verifies SP-2527~1]", func(t *testing.T) {
		ledger := w.ledgers(t)[0]
		if len(ledger.Attempts) != 2 || ledger.Attempts[0].Run != nil || ledger.Attempts[1].Number != 2 || ledger.HarnessErrors() != 1 {
			t.Errorf("ledger %+v", ledger)
		}
	})
}

func TestCalendarCap(t *testing.T) {
	w := newWorld(t)
	layout := study.Layout{Root: w.work}
	first := w.inputs.Order[0]
	stop := study.Run{Outcome: study.OutcomeQuotaStop, Attempt: 1}
	stop.StartedAt = start
	encoded, _ := json.Marshal(stop)
	if err := writeAll(layout.Records(first, 1), map[string]string{record.MetaFile: string(encoded)}); err != nil {
		t.Fatal(err)
	}
	w.clock.set(start.Add(runs.CalendarCap + time.Hour))
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("28 days after the first start no attempt starts, and each run takes its state at the cap [verifies SP-2529~1]", func(t *testing.T) {
		if len(w.session.specs) != 0 {
			t.Errorf("%d sessions after the cap", len(w.session.specs))
		}
		ledgers := w.ledgers(t)
		if ledgers[0].State(true) != study.OutcomeQuotaBlocked || ledgers[1].State(true) != study.OutcomeNotRun {
			t.Errorf("states %s %s", ledgers[0].State(true), ledgers[1].State(true))
		}
		if got := kinds(w.deviations(t)); !slices.Equal(got, []study.Kind{study.KindQuotaBlocked}) {
			t.Errorf("deviations %v", got)
		}
		printed := w.stdout.String()
		if !strings.Contains(printed, "calendar cap") || !strings.Contains(printed, string(study.OutcomeNotRun)) {
			t.Errorf("printed:\n%s", printed)
		}
	})
}

func TestProvenanceDeviation(t *testing.T) {
	w := newWorld(t)
	w.inputs.Toolchains = map[string]string{"python3": "Python 3.13.13"}
	w.session.stream = shortStream(t, resultLine("success", false, "done"))
	if err := w.batch().Run(context.Background(), w.invocation()); err != nil {
		t.Fatal(err)
	}

	t.Run("a toolchain version that differs from the freeze is a provenance deviation and the runs stay counted [verifies SP-2526~1] [verifies SP-2531~1]", func(t *testing.T) {
		got := kinds(w.deviations(t))
		if !slices.Equal(got, []study.Kind{study.KindProvenance, study.KindProvenance}) {
			t.Errorf("deviations %v", got)
		}
		for _, ledger := range w.ledgers(t) {
			if _, counted := ledger.Counted(); !counted {
				t.Errorf("%s is not counted", ledger.Name)
			}
		}
	})
}

func TestRefusals(t *testing.T) {
	t.Run("an order that differs from the freeze is refused before anything runs [verifies SP-2530~1]", func(t *testing.T) {
		w := newWorld(t)
		slices.Reverse(w.inputs.Order)
		err := w.batch().Run(context.Background(), w.invocation())
		if !errors.Is(err, runs.ErrOrder) || len(w.engine.argvs()) != 0 {
			t.Errorf("error %v, %d commands", err, len(w.engine.argvs()))
		}
	})

	t.Run("a CLI whose version does not begin with the pin is refused before any attempt [verifies SP-2513~1]", func(t *testing.T) {
		w := newWorld(t)
		w.engine.version = "2.1.296"
		err := w.batch().Run(context.Background(), w.invocation())
		if !errors.Is(err, app.ErrPinMismatch) || len(w.session.specs) != 0 {
			t.Errorf("error %v, %d sessions", err, len(w.session.specs))
		}
		if _, err := os.Stat(filepath.Join(w.work, study.RunsDir)); !errors.Is(err, os.ErrNotExist) {
			t.Error("an attempt directory was created")
		}
		var asked bool
		for _, argv := range w.engine.argvs() {
			asked = asked || (slices.Contains(argv, testClaude) && slices.Contains(argv, "--version"))
		}
		if !asked {
			t.Error("the image's CLI was not asked its version")
		}
	})

	t.Run("a freeze without an allowlist or quota patterns is refused [verifies SP-2530~1]", func(t *testing.T) {
		w := newWorld(t)
		w.inputs.QuotaPatterns = nil
		if err := w.batch().Run(context.Background(), w.invocation()); !errors.Is(err, runs.ErrInputs) {
			t.Errorf("error %v", err)
		}
	})
}

func TestDryRun(t *testing.T) {
	w := newWorld(t)
	inv := w.invocation()
	inv.DryRun = true
	if err := w.batch().Run(context.Background(), inv); err != nil {
		t.Fatal(err)
	}
	printed := w.stdout.String()

	t.Run("a dry run prints the order and every command of each attempt to come, and starts nothing [verifies SP-2532~1]", func(t *testing.T) {
		for _, wanted := range append([]string{
			"(standard input: CLAUDE_CODE_OAUTH_TOKEN=<stdin>)", dockerFront + " network create --internal", "--allow " + allowedHost + ":443",
			"permissions.deny: WebFetch, WebSearch", "check --json-out", "inspect --format", "restore shallnot.yaml", testClaude + " --version",
		}, w.inputs.Order...) {
			if !strings.Contains(printed, wanted) {
				t.Errorf("the plan lacks %q", wanted)
			}
		}
		if len(w.engine.argvs()) != 0 || len(w.session.specs) != 0 || *w.reads != 0 || strings.Contains(printed, testToken) {
			t.Errorf("the dry run started or read something: %d commands, %d sessions, %d reads", len(w.engine.argvs()), len(w.session.specs), *w.reads)
		}
	})
}

func TestRunMain(t *testing.T) {
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	work := tempDir(t)
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
