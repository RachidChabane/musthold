package runs_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/startup"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestStudyConfiguration(t *testing.T) {
	t.Run("loop-1.json parses with its toolchain and the arms control and treatment, and its order is the seeded order of the agent-loop runner [verifies SP-2510~1]", func(t *testing.T) {
		data, err := os.ReadFile(filepath.Join(repoFromHere, runs.DefaultConfig))
		if err != nil {
			t.Fatal(err)
		}
		parsed, err := config.Parse(data)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(parsed.Arms, []config.Arm{config.ArmControl, config.ArmTreatment}) || !slices.Equal(parsed.Toolchain, []string{"python3"}) || parsed.Seed != 20261025 {
			t.Errorf("arms %v, toolchain %v, seed %d", parsed.Arms, parsed.Toolchain, parsed.Seed)
		}
		var want []string
		for _, run := range parsed.Runs() {
			want = append(want, run.Name())
		}
		if got := runs.Order(parsed); !slices.Equal(got, want) || len(got) != len(parsed.Tasks)*2*parsed.Repetitions {
			t.Errorf("order %v, want %v", got, want)
		}
	})
}

func TestClassify(t *testing.T) {
	for _, tc := range []struct {
		name  string
		facts runs.Facts
		want  study.Outcome
	}{
		{"completed", runs.Facts{Started: true}, study.OutcomeCompleted},
		{"message cap", runs.Facts{Started: true, Killed: evidence.KilledTurnCap}, study.OutcomeCappedMessages},
		{"time cap", runs.Facts{Started: true, Killed: evidence.KilledTimeout}, study.OutcomeCappedTime},
		{"budget cap", runs.Facts{Started: true, BudgetCapped: true}, study.OutcomeCappedBudget},
		{"memory after the start", runs.Facts{Started: true, OOMKilled: true}, study.OutcomeCappedMemory},
		{"memory before the start", runs.Facts{OOMKilled: true}, study.OutcomeHarnessError},
		{"a failed preparation", runs.Facts{Harness: "shallnot init exited 1"}, study.OutcomeHarnessError},
		{"an interrupted runner", runs.Facts{Started: true, Killed: evidence.KilledInterrupted}, study.OutcomeHarnessError},
		{"a failed start", runs.Facts{Killed: startup.ReasonPlugins}, study.OutcomeHarnessError},
		{"no init event", runs.Facts{}, study.OutcomeHarnessError},
		{"another session model", runs.Facts{Started: true, ModelMismatch: "claude-other"}, study.OutcomeHarnessError},
		{"a quota stop", runs.Facts{Started: true, Quota: true, Killed: runs.ReasonQuota}, study.OutcomeQuotaStop},
		{"a quota stop before init", runs.Facts{Quota: true}, study.OutcomeQuotaStop},
	} {
		t.Run(tc.name+" [verifies SP-2524~1]", func(t *testing.T) {
			got, reason := runs.Classify(tc.facts)
			if got != tc.want || (got == study.OutcomeHarnessError) != (reason != "") {
				t.Errorf("%+v: %s (%q), want %s", tc.facts, got, reason, tc.want)
			}
		})
	}
}

func watch(t *testing.T, at time.Time) *runs.QuotaWatch {
	t.Helper()
	q, err := runs.NewQuotaWatch(runs.QuotaPatterns(), func() time.Time { return at })
	if err != nil {
		t.Fatal(err)
	}
	return q
}

func TestQuotaWatch(t *testing.T) {
	now := time.Date(2026, time.November, 3, 1, 0, 0, 0, time.UTC)
	t.Run("an error result that names a usage limit, and an API error with HTTP 429, are quota stops; other lines are not [verifies SP-2525~1]", func(t *testing.T) {
		for line, want := range map[string]bool{
			`{"type":"result","subtype":"success","is_error":true,"result":"Claude AI usage limit reached|1762138800"}`:    true,
			`{"type":"result","subtype":"success","is_error":true,"result":"You've hit your limit · resets 3am (UTC)"}`:    true,
			`{"type":"system","subtype":"api_retry","attempt":1,"error_status":429,"error":"rate_limit"}`:                  true,
			`{"type":"result","subtype":"error_during_execution","is_error":true,"api_error_status":429}`:                  true,
			`{"type":"result","subtype":"success","is_error":false,"result":"I checked the usage limit of the function."}`: false,
			`{"type":"result","subtype":"error_during_execution","is_error":true,"result":"the tool crashed"}`:             false,
			`{"type":"system","subtype":"api_retry","attempt":1,"error_status":529,"error":"overloaded"}`:                  false,
			`not json`: false,
		} {
			if got := watch(t, now).Line([]byte(line)) == runs.ReasonQuota; got != want {
				t.Errorf("%s: quota %v, want %v", line, got, want)
			}
		}
	})

	t.Run("only the first quota stop is reported, with the reset time it names [verifies SP-2525~1]", func(t *testing.T) {
		q := watch(t, now)
		if q.Line([]byte(`{"type":"result","is_error":true,"result":"Claude AI usage limit reached|1762138800"}`)) != runs.ReasonQuota {
			t.Fatal("no quota stop")
		}
		if q.Line([]byte(`{"type":"system","subtype":"api_retry","error_status":429}`)) != "" {
			t.Error("a second stop was reported")
		}
		stopped, reset := q.Stopped()
		if !stopped || reset == nil || !reset.Equal(time.Unix(1762138800, 0)) {
			t.Errorf("stopped %v, reset %v", stopped, reset)
		}
	})

	t.Run("the reset time is read from a Unix time, an RFC 3339 time, a UTC time and a time of day in a named zone [verifies SP-2525~1]", func(t *testing.T) {
		paris, err := time.LoadLocation("Europe/Paris")
		if err != nil {
			t.Skip("no time zone database")
		}
		for text, want := range map[string]time.Time{
			"usage limit reached|1762138800":                           time.Unix(1762138800, 0).UTC(),
			`{"resetsAt": 1762138800}`:                                 time.Unix(1762138800, 0).UTC(),
			"limit resets at 2026-11-03T05:30:00+01:00":                time.Date(2026, time.November, 3, 4, 30, 0, 0, time.UTC),
			"spend limit reached (daily; resets 2026-11-04 00:00 UTC)": time.Date(2026, time.November, 4, 0, 0, 0, 0, time.UTC),
			"You've hit your limit · resets 3pm (Europe/Paris)":        time.Date(2026, time.November, 3, 15, 0, 0, 0, paris).UTC(),
			"Usage limit reached · limit resets 1:30am (Europe/Paris)": time.Date(2026, time.November, 4, 1, 30, 0, 0, paris).UTC(),
		} {
			got := runs.ParseReset(text, now)
			if got == nil || !got.Equal(want) {
				t.Errorf("%q: %v, want %v", text, got, want)
			}
		}
		if runs.ParseReset("usage limit reached", now) != nil {
			t.Error("a reset was read from an error that names none")
		}
	})

	t.Run("a Unix reset time is read back whatever its value [verifies SP-2525~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			seconds := rapid.Int64Range(1_000_000_000, 9_999_999_999).Draw(t, "seconds")
			got := runs.ParseReset("Claude AI usage limit reached|"+strconv.FormatInt(seconds, 10), now)
			if got == nil || got.Unix() != seconds {
				t.Fatalf("%d gave %v", seconds, got)
			}
		})
	})

	t.Run("a pattern that is not a regular expression is refused [verifies SP-2525~1]", func(t *testing.T) {
		if _, err := runs.NewQuotaWatch([]string{"("}, time.Now); !errors.Is(err, runs.ErrQuotaPattern) {
			t.Errorf("error %v", err)
		}
	})
}

func attemptOf(number int, outcome study.Outcome) runs.Attempt {
	return runs.Attempt{Number: number, Run: &study.Run{Attempt: number, Outcome: outcome}}
}

func TestLedger(t *testing.T) {
	t.Run("harness errors use up the attempts, quota stops do not, and an attempt without run.json is a harness error [verifies SP-2527~1]", func(t *testing.T) {
		quota := runs.Ledger{Attempts: []runs.Attempt{attemptOf(1, study.OutcomeQuotaStop), attemptOf(2, study.OutcomeQuotaStop), attemptOf(3, study.OutcomeQuotaStop), attemptOf(4, study.OutcomeHarnessError)}}
		if quota.Done() || quota.Next() != 5 || quota.HarnessErrors() != 1 {
			t.Errorf("quota stops: done %v, next %d, errors %d", quota.Done(), quota.Next(), quota.HarnessErrors())
		}
		blocked := runs.Ledger{Attempts: []runs.Attempt{attemptOf(1, study.OutcomeHarnessError), {Number: 2}, attemptOf(3, study.OutcomeQuotaStop), attemptOf(4, study.OutcomeHarnessError)}}
		if !blocked.HarnessBlocked() || !blocked.Done() || blocked.State(false) != study.OutcomeHarnessBlocked {
			t.Errorf("three harness errors: blocked %v, state %s", blocked.HarnessBlocked(), blocked.State(false))
		}
		counted := runs.Ledger{Attempts: []runs.Attempt{attemptOf(1, study.OutcomeHarnessError), attemptOf(2, study.OutcomeCappedTime)}}
		if !counted.Done() || counted.State(true) != study.OutcomeCappedTime {
			t.Errorf("counted: state %s", counted.State(true))
		}
	})

	t.Run("the attempts are numbered from 1 and each next one follows the last, whatever came before [verifies SP-2527~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			outcomes := rapid.SliceOfN(rapid.SampledFrom([]study.Outcome{study.OutcomeQuotaStop, study.OutcomeHarnessError, ""}), 0, 8).Draw(t, "outcomes")
			var ledger runs.Ledger
			errorsSeen := 0
			for index, outcome := range outcomes {
				if ledger.Next() != index+1 {
					t.Fatalf("next %d after %d attempts", ledger.Next(), index)
				}
				attempt := runs.Attempt{Number: index + 1}
				if outcome != "" {
					attempt = attemptOf(index+1, outcome)
				}
				ledger.Attempts = append(ledger.Attempts, attempt)
				if outcome != study.OutcomeQuotaStop {
					errorsSeen++
				}
				if ledger.HarnessBlocked() != (errorsSeen >= study.MaxAttempts) {
					t.Fatalf("%d harness errors, blocked %v", errorsSeen, ledger.HarnessBlocked())
				}
			}
		})
	})

	t.Run("a run waits for the reset its last quota stop recorded, and only then [verifies SP-2528~1]", func(t *testing.T) {
		reset := start.Add(time.Hour)
		stop := attemptOf(1, study.OutcomeQuotaStop)
		stop.Run.QuotaReset = &reset
		if at := (runs.Ledger{Attempts: []runs.Attempt{stop}}).NotBefore(); at == nil || !at.Equal(reset) {
			t.Errorf("not before %v", at)
		}
		if at := (runs.Ledger{Attempts: []runs.Attempt{stop, attemptOf(2, study.OutcomeHarnessError)}}).NotBefore(); at != nil {
			t.Errorf("a later harness error still waits for %v", at)
		}
	})

	t.Run("the calendar cap ends the phase 28 days after the first start, and each run takes its state after its last attempt [verifies SP-2529~1]", func(t *testing.T) {
		first := runs.Attempt{Number: 1, Run: &study.Run{Outcome: study.OutcomeQuotaStop}}
		first.Run.StartedAt = start
		later := runs.Attempt{Number: 1, Run: &study.Run{Outcome: study.OutcomeHarnessError}}
		later.Run.StartedAt = start.Add(48 * time.Hour)
		ledgers := []runs.Ledger{{Name: "a", Attempts: []runs.Attempt{first}}, {Name: "b", Attempts: []runs.Attempt{later}}, {Name: "c"}}
		if got := runs.FirstStart(ledgers); got == nil || !got.Equal(start) {
			t.Fatalf("first start %v", got)
		}
		if runs.Capped(runs.FirstStart(ledgers), start.Add(runs.CalendarCap-time.Second)) || !runs.Capped(runs.FirstStart(ledgers), start.Add(runs.CalendarCap)) {
			t.Error("the cap is not at 28 days")
		}
		if runs.PhaseOver(ledgers, start.Add(time.Hour)) || !runs.PhaseOver(ledgers, start.Add(runs.CalendarCap)) {
			t.Error("the phase ends before the cap or not at it")
		}
		want := []study.Outcome{study.OutcomeQuotaBlocked, study.OutcomeHarnessBlocked, study.OutcomeNotRun}
		for index, ledger := range ledgers {
			if got := ledger.State(true); got != want[index] {
				t.Errorf("%s: %s, want %s", ledger.Name, got, want[index])
			}
			if got := ledger.State(false); got != "" {
				t.Errorf("%s before the cap: %s", ledger.Name, got)
			}
		}
	})

	t.Run("an order that is not the configuration's is refused [verifies SP-2530~1]", func(t *testing.T) {
		data, _ := os.ReadFile(filepath.Join(repoFromHere, runs.DefaultConfig))
		parsed, _ := config.Parse(data)
		order := runs.Order(parsed)
		if err := runs.CheckOrder(parsed, study.Inputs{Order: order}); err != nil {
			t.Fatal(err)
		}
		reversed := slices.Clone(order)
		slices.Reverse(reversed)
		if err := runs.CheckOrder(parsed, study.Inputs{Order: reversed}); !errors.Is(err, runs.ErrOrder) {
			t.Errorf("error %v", err)
		}
	})
}

func TestEquippedDigest(t *testing.T) {
	t.Run("the digest is the same wherever the run directory lies, and changes with any file [verifies SP-2517~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			names := rapid.SliceOfNDistinct(rapid.StringMatching(`[a-z]{1,6}(/[a-z]{1,6})?\.(py|md|yaml)`), 1, 5, rapid.ID[string]).Draw(t, "names")
			dirA := "/work/runs/" + rapid.StringMatching(`[a-z]{1,8}-[0-9]`).Draw(t, "a")
			dirB := "/elsewhere/" + rapid.StringMatching(`[a-z]{1,8}`).Draw(t, "b")
			inA, inB := map[string][]byte{}, map[string][]byte{}
			for _, name := range names {
				body := rapid.StringMatching(`[a-z ]{0,12}`).Draw(t, "body")
				inA[name], inB[name] = []byte(body+dirA+"/project"), []byte(body+dirB+"/project")
			}
			if runs.EquippedDigest(inA, dirA) != runs.EquippedDigest(inB, dirB) {
				t.Fatal("the digest depends on the run directory")
			}
			changed := map[string][]byte{}
			for name, content := range inA {
				changed[name] = content
			}
			changed[names[0]] = append(changed[names[0]], 'x')
			if runs.EquippedDigest(changed, dirA) == runs.EquippedDigest(inA, dirA) {
				t.Fatal("a changed file kept the digest")
			}
		})
	})
}

func TestProvenance(t *testing.T) {
	inputs := study.Inputs{
		ClaudeCode: study.ClaudeCode{Version: testPin, SHA256: "abc"}, Shallnot: study.Shallnot{Version: "shallnot 0.5.1"},
		Toolchains: map[string]string{"python3": "Python 3.13.14"}, CLIAuxiliaryModels: []string{"claude-haiku-5"},
	}
	versions := map[string]string{"claude": testPin + " (Claude Code)", runs.ShallnotVersionKey: "shallnot 0.5.1", "python3": "Python 3.13.14"}

	t.Run("an attempt that records the frozen versions and models has no provenance difference [verifies SP-2526~1]", func(t *testing.T) {
		if got := runs.Provenance(inputs, versions, "abc", testModel, []string{"claude-haiku-5", testModel}); len(got) != 0 {
			t.Errorf("differences %v", got)
		}
	})

	t.Run("a toolchain version, the binary and a model beyond the auxiliary ones are each a difference [verifies SP-2526~1]", func(t *testing.T) {
		skewed := map[string]string{"claude": testPin, runs.ShallnotVersionKey: "shallnot 0.5.1", "python3": "Python 3.13.15"}
		got := runs.Provenance(inputs, skewed, "def", testModel, []string{"claude-opus-9", testModel})
		if len(got) != 3 || !strings.Contains(strings.Join(got, "\n"), "claude-opus-9") || !strings.Contains(strings.Join(got, "\n"), "python3") {
			t.Errorf("differences %v", got)
		}
	})
}

func TestDeviationLog(t *testing.T) {
	dir := tempDir(t)
	stamped := &stamps{}
	log := runs.DeviationLog{Files: adapters.OSFiles{}, Path: filepath.Join(dir, study.DeviationsFile), Stamper: stamped}
	entry := func(kind study.Kind) study.Deviation {
		return study.Deviation{Time: start, Kind: kind, Scope: []string{"boxoffice-control-1"}, Section: "6.4", Says: "the rule", Happened: "what happened"}
	}
	for _, kind := range []study.Kind{study.KindHarnessError, study.KindTokenExposed} {
		if err := log.Log(context.Background(), entry(kind)); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("entries are appended under the log's title in the log's format, and the timestamped kinds reach the stamper [verifies SP-2531~1]", func(t *testing.T) {
		text := read(t, log.Path)
		if !strings.HasPrefix(text, study.DeviationsTitle+"\n") {
			t.Errorf("log:\n%s", text)
		}
		parsed := study.ParseDeviations(text)
		if len(parsed) != 2 || parsed[0].Kind != study.KindHarnessError || parsed[1].Kind != study.KindTokenExposed || parsed[1].Raw != "" {
			t.Errorf("entries %+v", parsed)
		}
		if len(stamped.lines) != 1 || !strings.Contains(stamped.lines[0], "token-exposed") {
			t.Errorf("stamped %v", stamped.lines)
		}
		if logged, _ := log.Logged(study.KindHarnessError, "boxoffice-control-1"); !logged {
			t.Error("the logged entry is not found")
		}
		if logged, _ := log.Logged(study.KindHarnessBlocked, "boxoffice-control-1"); logged {
			t.Error("an entry that was never logged is found")
		}
	})
}
