package analysis

import (
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stats"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// The H2 rule.
const (
	// MinFindingRuns is the treatment runs with a finding block H2 needs.
	MinFindingRuns = 10
	// halfRate is the share H2 tests against.
	halfRate = 0.5
	// DecisionNotTestable is H2 with fewer than MinFindingRuns such runs.
	DecisionNotTestable = "NOT TESTABLE"
)

// H2 is the share of treatment runs with a finding block that are corrected
// and then accepted, with loop-1's companion measures.
type H2 struct {
	Decision            string              `json:"decision"`
	Corrected           analyze.Rate        `json:"corrected"`
	P                   analyze.Probability `json:"p"`
	OnTarget            facts.Fraction      `json:"onTarget"`
	Strengthened        facts.Fraction      `json:"strengthened"`
	StrengthenedUnknown int                 `json:"strengthenedUnknown"`
	Left                facts.Fraction      `json:"left"`
}

// DecideH2 measures the correction by loop-1's rule and decides it by loop-2's.
func DecideH2(runs []record.Run) H2 {
	measured := analyze.DecideH2(factsOf(runs))
	corrected := measured.Corrected.Fraction
	p := stats.BinomialUpper(corrected.K, corrected.N)
	h2 := H2{
		Corrected: measured.Corrected, P: analyze.ProbabilityOf(p),
		OnTarget: measured.OnTarget, Strengthened: measured.Strengthened,
		StrengthenedUnknown: measured.StrengthenedUnknown, Left: measured.Left,
	}
	switch {
	case corrected.N < MinFindingRuns:
		h2.Decision = DecisionNotTestable
	case stats.Significant(p):
		h2.Decision = analyze.DecisionSupported
	case measured.Corrected.High < halfRate:
		h2.Decision = analyze.DecisionNotSupported
	default:
		h2.Decision = analyze.DecisionInconclusive
	}
	return h2
}

// ArmStops is one arm's runs with a first-stop grade and their shares.
type ArmStops struct {
	Graded   int `json:"graded"`
	Ungraded int `json:"ungraded"`
	// FirstStopDefect and Escape are over the graded runs.
	FirstStopDefect analyze.Rate `json:"firstStopDefect"`
	Escape          analyze.Rate `json:"escape"`
}

// Gate sets the instructions against the gate: the effect of the instructions
// and in-turn proofs at the first stop, and the effect of the gate from the
// first stop to the final change.
type Gate struct {
	PerArm map[string]ArmStops `json:"perArm"`
	// PLess and PMore are the stratified permutation test of the first-stop
	// defect over the graded runs of the strata.
	PLess analyze.Probability `json:"pLess"`
	PMore analyze.Probability `json:"pMore"`
	// Instructions is the control's first-stop defect share minus the
	// treatment's; GateEffect the treatment's first-stop defect share minus its
	// escape share, less the same difference in the control. They sum to the
	// difference of escape shares over the graded runs.
	Instructions float64 `json:"instructions"`
	GateEffect   float64 `json:"gateEffect"`
}

func graded(run record.Run) bool { return run.FirstStop != nil }

func firstStopDefective(run record.Run) bool { return run.FirstStop != nil && run.FirstStop.Defective }

// SetGate computes the instructions against the gate on counted runs.
func SetGate(runs []record.Run) Gate {
	arms := byArm(runs)
	gate := Gate{PerArm: map[string]ArmStops{}}
	shares := map[string][2]float64{}
	for _, arm := range analyze.Arms() {
		list := filter(arms[arm], graded)
		stops := ArmStops{
			Graded: len(list), Ungraded: len(arms[arm]) - len(list),
			FirstStopDefect: analyze.RateOf(count(list, firstStopDefective), len(list)),
			Escape:          analyze.RateOf(count(list, escaped), len(list)),
		}
		gate.PerArm[arm] = stops
		shares[arm] = [2]float64{stops.FirstStopDefect.Fraction.Float(), stops.Escape.Fraction.Float()}
	}
	control, treatment := shares[analyze.ArmControl], shares[analyze.ArmTreatment]
	gate.Instructions = control[0] - treatment[0]
	gate.GateEffect = (treatment[0] - treatment[1]) - (control[0] - control[1])
	included, _ := analyze.Eligible(factsOf(runs))
	exact := permutationOf(strata(filter(runs, graded), included, firstStopDefective))
	gate.PLess, gate.PMore = analyze.ProbabilityOf(exact.PLess), analyze.ProbabilityOf(exact.PMore)
	return gate
}

// Escape is one escape with its re-adjudication.
type Escape struct {
	Run       string           `json:"run"`
	Task      string           `json:"task"`
	Arm       string           `json:"arm"`
	Criterion string           `json:"criterion"`
	Sentence  string           `json:"sentence"`
	Verdicts  []record.Verdict `json:"verdicts"`
	Disputed  bool             `json:"disputed"`
}

// Readjudication is every escape with its adjudication, the disputed share,
// and the outcome decided again without the disputed escapes.
type Readjudication struct {
	Escapes     []Escape       `json:"escapes"`
	Disputed    facts.Fraction `json:"disputed"`
	Sensitivity Primary        `json:"sensitivity"`
}

func disputed(run record.Run) bool {
	return run.Escaped && run.Adjudication != nil && run.Adjudication.Disputed()
}

// Readjudicate lists the escapes and decides the outcome again without the runs
// of the disputed ones.
func Readjudicate(runs []record.Run) Readjudication {
	found := Readjudication{Escapes: []Escape{}}
	for _, run := range filter(runs, escaped) {
		entry := Escape{Run: run.Run, Task: run.Task, Arm: run.Arm, Verdicts: []record.Verdict{}, Disputed: disputed(run)}
		if run.Adjudication != nil {
			entry.Criterion, entry.Sentence, entry.Verdicts = run.Adjudication.Criterion, run.Adjudication.Sentence, run.Adjudication.Judges
		}
		found.Escapes = append(found.Escapes, entry)
	}
	found.Disputed = frac(count(runs, disputed), len(found.Escapes))
	found.Sensitivity = DecidePrimary(filter(runs, func(run record.Run) bool { return !disputed(run) }))
	return found
}
