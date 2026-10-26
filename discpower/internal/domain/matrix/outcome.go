// Package matrix classifies the executed evidence of a mutant run into the
// outcomes of the proof format.
package matrix

import (
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

// Evidence is everything one mutant run produced for the tests bound to one
// criterion.
type Evidence struct {
	// BuildOK is the result of the ecosystem's build check on the mutant.
	BuildOK bool
	// Bound lists the tests bound to the criterion.
	Bound []ecosystem.TestID
	// Run is the result of the suite run on the selection.
	Run ecosystem.RunResult
}

// Classify applies the outcome rules of ADR 0002:
//
//   - error: the build check failed, collection failed, the run timed out, no
//     test is bound, or a bound test is absent from the results or skipped
//     (unless another bound test already failed);
//   - killed: a bound test failed;
//   - survived: every bound test passed.
func Classify(evidence Evidence) format.Outcome {
	if !evidence.BuildOK || evidence.Run.CollectionFailed || evidence.Run.TimedOut || len(evidence.Bound) == 0 {
		return format.OutcomeError
	}
	allPassed := true
	for _, test := range evidence.Bound {
		switch evidence.Run.Outcomes[test] {
		case ecosystem.TestFailed:
			return format.OutcomeKilled
		case ecosystem.TestPassed:
		default:
			allPassed = false
		}
	}
	if allPassed {
		return format.OutcomeSurvived
	}
	return format.OutcomeError
}
