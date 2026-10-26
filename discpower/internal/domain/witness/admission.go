package witness

import (
	"slices"

	"github.com/RachidChabane/musthold/format"
)

// Outcome is what one run of a witness did.
type Outcome string

// Witness run outcomes. On a mutant, a failure counts only when it reproduces:
// a failure the rerun does not repeat is flaky, and an error is neither.
const (
	OutcomePassed  Outcome = "passed"
	OutcomeFailed  Outcome = "failed"
	OutcomeFlaky   Outcome = "flaky"
	OutcomeErrored Outcome = "errored"
)

// ReferenceRuns is the number of times a witness runs on the reference.
const ReferenceRuns = 3

// Reading is what the runs on the reference say about a witness.
type Reading string

// Readings of a witness on the reference.
const (
	// ReadingPasses: it passed every run.
	ReadingPasses Reading = "passes"
	// ReadingConflict: it failed every run. The writer read the criterion one
	// way and the reference does another: a question about the spec, never a
	// block.
	ReadingConflict Reading = "reading_conflict"
	// ReadingFlaky: it passed some runs and failed others.
	ReadingFlaky Reading = "flaky"
	// ReadingUnrunnable: some run errored or is missing.
	ReadingUnrunnable Reading = "unrunnable"
)

// ReadingOf reads the runs on the reference.
func ReadingOf(runs []Outcome) Reading {
	if len(runs) < ReferenceRuns {
		return ReadingUnrunnable
	}
	passed, failed := 0, 0
	for _, run := range runs {
		switch run {
		case OutcomePassed:
			passed++
		case OutcomeFailed:
			failed++
		default:
			return ReadingUnrunnable
		}
	}
	switch {
	case passed == len(runs):
		return ReadingPasses
	case failed == len(runs):
		return ReadingConflict
	}
	return ReadingFlaky
}

// ScopeAnswer is a scope judge's answer about one witness: does the criterion's
// text alone, under the precedence, fix the output the witness checks?
type ScopeAnswer string

// Scope answers.
const (
	ScopeFixed        ScopeAnswer = "FIXED"
	ScopeNotFixed     ScopeAnswer = "NOT_FIXED"
	ScopeUndetermined ScopeAnswer = "UNDETERMINED"
)

// ScopeVote is one scope judge's answer about a witness, with the provenance of
// its response.
type ScopeVote struct {
	Answer     ScopeAnswer      `json:"answer"`
	Confidence float64          `json:"confidence"`
	Isolation  format.Isolation `json:"isolation"`
	Marked     bool             `json:"marked,omitempty"`
}

// AtFloor reports whether the vote keeps judged pass credit under the floor.
func (v ScopeVote) AtFloor(floor format.Isolation) bool {
	return !v.Marked && v.Isolation.AtLeast(floor)
}

// Admission tells whether the planted refactors take part in admitting a
// witness.
type Admission string

// Admissions.
const (
	// AdmissionWithRefactors admits a witness that passes the reference and every
	// planted refactor of the admission input.
	AdmissionWithRefactors Admission = "with_refactors"
	// AdmissionReferenceOnly admits every witness that passes the reference, as
	// the module does on a project that names no planted refactor.
	AdmissionReferenceOnly Admission = "reference_only"
)

// ScopeRule is what a witness needs from the scope judges to be block-bearing:
// the number of votes and the confidence each FIXED vote must reach.
type ScopeRule struct {
	Votes     int
	Threshold float64
}

// Status is where a witness stands.
type Status string

// Statuses of a witness, from the least to the most power.
const (
	StatusUnrunnable      Status = "unrunnable"
	StatusFlaky           Status = "flaky"
	StatusReadingConflict Status = "reading_conflict"
	// StatusRefactorDropped: it passes the reference and fails a planted
	// refactor, so it pins behaviour no criterion states.
	StatusRefactorDropped Status = "refactor_dropped"
	// StatusEscalateOnly: admitted, but not held fixed by every scope judge.
	StatusEscalateOnly Status = "escalate_only"
	// StatusBlockBearing: admitted and held fixed by every scope judge.
	StatusBlockBearing Status = "block_bearing"
)

// Record is everything known of one witness: who wrote it, what it did on the
// reference and on the planted refactors, and what the scope judges answered.
type Record struct {
	Witness   Witness            `json:"witness"`
	Writer    Writer             `json:"writer"`
	Reference []Outcome          `json:"reference"`
	Refactors map[string]Outcome `json:"refactors,omitempty"`
	Scope     []ScopeVote        `json:"scope,omitempty"`
}

// Reading is what the reference runs say of the witness.
func (r Record) Reading() Reading { return ReadingOf(r.Reference) }

// PassesRefactors reports whether the witness passed every planted refactor.
func (r Record) PassesRefactors() bool {
	for _, outcome := range r.Refactors {
		if outcome != OutcomePassed {
			return false
		}
	}
	return true
}

// Admitted reports whether the witness is admitted under the admission.
func (r Record) Admitted(admission Admission) bool {
	return r.Reading() == ReadingPasses && (admission == AdmissionReferenceOnly || r.PassesRefactors())
}

// ScopeApproved reports whether the scope judges hold the witness's output fixed
// by its criterion's text: as many votes as the rule asks, every one FIXED at or
// above its threshold. Every vote keeps this power, whatever its isolation: the
// scope judges only ever gate a block, never create a pass.
func (r Record) ScopeApproved(rule ScopeRule) bool {
	if rule.Votes < 1 || len(r.Scope) < rule.Votes {
		return false
	}
	return !slices.ContainsFunc(r.Scope, func(vote ScopeVote) bool {
		return vote.Answer != ScopeFixed || vote.Confidence < rule.Threshold
	})
}

// Status says where the witness stands under the admission and the scope rule.
func (r Record) Status(admission Admission, rule ScopeRule) Status {
	switch r.Reading() {
	case ReadingUnrunnable:
		return StatusUnrunnable
	case ReadingFlaky:
		return StatusFlaky
	case ReadingConflict:
		return StatusReadingConflict
	}
	switch {
	case admission == AdmissionWithRefactors && !r.PassesRefactors():
		return StatusRefactorDropped
	case r.ScopeApproved(rule):
		return StatusBlockBearing
	}
	return StatusEscalateOnly
}

// CreditEligible reports whether a failure of the witness can earn pass credit:
// its writer and every scope judge answered at or above the isolation floor and
// unmarked. A witness that is not credit-eligible keeps its power to block.
func (r Record) CreditEligible(floor format.Isolation) bool {
	if !r.Writer.AtFloor(floor) {
		return false
	}
	return !slices.ContainsFunc(r.Scope, func(vote ScopeVote) bool { return !vote.AtFloor(floor) })
}
