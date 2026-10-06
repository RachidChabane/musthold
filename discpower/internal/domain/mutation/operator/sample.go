package operator

import (
	"slices"
	"strings"
)

const stratumSeparator = "\x00\x00"

// StratumKey names the stratum of a mutant: its site and its family.
func StratumKey(m Mutant) string {
	return m.Site.Key() + stratumSeparator + string(m.Family)
}

func compareMutants(a, b Mutant) int {
	if c := strings.Compare(StratumKey(a), StratumKey(b)); c != 0 {
		return c
	}
	return strings.Compare(a.ID, b.ID)
}

// Sample keeps at most kOp mutants, stratified by (site, family). The strata are
// visited in a seeded order and each round takes one mutant, drawn by seed, from
// every stratum that still has one, so the counts of two strata that both had
// candidates left differ by at most one. The result is in canonical order and
// is a pure function of the mutants as a set, kOp and the seed.
func Sample(mutants []Mutant, kOp int, seed Seed) []Mutant {
	ordered := slices.Clone(mutants)
	slices.SortFunc(ordered, compareMutants)
	if kOp <= 0 {
		return nil
	}
	if len(ordered) <= kOp {
		return ordered
	}

	strata, keys := groupByStratum(ordered)
	rng := streamFor(seed, "sample")
	shuffle(rng, keys)
	for _, key := range keys {
		shuffle(rng, strata[key])
	}

	picked := make([]Mutant, 0, kOp)
	for round := 0; len(picked) < kOp; round++ {
		for _, key := range keys {
			if round < len(strata[key]) && len(picked) < kOp {
				picked = append(picked, strata[key][round])
			}
		}
	}
	slices.SortFunc(picked, compareMutants)
	return picked
}

func groupByStratum(ordered []Mutant) (map[string][]Mutant, []string) {
	strata := map[string][]Mutant{}
	var keys []string
	for _, m := range ordered {
		key := StratumKey(m)
		if _, seen := strata[key]; !seen {
			keys = append(keys, key)
		}
		strata[key] = append(strata[key], m)
	}
	return strata, keys
}
