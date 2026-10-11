package vacuity_test

import (
	"fmt"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	parseFile = "src/invoicing/parse.py"
	totalFile = "src/invoicing/total.py"

	vacuousTest = ecosystem.TestID("tests/test_parse.py::test_parse_ok")
	killerTest  = ecosystem.TestID("tests/test_split.py::test_trailing_comma")
	otherTest   = ecosystem.TestID("tests/test_total.py::test_sum")
)

func runs(outcome vacuity.Run) []vacuity.Run {
	out := make([]vacuity.Run, vacuity.RunsPerOutcome)
	for index := range out {
		out[index] = outcome
	}
	return out
}

func line(file string, number int) vacuity.Line { return vacuity.Line{File: file, Line: number} }

func mutantID(label string) string { return format.SHA256Hex([]byte(label)) }

// differs is a reproduced difference on a reference that agrees with itself.
func differs() vacuity.Difference {
	return vacuity.Difference{
		Operation: "split_fields", Input: `"a,"`, Channel: format.ScanChannelReturnValue, Reference: `["a", ""]`, Mutant: `["a"]`,
		Reproduced: true, ReferenceAgrees: true,
	}
}

// fixture builds a scan input test by test and mutant by mutant.
type fixture struct {
	input vacuity.Input
}

func newFixture() *fixture {
	return &fixture{input: vacuity.Input{
		Reference: map[ecosystem.TestID][]vacuity.Run{},
		Coverage:  map[ecosystem.TestID]vacuity.Coverage{},
		Outcomes:  map[string]map[ecosystem.TestID][]vacuity.Run{},
	}}
}

// sampled adds a sampled test, green on the reference, that ran the lines
// itself.
func (f *fixture) sampled(test ecosystem.TestID, lines ...vacuity.Line) *fixture {
	f.input.Reference[test] = runs(vacuity.RunPassed)
	f.input.Sampled = append(f.input.Sampled, test)
	return f.owns(test, lines...)
}

// owns records lines the test ran itself.
func (f *fixture) owns(test ecosystem.TestID, lines ...vacuity.Line) *fixture {
	coverage := f.input.Coverage[test]
	coverage.Lines = append(coverage.Lines, lines...)
	coverage.Owned = append(coverage.Owned, lines...)
	f.input.Coverage[test] = coverage
	return f
}

// atStartUp records lines the test's process ran before any test started, so
// that the test ran them without owning them.
func (f *fixture) atStartUp(test ecosystem.TestID, lines ...vacuity.Line) *fixture {
	coverage := f.input.Coverage[test]
	coverage.Lines = append(coverage.Lines, lines...)
	f.input.Coverage[test] = coverage
	return f
}

// proven adds a mutant of one line proven by a differential difference, and
// returns its id.
func (f *fixture) proven(label string, at vacuity.Line) string {
	return f.mutant(vacuity.Mutant{ID: mutantID(label), Lines: []vacuity.Line{at}, Differences: []vacuity.Difference{differs()}})
}

func (f *fixture) mutant(m vacuity.Mutant) string {
	f.input.Mutants = append(f.input.Mutants, m)
	return m.ID
}

// outcome records the runs of a test on a mutant.
func (f *fixture) outcome(mutant string, test ecosystem.TestID, result []vacuity.Run) *fixture {
	if f.input.Outcomes[mutant] == nil {
		f.input.Outcomes[mutant] = map[ecosystem.TestID][]vacuity.Run{}
	}
	f.input.Outcomes[mutant][test] = result
	return f
}

// fiveSurvivedOnTwoSites gives the test five proven changes on two sites, all
// survived, and returns their ids.
func (f *fixture) fiveSurvivedOnTwoSites(test ecosystem.TestID) []string {
	sites := []vacuity.Line{line(parseFile, 41), line(parseFile, 41), line(parseFile, 41), line(parseFile, 44), line(parseFile, 44)}
	ids := make([]string, len(sites))
	for index, site := range sites {
		ids[index] = f.proven(fmt.Sprintf("%s/%d", test, index), site)
		f.outcome(ids[index], test, runs(vacuity.RunPassed))
	}
	f.owns(test, line(parseFile, 41), line(parseFile, 44))
	return ids
}

func classify(t *testing.T, input vacuity.Input) vacuity.Result {
	t.Helper()
	result, err := vacuity.Classify(input)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func verdictOf(t *testing.T, result vacuity.Result, test ecosystem.TestID) vacuity.Verdict {
	t.Helper()
	for _, verdict := range result.Verdicts {
		if verdict.Test == test {
			return verdict
		}
	}
	t.Fatalf("no verdict for %s", test)
	return vacuity.Verdict{}
}

func assertClass(t *testing.T, verdict vacuity.Verdict, class format.ScanClass, reason format.ScanUnprovenReason) {
	t.Helper()
	if verdict.Class != class || verdict.Reason != reason {
		t.Fatalf("%s is %s (%s), want %s (%s); changes %+v", verdict.Test, verdict.Class, verdict.Reason, class, reason, verdict.Changes)
	}
}
