package scan_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

const (
	verifiesOwnedDraw  = " [verifies SP-2125~2]"
	verifiesStartUpRun = " [verifies SP-2133~2]"
	// bootTest runs the shipping lines at start-up only, and its process needs
	// the line of the free-shipping threshold to start.
	bootTest = "tests/test_boot.py::test_boot_reads_the_fee"
	// thresholdLine is the line of the free-shipping threshold.
	thresholdLine = 8
)

// shippingLines are the lines of shipping.
var shippingLines = []int{8, 9, 10}

func mutatesOneOf(mutant format.ScanMutant, lines []int) bool {
	return slices.ContainsFunc(mutant.Lines, func(line format.ScanLine) bool { return slices.Contains(lines, line.Line) })
}

func scanTestNamed(report []format.ScanTest, test ecosystem.TestID) format.ScanTest {
	index := slices.IndexFunc(report, func(scanned format.ScanTest) bool { return scanned.ID == string(test) })
	if index < 0 {
		return format.ScanTest{}
	}
	return report[index]
}

func TestStartUpLines(t *testing.T) {
	t.Run("the allocation draws no operator mutant on lines every test runs only at start-up"+verifiesOwnedDraw, func(t *testing.T) {
		f := newFixture()
		f.suite.tests = fakeTests()
		delete(f.suite.tests, feeTest)
		delete(f.suite.tests, redTest)
		vacuous := f.suite.tests[vacuousTest]
		vacuous.startUp = shippingLines
		f.suite.tests[vacuousTest] = vacuous
		report := run(t, f, request())
		for _, mutant := range report.Statement.Predicate.Mutants {
			if !mutant.Sentinel && mutatesOneOf(mutant, shippingLines) {
				t.Fatalf("operator mutant %s drawn on %v", mutant.ID, mutant.Lines)
			}
		}
	})

	f := newFixture()
	vacuous := f.suite.tests[vacuousTest]
	vacuous.startUp = shippingLines
	f.suite.tests[vacuousTest] = vacuous
	f.suite.tests[bootTest] = fakeTest{file: "tests/test_boot.py", lines: []int{2, 3, thresholdLine}, needs: []int{thresholdLine}}
	report := run(t, f, request())
	tests := report.Statement.Predicate.Tests
	byID := map[string]format.ScanMutant{}
	for _, mutant := range report.Statement.Predicate.Mutants {
		byID[mutant.ID] = mutant
	}

	t.Run("a mutant on a line a test ran only at start-up runs against it, and its survival is no change of it"+verifiesStartUpRun, func(t *testing.T) {
		dispatched := false
		for _, selection := range f.suite.selections {
			dispatched = dispatched || slices.Contains(selection, vacuousTest)
		}
		onShipping := 0
		for _, mutant := range report.Statement.Predicate.Mutants {
			if mutant.Proven() && mutatesOneOf(mutant, shippingLines) {
				onShipping++
			}
		}
		if !dispatched || onShipping == 0 {
			t.Fatalf("dispatched %v, %d proven mutants on the shipping lines", dispatched, onShipping)
		}
		for _, change := range scanTestNamed(tests, vacuousTest).Changes {
			if change.Outcome == format.ScanSurvived && mutatesOneOf(byID[change.Mutant], shippingLines) {
				t.Fatalf("survival %s on a start-up-only line counted", change.Mutant)
			}
		}
	})

	t.Run("a proven mutant on a line a test owns that stops its process is listed apart from its changes"+verifiesStartUpRun, func(t *testing.T) {
		boot := scanTestNamed(tests, bootTest)
		if len(boot.NotStarted) == 0 {
			t.Fatalf("boot test = %+v", boot)
		}
		for _, id := range boot.NotStarted {
			if !byID[id].Proven() || !mutatesOneOf(byID[id], []int{thresholdLine}) {
				t.Fatalf("listed %s on %v", id, byID[id].Lines)
			}
			if slices.ContainsFunc(boot.Changes, func(change format.ScanChange) bool { return change.Mutant == id }) {
				t.Fatalf("%s is also a change", id)
			}
		}
	})
}
