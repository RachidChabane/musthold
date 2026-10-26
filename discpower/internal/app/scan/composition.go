package scan

import (
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/format"
)

// SearchOutcome is how far the differential search went on a mutant, the
// furthest first: it found a difference, it called the function and found
// none, it bound the function but could not call it, or it bound no function.
type SearchOutcome int

// Outcomes of the differential search, in the order of how far it went.
const (
	// SearchNotRun: the search did not run, for a killed mutant, a sentinel or
	// an ecosystem that cannot probe.
	SearchNotRun SearchOutcome = iota
	// SearchUnbound: no definition the mutant edits binds to an operation.
	SearchUnbound
	// SearchUncallable: an operation is bound, but a parameter has no known
	// type, it takes no parameter, or the probe could not call it.
	SearchUncallable
	// SearchNoDifference: the operation was called on every drawn input and
	// the mutant agreed with the reference.
	SearchNoDifference
	// SearchDifference: an input gave the mutant another result.
	SearchDifference
)

// Gap is why one mutant on the lines of an unproven test proves no change.
type Gap string

// Gaps of an unproven mutant.
const (
	// GapNotRun: the budget passed before the mutant ran.
	GapNotRun Gap = "not run within the budget"
	// GapDidNotBuild: the mutant did not splice or failed the build check,
	// which in Go also starts the test binaries of the packages it edits.
	GapDidNotBuild Gap = "that did not build or start"
	// GapIdentical: every edited site compiles to the reference's form.
	GapIdentical Gap = "identical when compiled"
	// GapDisqualifiedKill: a test kills it, but that test failed a sentinel.
	GapDisqualifiedKill Gap = "killed only by a test that fails a sentinel"
	// GapNoSearch: no test kills it and the ecosystem cannot probe.
	GapNoSearch Gap = "with no search in this ecosystem"
	// GapUnbound: no test kills it and no function it edits can be bound.
	GapUnbound Gap = "with no function to call"
	// GapUncallable: no test kills it and the bound function cannot be called.
	GapUncallable Gap = "with a function the search cannot call"
	// GapNoDifference: no test kills it and the search found no difference.
	GapNoDifference Gap = "searched without a difference"
)

// Gaps lists the gaps in the order a report shows them.
func Gaps() []Gap {
	return []Gap{GapNoDifference, GapUncallable, GapUnbound, GapNoSearch, GapDisqualifiedKill, GapIdentical, GapDidNotBuild, GapNotRun}
}

// gapOfSearch is the gap of a mutant no test kills, by how far the search
// went.
var gapOfSearch = map[SearchOutcome]Gap{
	SearchNotRun: GapNoSearch, SearchUnbound: GapUnbound, SearchUncallable: GapUncallable, SearchNoDifference: GapNoDifference,
}

// UnprovenTest is one unproven test with what its lines lack: the proven
// changes it counts and, by gap, the drawn operator mutants on its lines that
// prove nothing.
type UnprovenTest struct {
	Test   ecosystem.TestID
	Reason format.ScanUnprovenReason
	// Drawn counts the drawn mutants on the lines it ran, sentinels aside: the
	// operator mutants, and the model mutants of a scan that asked its host.
	Drawn int
	// Proven counts its changes: the pairs with proven mutants that count.
	Proven int
	// Gaps maps each unproven mutant on its lines to its gap.
	Gaps map[string]Gap
}

// unprovenTests explains every unproven test of the result, by test id.
func (e emission) unprovenTests(result vacuity.Result) []UnprovenTest {
	var explained []UnprovenTest
	for _, verdict := range result.Verdicts {
		if verdict.Class != format.ScanUnproven {
			continue
		}
		lines := scanscope.NewLineSet(e.reference.coverage[verdict.Test].Lines)
		test := UnprovenTest{Test: verdict.Test, Reason: verdict.Reason, Proven: len(verdict.Changes), Gaps: map[string]Gap{}}
		for index, candidate := range e.drawn.candidates {
			if candidate.Sentinel || !lines.Intersects(candidate.Lines) {
				continue
			}
			test.Drawn++
			if !result.Proven(candidate.ID) {
				test.Gaps[candidate.ID] = gapOf(e.executed[index])
			}
		}
		explained = append(explained, test)
	}
	slices.SortFunc(explained, func(a, b UnprovenTest) int { return strings.Compare(string(a.Test), string(b.Test)) })
	return explained
}

// stoppedSentinels counts the sentinels that ran and stopped a test's process
// before any test began in at least one run: a fault of the harness, since a
// sentinel keeps behaviour by construction.
func (e emission) stoppedSentinels() int {
	stopped := 0
	for index, candidate := range e.drawn.candidates {
		if candidate.Sentinel && e.executed[index].state == stateRan && stoppedOnce(e.executed[index].runs) {
			stopped++
		}
	}
	return stopped
}

// stoppedOnce reports whether a run of any test did not start.
func stoppedOnce(runs map[ecosystem.TestID][]vacuity.Run) bool {
	for _, testRuns := range runs {
		if slices.Contains(testRuns, vacuity.RunNotStarted) {
			return true
		}
	}
	return false
}

// gapOf is why a drawn mutant that no route proves proves nothing.
func gapOf(executed outcome) Gap {
	switch {
	case executed.state == stateNotRun:
		return GapNotRun
	case executed.state == stateDidNotBuild:
		return GapDidNotBuild
	case executed.identity:
		return GapIdentical
	case killedByOne(executed.runs):
		return GapDisqualifiedKill
	}
	if gap, known := gapOfSearch[executed.evidence.Search]; known {
		return gap
	}
	return GapNoDifference
}
