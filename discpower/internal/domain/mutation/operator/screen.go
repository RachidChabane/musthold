package operator

import (
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

// Normalizer is the language-specific half of F1: it reduces the source of one
// unit to a canonical form that ignores comments, docstrings and layout, and it
// fails when the source does not parse.
type Normalizer interface {
	Normalize(path string, target mutation.TargetKind, source string) (string, error)
}

// ScreenStats counts what the screen discarded.
type ScreenStats struct {
	// Stillborn mutants whose new source does not parse.
	Stillborn int
	// Trivial mutants equal to their original after normalization.
	Trivial int
	// Duplicate mutants equal to an earlier mutant after normalization.
	Duplicate int
}

// Screen applies F0 (the new source parses) and F1 (normalize, drop mutants equal
// to the original, deduplicate) to operator candidates, in candidate order.
// Sentinels never pass through it.
func Screen(candidates []Candidate, normalizer Normalizer) ([]Mutant, ScreenStats, error) {
	var stats ScreenStats
	var kept []Mutant
	seen := map[string]bool{}
	originals := map[string]string{}

	for _, c := range candidates {
		unit := unitOf(c)
		original, cached := originals[unit]
		if !cached {
			normalized, err := normalizer.Normalize(c.Edit.File, c.Edit.Target, c.Original)
			if err != nil {
				return nil, stats, err
			}
			original = normalized
			originals[unit] = original
		}
		mutated, err := normalizer.Normalize(c.Edit.File, c.Edit.Target, c.Edit.NewSource)
		if err != nil {
			stats.Stillborn++
			continue
		}
		if mutated == original {
			stats.Trivial++
			continue
		}
		key := unit + stratumSeparator + mutated
		if seen[key] {
			stats.Duplicate++
			continue
		}
		seen[key] = true
		mutant, err := NewMutant(c)
		if err != nil {
			return nil, stats, err
		}
		kept = append(kept, mutant)
	}
	return kept, stats, nil
}

func unitOf(c Candidate) string {
	return strings.Join([]string{c.Edit.File, string(c.Edit.Target), c.Edit.Qualname}, "\x00")
}
