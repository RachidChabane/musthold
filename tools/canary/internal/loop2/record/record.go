// Package record is a counted run of the loop-2 study as the analysis reads it:
// loop-1's facts of the run, the oracle's grade of the tree at the first
// end-of-turn call, the refusal cause of the final change and, for an escape,
// its re-adjudication (ADR 0017, "Secondary analyses"). Compose is pure; Load
// reads a records directory.
package record

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The files loop-2 adds to a counted run's records.
const (
	// FirstStopOracleFile is the oracle's grade of the tree at the first
	// end-of-turn call, in the format of oracle.json; absent when the session
	// made no call.
	FirstStopOracleFile = "first-stop-oracle.json"
	// AdjudicationFile is the re-adjudication of an escape.
	AdjudicationFile = "adjudication.json"
)

// The judges of the re-adjudication.
const (
	JudgeTerra = "gpt-5.6-terra"
	JudgeOpus  = "claude-opus-5-5"
)

// Judges lists the judges of the re-adjudication.
func Judges() []string { return []string{JudgeTerra, JudgeOpus} }

// Errors of a record.
var (
	ErrUnadjudicated = errors.New("an escape has no adjudication")
	ErrAdjudication  = errors.New("an adjudication does not hold one verdict of each judge")
)

// Grade is the oracle's grade of a tree: whether it is defective and the
// criteria it violates.
type Grade struct {
	Defective bool     `json:"defective"`
	Violated  []string `json:"violated"`
}

// OracleGrade reads the grade of an oracle.json.
func OracleGrade(oracle study.Oracle) Grade {
	violated := oracle.Violated
	if violated == nil {
		violated = []string{}
	}
	return Grade{Defective: oracle.Defective, Violated: violated}
}

// Cause is why the gate refused a run's final change.
type Cause string

// The refusal causes.
const (
	// CauseNone: the change is accepted.
	CauseNone Cause = "none"
	// CauseCap: the session ended at a cap.
	CauseCap Cause = "cap"
	// CauseFinding: a blocking finding comes from a proof's verdict.
	CauseFinding Cause = "finding"
	// CausePending: the only blocking proof findings are stale_proof or pending
	// reasons.
	CausePending Cause = "pending"
	// CauseTraceability: any other refusal.
	CauseTraceability Cause = "traceability"
)

// Refusal is the refusal cause of a final change with the gate's blocking
// findings.
type Refusal struct {
	Cause    Cause           `json:"cause"`
	Findings []facts.Finding `json:"findings"`
}

// kindCauses maps the kind loop-1 gives a set of blocking findings to its cause.
var kindCauses = map[facts.BlockKind]Cause{
	facts.KindFinding:      CauseFinding,
	facts.KindPending:      CausePending,
	facts.KindTraceability: CauseTraceability,
}

// RefusalOf derives the refusal cause of a final change from its acceptance,
// the attempt's outcome and gate-after.json.
func RefusalOf(accepted bool, outcome study.Outcome, gate *facts.Report) Refusal {
	if accepted {
		return Refusal{Cause: CauseNone, Findings: []facts.Finding{}}
	}
	kind, findings := facts.Classify(gate)
	if findings == nil {
		findings = []facts.Finding{}
	}
	if outcome.Capped() {
		return Refusal{Cause: CauseCap, Findings: findings}
	}
	return Refusal{Cause: kindCauses[kind], Findings: findings}
}

// Verdict is one judge's verdict on an escape.
type Verdict struct {
	Judge     string `json:"judge"`
	Defective bool   `json:"defective"`
	Reason    string `json:"reason"`
}

// Adjudication is adjudication.json: the spec sentence an escape breaks, its
// criterion and each judge's verdict, blind to the arm.
type Adjudication struct {
	Criterion string    `json:"criterion"`
	Sentence  string    `json:"sentence"`
	Judges    []Verdict `json:"judges"`
}

// Validate refuses an adjudication without exactly one verdict of each judge.
func (a Adjudication) Validate() error {
	names := make([]string, 0, len(a.Judges))
	for _, verdict := range a.Judges {
		names = append(names, verdict.Judge)
	}
	slices.Sort(names)
	want := Judges()
	slices.Sort(want)
	if !slices.Equal(names, want) {
		return fmt.Errorf("%w: %v", ErrAdjudication, names)
	}
	return nil
}

// Disputed reports whether a judge does not hold the escape defective.
func (a Adjudication) Disputed() bool {
	return slices.ContainsFunc(a.Judges, func(verdict Verdict) bool { return !verdict.Defective })
}

// Run is one counted run of loop-2.
type Run struct {
	facts.Facts
	// FirstStop is the grade of the tree at the first end-of-turn call; null
	// when the session made no call.
	FirstStop *Grade  `json:"firstStop"`
	Refusal   Refusal `json:"refusal"`
	// Adjudication is the re-adjudication of an escape; null for any other run.
	Adjudication *Adjudication `json:"adjudication"`
}

// Compose builds the record of a counted run named name from its decoded
// records, the oracle's grade of its first stop when there is one, and its
// adjudication. It refuses an escape without a valid adjudication.
func Compose(name string, in facts.Input, firstStop *study.Oracle, adjudication *Adjudication) (Run, error) {
	run := Run{Facts: facts.Compute(name, in)}
	run.Refusal = RefusalOf(run.Accepted, in.Run.Outcome, in.FinalGate)
	if firstStop != nil {
		grade := OracleGrade(*firstStop)
		run.FirstStop = &grade
	}
	if !run.Escaped {
		return run, nil
	}
	if adjudication == nil {
		return run, fmt.Errorf("%w: %s", ErrUnadjudicated, name)
	}
	if err := adjudication.Validate(); err != nil {
		return run, fmt.Errorf("%s: %w", name, err)
	}
	run.Adjudication = adjudication
	return run, nil
}
