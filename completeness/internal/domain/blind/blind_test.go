package blind_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/blind"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
)

func criteria(count int) []criterion.Criterion {
	list := make([]criterion.Criterion, count)
	for index := range list {
		id := fmt.Sprintf("INV-%d", index+1)
		list[index] = criterion.Criterion{
			Ref:       criterion.Ref{ID: id, Revision: 1},
			Statement: fmt.Sprintf("%s: WHEN a line %d is totalled THE SYSTEM SHALL add it (%s~1).", id, index, id),
		}
	}
	return list
}

func refsOf(list []criterion.Criterion) []criterion.Ref {
	refs := make([]criterion.Ref, len(list))
	for index, c := range list {
		refs[index] = c.Ref
	}
	return refs
}

func TestTokenize(t *testing.T) {
	list := criteria(12)

	t.Run("tokens and order are a pure function of the seed and the criteria [verifies SP-513~1]", func(t *testing.T) {
		first := blind.Tokenize(list, "seed", refsOf(list))
		second := blind.Tokenize(slices.Clone(list), "seed", refsOf(list))
		if first.Text != second.Text || !slices.Equal(first.Tokens, second.Tokens) {
			t.Fatal("the same work gave another payload")
		}
	})

	t.Run("another seed gives another token set and order [verifies SP-513~1]", func(t *testing.T) {
		first := blind.Tokenize(list, "seed", refsOf(list))
		other := blind.Tokenize(list, "another seed", refsOf(list))
		if first.Text == other.Text {
			t.Fatal("the seed changes nothing")
		}
	})

	t.Run("tokens are unique, opaque and map back to their criterion [verifies SP-513~1]", func(t *testing.T) {
		tokenized := blind.Tokenize(list, "seed", refsOf(list))
		seen := map[string]bool{}
		for _, token := range tokenized.Tokens {
			if seen[token] || !strings.HasPrefix(token, blind.TokenPrefix) || strings.Contains(token, "INV") {
				t.Fatalf("token %q", token)
			}
			seen[token] = true
			if _, found := tokenized.Refs[token]; !found {
				t.Fatalf("token %q maps to no criterion", token)
			}
		}
		if len(seen) != len(list) {
			t.Fatalf("%d tokens for %d criteria", len(seen), len(list))
		}
	})

	t.Run("no payload text names a criterion of the project, whatever the criteria and the seed [verifies SP-513~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 40).Draw(t, "count")
			seed := rapid.String().Draw(t, "seed")
			all := criteria(count)
			scrubbed := refsOf(all)
			tokenized := blind.Tokenize(all[:rapid.IntRange(1, count).Draw(t, "shown")], seed, scrubbed)
			if names := blind.Names(tokenized.Text, scrubbed); len(names) != 0 {
				t.Fatalf("the payload names %v:\n%s", names, tokenized.Text)
			}
		})
	})
}

func TestScrub(t *testing.T) {
	t.Run("references and bare identifiers are removed, the longer identifier first [verifies SP-513~1]", func(t *testing.T) {
		refs := []criterion.Ref{{ID: "INV-1", Revision: 1}, {ID: "INV-12", Revision: 3}}
		got := blind.Scrub("see INV-12~3, INV-1~1 and INV-12 or INV-1", refs)
		if strings.Contains(got, "INV") || strings.Contains(got, "~") {
			t.Fatalf("scrubbed text %q", got)
		}
		if got != "see the requirement, the requirement and the requirement or the requirement" {
			t.Fatalf("scrubbed text %q", got)
		}
	})

	t.Run("a text that names nothing is unchanged [verifies SP-513~1]", func(t *testing.T) {
		if got := blind.Scrub("plain text", []criterion.Ref{{ID: "INV-1", Revision: 1}}); got != "plain text" {
			t.Fatalf("scrubbed text %q", got)
		}
	})
}
