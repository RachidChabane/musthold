package analysis

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/catch"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
	rt "github.com/RachidChabane/musthold/tools/canary/internal/loop2/record/recordtest"
)

const (
	broken    = "T-1"
	moduleDP  = "musthold-dp"
	tolerance = 1e-9
)

var witness = rt.Finding(string(catch.SpecWitness), broken, moduleDP)

// arm builds runs of one task and arm: escapes accepted defective changes,
// then clean accepted ones up to runs, each with the options.
func arm(task, side string, runs, escapes int, options ...rt.Option) []record.Run {
	var built []record.Run
	for index := range runs {
		opts := append([]rt.Option{}, options...)
		if index < escapes {
			opts = append(opts, rt.Defective(broken), rt.Adjudicated(false))
		}
		built = append(built, rt.Run(task, side, index+1, opts...))
	}
	return built
}

// design builds tasks of runs per arm with the escapes per arm of each task.
func design(tasks []string, runs, controlEscapes, treatmentEscapes int) []record.Run {
	var all []record.Run
	for _, task := range tasks {
		all = append(all, arm(task, rt.Control, runs, controlEscapes)...)
		all = append(all, arm(task, rt.Treatment, runs, treatmentEscapes)...)
	}
	return all
}

func names(prefix string, n int) []string {
	var found []string
	for index := range n {
		found = append(found, prefix+string(rune('a'+index)))
	}
	return found
}

func TestDecidePrimary(t *testing.T) {
	t.Run("six tasks where control escapes 3 of 4 and treatment never: the reference's USEFUL GAIN [verifies SP-2809~1]", func(t *testing.T) {
		got := DecidePrimary(design(names("t", 6), 4, 3, 0))
		// experiments/loop-2/script/power: p_less 1.3281030862990755e-07,
		// p_more 0.9999999999999997, score(0.15) 4.148287854748612.
		if math.Abs(got.PLess.Value-1.3281030862990755e-07) > tolerance || math.Abs(got.PMore.Value-0.9999999999999997) > tolerance {
			t.Fatalf("tails %+v %+v", got.PLess, got.PMore)
		}
		if math.Abs(float64(got.ScoreAtMargin)-4.148287854748612) > 1e-8 || got.Outcome != decision.UsefulGain || got.Row != 3 {
			t.Fatalf("score %v, outcome %s row %d", got.ScoreAtMargin, got.Outcome, got.Row)
		}
		if got.Control.Fraction != (facts.Fraction{K: 18, N: 24}) || got.Treatment.Fraction != (facts.Fraction{K: 0, N: 24}) || got.RiskReduction != 0.75 {
			t.Fatalf("rates %+v %+v %v", got.Control, got.Treatment, got.RiskReduction)
		}
		if !got.IntervalAgrees || got.Interval.Low < decision.Margin || len(got.PerTask) != 6 || got.PerTask[0].Control != (facts.Fraction{K: 3, N: 4}) {
			t.Fatalf("interval %+v, per task %+v", got.Interval, got.PerTask)
		}
	})
	t.Run("the same counts with the treatment refusing its clean changes is REJECTION-ONLY [verifies SP-2809~1]", func(t *testing.T) {
		var runs []record.Run
		for _, task := range names("t", 6) {
			runs = append(runs, arm(task, rt.Control, 4, 3)...)
			runs = append(runs, arm(task, rt.Treatment, 4, 0, rt.Refused(record.CausePending))...)
		}
		got := DecidePrimary(runs)
		if got.Outcome != decision.RejectionOnly || got.Guards.G.Refused != (facts.Fraction{K: 24, N: 24}) || got.Guards.G.Holds {
			t.Fatalf("outcome %s, guards %+v", got.Outcome, got.Guards)
		}
		if got.Guards.Y.Control != (facts.Fraction{K: 6, N: 24}) || got.Guards.Y.Treatment != (facts.Fraction{K: 0, N: 24}) || got.Guards.Y.Holds {
			t.Fatalf("guard Y %+v", got.Guards.Y)
		}
	})
	t.Run("equal escape rates are the reference's INCONCLUSIVE, and a task with one run in an arm is excluded [verifies SP-2809~1]", func(t *testing.T) {
		runs := append(design(names("t", 6), 4, 1, 1), arm("lone", rt.Control, 3, 3)...)
		runs = append(runs, arm("lone", rt.Treatment, 1, 0)...)
		got := DecidePrimary(runs)
		if math.Abs(got.PLess.Value-0.6227966769798297) > tolerance || math.Abs(float64(got.ScoreAtMargin)+1.1188932344921536) > 1e-8 {
			t.Fatalf("evidence %+v %v", got.PLess, got.ScoreAtMargin)
		}
		if got.Outcome != decision.Inconclusive || !slices.Equal(got.Excluded, []string{"lone"}) || len(got.Tasks) != 6 {
			t.Fatalf("outcome %s, tasks %v, excluded %v", got.Outcome, got.Tasks, got.Excluded)
		}
	})
	t.Run("an infinite score survives summary.json [verifies SP-2809~1]", func(t *testing.T) {
		for _, value := range []Real{Real(math.Inf(1)), Real(math.Inf(-1)), 1.5} {
			data, err := json.Marshal(value)
			if err != nil {
				t.Fatal(err)
			}
			var back Real
			if err := json.Unmarshal(data, &back); err != nil || back != value {
				t.Fatalf("%v gives %s and back %v, %v", value, data, back, err)
			}
		}
	})
}

func TestFeasible(t *testing.T) {
	t.Run("at the cap the analysis needs 20 tasks with 6 counted runs per arm [verifies SP-2811~1]", func(t *testing.T) {
		enough := design(names("t", 20), 6, 0, 0)
		if got := Feasible(true, enough); !got.Runs || got.Tasks != 20 {
			t.Fatalf("20 tasks: %+v", got)
		}
		short := append(design(names("t", 19), 6, 0, 0), design([]string{"u"}, 5, 0, 0)...)
		got := Feasible(true, short)
		if got.Runs || got.Tasks != 19 || got.Reason == "" {
			t.Fatalf("19 tasks: %+v", got)
		}
		if !Feasible(false, short).Runs {
			t.Fatal("without the cap the analysis runs")
		}
	})
	t.Run("below the rule every outcome is INCONCLUSIVE with its reason [verifies SP-2811~1]", func(t *testing.T) {
		got := Analyse(true, design(names("t", 6), 4, 3, 0))
		for _, primary := range []Primary{got.Primary, got.MutationOnly, got.Readjudication.Sensitivity} {
			if primary.Outcome != decision.Inconclusive || primary.Reason != got.Feasibility.Reason || primary.Row != 7 {
				t.Fatalf("%+v", primary)
			}
		}
		if Analyse(false, design(names("t", 6), 4, 3, 0)).Primary.Outcome != decision.UsefulGain {
			t.Fatal("without the cap the outcome is decided")
		}
	})
	t.Run("the run phase ends 45 days after its first start [verifies SP-2810~1]", func(t *testing.T) {
		start := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
		attempts := map[string][]phase.Attempt{"t-control-1": {{Number: 1, Run: study.Run{Outcome: study.OutcomeCompleted}}}}
		attempts["t-control-1"][0].Run.StartedAt = start
		order := []string{"t-control-1", "t-treatment-1"}
		if Calendar.Judge(order, attempts, start.Add(CalendarCap-time.Second)).Ended {
			t.Fatal("ended before 45 days")
		}
		at := Calendar.Judge(order, attempts, start.Add(CalendarCap))
		if !at.Ended || !at.CapReached || at.Runs[1].State != study.OutcomeNotRun {
			t.Fatalf("at 45 days: %+v", at)
		}
	})
}

func corrected(task string, repetition int, right bool) record.Run {
	return rt.Run(task, rt.Treatment, repetition, rt.FindingBlock(right, rt.Finding("surviving_valid_mutant", broken, moduleDP)))
}

func TestDecideH2(t *testing.T) {
	t.Run("not testable under 10 runs with a finding block, then the exact binomial rule [verifies SP-2812~1]", func(t *testing.T) {
		runsOf := func(right, wrong int) []record.Run {
			var runs []record.Run
			for index := range right {
				runs = append(runs, corrected("t", index+1, true))
			}
			for index := range wrong {
				runs = append(runs, corrected("t", right+index+1, false))
			}
			return runs
		}
		cases := []struct {
			right, wrong int
			want         string
		}{
			// 9 of 9: p = 1/512, but fewer than 10 runs.
			{9, 0, DecisionNotTestable},
			// 9 of 10: P(X >= 9) = 11/1024 <= 0.05.
			{9, 1, analyze.DecisionSupported},
			// 8 of 10: P(X >= 8) = 56/1024 > 0.05; upper bound above 0.5.
			{8, 2, analyze.DecisionInconclusive},
			// 1 of 12: Clopper-Pearson upper bound 0.3848 < 0.5.
			{1, 11, analyze.DecisionNotSupported},
			{6, 6, analyze.DecisionInconclusive},
		}
		for _, c := range cases {
			got := DecideH2(runsOf(c.right, c.wrong))
			if got.Decision != c.want || got.Corrected.Fraction != (facts.Fraction{K: c.right, N: c.right + c.wrong}) {
				t.Errorf("%d of %d: %+v, want %s", c.right, c.right+c.wrong, got, c.want)
			}
		}
		if DecideH2(runsOf(9, 1)).P.Exact != "11/1024" {
			t.Fatal("the exact p of 9 of 10")
		}
	})
}

func TestSetGate(t *testing.T) {
	t.Run("the first-stop difference is the instructions, the rest of the escape difference the gate [verifies SP-2817~1]", func(t *testing.T) {
		var runs []record.Run
		// Control: 4 graded runs, 2 defective at the first stop, both escape; one
		// run without a grade.
		runs = append(runs,
			rt.Run("t", rt.Control, 1, rt.FirstStop(true, broken), rt.Defective(broken), rt.Adjudicated(false)),
			rt.Run("t", rt.Control, 2, rt.FirstStop(true, broken), rt.Defective(broken), rt.Adjudicated(false)),
			rt.Run("t", rt.Control, 3, rt.FirstStop(false)),
			rt.Run("t", rt.Control, 4, rt.FirstStop(false)),
			rt.Run("t", rt.Control, 5),
		)
		// Treatment: 4 graded runs, 1 defective at the first stop and corrected.
		runs = append(runs,
			rt.Run("t", rt.Treatment, 1, rt.FirstStop(true, broken)),
			rt.Run("t", rt.Treatment, 2, rt.FirstStop(false)),
			rt.Run("t", rt.Treatment, 3, rt.FirstStop(false)),
			rt.Run("t", rt.Treatment, 4, rt.FirstStop(false)),
		)
		got := SetGate(runs)
		control, treatment := got.PerArm[rt.Control], got.PerArm[rt.Treatment]
		if control.Graded != 4 || control.Ungraded != 1 || control.FirstStopDefect.Fraction != (facts.Fraction{K: 2, N: 4}) || control.Escape.Fraction != (facts.Fraction{K: 2, N: 4}) {
			t.Fatalf("control %+v", control)
		}
		if treatment.FirstStopDefect.Fraction != (facts.Fraction{K: 1, N: 4}) || treatment.Escape.Fraction != (facts.Fraction{K: 0, N: 4}) {
			t.Fatalf("treatment %+v", treatment)
		}
		// Instructions: 2/4 - 1/4 = 0.25. Gate: (1/4 - 0) - (2/4 - 2/4) = 0.25.
		// Their sum is the escape difference over graded runs, 2/4 - 0 = 0.5.
		if got.Instructions != 0.25 || got.GateEffect != 0.25 {
			t.Fatalf("instructions %v, gate %v", got.Instructions, got.GateEffect)
		}
		if got.PLess.Exact == "" || got.PMore.Exact == "" {
			t.Fatalf("test %+v", got)
		}
	})
}

func TestReadjudicate(t *testing.T) {
	runs := design(names("t", 6), 4, 3, 0)
	// Dispute every escape of the first two tasks.
	for index := range runs {
		if runs[index].Escaped && (runs[index].Task == "ta" || runs[index].Task == "tb") {
			rt.Adjudicated(true)(&runs[index])
		}
	}
	got := Readjudicate(runs)
	t.Run("each escape is listed with its sentence and verdicts, and disputed when a judge dissents [verifies SP-2818~1]", func(t *testing.T) {
		if len(got.Escapes) != 18 || got.Disputed != (facts.Fraction{K: 6, N: 18}) {
			t.Fatalf("escapes %d, disputed %v", len(got.Escapes), got.Disputed)
		}
		first := got.Escapes[0]
		if first.Criterion != broken || first.Sentence == "" || len(first.Verdicts) != 2 || !first.Disputed {
			t.Fatalf("first escape %+v", first)
		}
	})
	t.Run("the sensitivity decides again without the runs of the disputed escapes [verifies SP-2819~1]", func(t *testing.T) {
		// Without the disputed runs the first two tasks hold one control run each,
		// so they leave the strata.
		if len(got.Sensitivity.Tasks) != 4 || got.Sensitivity.Control.Fraction != (facts.Fraction{K: 12, N: 16}) {
			t.Fatalf("sensitivity %+v", got.Sensitivity)
		}
	})
}

func TestAnalyse(t *testing.T) {
	t.Run("the mutation-only recount counts spec-witness catches as escapes through the same rule [verifies SP-2816~1]", func(t *testing.T) {
		var runs []record.Run
		for _, task := range names("t", 6) {
			runs = append(runs, arm(task, rt.Control, 4, 3)...)
			for repetition := 1; repetition <= 4; repetition++ {
				options := []rt.Option{}
				if repetition <= 3 {
					options = append(options, rt.FirstStop(true, broken), rt.FindingBlock(true, witness))
				}
				runs = append(runs, rt.Run(task, rt.Treatment, repetition, options...))
			}
		}
		got := Analyse(false, runs)
		if got.Primary.Outcome != decision.UsefulGain || got.Catches.SpecWitness != (facts.Fraction{K: 18, N: 18}) || !got.Catches.MainlySpecWitness {
			t.Fatalf("primary %s, catches %+v", got.Primary.Outcome, got.Catches.SpecWitness)
		}
		if got.MutationOnly.Treatment.Fraction != (facts.Fraction{K: 18, N: 24}) || got.MutationOnly.Outcome == decision.UsefulGain || got.MutationOnly.Outcome == decision.Gain {
			t.Fatalf("mutation-only %+v", got.MutationOnly)
		}
		if got.MutationOnly.Guards.Y.Treatment != (facts.Fraction{K: 6, N: 24}) {
			t.Fatalf("the recount's guards read the recounted runs: %+v", got.MutationOnly.Guards.Y)
		}
	})
	t.Run("loop-1's S1 to S8 and sign tests are kept [verifies SP-2820~1]", func(t *testing.T) {
		runs := design(names("t", 6), 4, 3, 0)
		got := Analyse(false, runs).Kept
		if got.S1.Control.Fraction != (facts.Fraction{K: 18, N: 24}) || got.S3[rt.Treatment].Yield.Fraction != (facts.Fraction{K: 24, N: 24}) {
			t.Fatalf("S1 %+v, S3 %+v", got.S1.Control, got.S3)
		}
		if got.SignH1.Favourable != 6 || got.SignH1.Informative != 6 || got.SignH1.P.Exact != "1/64" {
			t.Fatalf("sign test %+v", got.SignH1)
		}
	})
}

func TestBuild(t *testing.T) {
	t.Run("the summary holds every run of the order with its record and the counted population [verifies SP-2825~1]", func(t *testing.T) {
		runs := design(names("t", 1), 2, 1, 0)
		records := map[string]record.Run{}
		var states []phase.RunState
		for _, run := range runs {
			records[run.Run] = run
			states = append(states, phase.RunState{Name: run.Run, Counted: 0, State: study.OutcomeCompleted, Attempts: []phase.Attempt{{Number: 1, Run: study.Run{Outcome: study.OutcomeCompleted}}}})
		}
		states = append(states, phase.RunState{Name: "ta-treatment-3", Counted: -1, State: study.OutcomeHarnessBlocked})
		got := Build(BuildInput{Phase: phase.Phase{Ended: true, Runs: states}, Records: records, Digests: map[string]string{"inputs.json": "abc"}})
		if len(got.Runs) != 5 || got.Runs[0].Refusal == nil || got.Runs[4].Facts != nil || got.Runs[4].Refusal != nil {
			t.Fatalf("runs %+v", got.Runs)
		}
		if got.Population.Counted[rt.Control] != 2 || got.Population.Missing[rt.Treatment][study.OutcomeHarnessBlocked] != 1 {
			t.Fatalf("population %+v", got.Population)
		}
		if got.Analysis.Primary.Control.Fraction != (facts.Fraction{K: 1, N: 2}) || got.Digests["inputs.json"] != "abc" {
			t.Fatalf("analysis %+v", got.Analysis.Primary)
		}
		if _, err := json.Marshal(got); err != nil {
			t.Fatal(err)
		}
	})
}
