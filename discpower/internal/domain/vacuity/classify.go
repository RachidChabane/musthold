package vacuity

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/format"
)

// What the claim "verifies nothing" needs: at least MinChanges proven changes in
// the lines the test ran, on at least MinSites sites.
const (
	MinChanges = 5
	MinSites   = 2
)

// ErrDuplicateMutant is returned when two mutants of an input share an id.
var ErrDuplicateMutant = errors.New("two mutants share an id")

// Input is everything a scan executed.
type Input struct {
	// Reference holds the runs on the reference of every test of the suite.
	Reference map[ecosystem.TestID][]Run
	// Sampled lists the tests the scan drew.
	Sampled []ecosystem.TestID
	// Coverage holds, per test, the lines it ran on the reference and the lines
	// it owns among them.
	Coverage map[ecosystem.TestID]Coverage
	Mutants  []Mutant
	// Outcomes holds, per mutant id, the runs of each test run against it.
	// Sentinels are included: a test that fails one proves nothing.
	Outcomes map[string]map[ecosystem.TestID][]Run
}

// Change is a test's settled outcome on one proven mutant whose mutated lines it
// ran.
type Change struct {
	Mutant  string
	Site    Line
	Outcome format.ScanOutcome
}

// Verdict is where one test of the suite stands.
type Verdict struct {
	Test   ecosystem.TestID
	Class  format.ScanClass
	Reason format.ScanUnprovenReason
	// Changes are the pairs of the test with the proven mutants it ran that
	// count, by the rule of pairKindOf, ordered by site then id; set for a
	// sampled green test only.
	Changes []Change
	// NotStarted are the proven mutants on lines the test owns that stopped it
	// from starting, by id; they count neither way. Set for a sampled green
	// test only.
	NotStarted []string
}

// Sites counts the distinct sites of the verdict's changes.
func (v Verdict) Sites() int {
	sites := map[Line]bool{}
	for _, change := range v.Changes {
		sites[change.Site] = true
	}
	return len(sites)
}

// Result is what a scan establishes.
type Result struct {
	// Verdicts holds every test of the suite, ranked: the class in the order of
	// format.ScanClasses, then the most changes first, then the test id
	// byte-wise.
	Verdicts []Verdict
	// Routes holds the routes of every proven mutant, by id.
	Routes map[string][]format.ScanRoute
	Counts format.ScanCounts
}

// Proven reports whether executed evidence proves the mutant changes behaviour.
func (r Result) Proven(mutant string) bool { return len(r.Routes[mutant]) > 0 }

// Classify computes the proven mutants, every test's verdict, the counts and the
// ranking. The result does not depend on the order of any list of the input.
func Classify(input Input) (Result, error) {
	byID := map[string]Mutant{}
	for _, mutant := range input.Mutants {
		if _, seen := byID[mutant.ID]; seen {
			return Result{}, fmt.Errorf("%w: %s", ErrDuplicateMutant, mutant.ID)
		}
		byID[mutant.ID] = mutant
	}
	scan := newScan(input, byID)
	result := Result{Routes: scan.routes()}
	for _, test := range scan.suite() {
		verdict := scan.verdict(test, result.Routes)
		result.Verdicts = append(result.Verdicts, verdict)
		result.Counts = result.Counts.Add(verdict.Class)
	}
	slices.SortFunc(result.Verdicts, compareVerdicts)
	return result, nil
}

// scan is an input indexed for classification.
type scan struct {
	input   Input
	mutants map[string]Mutant
	sampled map[ecosystem.TestID]bool
	// sentinelKillers are the tests that failed a run on a sentinel.
	sentinelKillers map[ecosystem.TestID]bool
}

func newScan(input Input, mutants map[string]Mutant) scan {
	s := scan{input: input, mutants: mutants, sampled: map[ecosystem.TestID]bool{}, sentinelKillers: map[ecosystem.TestID]bool{}}
	for _, test := range input.Sampled {
		s.sampled[test] = true
	}
	for id, mutant := range mutants {
		if !mutant.Sentinel {
			continue
		}
		for test, runs := range input.Outcomes[id] {
			if failedOnce(runs) {
				s.sentinelKillers[test] = true
			}
		}
	}
	return s
}

// suite lists every test the input names: those with reference runs and those
// sampled.
func (s scan) suite() []ecosystem.TestID {
	tests := slices.Collect(maps.Keys(s.input.Reference))
	for test := range s.sampled {
		if _, known := s.input.Reference[test]; !known {
			tests = append(tests, test)
		}
	}
	return tests
}

func (s scan) green(test ecosystem.TestID) bool { return Green(s.input.Reference[test]) }

// canProve reports whether a kill by the test proves a behaviour change.
func (s scan) canProve(test ecosystem.TestID) bool {
	return s.green(test) && !s.sentinelKillers[test]
}

func (s scan) routes() map[string][]format.ScanRoute {
	routes := map[string][]format.ScanRoute{}
	for id, mutant := range s.mutants {
		if found := routesOf(mutant, s.input.Outcomes[id], s.canProve); len(found) > 0 {
			routes[id] = found
		}
	}
	return routes
}

func (s scan) verdict(test ecosystem.TestID, routes map[string][]format.ScanRoute) Verdict {
	switch {
	case !s.green(test):
		return Verdict{Test: test, Class: format.ScanNotGreen}
	case !s.sampled[test]:
		return Verdict{Test: test, Class: format.ScanNotSampled}
	}
	verdict := Verdict{Test: test}
	verdict.Changes, verdict.NotStarted = s.pairs(test, routes)
	verdict.Class, verdict.Reason = s.judge(test, verdict)
	return verdict
}

// pairs reads the test's settled outcome on each proven mutant with a mutated
// line among the lines it ran, and sorts each pair by the rule of pairKindOf:
// the changes, ordered by site then id, and the mutants that stopped it from
// starting, by id.
func (s scan) pairs(test ecosystem.TestID, routes map[string][]format.ScanRoute) ([]Change, []string) {
	coverage := s.input.Coverage[test]
	ran, owned := setOf(coverage.Lines), setOf(coverage.Owned)
	var changes []Change
	var notStarted []string
	for id, mutant := range s.mutants {
		if len(routes[id]) == 0 || !ran.holdsAny(mutant.Lines) {
			continue
		}
		settled := Settle(s.input.Outcomes[id][test])
		switch pairKindOf(settled, owned.holdsAny(mutant.Lines)) {
		case pairChange:
			outcome, _ := settled.Recorded()
			changes = append(changes, Change{Mutant: id, Site: mutant.Site(), Outcome: outcome})
		case pairNotStarted:
			notStarted = append(notStarted, id)
		}
	}
	slices.SortFunc(changes, func(a, b Change) int {
		return cmp.Or(format.CompareScanLines(a.Site, b.Site), strings.Compare(a.Mutant, b.Mutant))
	})
	slices.Sort(notStarted)
	return changes, notStarted
}

// judge classifies a sampled green test, in the order of ADR 0014: a test that
// failed a sentinel is unproven; a test that kills a change caught it; a test
// that survives enough changes on enough sites verifies nothing; any other
// test is unproven.
func (s scan) judge(test ecosystem.TestID, verdict Verdict) (format.ScanClass, format.ScanUnprovenReason) {
	outcomes := map[format.ScanOutcome]int{}
	for _, change := range verdict.Changes {
		outcomes[change.Outcome]++
	}
	switch {
	case s.sentinelKillers[test]:
		return format.ScanUnproven, format.ScanKillsSentinel
	case outcomes[format.ScanKilled] > 0:
		return format.ScanCaught, ""
	case outcomes[format.ScanInconclusive] > 0:
		return format.ScanUnproven, format.ScanInconclusiveOutcome
	case len(verdict.Changes) >= MinChanges && verdict.Sites() >= MinSites:
		return format.ScanVerifiesNothing, ""
	}
	return format.ScanUnproven, format.ScanTooFewChanges
}

// compareVerdicts ranks verdicts: by class in the order of format.ScanClasses,
// then the most changes first, then the test id byte-wise.
func compareVerdicts(a, b Verdict) int {
	return cmp.Or(
		cmp.Compare(slices.Index(format.ScanClasses(), a.Class), slices.Index(format.ScanClasses(), b.Class)),
		cmp.Compare(len(b.Changes), len(a.Changes)),
		strings.Compare(string(a.Test), string(b.Test)),
	)
}
