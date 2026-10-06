package operator

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

var errNotParsable = errors.New("not parsable")

// whitespaceNormalizer treats a unit as a list of words and refuses any unit
// containing "(".
type whitespaceNormalizer struct{}

func (whitespaceNormalizer) Normalize(_ string, _ mutation.TargetKind, source string) (string, error) {
	if strings.Contains(source, "(") {
		return "", errNotParsable
	}
	return strings.Join(strings.Fields(source), " "), nil
}

func candidateOf(original, changed string, span int) Candidate {
	return Candidate{
		Site:     Site{File: "m.py", Qualname: "f", Target: mutation.TargetDefinition, Span: ecosystem.Span{Start: span, End: span + 1}},
		Family:   FamilyRelational,
		Original: original,
		Edit:     mutation.Edit{File: "m.py", Qualname: "f", Target: mutation.TargetDefinition, NewSource: changed},
	}
}

func TestScreen(t *testing.T) {
	const original = "return a < b"
	candidates := []Candidate{
		candidateOf(original, "return a <= b", 1),
		candidateOf(original, "return  a   < b", 2),
		candidateOf(original, "return a == b(", 3),
		candidateOf(original, "return a <= b", 4),
		candidateOf(original, "return a > b", 5),
	}

	t.Run("stillborn, trivial and duplicate mutants are dropped in order [verifies SP-86~1]", func(t *testing.T) {
		kept, stats, err := Screen(candidates, whitespaceNormalizer{})
		if err != nil {
			t.Fatal(err)
		}
		if stats != (ScreenStats{Stillborn: 1, Trivial: 1, Duplicate: 1}) {
			t.Fatalf("stats = %+v", stats)
		}
		var sources []string
		for _, m := range kept {
			sources = append(sources, m.EditSet.Edits[0].NewSource)
			if m.IsSentinel() || m.Origin != OriginOperator {
				t.Fatalf("operator mutant marked %q", m.Origin)
			}
		}
		if !slices.Equal(sources, []string{"return a <= b", "return a > b"}) {
			t.Fatalf("kept %v", sources)
		}
	})

	t.Run("an original the normalizer rejects is an error [verifies SP-86~1]", func(t *testing.T) {
		if _, _, err := Screen([]Candidate{candidateOf("return f(", "return g", 1)}, whitespaceNormalizer{}); !errors.Is(err, errNotParsable) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("the same change at two sites is a duplicate [verifies SP-86~1]", func(t *testing.T) {
		same := []Candidate{candidateOf(original, "return a <= b", 1), candidateOf(original, "return a <= b", 9)}
		kept, stats, err := Screen(same, whitespaceNormalizer{})
		if err != nil || len(kept) != 1 || stats.Duplicate != 1 {
			t.Fatalf("kept %d, stats %+v, err %v", len(kept), stats, err)
		}
	})
}
