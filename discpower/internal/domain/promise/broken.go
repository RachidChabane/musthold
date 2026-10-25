package promise

import (
	"cmp"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

// WitnessStanding is what the scan knows of one witness of a promise.
type WitnessStanding struct {
	ID      string
	Promise string
	// Admitted: the witness set passed the reference on every admission run.
	Admitted bool
	// Stable: the admitted set also passed every run of the final stage.
	Stable bool
	// Votes are the answers of the scope judges.
	Votes []witness.ScopeVote
}

// BlockBearing reports whether the witness can back the one finding of a
// promise: admitted, stable on the reference, and held fixed by the quote alone
// by every one of the ScopeVotesPerPromise scope votes at the confidence
// threshold.
func (w WitnessStanding) BlockBearing(threshold float64) bool {
	rule := witness.ScopeRule{Votes: ScopeVotesPerPromise, Threshold: threshold}
	return w.Admitted && w.Stable && witness.Record{Scope: w.Votes}.ScopeApproved(rule)
}

// MutantEvidence is what a scan ran on one mutant.
type MutantEvidence struct {
	ID       string
	Sentinel bool
	// Witness is the outcome of each witness on the mutant, by witness id; a
	// failure counts only when it reproduced.
	Witness map[string]witness.Outcome
	// Suite are the runs of the whole suite on the mutant.
	Suite []witness.Outcome
}

// BrokenPromises lists the promises the mutant breaks: those with a
// block-bearing witness that fails on it, when the mutant is no sentinel and the
// whole suite passes on it every one of the RunsPerScanOutcome runs. Each
// promise appears once, with its first failing witness by id, in promise order.
func BrokenPromises(mutant MutantEvidence, witnesses []WitnessStanding, threshold float64) []format.ScanBrokenPromise {
	if mutant.Sentinel || !passesAll(mutant.Suite, format.RunsPerScanOutcome) {
		return nil
	}
	ordered := slices.Clone(witnesses)
	slices.SortFunc(ordered, func(a, b WitnessStanding) int {
		return cmp.Or(cmp.Compare(a.Promise, b.Promise), cmp.Compare(a.ID, b.ID))
	})
	var broken []format.ScanBrokenPromise
	for _, candidate := range ordered {
		if mutant.Witness[candidate.ID] != witness.OutcomeFailed || !candidate.BlockBearing(threshold) {
			continue
		}
		if len(broken) > 0 && broken[len(broken)-1].Promise == candidate.Promise {
			continue
		}
		broken = append(broken, format.ScanBrokenPromise{Promise: candidate.Promise, Mutant: mutant.ID, Witness: candidate.ID})
	}
	return broken
}
