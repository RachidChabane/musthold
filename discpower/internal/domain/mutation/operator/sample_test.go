package operator

import (
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// syntheticMutant builds a mutant of a site and a family; variant tells mutants
// of one stratum apart.
func syntheticMutant(t failer, siteIndex int, family Family, variant int) Mutant {
	t.Helper()
	qualname := fmt.Sprintf("f%d", siteIndex)
	mutant, err := NewMutant(Candidate{
		Site:     Site{File: "m.py", Qualname: qualname, Target: mutation.TargetDefinition, Span: ecosystem.Span{Start: siteIndex, End: siteIndex + 1}},
		Family:   family,
		Original: "def " + qualname + "(): pass",
		Edit: mutation.Edit{
			File: "m.py", Qualname: qualname, Target: mutation.TargetDefinition,
			NewSource: fmt.Sprintf("def %s(): %s%d", qualname, family, variant),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return mutant
}

// pool builds sites x families x variants mutants where stratum (s, f) holds
// 1 + (s+f) % maxVariants variants.
func pool(t failer, sites, maxVariants int) []Mutant {
	t.Helper()
	families := Families()[:4]
	var all []Mutant
	for site := 0; site < sites; site++ {
		for index, family := range families {
			for variant := 0; variant <= (site+index)%maxVariants; variant++ {
				all = append(all, syntheticMutant(t, site, family, variant))
			}
		}
	}
	return all
}

type failer interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func ids(mutants []Mutant) []string {
	out := make([]string, len(mutants))
	for i, m := range mutants {
		out[i] = m.ID
	}
	return out
}

func countByStratum(mutants []Mutant) map[string]int {
	counts := map[string]int{}
	for _, m := range mutants {
		counts[StratumKey(m)]++
	}
	return counts
}

func TestSample(t *testing.T) {
	all := pool(t, 12, 4)

	t.Run("the default cap is 400 [verifies SP-87~1]", func(t *testing.T) {
		if DefaultKOp != 400 || DefaultConfig().KOp != 400 {
			t.Fatalf("DefaultKOp = %d", DefaultKOp)
		}
	})

	t.Run("a pool within the cap is kept whole [verifies SP-87~1]", func(t *testing.T) {
		if got := Sample(all, len(all)+10, 7); len(got) != len(all) {
			t.Fatalf("kept %d of %d", len(got), len(all))
		}
	})

	t.Run("a non-positive cap keeps nothing [verifies SP-87~1]", func(t *testing.T) {
		if got := Sample(all, 0, 7); len(got) != 0 {
			t.Fatalf("kept %d", len(got))
		}
	})

	t.Run("the cap is reached exactly and the strata are balanced [verifies SP-87~1]", func(t *testing.T) {
		capacity := countByStratum(all)
		for _, k := range []int{1, 5, len(capacity) - 1, len(capacity), len(capacity) + 3, 2 * len(capacity), len(all) - 1} {
			picked := Sample(all, k, 11)
			if len(picked) != k {
				t.Fatalf("k=%d kept %d", k, len(picked))
			}
			assertBalanced(t, picked, capacity)
		}
	})

	t.Run("every stratum is represented once the cap allows [verifies SP-87~1]", func(t *testing.T) {
		capacity := countByStratum(all)
		picked := countByStratum(Sample(all, len(capacity), 3))
		if len(picked) != len(capacity) {
			t.Fatalf("%d of %d strata represented", len(picked), len(capacity))
		}
	})

	t.Run("the cap and the balance hold for any pool, cap and seed [verifies SP-87~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			sites := rapid.IntRange(1, 20).Draw(t, "sites")
			maxVariants := rapid.IntRange(1, 5).Draw(t, "variants")
			mutants := pool(t, sites, maxVariants)
			k := rapid.IntRange(0, len(mutants)+5).Draw(t, "k")
			seed := Seed(rapid.Uint64().Draw(t, "seed"))
			picked := Sample(mutants, k, seed)
			if len(picked) != min(k, len(mutants)) {
				t.Fatalf("k=%d kept %d of %d", k, len(picked), len(mutants))
			}
			assertBalanced(t, picked, countByStratum(mutants))
		})
	})

	t.Run("the same seed gives the same sample whatever the input order [verifies SP-88~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			mutants := pool(t, rapid.IntRange(2, 15).Draw(t, "sites"), 4)
			k := rapid.IntRange(1, len(mutants)).Draw(t, "k")
			seed := Seed(rapid.Uint64().Draw(t, "seed"))
			shuffled := rapid.Permutation(mutants).Draw(t, "shuffled")
			if !slices.Equal(ids(Sample(mutants, k, seed)), ids(Sample(shuffled, k, seed))) {
				t.Fatal("input order changed the sample")
			}
		})
	})

	t.Run("fixed seed pairs give different samples [verifies SP-88~1]", func(t *testing.T) {
		seeds := []Seed{1, 2, 3, 0x5eed5eed}
		samples := map[string]Seed{}
		for _, seed := range seeds {
			key := fmt.Sprint(ids(Sample(all, 15, seed)))
			if other, clash := samples[key]; clash {
				t.Fatalf("seeds %d and %d gave the same sample", other, seed)
			}
			samples[key] = seed
		}
	})

	t.Run("repeating a run with the same seed gives the same sample [verifies SP-88~1]", func(t *testing.T) {
		first := Sample(all, 15, LocalSeed)
		second := Sample(slices.Clone(all), 15, LocalSeed)
		if !slices.Equal(ids(first), ids(second)) {
			t.Fatal("repeated sampling differs")
		}
	})
}

// assertBalanced checks that no stratum with candidates left trails another by
// more than one.
func assertBalanced(t failer, picked []Mutant, capacity map[string]int) {
	t.Helper()
	counts := countByStratum(picked)
	highest := 0
	for _, c := range counts {
		highest = max(highest, c)
	}
	for key, available := range capacity {
		if counts[key] < highest-1 && counts[key] < available {
			t.Fatalf("stratum %q holds %d of %d while another holds %d", key, counts[key], available, highest)
		}
		if counts[key] > available {
			t.Fatalf("stratum %q over-drawn", key)
		}
	}
}
