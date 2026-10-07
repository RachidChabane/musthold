package masking_test

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

const (
	seedA = "tests.test_parcel.test_zero_quantity_is_rejected"
	seedB = "tests.test_parcel.test_single_unit_costs_its_price"
)

func criteria(count int) []criterion.Criterion {
	list := make([]criterion.Criterion, count)
	for index := range list {
		ref := criterion.Ref{ID: fmt.Sprintf("PAR-%d", index+1), Revision: 1}
		list[index] = criterion.Criterion{
			Ref:       ref,
			Statement: fmt.Sprintf("WHEN case %d occurs THE SYSTEM SHALL answer %d (see %s).", index+1, index+1, ref),
		}
	}
	return list
}

func TestTokenize(t *testing.T) {
	t.Run("criteria are shown under unique opaque tokens that name no criterion [verifies SP-300~1]", func(t *testing.T) {
		tokenized := masking.Tokenize(criteria(12), seedA)
		if len(tokenized.Tokens) != 12 || len(tokenized.Refs) != 12 {
			t.Fatalf("tokens %d refs %d", len(tokenized.Tokens), len(tokenized.Refs))
		}
		seen := map[string]bool{}
		for _, token := range tokenized.Tokens {
			if !strings.HasPrefix(token, masking.TokenPrefix) || seen[token] {
				t.Fatalf("token %q is not unique and prefixed", token)
			}
			seen[token] = true
			if strings.Contains(token, "PAR") {
				t.Fatalf("token %q names a criterion", token)
			}
		}
	})

	t.Run("the tokens and their order are a pure function of the seed and the criteria [verifies SP-300~1]", func(t *testing.T) {
		first, second := masking.Tokenize(criteria(10), seedA), masking.Tokenize(criteria(10), seedA)
		if first.Text != second.Text || !slices.Equal(first.Tokens, second.Tokens) {
			t.Fatal("the same inputs gave different tokens")
		}
		other := masking.Tokenize(criteria(10), seedB)
		if other.Text == first.Text {
			t.Fatal("another seed gave the same tokens and order")
		}
		reversed := slices.Clone(criteria(10))
		slices.Reverse(reversed)
		if masking.Tokenize(reversed, seedA).Text != first.Text {
			t.Fatal("the order of the criteria given changed the result")
		}
	})

	t.Run("every token maps back to its criterion and shows its statement [verifies SP-300~1]", func(t *testing.T) {
		tokenized := masking.Tokenize(criteria(5), seedA)
		for token, ref := range tokenized.Refs {
			if !strings.Contains(tokenized.Statements[token], fmt.Sprintf("case %d occurs", indexOf(ref)+1)) {
				t.Errorf("token %s of %s shows %q", token, ref, tokenized.Statements[token])
			}
		}
	})
}

func indexOf(ref criterion.Ref) int {
	var number int
	_, _ = fmt.Sscanf(ref.ID, "PAR-%d", &number)
	return number - 1
}

func TestScrub(t *testing.T) {
	t.Run("a statement shows no criterion reference or identifier [verifies SP-302~1]", func(t *testing.T) {
		tokenized := masking.Tokenize(criteria(3), seedA)
		for _, line := range strings.Split(tokenized.Text, "\n") {
			if strings.Contains(line, "PAR-") || strings.Contains(line, "~1") {
				t.Errorf("line %q names a criterion", line)
			}
		}
		if !strings.Contains(tokenized.Text, masking.Redacted) {
			t.Error("the reference was removed without a trace")
		}
	})

	t.Run("longer identifiers are scrubbed before the identifiers they contain [verifies SP-302~1]", func(t *testing.T) {
		refs := []criterion.Ref{{ID: "SP-1", Revision: 1}, {ID: "SP-10", Revision: 1}}
		scrubbed := masking.Scrub("SP-10 and SP-1 and SP-10~1", refs)
		if strings.Contains(scrubbed, "SP-") {
			t.Fatalf("scrubbed %q", scrubbed)
		}
	})
}

func TestLeaks(t *testing.T) {
	refs := []criterion.Ref{{ID: "PAR-3", Revision: 1}}
	const name = "test_zero_quantity_is_rejected"

	t.Run("an item with a criterion reference, an identifier or the marker word leaks [verifies SP-301~2]", func(t *testing.T) {
		for _, item := range []string{"x = 'PAR-3~1'", "PAR-3", "@verifies('x')", "def Verifies(): pass"} {
			if err := masking.Ensure(item, name, refs); !errors.Is(err, masking.ErrLeak) {
				t.Errorf("%q: error %v", item, err)
			}
		}
	})

	t.Run("an item with three consecutive words of the test name leaks [verifies SP-301~2]", func(t *testing.T) {
		for _, item := range []string{"zero_quantity_is_rejected", "assert zeroQuantityIs", "ZERO quantity IS"} {
			if err := masking.Ensure(item, name, refs); !errors.Is(err, masking.ErrLeak) {
				t.Errorf("%q: error %v", item, err)
			}
		}
	})

	t.Run("two words of the test name do not leak and the test prefix is ignored [verifies SP-301~2]", func(t *testing.T) {
		for _, item := range []string{"zero_quantity", "def test_case(): assert line_total(0) == 0", "zero quantity rejected"} {
			if err := masking.Ensure(item, name, refs); err != nil {
				t.Errorf("%q: error %v", item, err)
			}
		}
		if leaks := masking.NameRuns("test zero quantity", name); len(leaks) != 0 {
			t.Errorf("the prefix word counted: %v", leaks)
		}
	})

	t.Run("the ids of a parametrized run are not words of the test name [verifies SP-301~2]", func(t *testing.T) {
		const parametrized = "test_total_is_exact[10-200-180.0]"
		if got := masking.NameWithoutParameters(parametrized); got != "test_total_is_exact" {
			t.Fatalf("name %q", got)
		}
		for _, item := range []string{"@pytest.mark.parametrize('a,b,c', [(10, 200, 180.0)])", "x = [10, 200, 180, 0]", "10 200 180 0"} {
			if err := masking.Ensure(item, parametrized, refs); err != nil {
				t.Errorf("%q: error %v", item, err)
			}
		}
		if err := masking.Ensure("total is exact", parametrized, refs); !errors.Is(err, masking.ErrLeak) {
			t.Errorf("a run of the name was not found: %v", err)
		}
	})

	t.Run("a camel-case test name is read word by word [verifies SP-301~2]", func(t *testing.T) {
		runs := masking.NameRuns("a zero quantity is here", "TestZeroQuantityIsRejected")
		if len(runs) != 1 || runs[0] != "zero quantity is" {
			t.Fatalf("runs %v", runs)
		}
	})
}

func TestMaskingProperties(t *testing.T) {
	t.Run("tokens are unique for any set of criteria and any seed [verifies SP-300~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 300).Draw(t, "count")
			seed := rapid.String().Draw(t, "seed")
			tokenized := masking.Tokenize(criteria(count), seed)
			unique := map[string]bool{}
			for _, token := range tokenized.Tokens {
				unique[token] = true
			}
			if len(unique) != count {
				t.Fatalf("%d distinct tokens for %d criteria", len(unique), count)
			}
		})
	})

	t.Run("the ids of a parametrized run never change what leaks [verifies SP-905~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			item := rapid.StringMatching(`[a-z0-9 _.,()\[\]-]{0,60}`).Draw(t, "item")
			ids := rapid.StringMatching(`[0-9a-z.-]{1,20}`).Draw(t, "ids")
			const name = "test_zero_quantity_is_rejected"
			if got, want := masking.NameRuns(item, name+"["+ids+"]"), masking.NameRuns(item, name); !slices.Equal(got, want) {
				t.Fatalf("runs %v with ids %q, %v without", got, ids, want)
			}
		})
	})

	t.Run("a text that holds a declared identifier always leaks [verifies SP-301~2]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			id := rapid.StringMatching(`[A-Z]{2,5}-[0-9]{1,3}`).Draw(t, "id")
			prefix := rapid.StringMatching(`[a-z ]{0,10}`).Draw(t, "prefix")
			suffix := rapid.StringMatching(`[a-z ]{0,10}`).Draw(t, "suffix")
			refs := []criterion.Ref{{ID: id, Revision: 1}}
			if masking.Ensure(prefix+id+suffix, "test_x", refs) == nil {
				t.Fatalf("no leak found in %q", prefix+id+suffix)
			}
		})
	})
}
