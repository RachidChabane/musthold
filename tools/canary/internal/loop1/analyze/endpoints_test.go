package analyze

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// run is a counted run of the task and arm, accepted and clean unless changed.
func run(task, arm string, repetition int, change ...func(*facts.Facts)) facts.Facts {
	f := facts.Facts{
		Run: fmt.Sprintf("%s-%s-%d", task, arm, repetition), Task: task, Arm: arm, Repetition: repetition,
		Outcome: study.OutcomeCompleted, Accepted: true, Hits: map[study.Detector]int{}, BlockCounts: map[facts.BlockKind]int{},
		Strength: facts.Strength{Final: &facts.Fraction{K: 2, N: 4}},
	}
	for _, apply := range change {
		apply(&f)
	}
	f.Escaped = f.Accepted && f.Defective
	return f
}

func escaped(f *facts.Facts)   { f.Defective, f.Accepted = true, true }
func defective(f *facts.Facts) { f.Defective, f.Accepted = true, false }
func rejected(f *facts.Facts)  { f.Accepted = false }

// arm adds n runs of the task and arm, the first k of them changed.
func arm(task, name string, n, k int, change func(*facts.Facts)) []facts.Facts {
	var runs []facts.Facts
	for index := range n {
		if index < k {
			runs = append(runs, run(task, name, index+1, change))
		} else {
			runs = append(runs, run(task, name, index+1))
		}
	}
	return runs
}

// tasks builds tasks of three runs per arm with the escapes given per task.
func tasks(escapes [][2]int) []facts.Facts {
	var runs []facts.Facts
	for index, pair := range escapes {
		task := fmt.Sprintf("task%d", index+1)
		runs = append(runs, arm(task, ArmControl, 3, pair[0], escaped)...)
		runs = append(runs, arm(task, ArmTreatment, 3, pair[1], escaped)...)
	}
	return runs
}

func TestH1(t *testing.T) {
	t.Run("one task's three control escapes decide SUPPORTED at p = 1/20 [verifies SP-2596~1]", func(t *testing.T) {
		// Task 1: 3 control escapes, 0 treatment; tasks 2 to 5: none. The
		// hypergeometric of task 1 gives P(0) = 1/20 and the others are point masses,
		// so p_less = 1/20 <= 0.05. The treatment arm has 15 clean runs, all
		// accepted: false rejection 0/15, so G holds.
		got := DecideH1(tasks([][2]int{{3, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}))
		if got.Decision != DecisionSupported || got.PLess.Exact != "1/20" || got.PMore.Exact != "1" || got.PMin.Exact != "1/20" {
			t.Fatalf("got %+v", got)
		}
		if !got.Guard.Holds || got.Guard.FalseRejection != (Fraction{K: 0, N: 15}) || len(got.Tasks) != 5 {
			t.Fatalf("guard %+v", got.Guard)
		}
	})
	t.Run("the guard decides between SUPPORTED and REJECTION-ONLY at 1/3 [verifies SP-2596~1]", func(t *testing.T) {
		build := func(rejectedRuns int) []facts.Facts {
			runs := tasks([][2]int{{3, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}})
			for index := range runs {
				if runs[index].Arm == ArmTreatment && rejectedRuns > 0 {
					runs[index].Accepted, rejectedRuns = false, rejectedRuns-1
				}
			}
			return runs
		}
		// 5 of 15 clean treatment runs rejected: 5·3 = 15 <= 1·15, G holds.
		if got := DecideH1(build(5)); got.Decision != DecisionSupported || got.Guard.FalseRejection != (Fraction{K: 5, N: 15}) {
			t.Fatalf("got %+v", got)
		}
		// 6 of 15: 18 > 15, G fails.
		if got := DecideH1(build(6)); got.Decision != DecisionRejectionOnly || got.Guard.Holds {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a treatment arm with no clean run fails the guard [verifies SP-2596~1]", func(t *testing.T) {
		runs := tasks([][2]int{{3, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}})
		for index := range runs {
			if runs[index].Arm == ArmTreatment {
				runs[index].Defective, runs[index].Accepted = true, false
			}
		}
		if got := DecideH1(runs); got.Decision != DecisionRejectionOnly || got.Guard.Holds || got.Guard.FalseRejection.N != 0 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("treatment escapes decide CONTRADICTED [verifies SP-2596~1]", func(t *testing.T) {
		got := DecideH1(tasks([][2]int{{0, 3}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}))
		if got.Decision != DecisionContradicted || got.PMore.Exact != "1/20" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("fewer than five tasks with two runs per arm is INCONCLUSIVE blocked [verifies SP-2596~1]", func(t *testing.T) {
		runs := tasks([][2]int{{3, 0}, {3, 0}, {3, 0}, {3, 0}})
		runs = append(runs, arm("short", ArmControl, 3, 3, escaped)...)
		runs = append(runs, arm("short", ArmTreatment, 1, 0, nil)...)
		got := DecideH1(runs)
		if got.Decision != DecisionInconclusive || got.Reason != ReasonBlocked || slices.Contains(got.Tasks, "short") || got.Excluded[0] != "short" {
			t.Fatalf("got %+v", got)
		}
		if got.Control.Fraction != (Fraction{K: 12, N: 12}) {
			t.Fatalf("the effect covers the included tasks: %+v", got.Control)
		}
	})
	t.Run("no escape gives p_min = 1 and the floor reason [verifies SP-2596~1]", func(t *testing.T) {
		got := DecideH1(tasks([][2]int{{0, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}}))
		if got.Decision != DecisionInconclusive || got.Reason != ReasonFloor || got.PMin.Exact != "1" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("a testable but unclear result names neither reason [verifies SP-2596~1]", func(t *testing.T) {
		// Task 1: 3 control escapes, 0 treatment; task 2: 1 control, 2 treatment;
		// both have 3 escapes among 6 runs, P(0) = P(3) = 1/20, P(1) = P(2) = 9/20.
		// Observed T = 2. p_min = P(0)P(0) = 1/400.
		// p_less = P(T <= 2) = [1·1 + (9·1 + 1·9) + (9·9 + 9·1 + 1·9)]/400 = 118/400 = 59/200.
		// p_more = 1 - P(T <= 1) = 1 - 19/400 = 381/400.
		got := DecideH1(tasks([][2]int{{3, 0}, {1, 2}, {0, 0}, {0, 0}, {0, 0}}))
		if got.Decision != DecisionInconclusive || got.Reason != ReasonNeither || got.PMin.Exact != "1/400" ||
			got.PLess.Exact != "59/200" || got.PMore.Exact != "381/400" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("the effect table gives each arm's escapes, intervals and risk difference [verifies SP-2598~1]", func(t *testing.T) {
		got := DecideH1(tasks([][2]int{{3, 0}, {1, 2}, {0, 0}, {0, 0}, {0, 0}}))
		// Control 4/15, treatment 2/15: risk difference 2/15 - 4/15 = -2/15.
		if got.Control.Fraction != (Fraction{K: 4, N: 15}) || got.Treatment.Fraction != (Fraction{K: 2, N: 15}) ||
			math.Abs(got.RiskDifference+2.0/15) > 1e-12 || got.Control.Low <= 0 || got.Control.High >= 1 {
			t.Fatalf("got %+v", got.Permutation)
		}
		if len(got.PerTask) != 5 || got.PerTask[1] != (TaskRow{Task: "task2", Control: Fraction{K: 1, N: 3}, Treatment: Fraction{K: 2, N: 3}}) {
			t.Fatalf("per task %+v", got.PerTask)
		}
	})
	t.Run("the result does not depend on the order of the runs [verifies SP-2596~1]", rapid.MakeCheck(func(t *rapid.T) {
		var runs []facts.Facts
		for index := range rapid.IntRange(1, 7).Draw(t, "tasks") {
			task := fmt.Sprintf("t%d", index)
			for _, name := range Arms() {
				n := rapid.IntRange(1, 4).Draw(t, "runs")
				runs = append(runs, arm(task, name, n, rapid.IntRange(0, n).Draw(t, "escapes"), escaped)...)
			}
		}
		want := DecideH1(runs)
		shuffled := rapid.Permutation(runs).Draw(t, "order")
		got := DecideH1(shuffled)
		if got.Decision != want.Decision || got.PLess != want.PLess || got.PMore != want.PMore || got.PMin != want.PMin || got.Reason != want.Reason {
			t.Fatalf("got %+v, want %+v", got, want)
		}
		if got.PLess.Value+got.PMore.Value < 1-1e-12 {
			t.Fatalf("p_less + p_more = %v", got.PLess.Value+got.PMore.Value)
		}
	}))
}

// withBlocks gives a treatment or control run finding blocks, the last as given.
func withBlocks(kind facts.BlockKind, last facts.Block) func(*facts.Facts) {
	return func(f *facts.Facts) {
		last.Kind = kind
		f.Blocks = append(f.Blocks, facts.Block{Kind: kind, Call: 1}, last)
		f.BlockCounts[kind] += 2
	}
}

func h2Runs(corrected, total int) []facts.Facts {
	yes := true
	var runs []facts.Facts
	for index := range total {
		block := facts.Block{Call: 2, Left: true}
		if index < corrected {
			block = facts.Block{Call: 2, Corrected: true, OnTarget: index%2 == 0, Strengthened: &yes, PassingCall: 3}
		}
		runs = append(runs, run(fmt.Sprintf("t%d", index%3), ArmTreatment, index+1, withBlocks(facts.KindFinding, block)))
	}
	runs = append(runs, run("t0", ArmTreatment, 99))
	return runs
}

func TestH2(t *testing.T) {
	t.Run("the exact binomial test and the interval decide H2 [verifies SP-2599~1]", func(t *testing.T) {
		// 6 of 6: P(X >= 6) = 1/64 <= 0.05, SUPPORTED.
		if got := DecideH2(h2Runs(6, 6)); got.Decision != DecisionSupported || got.P.Exact != "1/64" || got.Corrected.Fraction != (Fraction{K: 6, N: 6}) {
			t.Fatalf("got %+v", got)
		}
		// 0 of 6: the upper bound 1 - 0.025^(1/6) = 0.459 is below 0.5, NOT SUPPORTED.
		if got := DecideH2(h2Runs(0, 6)); got.Decision != DecisionNotSupported || got.Corrected.High >= 0.5 {
			t.Fatalf("got %+v", got)
		}
		// 4 of 6: P(X >= 4) = 22/64, and the upper bound is above 0.5.
		if got := DecideH2(h2Runs(4, 6)); got.Decision != DecisionInconclusive || got.P.Exact != "11/32" || got.Reason != "" {
			t.Fatalf("got %+v", got)
		}
		// 5 runs with a finding block, all corrected: too few runs.
		if got := DecideH2(h2Runs(5, 5)); got.Decision != DecisionInconclusive || got.Reason != ReasonFewRuns {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("the companions of the correction rate [verifies SP-2599~1]", func(t *testing.T) {
		runs := h2Runs(4, 7)
		runs = append(runs,
			run("t1", ArmControl, 1, withBlocks(facts.KindTraceability, facts.Block{Corrected: true})),
			run("t1", ArmControl, 2, withBlocks(facts.KindTraceability, facts.Block{Left: true})),
			run("t1", ArmTreatment, 50, withBlocks(facts.KindTraceability, facts.Block{Corrected: true})),
		)
		runs[0].Blocks[1].Strengthened = nil
		got := DecideH2(runs)
		// The corrected runs are 1 to 4; runs 1 and 3 are on target; run 1's shares
		// are missing. Runs 5 to 7 are left.
		if got.OnTarget != (Fraction{K: 2, N: 4}) || got.Strengthened != (Fraction{K: 3, N: 4}) || got.StrengthenedUnknown != 1 ||
			got.Left != (Fraction{K: 3, N: 7}) {
			t.Fatalf("got %+v", got)
		}
		if got.Traceability[ArmControl] != (Fraction{K: 1, N: 2}) || got.Traceability[ArmTreatment] != (Fraction{K: 1, N: 1}) {
			t.Fatalf("traceability %+v", got.Traceability)
		}
	})
}

func killedRun(task, name string, repetition, killed int) facts.Facts {
	return run(task, name, repetition, func(f *facts.Facts) { f.Strength.Final = &facts.Fraction{K: killed, N: 4} })
}

func TestS2(t *testing.T) {
	t.Run("the killed violations are tested by enumeration and by the sign test [verifies SP-2600~1]", func(t *testing.T) {
		// One task, control kills 1 and 2, treatment 3 and 3, of 4 violations each.
		// The six assignments of two treatment labels to {1, 2, 3, 3} give the sums
		// 3, 4, 4, 5, 5, 6: the observed 6 has probability 1/6.
		// Mean shares: treatment 3/4, control 3/8, a positive difference; one
		// informative task, so the sign test gives 1/2.
		runs := []facts.Facts{killedRun("a", ArmControl, 1, 1), killedRun("a", ArmControl, 2, 2), killedRun("a", ArmTreatment, 1, 3), killedRun("a", ArmTreatment, 2, 3)}
		got := DecideS2(runs)
		if got.Observed != 6 || got.P.Exact != "1/6" || got.Sign.Favourable != 1 || got.Sign.Informative != 1 || got.Sign.P.Exact != "1/2" {
			t.Fatalf("got %+v", got)
		}
		runs[0].Strength.Final = nil
		if missing := DecideS2(append(runs, killedRun("a", ArmControl, 3, 0))); missing.MissingShare != 1 {
			t.Fatalf("got %+v", missing)
		}
	})
}

func TestSecondary(t *testing.T) {
	t.Run("S1, yield, false rejection, blocks, hits, cost, voluntary proofs and criteria [verifies SP-2601~1]", func(t *testing.T) {
		usd := 2.0
		runs := []facts.Facts{
			run("a", ArmControl, 1, defective, func(f *facts.Facts) {
				f.Violated = []string{"A-2"}
				f.Voluntary = []facts.Voluntary{{Call: "Bash: musthold-dp prove --changed", Findings: []string{"awaiting_host"}}}
			}),
			run("a", ArmControl, 2, escaped, func(f *facts.Facts) { f.Violated = []string{"A-2", "A-3"} }),
			run("a", ArmTreatment, 1, rejected, func(f *facts.Facts) {
				f.Hits[study.DetectorSkipped] = 1
				f.BlockCounts[facts.KindFinding] = 2
				f.FindingCodes = map[string]int{"surviving_valid_mutant": 2}
				f.GiveUps = 1
				f.Cost.NotionalUSD, f.Cost.Messages = &usd, 10
			}),
			run("a", ArmTreatment, 2, func(f *facts.Facts) { f.Cost.Messages = 30 }),
		}
		found := secondary(runs)
		if found.S3[ArmControl].Yield.Fraction != (Fraction{K: 1, N: 2}) || found.S3[ArmTreatment].FalseRejection.Fraction != (Fraction{K: 1, N: 2}) ||
			found.S3[ArmControl].FalseRejection.Fraction != (Fraction{K: 0, N: 0}) {
			t.Fatalf("s3 %+v", found.S3)
		}
		if s4 := found.S4[ArmTreatment]; s4.Blocks[facts.KindFinding] != 2 || s4.RunsWith[facts.KindFinding] != (Fraction{K: 1, N: 2}) || s4.GiveUps != 1 || s4.FindingCodes["surviving_valid_mutant"] != 2 {
			t.Fatalf("s4 %+v", s4)
		}
		if found.S5[ArmTreatment][study.DetectorSkipped] != (Fraction{K: 1, N: 2}) || found.S5[ArmControl][study.DetectorSkipped] != (Fraction{K: 0, N: 2}) {
			t.Fatalf("s5 %+v", found.S5)
		}
		messages := found.S6.PerArm[ArmTreatment][0]
		if messages.Measure != MeasureMessages || messages.Median != 20 || messages.Max != 30 || *messages.PerAccepted != 40 {
			t.Fatalf("s6 %+v", messages)
		}
		if found.S6.PerTask[0].Differences[MeasureNotionalUSD] != 1 || found.S6.PerTask[0].Differences[MeasureMessages] != 20 {
			t.Fatalf("s6 per task %+v", found.S6.PerTask)
		}
		if found.S7.Runs != (Fraction{K: 1, N: 2}) || found.S7.List[0].Findings[0] != "awaiting_host" {
			t.Fatalf("s7 %+v", found.S7)
		}
		if len(found.S8) != 2 || found.S8[0] != (CriterionRow{Task: "a", Criterion: "A-2", Control: Fraction{K: 2, N: 2}, Treatment: Fraction{K: 0, N: 2}}) {
			t.Fatalf("s8 %+v", found.S8)
		}
		s1 := core(runs).S1
		if s1.Control.Fraction != (Fraction{K: 2, N: 2}) || s1.Treatment.Fraction != (Fraction{K: 0, N: 2}) {
			t.Fatalf("s1 %+v", s1)
		}
	})
}

func TestSensitivity(t *testing.T) {
	t.Run("the analyses are repeated on the three subsets [verifies SP-2602~1]", func(t *testing.T) {
		runs := tasks([][2]int{{3, 0}, {0, 0}, {0, 0}, {0, 0}, {0, 0}})
		for index := range runs {
			if runs[index].Task == "task1" {
				runs[index].Task = InSampleTask
			}
		}
		runs[0].Hits[study.DetectorNetwork] = 1
		runs[1].Outcome, runs[1].Capped = study.OutcomeCappedTime, true
		got := sensitivity(runs)
		if got.WithoutInSample.Runs != 24 || got.WithoutHits.Runs != 29 || got.WithoutCapped.Runs != 29 {
			t.Fatalf("got %d %d %d", got.WithoutInSample.Runs, got.WithoutHits.Runs, got.WithoutCapped.Runs)
		}
		if got.WithoutInSample.H1.Decision != DecisionInconclusive || got.WithoutInSample.H1.Reason != ReasonBlocked {
			t.Fatalf("without the in-sample task %+v", got.WithoutInSample.H1)
		}
		// One task with a positive difference of escape shares, 3/3 - 0/3: p = 1/2.
		if got.SignH1.Favourable != 1 || got.SignH1.Informative != 1 || got.SignH1.P.Exact != "1/2" {
			t.Fatalf("sign %+v", got.SignH1)
		}
	})
	t.Run("the sign test reads shares, which differ from counts when the arms differ in size [verifies SP-2602~1]", func(t *testing.T) {
		// Control escapes 1 of 2 runs, treatment 1 of 3: the counts tie, but the
		// shares differ by 1/2 - 1/3 = 1/6 in favour of the treatment.
		runs := append(arm("a", ArmControl, 2, 1, escaped), arm("a", ArmTreatment, 3, 1, escaped)...)
		got := sensitivity(runs).SignH1
		if got.Favourable != 1 || got.Informative != 1 {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("the per-task majority of corrected runs [verifies SP-2602~1]", func(t *testing.T) {
		// h2Runs(4, 7) spreads runs over t0, t1 and t2: t0 holds runs 1, 4, 7
		// (corrected, corrected, left), t1 runs 2, 5 (corrected, left), t2 runs 3, 6
		// (corrected, left). Majorities: t0 positive, t1 and t2 tied.
		got := sign(majorityDifferences(h2Runs(4, 7)))
		if got.Favourable != 1 || got.Informative != 1 {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestBuild(t *testing.T) {
	t.Run("every run of the order is listed with its attempts and state [verifies SP-2603~1]", func(t *testing.T) {
		counted := run("box-office", ArmControl, 1)
		attempt := func(n int, outcome study.Outcome) phase.Attempt {
			a := phase.Attempt{Number: n, Run: study.Run{Attempt: n, Outcome: outcome}}
			a.Run.Task, a.Run.Arm, a.Run.Repetition = "box-office", ArmControl, 1
			return a
		}
		judged := phase.Phase{Ended: true, CapReached: true, Runs: []phase.RunState{
			{Name: "box-office-control-1", Attempts: []phase.Attempt{attempt(1, study.OutcomeQuotaStop), attempt(2, study.OutcomeCompleted)}, Counted: 1, State: study.OutcomeCompleted},
			{Name: "box-office-treatment-1", Attempts: []phase.Attempt{attempt(1, study.OutcomeHarnessError)}, Counted: -1, State: study.OutcomeHarnessBlocked},
			{Name: "box-office-treatment-2", Counted: -1, State: study.OutcomeNotRun},
		}}
		deviations := []study.Deviation{
			{Kind: study.KindProtocol, Scope: []string{"box-office"}}, {Kind: study.KindProtocol, Scope: []string{study.ScopeNone}},
			{Kind: study.KindQuotaStop, Scope: []string{"box-office-control-1"}},
		}
		got := Build(BuildInput{Phase: judged, Facts: map[string]facts.Facts{"box-office-control-1": counted}, Deviations: deviations})
		if len(got.Runs) != 3 || got.Runs[0].Facts == nil || len(got.Runs[0].Attempts) != 2 || got.Runs[2].Task != "box-office" ||
			got.Runs[2].Arm != ArmTreatment || got.Runs[2].Repetition != 2 || got.Runs[1].Facts != nil {
			t.Fatalf("runs %+v", got.Runs)
		}
		population := got.Population
		if population.Counted[ArmControl] != 1 || population.Void[ArmControl][study.OutcomeQuotaStop] != 1 ||
			population.Void[ArmTreatment][study.OutcomeHarnessError] != 1 || population.Missing[ArmTreatment][study.OutcomeHarnessBlocked] != 1 ||
			population.Missing[ArmTreatment][study.OutcomeNotRun] != 1 {
			t.Fatalf("population %+v", population)
		}
		if got.Deviations.Counts[study.KindProtocol] != 2 || got.Analysis.ProtocolDeviations != 1 {
			t.Fatalf("deviations %+v %d", got.Deviations, got.Analysis.ProtocolDeviations)
		}
	})
}
