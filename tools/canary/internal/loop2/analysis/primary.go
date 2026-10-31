// Package analysis computes the endpoints of the loop-2 study from its counted
// run records (ADR 0017, "Outcome", "Secondary analyses", "Steps and stops"):
// the outcome with its evidence, the calendar rule, the mutation-only recount,
// the catches, H2, the instructions against the gate, the re-adjudication,
// loop-1's kept endpoints, and the summary that holds them. It is pure.
package analysis

import (
	"encoding/json"
	"math"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/stats"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/mn"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// Real is a number that may be infinite, written in JSON as a number, or as
// "+Inf" or "-Inf".
type Real float64

// The JSON texts of the infinities.
const (
	positiveInfinity = "+Inf"
	negativeInfinity = "-Inf"
)

// MarshalJSON writes the number, or its infinity as a string.
func (r Real) MarshalJSON() ([]byte, error) {
	switch {
	case math.IsInf(float64(r), 1):
		return json.Marshal(positiveInfinity)
	case math.IsInf(float64(r), -1):
		return json.Marshal(negativeInfinity)
	}
	return json.Marshal(float64(r))
}

// UnmarshalJSON reads what MarshalJSON writes.
func (r *Real) UnmarshalJSON(data []byte) error {
	var text string
	if json.Unmarshal(data, &text) == nil {
		*r = Real(math.Inf(1))
		if text == negativeInfinity {
			*r = Real(math.Inf(-1))
		}
		return nil
	}
	var value float64
	err := json.Unmarshal(data, &value)
	*r = Real(value)
	return err
}

// Primary is the outcome of ADR 0017's table with its evidence.
type Primary struct {
	Outcome decision.Outcome `json:"outcome"`
	Row     int              `json:"row"`
	Reading string           `json:"reading"`
	Verdict decision.Verdict `json:"verdict"`
	// Reason names why the calendar rule made the outcome INCONCLUSIVE.
	Reason string `json:"reason,omitempty"`
	// Tasks are the strata; Excluded the tasks with fewer than two counted runs
	// in an arm.
	Tasks    []string            `json:"tasks"`
	Excluded []string            `json:"excluded"`
	PLess    analyze.Probability `json:"pLess"`
	PMore    analyze.Probability `json:"pMore"`
	// ScoreAtMargin is the stratified score at δ, and Interval [L, U].
	ScoreAtMargin Real        `json:"scoreAtMargin"`
	Interval      mn.Interval `json:"interval"`
	// IntervalAgrees says whether [L, U] reads δ as the score at δ does.
	IntervalAgrees bool            `json:"intervalAgrees"`
	Guards         decision.Guards `json:"guards"`
	// Control and Treatment are each arm's escapes over its counted runs, and
	// RiskReduction the control's share minus the treatment's.
	Control       analyze.Rate      `json:"control"`
	Treatment     analyze.Rate      `json:"treatment"`
	RiskReduction float64           `json:"riskReduction"`
	PerTask       []analyze.TaskRow `json:"perTask"`
}

// factsOf is the loop-1 facts of the runs.
func factsOf(runs []record.Run) []facts.Facts {
	found := make([]facts.Facts, len(runs))
	for index, run := range runs {
		found[index] = run.Facts
	}
	return found
}

func byArm(runs []record.Run) map[string][]record.Run {
	split := map[string][]record.Run{}
	for _, run := range runs {
		split[run.Arm] = append(split[run.Arm], run)
	}
	return split
}

func count(runs []record.Run, holds func(record.Run) bool) int {
	n := 0
	for _, run := range runs {
		if holds(run) {
			n++
		}
	}
	return n
}

func filter(runs []record.Run, keep func(record.Run) bool) []record.Run {
	kept := []record.Run{}
	for _, run := range runs {
		if keep(run) {
			kept = append(kept, run)
		}
	}
	return kept
}

func inTasks(runs []record.Run, tasks []string) []record.Run {
	return filter(runs, func(run record.Run) bool { return slices.Contains(tasks, run.Task) })
}

func orNone(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}

func frac(k, n int) facts.Fraction { return facts.Fraction{K: k, N: n} }

func escaped(run record.Run) bool { return run.Escaped }

// strata counts the event per arm of each task, in the order of the tasks.
func strata(runs []record.Run, tasks []string, event func(record.Run) bool) []analyze.TaskRow {
	rows := make([]analyze.TaskRow, 0, len(tasks))
	for _, task := range tasks {
		arms := byArm(inTasks(runs, []string{task}))
		control, treatment := arms[analyze.ArmControl], arms[analyze.ArmTreatment]
		rows = append(rows, analyze.TaskRow{
			Task: task, Control: frac(count(control, event), len(control)), Treatment: frac(count(treatment, event), len(treatment)),
		})
	}
	return rows
}

func permutationOf(rows []analyze.TaskRow) stats.Permutation {
	tested := make([]stats.Stratum, len(rows))
	for index, row := range rows {
		tested[index] = stats.Stratum{ControlRuns: row.Control.N, ControlEvents: row.Control.K, TreatmentRuns: row.Treatment.N, TreatmentEvents: row.Treatment.K}
	}
	return stats.StratifiedPermutation(tested)
}

func cellsOf(rows []analyze.TaskRow) []mn.Cell {
	cells := make([]mn.Cell, len(rows))
	for index, row := range rows {
		cells[index] = mn.Cell{ControlEvents: row.Control.K, ControlRuns: row.Control.N, TreatmentEvents: row.Treatment.K, TreatmentRuns: row.Treatment.N}
	}
	return cells
}

func totals(rows []analyze.TaskRow) (control, treatment facts.Fraction) {
	for _, row := range rows {
		control = frac(control.K+row.Control.K, control.N+row.Control.N)
		treatment = frac(treatment.K+row.Treatment.K, treatment.N+row.Treatment.N)
	}
	return control, treatment
}

// countsOf counts what the guards read on the runs.
func countsOf(runs []record.Run) decision.Counts {
	arms := byArm(runs)
	control, treatment := arms[analyze.ArmControl], arms[analyze.ArmTreatment]
	clean := func(run record.Run) bool { return !run.Defective }
	acceptedClean := func(run record.Run) bool { return run.Accepted && !run.Defective }
	return decision.Counts{
		TreatmentClean:        count(treatment, clean),
		TreatmentRefusedClean: count(treatment, func(run record.Run) bool { return !run.Defective && !run.Accepted }),
		ControlRuns:           len(control), ControlAcceptedClean: count(control, acceptedClean),
		TreatmentRuns: len(treatment), TreatmentAcceptedClean: count(treatment, acceptedClean),
	}
}

// DecidePrimary decides the outcome on counted runs: the strata are the tasks
// with at least two counted runs in each arm.
func DecidePrimary(runs []record.Run) Primary {
	included, excluded := analyze.Eligible(factsOf(runs))
	population := inTasks(runs, included)
	rows := strata(population, included, escaped)
	exact := permutationOf(rows)
	cells := cellsOf(rows)
	score := mn.Score(cells, decision.Margin)
	guards := decision.EvaluateGuards(countsOf(population))
	row := decision.Decide(decision.Evidence{PLess: exact.PLess, PMore: exact.PMore, ScoreAtMargin: score, GuardsHold: guards.Hold()})
	interval := mn.ScoreInterval(cells, decision.Z)
	control, treatment := totals(rows)
	return Primary{
		Outcome: row.Outcome, Row: row.Number, Reading: row.Reading, Verdict: row.Verdict,
		Tasks: orNone(included), Excluded: orNone(excluded),
		PLess: analyze.ProbabilityOf(exact.PLess), PMore: analyze.ProbabilityOf(exact.PMore),
		ScoreAtMargin: Real(score), Interval: interval,
		IntervalAgrees: (interval.Low >= decision.Margin) == (score >= decision.Z) && (interval.High < decision.Margin) == (score < -decision.Z),
		Guards:         guards,
		Control:        analyze.RateOf(control.K, control.N), Treatment: analyze.RateOf(treatment.K, treatment.N),
		RiskReduction: control.Float() - treatment.Float(),
		PerTask:       rows,
	}
}
