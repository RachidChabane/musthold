package operator

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
)

func sentinelOf(kind SentinelKind, qualname string, index int) SentinelCandidate {
	target := mutation.TargetDefinition
	if kind == SentinelUnusedConstant {
		target = mutation.TargetModuleAddition
	}
	return SentinelCandidate{
		Kind:     kind,
		Site:     Site{File: "m.py", Qualname: qualname, Target: target, Span: ecosystem.Span{Start: index, End: index}},
		Original: "def " + qualname + "(): pass",
		Edit:     mutation.Edit{File: "m.py", Qualname: qualname, Target: target, NewSource: fmt.Sprintf("%s %s %d", kind, qualname, index)},
	}
}

func catalogue() []SentinelCandidate {
	var all []SentinelCandidate
	for i := range 4 {
		all = append(all, sentinelOf(SentinelDeadAssignment, fmt.Sprintf("f%d", i), i))
		all = append(all, sentinelOf(SentinelAlphaRename, fmt.Sprintf("g%d", i), i))
	}
	return append(all, sentinelOf(SentinelUnusedConstant, "_UNUSED", 99))
}

func TestDrawSentinels(t *testing.T) {
	t.Run("two sentinels of two kinds are drawn per file, marked as sentinels [verifies SP-89~1]", func(t *testing.T) {
		drawn, err := DrawSentinels("m.py", catalogue(), LocalSeed)
		if err != nil {
			t.Fatal(err)
		}
		if len(drawn) != SentinelsPerFile {
			t.Fatalf("drawn %d", len(drawn))
		}
		kinds := map[SentinelKind]bool{}
		for _, m := range drawn {
			if !m.IsSentinel() || m.Family != "" || m.Sentinel == "" || m.ID == "" {
				t.Fatalf("not a well-formed sentinel: %+v", m)
			}
			kinds[m.Sentinel] = true
		}
		if len(kinds) != 2 {
			t.Fatalf("kinds = %v", kinds)
		}
	})

	t.Run("one available kind yields two distinct sentinels of it [verifies SP-89~1]", func(t *testing.T) {
		only := []SentinelCandidate{sentinelOf(SentinelAlphaRename, "a", 1), sentinelOf(SentinelAlphaRename, "b", 2), sentinelOf(SentinelAlphaRename, "c", 3)}
		drawn, err := DrawSentinels("m.py", only, 5)
		if err != nil || len(drawn) != 2 || drawn[0].ID == drawn[1].ID {
			t.Fatalf("drawn %+v, %v", drawn, err)
		}
	})

	t.Run("fewer candidates than the quota are all drawn [verifies SP-89~1]", func(t *testing.T) {
		drawn, err := DrawSentinels("m.py", []SentinelCandidate{sentinelOf(SentinelUnusedConstant, "_U", 1)}, 5)
		if err != nil || len(drawn) != 1 {
			t.Fatalf("drawn %d, %v", len(drawn), err)
		}
		if none, _ := DrawSentinels("m.py", nil, 5); len(none) != 0 {
			t.Fatalf("drew %d from nothing", len(none))
		}
	})

	t.Run("the draw depends on the seed and the file, not on the order of candidates [verifies SP-89~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			all := catalogue()
			seed := Seed(rapid.Uint64().Draw(t, "seed"))
			shuffled := rapid.Permutation(all).Draw(t, "shuffled")
			first, err1 := DrawSentinels("m.py", all, seed)
			second, err2 := DrawSentinels("m.py", shuffled, seed)
			if err1 != nil || err2 != nil || !slices.Equal(ids(first), ids(second)) {
				t.Fatalf("draws differ: %v %v", ids(first), ids(second))
			}
		})
	})

	t.Run("fixed seeds draw different sentinels [verifies SP-89~1]", func(t *testing.T) {
		seen := map[string]bool{}
		for seed := Seed(1); seed <= 12; seed++ {
			drawn, err := DrawSentinels("m.py", catalogue(), seed)
			if err != nil {
				t.Fatal(err)
			}
			seen[fmt.Sprint(ids(drawn))] = true
		}
		if len(seen) < 4 {
			t.Fatalf("12 seeds produced only %d distinct draws", len(seen))
		}
	})

	t.Run("the catalogue lists the three kinds [verifies SP-89~1]", func(t *testing.T) {
		if got := SentinelKinds(); len(got) != 3 {
			t.Fatalf("kinds = %v", got)
		}
	})
}

func TestNeutralName(t *testing.T) {
	style := func(word, suffix string) string { return word + "_" + suffix }

	t.Run("a name is fixed by seed and label, and varies with either [verifies SP-89~1]", func(t *testing.T) {
		first := NeutralName(1, "a", "", style)
		if first != NeutralName(1, "a", "", style) {
			t.Fatal("same inputs gave different names")
		}
		seen := map[string]bool{}
		for seed := Seed(0); seed < 30; seed++ {
			seen[NeutralName(seed, "a", "", style)] = true
			seen[NeutralName(1, fmt.Sprint("label", seed), "", style)] = true
		}
		if len(seen) < 40 {
			t.Fatalf("only %d distinct names", len(seen))
		}
	})

	t.Run("a name never occurs in the text of the file [verifies SP-89~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			taken := rapid.StringMatching(`[a-z_0-9 ]{0,200}`).Draw(t, "taken")
			name := NeutralName(Seed(rapid.Uint64().Draw(t, "seed")), "x", taken, style)
			if strings.Contains(taken, name) {
				t.Fatalf("%q occurs in the file", name)
			}
		})
	})
}
