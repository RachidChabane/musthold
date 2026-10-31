package report

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/reportlint"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/analysis"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/catch"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
	rt "github.com/RachidChabane/musthold/tools/canary/internal/loop2/record/recordtest"
)

const (
	spec     = "../../../specs/loop-2-analysis.md"
	adr      = "../../../../../docs/adr/0017-the-loop-2-decision.md"
	broken   = "T-1"
	moduleDP = "musthold-dp"
)

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// flat joins the lines of a text with single spaces.
func flat(text string) string { return strings.Join(strings.Fields(text), " ") }

// scenario builds runs on six tasks of four runs per arm: the control escapes
// in controlEscapes runs of each task; in the treatment, caught runs are
// defective at the first stop and corrected after a spec-witness finding.
func scenario(controlEscapes, caught, treatmentEscapes int) []record.Run {
	var runs []record.Run
	for _, task := range []string{"ta", "tb", "tc", "td", "te", "tf"} {
		for repetition := 1; repetition <= 4; repetition++ {
			var control, treatment []rt.Option
			if repetition <= controlEscapes {
				control = append(control, rt.Defective(broken), rt.Adjudicated(repetition == 1))
			}
			switch {
			case repetition <= caught:
				treatment = append(treatment, rt.FirstStop(true, broken), rt.FindingBlock(true, rt.Finding(string(catch.SpecWitness), broken, moduleDP)))
			case repetition <= caught+treatmentEscapes:
				treatment = append(treatment, rt.FirstStop(true, broken), rt.Defective(broken), rt.Adjudicated(false))
			default:
				treatment = append(treatment, rt.FirstStop(false))
			}
			runs = append(runs, rt.Run(task, rt.Control, repetition, control...), rt.Run(task, rt.Treatment, repetition, treatment...))
		}
	}
	return runs
}

// summaryOf builds the summary of the runs as analyze writes it: built, then
// written to JSON and read back.
func summaryOf(t *testing.T, runs []record.Run, capReached bool) (analysis.Summary, []byte) {
	t.Helper()
	start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	records := map[string]record.Run{}
	var states []phase.RunState
	for _, run := range runs {
		records[run.Run] = run
		attempt := phase.Attempt{Number: 1, Run: study.Run{Outcome: study.OutcomeCompleted}}
		attempt.Run.StartedAt = start
		states = append(states, phase.RunState{Name: run.Run, Counted: 0, State: study.OutcomeCompleted, Attempts: []phase.Attempt{attempt}})
	}
	built := analysis.Build(analysis.BuildInput{
		Phase:   phase.Phase{Ended: true, CapReached: capReached, FirstStart: start, Cap: start.Add(analysis.CalendarCap), Runs: states},
		Records: records,
		Provenance: analysis.Provenance{
			Tags: []analyze.Tag{{Name: analysis.TagPrereg, Commit: "abc123"}}, InputsDigest: "def456", Tasks: []string{"ta"},
		},
		Digests: map[string]string{study.InputsFile: "def456"},
	})
	data, err := json.Marshal(built)
	if err != nil {
		t.Fatal(err)
	}
	var written analysis.Summary
	if err := json.Unmarshal(data, &written); err != nil {
		t.Fatal(err)
	}
	return written, data
}

func TestText(t *testing.T) {
	t.Run("the conclusion templates are the spec's [verifies SP-2826~1]", func(t *testing.T) {
		text := read(t, spec)
		for _, template := range Templates() {
			if !strings.Contains(text, `: "`+template+`"`) {
				t.Errorf("not in the spec: %s", template)
			}
		}
	})
	t.Run("the limits and the regime are ADR 0017's verbatim [verifies SP-2826~1]", func(t *testing.T) {
		text := flat(read(t, adr))
		if !strings.Contains(text, "**Limits stated in advance.** "+Limits) {
			t.Error("the limits are not ADR 0017's")
		}
		if !strings.Contains(text, "a negative result names its regime: "+Regime+".") {
			t.Error("the regime is not ADR 0017's")
		}
	})
}

func TestRender(t *testing.T) {
	t.Run("the report holds the conclusions, the outcome with its row and reading, every endpoint, the runs and the limits [verifies SP-2826~1]", func(t *testing.T) {
		summary, _ := summaryOf(t, scenario(3, 3, 0), false)
		got := Render(summary)
		if summary.Analysis.Primary.Outcome != decision.UsefulGain || !summary.Analysis.Catches.MainlySpecWitness {
			t.Fatalf("the scenario gives %s, mainly spec witness %v", summary.Analysis.Primary.Outcome, summary.Analysis.Catches.MainlySpecWitness)
		}
		sections := []string{
			"# loop-2 results", "## Provenance", "## Conclusions", OutcomeConclusion(summary.Analysis), SpecWitnessConclusion(summary.Analysis.Catches),
			H2Conclusion(summary.Analysis.H2), "## Outcome", OutcomeLine(summary.Analysis.Primary), "## Mutation-only effect",
			"## Catches and attribution", "## H2, correction", "## Instructions against the gate", "## Re-adjudication",
			"## Sensitivity without the disputed escapes", "## loop-1's secondary endpoints (unadjusted)", "**S8 per-criterion defects**",
			"sign test over tasks", "## Runs", "## Deviations", "## Limits", Limits, analyze.ExploratoryMarker,
		}
		last := -1
		for _, section := range sections {
			at := strings.Index(got, section)
			if at < 0 || at < last {
				t.Fatalf("%q missing or out of order", section)
			}
			last = at
		}
		if !strings.Contains(got, "| `ta-treatment-1` | completed | 1 completed | true | false | true | none | 1 |") {
			t.Fatal("the run table does not show a corrected catch")
		}
	})
	t.Run("at the calendar cap below the rule the conclusion is the inconclusive template with its reason [verifies SP-2826~1]", func(t *testing.T) {
		summary, _ := summaryOf(t, scenario(3, 3, 0), true)
		got := Render(summary)
		if !strings.Contains(got, OutcomeConclusion(summary.Analysis)) || !strings.Contains(got, summary.Analysis.Feasibility.Reason) ||
			strings.Contains(got, SpecWitnessConclusion(summary.Analysis.Catches)) {
			t.Fatal("the cap's conclusion")
		}
	})
}

func lint(t *testing.T, report string, summary []byte) []reportlint.Problem {
	t.Helper()
	problems, err := Lint(reportlint.Input{Report: report, Summary: summary})
	if err != nil {
		t.Fatal(err)
	}
	return problems
}

func hasRule(problems []reportlint.Problem, rule reportlint.Rule) bool {
	for _, problem := range problems {
		if problem.Rule == rule {
			return true
		}
	}
	return false
}

func TestLint(t *testing.T) {
	t.Run("a rendered report is clean in every scenario [verifies SP-2827~1]", func(t *testing.T) {
		for _, c := range []struct {
			name                     string
			control, caught, escapes int
			capReached               bool
		}{
			{"gain owed to the spec witness", 3, 3, 0, false},
			{"no difference", 1, 0, 1, false},
			{"harm", 0, 0, 3, false},
			{"inconclusive at the cap", 3, 3, 0, true},
		} {
			summary, data := summaryOf(t, scenario(c.control, c.caught, c.escapes), c.capReached)
			if problems := lint(t, Render(summary), data); len(problems) > 0 {
				t.Errorf("%s: %v", c.name, problems)
			}
		}
	})
	t.Run("loop-1's rules apply with loop-2's templates [verifies SP-2827~1]", func(t *testing.T) {
		summary, data := summaryOf(t, scenario(3, 3, 0), false)
		report := Render(summary) + "\nThe product works on 7/9 tasks.\nAbout 40 % of them.\n"
		problems := lint(t, report, data)
		for _, rule := range []reportlint.Rule{reportlint.RuleWord, reportlint.RuleFraction, reportlint.RulePercent} {
			if !hasRule(problems, rule) {
				t.Errorf("no %s problem in %v", rule, problems)
			}
		}
	})
	t.Run("the report must state summary.json's outcome and reading [verifies SP-2827~1]", func(t *testing.T) {
		summary, data := summaryOf(t, scenario(3, 3, 0), false)
		report := strings.Replace(Render(summary), OutcomeLine(summary.Analysis.Primary), "- outcome: **GAIN**, row 5 of ADR 0017's table: gain shown; whether it reaches δ stays open; a gain is shown.", 1)
		if problems := lint(t, report, data); !hasRule(problems, RuleOutcome) || problems[0].Line == 0 {
			t.Fatalf("problems %v", problems)
		}
	})
	t.Run("the spec-witness claim must match summary.json either way [verifies SP-2827~1]", func(t *testing.T) {
		summary, data := summaryOf(t, scenario(3, 3, 0), false)
		dropped := strings.Replace(Render(summary), SpecWitnessConclusion(summary.Analysis.Catches), "", 1)
		if !hasRule(lint(t, dropped, data), RuleSpecWitness) {
			t.Fatal("a missing claim passed")
		}
		null, nullData := summaryOf(t, scenario(1, 0, 1), false)
		claimed := Render(null) + "\n" + SpecWitnessConclusion(summary.Analysis.Catches) + "\n"
		if !hasRule(lint(t, claimed, nullData), RuleSpecWitness) {
			t.Fatal("an unearned claim passed")
		}
	})
}
