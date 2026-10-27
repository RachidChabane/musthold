package reportlint

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/conclusion"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const summary = `{"h1": {"control": "3/15", "bound": "1/3", "tasks": ["a"]}, "runs": [{"n": "2/4"}]}`

const deviationLog = "# Deviations\n" +
	"- 2026-11-03T08:00:00Z | protocol | all | §6.5 | the batch resumes inside the window | the operator resumed it at 08:00\n" +
	"the operator paused the batch for a day\n"

func lint(t *testing.T, report string) []Problem {
	t.Helper()
	problems, err := Lint(Input{Report: report, Summary: []byte(summary), Deviations: deviationLog})
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

func rules(problems []Problem) []Rule {
	var found []Rule
	for _, problem := range problems {
		found = append(found, problem.Rule)
	}
	return found
}

func expect(t *testing.T, report string, want ...Rule) {
	t.Helper()
	got := rules(lint(t, report))
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("%q: got %v, want %v", report, got, want)
	}
}

func TestWords(t *testing.T) {
	t.Run("a banned word fails as a whole word in any case [verifies SP-2606~1]", func(t *testing.T) {
		for _, word := range []string{"proves", "Proven", "GUARANTEE", "guarantees", "safe", "validated", "confirms", "Works"} {
			expect(t, "The gate "+word+" it.", RuleWord)
		}
		expect(t, "The workshop is unsafe, the reworks were proofed and the guaranteed rate held.")
	})
	t.Run("fenced blocks, inline code and quoted lines are skipped [verifies SP-2606~1]", func(t *testing.T) {
		expect(t, "```\nthis works\n9/10\n```\nThe `safe` flag and ``it works`` stay.\n> the gate works\n  > also quoted 7/8\n~~~\nproven\n~~~")
		expect(t, "An unclosed `span works", RuleWord)
	})
	t.Run("a filled conclusion template is exempt from the word rule [verifies SP-2606~1]", func(t *testing.T) {
		filled := conclusion.Fill(conclusion.H1Inconclusive, map[string]string{"reason": "it works", "k_t": "3", "n_t": "15", "k_c": "3", "n_c": "15"})
		expect(t, filled)
		expect(t, filled+" It works.", RuleWord)
	})
}

func TestFractionsAndPercent(t *testing.T) {
	t.Run("a fraction must be in summary.json [verifies SP-2607~1]", func(t *testing.T) {
		expect(t, "Escapes: 3/15 against the bound 1/3, and 2/4 runs.")
		expect(t, "Escapes: 4/15.", RuleFraction)
		expect(t, "Dates read 2026-11-03 and p = 0.05.")
	})
	t.Run("a percentage needs a fraction before it on its line [verifies SP-2607~1]", func(t *testing.T) {
		expect(t, "3/15 runs (20 %) escaped.")
		expect(t, "20 % of the runs escaped.", RulePercent)
		expect(t, "20 % of 3/15 runs escaped.", RulePercent)
		expect(t, "3/15 runs, 20 %, then 7 %.")
	})
}

func TestDollars(t *testing.T) {
	t.Run("dollars and USD need the word notional on their line [verifies SP-2608~1]", func(t *testing.T) {
		expect(t, "Cost: 3.20 notional USD per run.")
		expect(t, "Cost: $3.20, notional.")
		expect(t, "| Notional USD | 3.20 |")
		expect(t, "Cost: $3.20 per run.", RuleDollar)
		expect(t, "Cost: 3.20 USD per run.", RuleDollar)
		expect(t, "The USDA is no currency.")
		expect(t, "Spent $`x` here.", RuleDollar)
	})
}

func TestDeviationText(t *testing.T) {
	t.Run("the text of a deviation entry fails [verifies SP-2609~1]", func(t *testing.T) {
		expect(t, "Deviations: protocol 2; see DEVIATIONS.md.")
		expect(t, "Copied: 2026-11-03T08:00:00Z | protocol | all | §6.5 | the batch resumes inside the window | the operator resumed it at 08:00", RuleDeviation)
		expect(t, "Note: the operator paused the batch for a day.", RuleDeviation)
		expect(t, "The operator noted that the operator resumed it at 08:00.")
		expect(t, "> the operator paused the batch for a day")
	})
	t.Run("a short field of an entry does not match a number of the report [verifies SP-2609~1]", func(t *testing.T) {
		log := "- 2026-11-05T08:00:00Z | quota-stop | a-control-2 | §6.5 | a quota stop voids the run | 429"
		problems, err := Lint(Input{Report: "| tokens | 4290.00 | 429 |", Summary: []byte(summary), Deviations: log})
		if err != nil || len(problems) != 0 {
			t.Fatalf("got %v %v", problems, err)
		}
	})
}

// counted is a counted run.
func counted(task, arm string, repetition int, escaped bool) facts.Facts {
	usd := 1.25
	f := facts.Facts{
		Run: fmt.Sprintf("%s-%s-%d", task, arm, repetition), Task: task, Arm: arm, Repetition: repetition, Attempt: 1,
		Outcome: study.OutcomeCompleted, Accepted: true, Defective: escaped, Escaped: escaped, Violated: []string{},
		BlockCounts: map[facts.BlockKind]int{}, FindingCodes: map[string]int{}, Hits: map[study.Detector]int{},
		Strength: facts.Strength{Final: &facts.Fraction{K: 2, N: 4}, Equipped: &facts.Fraction{K: 1, N: 4}},
		Cost:     facts.Cost{Messages: 40, NotionalUSD: &usd, Tokens: map[string]facts.Tokens{"claude-sonnet-5-5": {Input: 10}}},
	}
	if escaped {
		f.Violated = []string{"T-2"}
	}
	if arm == analyze.ArmTreatment {
		yes := true
		f.Blocks = []facts.Block{{Call: 1, Kind: facts.KindFinding, Findings: []facts.Finding{{Code: "surviving_valid_mutant", Criterion: "T-1"}}, Edits: 1, PassingCall: 2, Corrected: true, OnTarget: true, Strengthened: &yes}}
		f.BlockCounts[facts.KindFinding], f.FindingCodes["surviving_valid_mutant"] = 1, 1
	} else {
		f.Voluntary = []facts.Voluntary{{Line: 3, Call: "Bash: musthold-dp prove --changed", Findings: []string{"awaiting_host"}}}
		f.Hits[study.DetectorSkipped] = 1
	}
	return f
}

// buildSummary builds the summary of a study of the tasks, the first task's control
// runs escaping.
func buildSummary(tasks int) analyze.Summary {
	judged := phase.Phase{Ended: true, CapReached: true, FirstStart: time.Date(2026, 11, 2, 21, 0, 0, 0, time.UTC)}
	judged.Cap = judged.FirstStart.Add(phase.CalendarCap)
	all := map[string]facts.Facts{}
	for index := range tasks {
		task := fmt.Sprintf("task-%d", index+1)
		if index == 0 {
			task = analyze.InSampleTask
		}
		for _, arm := range analyze.Arms() {
			for repetition := 1; repetition <= 3; repetition++ {
				f := counted(task, arm, repetition, index == 0 && arm == analyze.ArmControl)
				all[f.Run] = f
				attempt := phase.Attempt{Number: 1, Run: study.Run{Attempt: 1, Outcome: study.OutcomeCompleted}}
				judged.Runs = append(judged.Runs, phase.RunState{Name: f.Run, Attempts: []phase.Attempt{attempt}, Counted: 0, State: study.OutcomeCompleted})
			}
		}
	}
	judged.Runs = append(judged.Runs, phase.RunState{Name: "late-task-control-1", Counted: -1, State: study.OutcomeNotRun})
	at := time.Date(2026, 10, 26, 15, 55, 27, 0, time.UTC)
	return analyze.Build(analyze.BuildInput{
		Phase: judged, Facts: all, Deviations: study.ParseDeviations(deviationLog),
		Provenance: analyze.Provenance{
			Tags:         []analyze.Tag{{Name: analyze.TagPrereg, Commit: "9058fee", Time: &at}, {Name: analyze.TagInputs, Commit: "abc"}},
			InputsDigest: "00ff", Tasks: []string{analyze.InSampleTask, "task-2"},
			Dropped: []analyze.Dropped{{Domain: "water bill", Reason: "A5 diagonal works on no violation"}},
		},
		Digests: map[string]string{"inputs.json": "00ff"},
	})
}

func TestRenderedReport(t *testing.T) {
	for _, tasks := range []int{5, 4} {
		t.Run(fmt.Sprintf("the report rendered from a summary of %d tasks passes the lint [verifies SP-2604~1]", tasks), func(t *testing.T) {
			data, err := json.Marshal(buildSummary(tasks))
			if err != nil {
				t.Fatal(err)
			}
			var written analyze.Summary
			if err := json.Unmarshal(data, &written); err != nil {
				t.Fatal(err)
			}
			report := analyze.Render(written)
			problems, err := Lint(Input{Report: report, Summary: data, Deviations: deviationLog})
			if err != nil {
				t.Fatal(err)
			}
			if len(problems) > 0 {
				t.Fatalf("problems %v in\n%s", problems, report)
			}
			order := []string{"## Provenance", "## Conclusions", "## H1", "## H2", "## Secondary endpoints", "## Sensitivity analyses", "## Runs", "## Deviations", "## Limits", "## Exploratory discussion"}
			last := -1
			for _, heading := range order {
				at := strings.Index(report, heading)
				if at <= last {
					t.Fatalf("%s is out of order", heading)
				}
				last = at
			}
			if !strings.Contains(report, conclusion.Limits) || !strings.HasSuffix(strings.TrimSpace(report), analyze.ExploratoryMarker) ||
				!strings.Contains(report, "`late-task-control-1` | none | not-run") ||
				!strings.Contains(report, fmt.Sprintf("- treatment: %d counted runs", 3*tasks)) {
				t.Fatalf("report\n%s", report)
			}
			h1 := analyze.H1Conclusion(written.Analysis.H1)
			supported := conclusion.Pattern(conclusion.H1Supported).MatchString(h1)
			if !strings.Contains(report, h1) || supported != (tasks == 5) {
				t.Fatalf("conclusion %q", h1)
			}
		})
	}
}

func TestVerb(t *testing.T) {
	t.Run("the verb reads results/ and fails on a problem [verifies SP-2606~1]", func(t *testing.T) {
		repo := t.TempDir()
		results := filepath.Join(repo, "experiments", "loop-1", analyze.ResultsDir)
		write := func(name, content string) {
			if err := os.MkdirAll(results, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(results, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		write(analyze.SummaryFile, summary)
		write(analyze.ReportFile, "Escapes: 3/15.\n")
		var stdout, stderr bytes.Buffer
		args := []string{"--repo", repo, "--work", t.TempDir()}
		if code := Main(context.Background(), args, &stdout, &stderr); code != exitOK {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
		write(analyze.ReportFile, "Escapes: 4/15, which proves it.\n")
		write(study.DeviationsFile, deviationLog)
		stderr.Reset()
		if code := Main(context.Background(), args, &stdout, &stderr); code != exitRefused || !strings.Contains(stderr.String(), "REPORT.md:1: banned word: proves") {
			t.Fatalf("exit %d: %s", code, stderr.String())
		}
		if code := Main(context.Background(), []string{"extra"}, &stdout, &stderr); code != exitUsage {
			t.Fatalf("exit %d", code)
		}
	})
}
