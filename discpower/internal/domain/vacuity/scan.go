package vacuity

import (
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// ScanTests gives the verdicts of the sampled green tests in the form of the
// scan statement: ordered by test id, each change and each mutant that stopped
// the test from starting by mutant id.
func (r Result) ScanTests() []format.ScanTest {
	var tests []format.ScanTest
	for _, verdict := range r.Verdicts {
		if !slices.Contains(format.ScanVerdicts(), verdict.Class) {
			continue
		}
		test := format.ScanTest{ID: string(verdict.Test), Class: verdict.Class, Reason: verdict.Reason, NotStarted: slices.Sorted(slices.Values(verdict.NotStarted))}
		for _, change := range verdict.Changes {
			test.Changes = append(test.Changes, format.ScanChange{Mutant: change.Mutant, Outcome: change.Outcome})
		}
		slices.SortFunc(test.Changes, func(a, b format.ScanChange) int { return strings.Compare(a.Mutant, b.Mutant) })
		tests = append(tests, test)
	}
	slices.SortFunc(tests, func(a, b format.ScanTest) int { return strings.Compare(a.ID, b.ID) })
	return tests
}
