package question_test

import (
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/question"
)

func TestTemplates(t *testing.T) {
	t.Run("a missing dimension asks for its behaviour or an out-of-scope declaration, and flags a low-confidence answer [verifies SP-518~1]", func(t *testing.T) {
		plain := question.Missing(dimension.Boundaries, false)
		if plain.Dimension != "boundaries" || !strings.Contains(plain.Text, "boundary values") || !strings.Contains(plain.Text, "Out of scope (boundaries): <reason>") {
			t.Fatalf("question %+v", plain)
		}
		flagged := question.Missing(dimension.Boundaries, true)
		if !strings.HasPrefix(flagged.Text, question.LowConfidenceMarker) || strings.TrimPrefix(flagged.Text, question.LowConfidenceMarker) != plain.Text {
			t.Fatalf("flagged %q", flagged.Text)
		}
		if len(plain.RequirementIDs) != 0 {
			t.Fatalf("a spec-wide question names requirements %v", plain.RequirementIDs)
		}
	})

	t.Run("an unjustified change names the function and the file and no requirement [verifies SP-518~1]", func(t *testing.T) {
		got := question.UnjustifiedChange("src/invoicing/totals.py", "apply_discount")
		if got.Dimension != "unjustified_change" || !strings.Contains(got.Text, "apply_discount") || !strings.Contains(got.Text, "src/invoicing/totals.py") {
			t.Fatalf("question %+v", got)
		}
		if len(got.RequirementIDs) != 0 {
			t.Fatalf("requirements %v", got.RequirementIDs)
		}
	})

	t.Run("unconstrained behaviour names the file, the changed functions, the count and the first three mutants, and one requirement [verifies SP-518~1]", func(t *testing.T) {
		mutants := []string{"dddddddddddddddddddddddd", "aaaaaaaaaaaaaaaaaaaaaaaa", "cccccccccccccccccccccccc", "bbbbbbbbbbbbbbbbbbbbbbbb"}
		got := question.UnconstrainedBehaviour("totals.go", []string{"Total", "Amount"}, "INV-1", mutants)
		if got.Dimension != "unconstrained_behaviour" || !slices.Equal(got.RequirementIDs, []string{"INV-1"}) {
			t.Fatalf("question %+v", got)
		}
		for _, want := range []string{"totals.go", "Amount, Total", "4 ways", "aaaaaaaaaaaa, bbbbbbbbbbbb, cccccccccccc"} {
			if !strings.Contains(got.Text, want) {
				t.Errorf("the text lacks %q: %s", want, got.Text)
			}
		}
		if strings.Contains(got.Text, "dddddddddddd") {
			t.Error("a fourth mutant is shown")
		}
	})

	t.Run("an unclear requirement asks for a reading or for an observable outcome, by its standing [verifies SP-518~1]", func(t *testing.T) {
		ambiguous := question.Unclear("INV-2", prompt.Ambiguous)
		untestable := question.Unclear("INV-2", prompt.Untestable)
		if ambiguous.Dimension != "clarity" || untestable.Dimension != "clarity" || ambiguous.Text == untestable.Text {
			t.Fatalf("questions %+v %+v", ambiguous, untestable)
		}
		if !strings.Contains(ambiguous.Text, "more than one way") || !strings.Contains(untestable.Text, "observable") {
			t.Fatalf("texts %q %q", ambiguous.Text, untestable.Text)
		}
		if !slices.Equal(ambiguous.RequirementIDs, []string{"INV-2"}) {
			t.Fatalf("requirements %v", ambiguous.RequirementIDs)
		}
	})

	t.Run("a template is a pure function of its signal [verifies SP-518~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			d := rapid.SampledFrom(dimension.Checklist()).Draw(t, "dimension")
			low := rapid.Bool().Draw(t, "low")
			file, function := rapid.StringMatching(`[a-z/]{1,12}\.py`).Draw(t, "file"), rapid.StringMatching(`[a-z_]{1,10}`).Draw(t, "function")
			if question.Missing(d, low).Text != question.Missing(d, low).Text ||
				question.UnjustifiedChange(file, function).Text != question.UnjustifiedChange(file, function).Text {
				t.Fatal("a template gave two texts for one signal")
			}
		})
	})
}

func TestNormalize(t *testing.T) {
	a := question.Question{Dimension: "boundaries", Text: "What at 15?", RequirementIDs: []string{"INV-2", "INV-1"}}
	b := question.Question{Dimension: "boundaries", Text: "What at 15?", RequirementIDs: []string{"INV-3", "INV-1"}}
	c := question.Question{Dimension: "clarity", Text: "Say more."}
	d := question.Question{Dimension: "boundaries", Text: "And at 0?"}

	t.Run("identical questions merge their requirements, sorted without repeats, and questions are sorted [verifies SP-518~1]", func(t *testing.T) {
		got := question.Normalize([]question.Question{c, a, d, b})
		if len(got) != 3 {
			t.Fatalf("%d questions: %+v", len(got), got)
		}
		if got[0].Text != "And at 0?" || got[1].Text != "What at 15?" || got[2].Dimension != "clarity" {
			t.Fatalf("order %+v", got)
		}
		if !slices.Equal(got[1].RequirementIDs, []string{"INV-1", "INV-2", "INV-3"}) {
			t.Fatalf("merged requirements %v", got[1].RequirementIDs)
		}
		if got[0].RequirementIDs != nil {
			t.Fatalf("an empty requirement list is %v", got[0].RequirementIDs)
		}
	})

	t.Run("the result is strictly sorted and the same for any order of the input [verifies SP-518~1]", func(t *testing.T) {
		pool := []question.Question{a, b, c, d, question.Missing(dimension.Concurrency, false), question.UnjustifiedChange("x.go", "f")}
		rapid.Check(t, func(t *rapid.T) {
			shuffled := rapid.Permutation(slices.Clone(pool)).Draw(t, "order")
			extra := rapid.SliceOfN(rapid.SampledFrom(pool), 0, 4).Draw(t, "repeats")
			got := question.Normalize(append(shuffled, extra...))
			want := question.Normalize(pool)
			if len(got) != len(want) {
				t.Fatalf("%d questions, want %d", len(got), len(want))
			}
			for index := range got {
				if got[index].Dimension != want[index].Dimension || got[index].Text != want[index].Text || !slices.Equal(got[index].RequirementIDs, want[index].RequirementIDs) {
					t.Fatalf("question %d: %+v, want %+v", index, got[index], want[index])
				}
				if index > 0 && got[index-1].Dimension+got[index-1].Text >= got[index].Dimension+got[index].Text {
					t.Fatalf("questions %d and %d are not strictly ordered", index-1, index)
				}
			}
		})
	})

	t.Run("normalizing leaves its input untouched [verifies SP-518~1]", func(t *testing.T) {
		input := []question.Question{b, a}
		question.Normalize(input)
		if !slices.Equal(input[0].RequirementIDs, []string{"INV-3", "INV-1"}) {
			t.Fatalf("the input was changed: %v", input[0].RequirementIDs)
		}
	})
}
