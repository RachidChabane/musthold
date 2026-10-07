// Package matrix classifies a test by the mutants it kills (ADR 0003,
// "Alignment"): the share of each criterion's VALID mutants it kills, the share
// of UNATTRIBUTED mutants, and whether it kills a sentinel. The functions are
// pure over kill sets; nothing here knows where the kills came from.
package matrix

import (
	"slices"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

// Signature is the shape of a test's kills.
type Signature string

// Signatures of a test.
const (
	// Absent: no discriminating-power evidence exists for the test.
	Absent Signature = "ABSENT"
	// UndefinedOwn: the declared criterion has no VALID mutant in its pool.
	UndefinedOwn Signature = "UNDEFINED_OWN"
	// Inert: the test kills no VALID or UNATTRIBUTED mutant.
	Inert Signature = "INERT"
	// OffTarget: the test kills no VALID mutant of its criterion, but kills
	// another criterion's VALID mutant or an UNATTRIBUTED one.
	OffTarget Signature = "OFF_TARGET"
	// Partial: the test kills fewer than theta_own of its criterion's mutants.
	Partial Signature = "PARTIAL"
	// Specific: the test kills its criterion's mutants and few of the others'.
	Specific Signature = "SPECIFIC"
	// Overbroad: the test kills its criterion's mutants and many of the others'.
	Overbroad Signature = "OVERBROAD"
)

// Mutant is one mutant of a pool with the tests that killed it.
type Mutant struct {
	KilledBy map[string]bool
}

// Facts are the pools a matrix is computed from. Pools holds, per criterion, the
// VALID mutants that are owner-attributed or reasoning-judged and not collateral;
// Unattributed the UNATTRIBUTED mutants of any grade; Sentinels the sentinel
// mutants. A test is named by its result identity.
type Facts struct {
	Pools        map[criterion.Ref][]Mutant
	Unattributed []Mutant
	Sentinels    []Mutant
}

// Thresholds are the cut points of the classification.
type Thresholds struct {
	Own, Other, None float64
}

// Result is the classification of one test against one declared criterion.
type Result struct {
	Signature Signature `json:"signature"`
	// Own is the share of the declared criterion's VALID mutants the test kills;
	// nil when the criterion has none.
	Own *float64 `json:"own,omitempty"`
	// Other is the largest share of another criterion's VALID mutants it kills.
	Other float64 `json:"other"`
	// None is the share of UNATTRIBUTED mutants it kills.
	None float64 `json:"none"`
	// DC is own minus the mean share over the other criteria; nil when own is.
	DC *float64 `json:"dc,omitempty"`
	// Sentinel is true when the test kills a sentinel mutant.
	Sentinel bool `json:"sentinel"`
	// Contaminated is Sentinel: a test that reads the code it checks.
	Contaminated bool `json:"contaminated"`
	// PinsOffspec is true when the share of UNATTRIBUTED mutants killed exceeds
	// theta_none: the test pins behaviour no criterion states.
	PinsOffspec bool `json:"pinsOffspec"`
	// CriteriaKilled lists the criteria whose VALID mutants the test kills.
	CriteriaKilled []string `json:"criteriaKilled"`
}

// Absence is the result for a test with no discriminating-power evidence.
func Absence() Result { return Result{Signature: Absent, CriteriaKilled: []string{}} }

// Eligible reports whether the matrix corroborates alignment: a SPECIFIC or
// OVERBROAD test that is neither contaminated nor pinning off-spec behaviour.
func (r Result) Eligible() bool {
	return (r.Signature == Specific || r.Signature == Overbroad) && !r.Contaminated && !r.PinsOffspec
}

// Negative reports whether the matrix speaks against alignment: an OFF_TARGET,
// INERT or PARTIAL test, or one that pins off-spec behaviour. A matrix that is
// absent or undefined for the criterion is silent, not negative.
func (r Result) Negative() bool {
	switch r.Signature {
	case OffTarget, Inert, Partial:
		return true
	}
	return r.PinsOffspec
}

// Classify classifies the test against the declared criterion.
func Classify(facts Facts, test string, declared criterion.Ref, thresholds Thresholds) Result {
	shares := validShares(facts.Pools, test)
	own, hasOwn := shares[declared]
	other, otherMean, killedCriteria := othersOf(shares, declared)
	none := shareKilled(facts.Unattributed, test)
	result := Result{
		Other: other, None: none,
		Sentinel: anyKilled(facts.Sentinels, test), CriteriaKilled: killedCriteria,
	}
	result.Contaminated = result.Sentinel
	result.PinsOffspec = none > thresholds.None
	if hasOwn {
		result.Own = &own
		difference := own - otherMean
		result.DC = &difference
	}
	result.Signature = signatureOf(result, hasOwn, thresholds)
	return result
}

// validShares gives, per criterion whose pool is not empty, the share of the
// pool the test kills.
func validShares(pools map[criterion.Ref][]Mutant, test string) map[criterion.Ref]float64 {
	shares := make(map[criterion.Ref]float64, len(pools))
	for ref, pool := range pools {
		if len(pool) > 0 {
			shares[ref] = shareKilled(pool, test)
		}
	}
	return shares
}

func shareKilled(pool []Mutant, test string) float64 {
	if len(pool) == 0 {
		return 0
	}
	killed := 0
	for _, mutant := range pool {
		if mutant.KilledBy[test] {
			killed++
		}
	}
	return float64(killed) / float64(len(pool))
}

func anyKilled(pool []Mutant, test string) bool {
	return slices.ContainsFunc(pool, func(mutant Mutant) bool { return mutant.KilledBy[test] })
}

// othersOf returns the largest share, the mean share and the criteria killed,
// over the criteria other than the declared one.
func othersOf(shares map[criterion.Ref]float64, declared criterion.Ref) (largest, mean float64, killed []string) {
	killed = []string{}
	count, total := 0, 0.0
	refs := make([]criterion.Ref, 0, len(shares))
	for ref := range shares {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, criterion.Ref.Compare)
	for _, ref := range refs {
		share := shares[ref]
		if share > 0 {
			killed = append(killed, ref.String())
		}
		if ref == declared {
			continue
		}
		largest = max(largest, share)
		total += share
		count++
	}
	if count > 0 {
		mean = total / float64(count)
	}
	return largest, mean, killed
}

func signatureOf(result Result, hasOwn bool, thresholds Thresholds) Signature {
	switch {
	case !hasOwn:
		return UndefinedOwn
	case len(result.CriteriaKilled) == 0 && result.None == 0:
		return Inert
	case *result.Own == 0:
		return OffTarget
	case *result.Own < thresholds.Own:
		return Partial
	case result.Other <= thresholds.Other:
		return Specific
	}
	return Overbroad
}
