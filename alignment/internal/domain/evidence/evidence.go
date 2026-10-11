// Package evidence reads discriminating-power Statements into the pools a
// matrix is computed from. A pool holds only what the claim-power table lets
// carry weight: VALID mutants that are owner-attributed, witnessed on execution
// or reasoning-judged and not collateral, UNATTRIBUTED mutants of any grade, and
// sentinels.
package evidence

import (
	"slices"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/matrix"
	"github.com/RachidChabane/spec-proof/format"
)

// qualifyingBases are the decision bases whose VALID verdicts enter a pool: a
// failed example of the criterion, a failed block-bearing witness of it, and the
// judges.
var qualifyingBases = []format.DecisionBasis{format.BasisSpecExample, format.BasisExecution, format.BasisJudges}

// Pools are the three pools of one Statement.
type Pools struct {
	Valid        []matrix.Mutant
	Unattributed []matrix.Mutant
	Sentinels    []matrix.Mutant
}

// Covers reports whether the Statement holds results for the test, named as the
// Statement names it.
func Covers(statement format.DiscriminatingPower, test string) bool {
	return slices.Contains(statement.Tests, test)
}

// PoolsOf sorts the mutants of a Statement into pools.
func PoolsOf(statement format.DiscriminatingPower) Pools {
	var pools Pools
	for _, mutant := range statement.Mutants {
		entry := matrix.Mutant{KilledBy: killers(mutant)}
		switch {
		case mutant.Sentinel:
			pools.Sentinels = append(pools.Sentinels, entry)
		case isUnattributed(mutant):
			pools.Unattributed = append(pools.Unattributed, entry)
		case isQualifyingValid(mutant):
			pools.Valid = append(pools.Valid, entry)
		}
	}
	return pools
}

func killers(mutant format.Mutant) map[string]bool {
	killed := map[string]bool{}
	for _, result := range mutant.Results {
		if result.Outcome.Value == format.OutcomeKilled {
			killed[result.Test] = true
		}
	}
	return killed
}

func isUnattributed(mutant format.Mutant) bool {
	return mutant.Extensions != nil && mutant.Extensions.Unattributed
}

func isQualifyingValid(mutant format.Mutant) bool {
	if mutant.Validity.Value != format.ValidityValid || mutant.Extensions == nil || mutant.Extensions.Collateral {
		return false
	}
	decision := mutant.Extensions.Decision
	return decision != nil && slices.Contains(qualifyingBases, decision.Basis)
}

// FactsFor builds the facts of one test from the Statements that cover it: each
// criterion's pool, and the unattributed and sentinel mutants of those Statements.
// A Statement that does not list the test holds no result for it, and counting
// its mutants would dilute every share, so it is left out.
func FactsFor(test string, statements map[criterion.Ref]format.DiscriminatingPower) matrix.Facts {
	facts := matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{}}
	refs := make([]criterion.Ref, 0, len(statements))
	for ref := range statements {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, criterion.Ref.Compare)
	for _, ref := range refs {
		statement := statements[ref]
		if !Covers(statement, test) {
			continue
		}
		pools := PoolsOf(statement)
		facts.Pools[ref] = pools.Valid
		facts.Unattributed = append(facts.Unattributed, pools.Unattributed...)
		facts.Sentinels = append(facts.Sentinels, pools.Sentinels...)
	}
	return facts
}
