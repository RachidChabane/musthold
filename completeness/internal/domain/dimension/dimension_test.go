package dimension_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
)

func TestChecklist(t *testing.T) {
	t.Run("checklist c1 holds the eight dimensions in their fixed order [verifies SP-500~1]", func(t *testing.T) {
		want := []dimension.Dimension{
			"error_handling", "boundaries", "permissions", "concurrency", "compatibility",
			"input_validation", "time_and_locale", "state_and_persistence",
		}
		if got := dimension.Checklist(); !slices.Equal(got, want) {
			t.Fatalf("checklist %v", got)
		}
		if dimension.ChecklistVersion != "c1" {
			t.Fatalf("version %q", dimension.ChecklistVersion)
		}
	})

	t.Run("a caller cannot change the checklist [verifies SP-500~1]", func(t *testing.T) {
		list := dimension.Checklist()
		list[0] = "tampered"
		if dimension.Checklist()[0] != dimension.ErrorHandling {
			t.Fatal("the checklist was changed through a returned slice")
		}
	})

	t.Run("every dimension has a description and a phrase for the questions [verifies SP-500~1]", func(t *testing.T) {
		for _, d := range dimension.Checklist() {
			if d.Description() == "" || d.Phrase() == "" {
				t.Errorf("%s lacks a description or a phrase", d)
			}
		}
	})

	t.Run("a name is known exactly when it is on the checklist [verifies SP-500~1]", func(t *testing.T) {
		if !dimension.IsKnown("boundaries") || dimension.IsKnown("telepathy") || dimension.IsKnown("") {
			t.Fatal("IsKnown disagrees with the checklist")
		}
	})

	t.Run("the standings are COVERED, NOT_APPLICABLE and MISSING [verifies SP-500~1]", func(t *testing.T) {
		want := []dimension.Standing{"COVERED", "NOT_APPLICABLE", "MISSING"}
		if got := dimension.Standings(); !slices.Equal(got, want) {
			t.Fatalf("standings %v", got)
		}
	})
}

func TestRemaining(t *testing.T) {
	t.Run("declared dimensions are removed and the order is kept [verifies SP-501~1]", func(t *testing.T) {
		got := dimension.Remaining([]string{"permissions", "concurrency"})
		if len(got) != 6 || slices.Contains(got, dimension.Permissions) || slices.Contains(got, dimension.Concurrency) {
			t.Fatalf("remaining %v", got)
		}
		if got[0] != dimension.ErrorHandling || got[2] != dimension.Compatibility {
			t.Fatalf("remaining %v", got)
		}
	})

	t.Run("a declaration outside the checklist excludes nothing [verifies SP-501~1]", func(t *testing.T) {
		if got := dimension.Remaining([]string{"telepathy"}); len(got) != 8 {
			t.Fatalf("remaining %v", got)
		}
	})

	t.Run("declaring every dimension leaves none [verifies SP-501~1]", func(t *testing.T) {
		var all []string
		for _, d := range dimension.Checklist() {
			all = append(all, string(d))
		}
		if got := dimension.Remaining(all); len(got) != 0 {
			t.Fatalf("remaining %v", got)
		}
	})

	t.Run("no declared dimension is ever remaining, whatever is declared [verifies SP-501~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			declared := rapid.SliceOf(rapid.SampledFrom(append(toNames(dimension.Checklist()), "telepathy", ""))).Draw(t, "declared")
			remaining := dimension.Remaining(declared)
			for _, d := range remaining {
				if slices.Contains(declared, string(d)) {
					t.Fatalf("%s is remaining though declared", d)
				}
			}
			for _, d := range dimension.Checklist() {
				if !slices.Contains(declared, string(d)) && !slices.Contains(remaining, d) {
					t.Fatalf("%s was dropped without a declaration", d)
				}
			}
		})
	})
}

func toNames(dimensions []dimension.Dimension) []string {
	names := make([]string, len(dimensions))
	for index, d := range dimensions {
		names[index] = string(d)
	}
	return names
}
