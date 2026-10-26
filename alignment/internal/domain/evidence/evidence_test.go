package evidence_test

import (
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/evidence"
	"github.com/RachidChabane/musthold/alignment/internal/domain/matrix"
	"github.com/RachidChabane/musthold/format"
)

const (
	testOne = "tests.test_parcel::test_line_total"
	testTwo = "tests.test_parcel::test_zero_quantity"
)

func validity(value format.Validity) format.Claim[format.Validity] {
	return format.Claim[format.Validity]{Value: value, Class: format.ClassJudged}
}

func results(outcomes map[string]format.Outcome) []format.TestResult {
	var list []format.TestResult
	for _, test := range []string{testOne, testTwo} {
		if outcome, found := outcomes[test]; found {
			list = append(list, format.TestResult{Test: test, Outcome: format.Executed(outcome)})
		}
	}
	return list
}

func mutant(digest string, claim format.Claim[format.Validity], extensions *format.MutantExtensions, outcomes map[string]format.Outcome) format.Mutant {
	return format.Mutant{Digest: digest, Validity: claim, Results: results(outcomes), Extensions: extensions}
}

func basis(value format.DecisionBasis) *format.Decision { return &format.Decision{Basis: value} }

var (
	killedOne = map[string]format.Outcome{testOne: format.OutcomeKilled, testTwo: format.OutcomeSurvived}
	killedTwo = map[string]format.Outcome{testOne: format.OutcomeSurvived, testTwo: format.OutcomeKilled}
	erroring  = map[string]format.Outcome{testOne: format.OutcomeError, testTwo: format.OutcomeSurvived}
)

func statement() format.DiscriminatingPower {
	return format.DiscriminatingPower{
		Tests: []string{testOne, testTwo},
		Mutants: []format.Mutant{
			mutant("a", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisJudges)}, killedOne),
			mutant("b", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisSpecExample)}, killedTwo),
			mutant("c", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisSystemOne)}, killedOne),
			mutant("d", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisJudges), Collateral: true}, killedOne),
			mutant("e", validity(format.ValidityValid), nil, killedOne),
			mutant("f", validity(format.ValidityNotViolating), &format.MutantExtensions{Unattributed: true, Decision: basis(format.BasisSystemOne)}, killedOne),
			mutant("g", validity(format.ValidityNotViolating), &format.MutantExtensions{OffTarget: "PAR-2~1"}, killedOne),
			mutant("h", validity(format.ValidityEquivalent), nil, killedOne),
			func() format.Mutant {
				sentinel := mutant("i", validity(format.ValidityEquivalent), nil, killedTwo)
				sentinel.Sentinel = true
				return sentinel
			}(),
			mutant("j", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisJudges)}, erroring),
			mutant("k", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisExecution)}, killedTwo),
			mutant("l", validity(format.ValidityValid), &format.MutantExtensions{Decision: basis(format.BasisExecution), Collateral: true}, killedOne),
		},
	}
}

func TestPools(t *testing.T) {
	pools := evidence.PoolsOf(statement())

	t.Run("only owner-attributed, witnessed or judged, non-collateral VALID mutants enter the pool [verifies SP-303~2]", func(t *testing.T) {
		if len(pools.Valid) != 4 {
			t.Fatalf("%d valid mutants, want a, b, j and k", len(pools.Valid))
		}
		if witnessed := pools.Valid[3]; !witnessed.KilledBy[testTwo] || witnessed.KilledBy[testOne] {
			t.Errorf("mutant k: %v", witnessed.KilledBy)
		}
	})

	t.Run("a kill is an outcome killed and an error is not a kill [verifies SP-303~2]", func(t *testing.T) {
		if !pools.Valid[0].KilledBy[testOne] || pools.Valid[0].KilledBy[testTwo] {
			t.Errorf("mutant a: %v", pools.Valid[0].KilledBy)
		}
		if len(pools.Valid[2].KilledBy) != 0 {
			t.Errorf("mutant j: %v", pools.Valid[2].KilledBy)
		}
	})

	t.Run("unattributed mutants of any grade feed the none share, off-target and equivalent ones do not [verifies SP-303~2]", func(t *testing.T) {
		if len(pools.Unattributed) != 1 || !pools.Unattributed[0].KilledBy[testOne] {
			t.Fatalf("unattributed %v", pools.Unattributed)
		}
	})

	t.Run("a sentinel is kept apart whatever its validity [verifies SP-303~2]", func(t *testing.T) {
		if len(pools.Sentinels) != 1 || !pools.Sentinels[0].KilledBy[testTwo] {
			t.Fatalf("sentinels %v", pools.Sentinels)
		}
	})
}

func TestFacts(t *testing.T) {
	ref := criterion.Ref{ID: "PAR-1", Revision: 1}
	other := criterion.Ref{ID: "PAR-2", Revision: 1}

	t.Run("a Statement that does not list the test is left out of its facts [verifies SP-303~2]", func(t *testing.T) {
		unrelated := statement()
		unrelated.Tests = []string{"tests.test_other::test_x"}
		facts := evidence.FactsFor(testOne, map[criterion.Ref]format.DiscriminatingPower{ref: statement(), other: unrelated})
		if _, listed := facts.Pools[other]; listed || len(facts.Pools[ref]) != 4 {
			t.Fatalf("pools %v", facts.Pools)
		}
	})

	t.Run("the facts classify the test with the matrix thresholds [verifies SP-304~1]", func(t *testing.T) {
		facts := evidence.FactsFor(testOne, map[criterion.Ref]format.DiscriminatingPower{ref: statement()})
		got := matrix.Classify(facts, testOne, ref, matrix.Thresholds{Own: 0.5, Other: 0.2, None: 0.2})
		if got.Own == nil || *got.Own != 1.0/4.0 || got.Signature != matrix.Partial || !got.PinsOffspec {
			t.Fatalf("result %+v", got)
		}
	})
}
