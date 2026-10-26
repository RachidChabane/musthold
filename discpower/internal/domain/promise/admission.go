package promise

import (
	"slices"

	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

// Runs of a witness set on the reference.
const (
	// AdmissionRuns are the runs that admit a set.
	AdmissionRuns = witness.ReferenceRuns
	// FinalRuns are the runs of the final stage that confirm an admitted set.
	FinalRuns = format.RunsPerScanOutcome
	// BackingRuns are the runs a witness must pass to back a claim.
	BackingRuns = AdmissionRuns + FinalRuns
)

// QuestionReason says why a witness is a question about its promise.
type QuestionReason string

// Reasons of a question.
const (
	// QuestionReferenceFails: the reference fails the witness.
	QuestionReferenceFails QuestionReason = "reference_fails"
	// QuestionUnstable: the witness passed the admission and then failed a run
	// of the final stage.
	QuestionUnstable QuestionReason = "unstable_on_reference"
)

// Question is a witness that is a question about its promise, never a finding.
type Question struct {
	Promise string
	Witness string
	Reason  QuestionReason
}

// Decision is what runs on the reference say of a witness set: it holds, or it
// becomes a question, or the runs say nothing of it.
type Decision struct {
	Holds bool
	// Question is set when the set does not hold because the reference fails it.
	Question QuestionReason
}

// Admit decides a witness set from its admission runs: it is admitted when it
// passes the reference on every one of the AdmissionRuns. A set the reference
// fails on any run is a question; a set whose runs errored or are missing is
// neither admitted nor a question.
func Admit(runs []witness.Outcome) Decision {
	if passesAll(runs, AdmissionRuns) {
		return Decision{Holds: true}
	}
	if slices.ContainsFunc(runs, failed) {
		return Decision{Question: QuestionReferenceFails}
	}
	return Decision{}
}

// Confirm decides an admitted witness set from its admission runs and the runs
// of the final stage: it backs a claim only when all BackingRuns passed.
// Otherwise it is a question that it is unstable on the reference, and the
// routes it would have proved are withheld.
func Confirm(admission, final []witness.Outcome) Decision {
	if passesAll(admission, AdmissionRuns) && passesAll(final, FinalRuns) {
		return Decision{Holds: true}
	}
	return Decision{Question: QuestionUnstable}
}

func passesAll(runs []witness.Outcome, required int) bool {
	return len(runs) >= required && !slices.ContainsFunc(runs, func(run witness.Outcome) bool {
		return run != witness.OutcomePassed
	})
}

func failed(run witness.Outcome) bool {
	return run == witness.OutcomeFailed || run == witness.OutcomeFlaky
}
