package scanscope_test

import (
	"fmt"
	"maps"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	verifiesAllocation = " [verifies SP-2125~2]"

	allocatedFile      = "pkg/code.py"
	maxAllocatedLines  = 40
	maxSpanLines       = 3
	maxCandidates      = 60
	maxAllocatedTests  = 8
	maxCoveredLines    = 15
	maxAllocatedBudget = 50
)

var families = []string{"relational", "arithmetic", "boolean", "constant"}

func line(number int) vacuity.Line { return vacuity.Line{File: allocatedFile, Line: number} }

// drawCandidates draws candidates of one file, each mutating a short span of
// lines, with distinct ids.
func drawCandidates(t *rapid.T) []scanscope.Candidate {
	count := rapid.IntRange(0, maxCandidates).Draw(t, "candidates")
	candidates := make([]scanscope.Candidate, 0, count)
	for index := range count {
		first := rapid.IntRange(1, maxAllocatedLines).Draw(t, fmt.Sprintf("first line of %d", index))
		var lines []vacuity.Line
		for offset := range rapid.IntRange(1, maxSpanLines).Draw(t, fmt.Sprintf("lines of %d", index)) {
			lines = append(lines, line(first+offset))
		}
		candidates = append(candidates, scanscope.Candidate{
			ID:     fmt.Sprintf("m%03d", index),
			Family: rapid.SampledFrom(families).Draw(t, fmt.Sprintf("family of %d", index)),
			Lines:  lines,
		})
	}
	return candidates
}

// drawTestLines draws the lines each sampled test ran.
func drawTestLines(t *rapid.T) map[ecosystem.TestID]scanscope.LineSet {
	tests := map[ecosystem.TestID]scanscope.LineSet{}
	for index := range rapid.IntRange(1, maxAllocatedTests).Draw(t, "tests") {
		set := scanscope.LineSet{}
		for range rapid.IntRange(0, maxCoveredLines).Draw(t, fmt.Sprintf("covered by %d", index)) {
			set[line(rapid.IntRange(1, maxAllocatedLines).Draw(t, "covered line"))] = true
		}
		tests[ecosystem.TestID(fmt.Sprintf("tests/test_code.py::test_%d", index))] = set
	}
	return tests
}

// pools counts, per test, the candidates with a mutated line among its lines.
func pools(candidates []scanscope.Candidate, tests map[ecosystem.TestID]scanscope.LineSet) map[ecosystem.TestID]int {
	counts := map[ecosystem.TestID]int{}
	for test, set := range tests {
		for _, candidate := range candidates {
			if set.Intersects(candidate.Lines) {
				counts[test]++
			}
		}
	}
	return counts
}

// drawnAmong counts, per test, the drawn candidates with a mutated line among
// its lines.
func drawnAmong(candidates []scanscope.Candidate, drawn []string, tests map[ecosystem.TestID]scanscope.LineSet) map[ecosystem.TestID]int {
	var kept []scanscope.Candidate
	for _, candidate := range candidates {
		if slices.Contains(drawn, candidate.ID) {
			kept = append(kept, candidate)
		}
	}
	return pools(kept, tests)
}

func TestAllocate(t *testing.T) {
	t.Run("the allocation is the same for the same seed whatever the order of the candidates"+verifiesAllocation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			candidates, tests := drawCandidates(t), drawTestLines(t)
			budget := rapid.IntRange(0, maxAllocatedBudget).Draw(t, "budget")
			seed := rapid.Uint64().Draw(t, "seed")
			reversed := slices.Clone(candidates)
			slices.Reverse(reversed)
			first, second := scanscope.Allocate(candidates, tests, budget, seed), scanscope.Allocate(reversed, tests, budget, seed)
			if !slices.Equal(first.Drawn, second.Drawn) || !maps.Equal(first.PerTest, second.PerTest) {
				t.Fatalf("%v != %v", first, second)
			}
		})
	})

	t.Run("the allocation draws at most the budget, distinct and sorted, each a candidate a sampled test ran"+verifiesAllocation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			candidates, tests := drawCandidates(t), drawTestLines(t)
			budget := rapid.IntRange(0, maxAllocatedBudget).Draw(t, "budget")
			allocation := scanscope.Allocate(candidates, tests, budget, rapid.Uint64().Draw(t, "seed"))
			if len(allocation.Drawn) > max(budget, 0) || !slices.IsSorted(allocation.Drawn) || len(slices.Compact(slices.Clone(allocation.Drawn))) != len(allocation.Drawn) {
				t.Fatalf("drawn %v for a budget of %d", allocation.Drawn, budget)
			}
			for _, id := range allocation.Drawn {
				index := slices.IndexFunc(candidates, func(c scanscope.Candidate) bool { return c.ID == id })
				if index < 0 || !slices.ContainsFunc(slices.Collect(maps.Values(tests)), func(set scanscope.LineSet) bool { return set.Intersects(candidates[index].Lines) }) {
					t.Fatalf("drew %s, which no sampled test ran", id)
				}
			}
		})
	})

	t.Run("every sampled test gets at least the fewer of its candidates and its share of the budget"+verifiesAllocation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			candidates, tests := drawCandidates(t), drawTestLines(t)
			budget := rapid.IntRange(0, maxAllocatedBudget).Draw(t, "budget")
			allocation := scanscope.Allocate(candidates, tests, budget, rapid.Uint64().Draw(t, "seed"))
			available := pools(candidates, tests)
			share := 0
			if len(available) > 0 {
				share = budget / len(available)
			}
			drawn := drawnAmong(candidates, allocation.Drawn, tests)
			for test, pool := range available {
				if drawn[test] < min(pool, share) || allocation.PerTest[test] != drawn[test] {
					t.Fatalf("%s got %d (reported %d) of %d candidates; its share is %d", test, drawn[test], allocation.PerTest[test], pool, share)
				}
			}
		})
	})

	t.Run("a budget of every candidate draws every candidate a sampled test ran"+verifiesAllocation, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			candidates, tests := drawCandidates(t), drawTestLines(t)
			allocation := scanscope.Allocate(candidates, tests, len(candidates), rapid.Uint64().Draw(t, "seed"))
			if !maps.Equal(drawnAmong(candidates, allocation.Drawn, tests), pools(candidates, tests)) {
				t.Fatalf("drawn %v", allocation.Drawn)
			}
		})
	})

	t.Run("a test whose lines hold two sites draws its second site before a second candidate of its first"+verifiesAllocation, func(t *testing.T) {
		var candidates []scanscope.Candidate
		for index := range 4 {
			candidates = append(candidates, scanscope.Candidate{ID: fmt.Sprintf("a%d", index), Family: families[index], Lines: []vacuity.Line{line(1)}})
		}
		candidates = append(candidates, scanscope.Candidate{ID: "z", Family: families[0], Lines: []vacuity.Line{line(2)}})
		tests := map[ecosystem.TestID]scanscope.LineSet{"t::one": scanscope.NewLineSet([]vacuity.Line{line(1), line(2)})}
		for seed := range uint64(20) {
			if allocation := scanscope.Allocate(candidates, tests, 2, seed); !slices.Contains(allocation.Drawn, "z") {
				t.Fatalf("seed %d drew %v", seed, allocation.Drawn)
			}
		}
	})

	t.Run("a test draws a new family before a family it already has on the same site"+verifiesAllocation, func(t *testing.T) {
		candidates := []scanscope.Candidate{
			{ID: "a", Family: families[0], Lines: []vacuity.Line{line(1)}},
			{ID: "b", Family: families[0], Lines: []vacuity.Line{line(1)}},
			{ID: "c", Family: families[1], Lines: []vacuity.Line{line(1)}},
		}
		tests := map[ecosystem.TestID]scanscope.LineSet{"t::one": scanscope.NewLineSet([]vacuity.Line{line(1)})}
		for seed := range uint64(20) {
			if allocation := scanscope.Allocate(candidates, tests, 2, seed); !slices.Contains(allocation.Drawn, "c") {
				t.Fatalf("seed %d drew %v", seed, allocation.Drawn)
			}
		}
	})

	t.Run("a candidate both of two needy tests ran is drawn before one only the drawing test ran"+verifiesAllocation, func(t *testing.T) {
		candidates := []scanscope.Candidate{
			{ID: "shared", Family: families[0], Lines: []vacuity.Line{line(1)}},
			{ID: "own", Family: families[0], Lines: []vacuity.Line{line(2)}},
			{ID: "other", Family: families[0], Lines: []vacuity.Line{line(3)}},
		}
		tests := map[ecosystem.TestID]scanscope.LineSet{
			"t::one": scanscope.NewLineSet([]vacuity.Line{line(1), line(2)}),
			"t::two": scanscope.NewLineSet([]vacuity.Line{line(1), line(3)}),
		}
		for seed := range uint64(20) {
			allocation := scanscope.Allocate(candidates, tests, 1, seed)
			if !slices.Equal(allocation.Drawn, []string{"shared"}) || allocation.PerTest["t::one"] != 1 || allocation.PerTest["t::two"] != 1 {
				t.Fatalf("seed %d: %+v", seed, allocation)
			}
		}
	})

	t.Run("every test reaches the per-test quota before any test draws past it"+verifiesAllocation, func(t *testing.T) {
		var candidates []scanscope.Candidate
		tests := map[ecosystem.TestID]scanscope.LineSet{}
		for test := range 3 {
			set := scanscope.LineSet{}
			for index := range 3 * scanscope.CandidatesPerTest {
				number := 100*(test+1) + index
				set[line(number)] = true
				candidates = append(candidates, scanscope.Candidate{ID: fmt.Sprintf("t%d-%02d", test, index), Family: families[index%len(families)], Lines: []vacuity.Line{line(number)}})
			}
			tests[ecosystem.TestID(fmt.Sprintf("t::%d", test))] = set
		}
		allocation := scanscope.Allocate(candidates, tests, 3*scanscope.CandidatesPerTest+1, 7)
		for test, count := range allocation.PerTest {
			if count < scanscope.CandidatesPerTest {
				t.Fatalf("%s got %d: %v", test, count, allocation.PerTest)
			}
		}
	})
}
