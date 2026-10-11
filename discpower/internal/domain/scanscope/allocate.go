package scanscope

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// CandidatesPerTest is how many drawn mutants among its own lines each sampled
// test is given before any test is given more: the proven changes the claim
// "verifies nothing" needs, and one more for a mutant no route proves.
const CandidatesPerTest = vacuity.MinChanges + 1

// Labels of the draws an allocation makes.
const (
	labelAllocationCandidates = "allocate candidates"
	labelAllocationTests      = "allocate tests"
)

// Candidate is an operator mutant a scan may draw: its id, its operator family
// and the lines it mutates.
type Candidate struct {
	ID     string
	Family string
	Lines  []vacuity.Line
}

// site is where the claim counts the candidate: its first mutated line.
func (c Candidate) site() vacuity.Line { return slices.MinFunc(c.Lines, format.CompareScanLines) }

// Allocation is the draw of a scan: the candidates it runs and how many of them
// each sampled test ran a mutated line of.
type Allocation struct {
	// Drawn are the ids of the drawn candidates, sorted.
	Drawn []string
	// PerTest counts, for each sampled test with a candidate among its lines,
	// the drawn candidates with a mutated line among them.
	PerTest map[ecosystem.TestID]int
}

// Allocate draws at most budget candidates, round-robin over the sampled tests:
// each turn goes to a test with the fewest drawn candidates among its lines,
// first among the tests below CandidatesPerTest, then among all, and draws the
// candidate of its lines that, in order, gives it a site while it has fewer
// than vacuity.MinSites, runs in the most tests still below CandidatesPerTest,
// gives it a new site, gives it a new family, and comes first in the seeded
// order. A drawn candidate counts for every test that ran one of its lines. A
// candidate no sampled test ran is never drawn. The result is a pure function
// of the candidates as a set, the lines of each test, the budget and the seed.
func Allocate(candidates []Candidate, lines map[ecosystem.TestID]LineSet, budget int, seed uint64) Allocation {
	allocator := newAllocator(candidates, lines, seed)
	for _, phase := range []admission{allocator.belowQuota, allocator.everyTest} {
		allocator.fill(phase, budget)
	}
	return allocator.allocation()
}

// admission tells whether a test may take a turn in a phase of the allocation.
type admission func(test int) bool

// allocator holds the state of one allocation; tests and candidates are
// addressed by their place in the seeded orders.
type allocator struct {
	candidates []Candidate
	tests      []ecosystem.TestID
	// pools lists, per test, the candidates with a mutated line among its
	// lines, in the seeded order.
	pools [][]int
	// runners lists, per candidate, the tests that ran one of its lines.
	runners [][]int
	// left counts, per test, the candidates of its pool not drawn yet.
	left     []int
	taken    []bool
	drawn    []string
	counts   []int
	sites    []map[vacuity.Line]bool
	families []map[string]bool
}

func newAllocator(candidates []Candidate, lines map[ecosystem.TestID]LineSet, seed uint64) *allocator {
	drawSeed := DrawSeed(seed)
	ordered := distinctCandidates(candidates)
	operator.Shuffle(drawSeed, labelAllocationCandidates, ordered)
	tests := make([]ecosystem.TestID, 0, len(lines))
	for test := range lines {
		tests = append(tests, test)
	}
	slices.SortFunc(tests, compareIDs)
	operator.Shuffle(drawSeed, labelAllocationTests, tests)

	a := &allocator{
		candidates: ordered, tests: tests,
		pools: make([][]int, len(tests)), runners: make([][]int, len(ordered)), left: make([]int, len(tests)), taken: make([]bool, len(ordered)),
		counts: make([]int, len(tests)), sites: make([]map[vacuity.Line]bool, len(tests)), families: make([]map[string]bool, len(tests)),
	}
	for test := range tests {
		a.sites[test], a.families[test] = map[vacuity.Line]bool{}, map[string]bool{}
	}
	for index, candidate := range ordered {
		for test, id := range tests {
			if lines[id].Intersects(candidate.Lines) {
				a.pools[test] = append(a.pools[test], index)
				a.runners[index] = append(a.runners[index], test)
				a.left[test]++
			}
		}
	}
	return a
}

// distinctCandidates keeps one candidate per id, with lines, in id order.
func distinctCandidates(candidates []Candidate) []Candidate {
	kept := slices.DeleteFunc(slices.Clone(candidates), func(c Candidate) bool { return len(c.Lines) == 0 })
	slices.SortFunc(kept, func(a, b Candidate) int { return strings.Compare(a.ID, b.ID) })
	return slices.CompactFunc(kept, func(a, b Candidate) bool { return a.ID == b.ID })
}

func (a *allocator) belowQuota(test int) bool { return a.counts[test] < CandidatesPerTest }

func (a *allocator) everyTest(int) bool { return true }

// eligible reports whether the test may take a turn: the phase admits it and
// one of its candidates is left.
func (a *allocator) eligible(test int, phase admission) bool {
	return phase(test) && a.left[test] > 0
}

// fill gives rounds of turns in the phase until the budget is spent or no test
// is eligible.
func (a *allocator) fill(phase admission, budget int) {
	for len(a.drawn) < budget {
		if !a.round(phase, budget) {
			return
		}
	}
}

// round gives one turn, in the seeded order, to every eligible test that has
// no more candidates than the fewest any eligible test has, and reports
// whether a test was eligible. A round with an eligible test draws at least
// one candidate.
func (a *allocator) round(phase admission, budget int) bool {
	level, found := 0, false
	for test := range a.tests {
		if a.eligible(test, phase) && (!found || a.counts[test] < level) {
			level, found = a.counts[test], true
		}
	}
	if !found {
		return false
	}
	for test := range a.tests {
		if len(a.drawn) >= budget {
			break
		}
		if a.counts[test] <= level && a.eligible(test, phase) {
			a.draw(a.best(test))
		}
	}
	return true
}

// best is the candidate the test draws on its turn.
func (a *allocator) best(test int) int {
	chosen, chosenRank := -1, rank{}
	for _, index := range a.pools[test] {
		if a.taken[index] {
			continue
		}
		if candidateRank := a.rankOf(test, index); chosen < 0 || candidateRank.before(chosenRank) {
			chosen, chosenRank = index, candidateRank
		}
	}
	return chosen
}

// rank orders the candidates of a turn; the seeded order breaks every tie.
type rank struct {
	neededSite  bool
	belowQuota  int
	newSite     bool
	newFamily   bool
	seededOrder int
}

func (r rank) before(other rank) bool {
	switch {
	case r.neededSite != other.neededSite:
		return r.neededSite
	case r.belowQuota != other.belowQuota:
		return r.belowQuota > other.belowQuota
	case r.newSite != other.newSite:
		return r.newSite
	case r.newFamily != other.newFamily:
		return r.newFamily
	}
	return r.seededOrder < other.seededOrder
}

func (a *allocator) rankOf(test, index int) rank {
	candidate := a.candidates[index]
	newSite := !a.sites[test][candidate.site()]
	belowQuota := 0
	for _, runner := range a.runners[index] {
		if a.belowQuota(runner) {
			belowQuota++
		}
	}
	return rank{
		neededSite:  newSite && len(a.sites[test]) < vacuity.MinSites,
		belowQuota:  belowQuota,
		newSite:     newSite,
		newFamily:   !a.families[test][candidate.Family],
		seededOrder: index,
	}
}

// draw takes the candidate and counts it for every test that ran one of its
// lines.
func (a *allocator) draw(index int) {
	candidate := a.candidates[index]
	a.taken[index] = true
	a.drawn = append(a.drawn, candidate.ID)
	for _, test := range a.runners[index] {
		a.counts[test]++
		a.left[test]--
		a.sites[test][candidate.site()] = true
		a.families[test][candidate.Family] = true
	}
}

func (a *allocator) allocation() Allocation {
	result := Allocation{Drawn: slices.Sorted(slices.Values(a.drawn)), PerTest: map[ecosystem.TestID]int{}}
	for test, id := range a.tests {
		if len(a.pools[test]) > 0 {
			result.PerTest[id] = a.counts[test]
		}
	}
	return result
}
