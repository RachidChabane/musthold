package scan_test

import (
	"context"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

const verifiesConfirmation = " [verifies SP-2144~1]"

// Tests of the fake suite that read what another test leaves behind.
const (
	tableLoads  = "tests/test_discount.py::test_discount_table_loads"
	loadedTable = "tests/test_discount.py::test_discount_of_the_loaded_table"
	seedsTable  = "tests/test_discount.py::test_discount_seeds_the_table"
	readsTable  = "tests/test_discount.py::test_discount_reads_the_table"
	ratesLoad   = "tests/test_shipping.py::test_shipping_rates_load"
	loadedRates = "tests/test_shipping.py::test_shipping_of_the_loaded_rates"
	anyTotal    = "tests/test_shipping.py::test_shipping_accepts_any_total"
	// confirmationBudget is a budget the mutants of the fixture never reach,
	// and pastBudget a jump of the clock beyond it.
	confirmationBudget = time.Hour
	pastBudget         = 2 * time.Hour
	referenceDir       = "reference"
)

func findingOf(report scan.Report, test ecosystem.TestID) (scan.Finding, bool) {
	for _, finding := range report.Findings {
		if finding.Test == test {
			return finding, true
		}
	}
	return scan.Finding{}, false
}

// provenOn counts the proven mutants of the scan with a mutated line among the
// lines of the totals module, and, among them, those a test kills.
func provenOn(report scan.Report, lines []int, killer ecosystem.TestID) (proven, killed int) {
	for _, mutant := range report.Statement.Predicate.Mutants {
		if !mutant.Proven() || !mutatesOneOf(mutant, lines) {
			continue
		}
		proven++
		if slices.Contains(mutant.Routes, format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: string(killer)}) {
			killed++
		}
	}
	return proven, killed
}

func TestConfirmationRecordsATestAlone(t *testing.T) {
	f := newFixture()
	f.suite.oneRun = true
	f.suite.tests[tableLoads] = fakeTest{file: "tests/test_discount.py"}
	f.suite.tests[loadedTable] = fakeTest{file: "tests/test_discount.py", setter: tableLoads, gated: []int{2, 3, 4}}
	report := run(t, f, request())

	t.Run("a test that owns lines only once another test ran is recorded alone and makes no claim on them"+verifiesConfirmation, func(t *testing.T) {
		if proven, _ := provenOn(report, []int{2, 3, 4}, ""); proven < vacuity.MinChanges {
			t.Fatalf("%d proven mutants on the lines the test owns beside its setter", proven)
		}
		test := scanTestNamed(report.Statement.Predicate.Tests, loadedTable)
		if test.Class != format.ScanUnproven || test.Reason != format.ScanTooFewChanges || len(test.Changes) != 0 || !slices.Contains(f.suite.alone, loadedTable) {
			t.Fatalf("%+v, recorded alone %v", test, f.suite.alone)
		}
		if _, found := findingOf(report, loadedTable); found {
			t.Fatalf("findings %+v", report.Findings)
		}
	})
	t.Run("a claim the test upholds alone stands, and no other verdict moves"+verifiesConfirmation, func(t *testing.T) {
		if classOf(report, vacuousTest) != format.ScanVerifiesNothing || classOf(report, exactTest) != format.ScanCaught || classOf(report, feeTest) != format.ScanCaught {
			t.Fatalf("tests %+v", report.Statement.Predicate.Tests)
		}
	})
	t.Run("only the tests that verify nothing are recorded alone"+verifiesConfirmation, func(t *testing.T) {
		for _, test := range f.suite.alone {
			if test != vacuousTest && test != loadedTable {
				t.Fatalf("recorded alone %v", f.suite.alone)
			}
		}
	})
}

func TestConfirmationPrintsTheConfirmedCount(t *testing.T) {
	f := newFixture()
	f.suite.oneRun = true
	f.suite.tests[tableLoads] = fakeTest{file: "tests/test_discount.py"}
	vacuous := f.suite.tests[vacuousTest]
	vacuous.setter, vacuous.gated = tableLoads, []int{4}
	f.suite.tests[vacuousTest] = vacuous
	report := run(t, f, request())

	t.Run("a claim drops the changes on lines the test owns only beside another test, and counts the rest"+verifiesConfirmation, func(t *testing.T) {
		if _, killed := provenOn(report, []int{4}, exactTest); killed == 0 {
			t.Fatal("no proven change on the line the test owns beside its setter")
		}
		test := scanTestNamed(report.Statement.Predicate.Tests, vacuousTest)
		byID := map[string]format.ScanMutant{}
		for _, mutant := range report.Statement.Predicate.Mutants {
			byID[mutant.ID] = mutant
		}
		for _, change := range test.Changes {
			if mutatesOneOf(byID[change.Mutant], []int{4}) {
				t.Fatalf("%s cites %s on the line it owns only beside its setter", vacuousTest, change.Mutant)
			}
		}
		finding, found := findingOf(report, vacuousTest)
		if test.Class != format.ScanVerifiesNothing || !found || finding.Changes != len(test.Changes) || finding.Changes < vacuity.MinChanges {
			t.Fatalf("%+v, finding %+v", test, finding)
		}
	})
}

func TestConfirmationRunsATestAlone(t *testing.T) {
	f := newFixture()
	f.suite.tests[seedsTable] = fakeTest{file: "tests/test_discount.py", lines: []int{2, 3, 4}}
	f.suite.tests[readsTable] = fakeTest{file: "tests/test_discount.py", lines: []int{2, 4}, setter: seedsTable, checksUnset: []int{2, 4}}
	report := run(t, f, request())

	t.Run("a test that passes a change only beside the test that set its state is unproven"+verifiesConfirmation, func(t *testing.T) {
		together := slices.ContainsFunc(f.suite.selections, func(selection []ecosystem.TestID) bool {
			return slices.Contains(selection, seedsTable) && slices.Contains(selection, readsTable)
		})
		alone := slices.ContainsFunc(f.suite.selections, func(selection []ecosystem.TestID) bool {
			return slices.Equal(selection, []ecosystem.TestID{readsTable})
		})
		test := scanTestNamed(report.Statement.Predicate.Tests, readsTable)
		if !together || !alone || test.Class != format.ScanUnproven || test.Reason != format.ScanInconclusiveOutcome {
			t.Fatalf("together %v, alone %v, %+v", together, alone, test)
		}
		if !slices.ContainsFunc(test.Changes, func(change format.ScanChange) bool { return change.Outcome == format.ScanInconclusive }) {
			t.Fatalf("changes %+v", test.Changes)
		}
	})
	t.Run("the test that set the state keeps its claim"+verifiesConfirmation, func(t *testing.T) {
		if classOf(report, seedsTable) != format.ScanVerifiesNothing {
			t.Fatalf("%+v", scanTestNamed(report.Statement.Predicate.Tests, seedsTable))
		}
	})
	t.Run("a selection that holds the test without the test that sets its state holds it alone"+verifiesConfirmation, func(t *testing.T) {
		for _, selection := range f.suite.selections {
			if slices.Contains(selection, readsTable) && len(selection) > 1 && !slices.Contains(selection, seedsTable) {
				t.Fatalf("selection %v", selection)
			}
		}
	})
}

func TestConfirmationOfKillers(t *testing.T) {
	f := newFixture()
	defaults := fakeTests()
	f.suite.tests = map[ecosystem.TestID]fakeTest{
		vacuousTest: defaults[vacuousTest],
		exactTest:   defaults[exactTest],
		ratesLoad:   {file: "tests/test_shipping.py", lines: []int{8}},
		loadedRates: {file: "tests/test_shipping.py", lines: []int{8, 9, 10}, setter: ratesLoad, failsUnset: true},
		anyTotal:    {file: "tests/test_shipping.py", lines: []int{8, 9, 10}},
	}
	var onReference [][]ecosystem.TestID
	var mutex sync.Mutex
	f.suite.onSelection = func(dir string, tests []ecosystem.TestID) {
		mutex.Lock()
		defer mutex.Unlock()
		if dir == referenceDir {
			onReference = append(onReference, tests)
		}
	}
	report := run(t, f, request())

	t.Run("a change only a killer that fails alone proves leaves the test's changes"+verifiesConfirmation, func(t *testing.T) {
		if _, killed := provenOn(report, []int{9, 10}, loadedRates); killed < vacuity.MinChanges {
			t.Fatalf("%d changes %s kills on the lines %s owns", killed, loadedRates, anyTotal)
		}
		test := scanTestNamed(report.Statement.Predicate.Tests, anyTotal)
		if test.Class != format.ScanUnproven || test.Reason != format.ScanTooFewChanges || len(test.Changes) != 0 {
			t.Fatalf("%+v", test)
		}
		if !slices.ContainsFunc(onReference, func(tests []ecosystem.TestID) bool { return slices.Equal(tests, []ecosystem.TestID{loadedRates}) }) {
			t.Fatalf("runs on the reference %v", onReference)
		}
	})
	t.Run("the killer, its kills and the routes they give stand"+verifiesConfirmation, func(t *testing.T) {
		if classOf(report, loadedRates) != format.ScanCaught || report.Mutants.Proven == 0 {
			t.Fatalf("%+v, mutants %+v", scanTestNamed(report.Statement.Predicate.Tests, loadedRates), report.Mutants)
		}
	})
	t.Run("a change a killer upholds alone keeps its place"+verifiesConfirmation, func(t *testing.T) {
		if _, killed := provenOn(report, []int{2}, exactTest); killed == 0 || classOf(report, vacuousTest) != format.ScanVerifiesNothing {
			t.Fatalf("%+v", scanTestNamed(report.Statement.Predicate.Tests, vacuousTest))
		}
	})
}

func TestConfirmationPastTheBudget(t *testing.T) {
	f := newFixture()
	req := request()
	req.Budget = confirmationBudget
	deadline := f.clock.peek().Add(f.clock.step + confirmationBudget)
	var afterDeadline atomic.Int32
	f.suite.onSelection = func(dir string, tests []ecosystem.TestID) {
		if dir == referenceDir {
			f.clock.jump(pastBudget)
		}
		if f.clock.peek().After(deadline) && slices.Equal(tests, []ecosystem.TestID{vacuousTest}) {
			afterDeadline.Add(1)
		}
	}
	report := run(t, f, req)

	t.Run("a claim is confirmed once the budget has passed, as its own phase"+verifiesConfirmation, func(t *testing.T) {
		if report.Partial || afterDeadline.Load() == 0 || classOf(report, vacuousTest) != format.ScanVerifiesNothing || report.Phases.Confirmation <= 0 {
			t.Fatalf("partial %v, %d runs past the budget, %+v, phases %+v", report.Partial, afterDeadline.Load(), scanTestNamed(report.Statement.Predicate.Tests, vacuousTest), report.Phases)
		}
	})
}

func TestConfirmationThatFails(t *testing.T) {
	f := newFixture()
	f.suite.oneRun, f.suite.aloneFails = true, true
	report := run(t, f, request())

	t.Run("a test whose recording alone fails is unproven, each of its changes inconclusive"+verifiesConfirmation, func(t *testing.T) {
		test := scanTestNamed(report.Statement.Predicate.Tests, vacuousTest)
		if test.Class != format.ScanUnproven || test.Reason != format.ScanInconclusiveOutcome || len(test.Changes) == 0 || len(report.Findings) != 0 {
			t.Fatalf("%+v, findings %+v", test, report.Findings)
		}
		for _, change := range test.Changes {
			if change.Outcome != format.ScanInconclusive {
				t.Fatalf("changes %+v", test.Changes)
			}
		}
	})
}

func TestReplayRunsEachTestAlone(t *testing.T) {
	f := newFixture()
	report := run(t, f, request())
	record := scan.RecordOf(report, report.Findings[0])
	record.Route = format.ScanRoute{Kind: format.ScanRouteKilledByTest, Test: exactTest}
	before := len(f.suite.selections)
	if _, err := f.service(t).Replay(context.Background(), "/work/invoicing", record); err != nil {
		t.Fatal(err)
	}

	t.Run("the test and its killer each run in a selection of their own"+verifiesReplay, func(t *testing.T) {
		replayed := f.suite.selections[before:]
		for _, test := range []ecosystem.TestID{vacuousTest, exactTest} {
			if !slices.ContainsFunc(replayed, func(selection []ecosystem.TestID) bool { return slices.Equal(selection, []ecosystem.TestID{test}) }) {
				t.Fatalf("selections %v", replayed)
			}
		}
		for _, selection := range replayed {
			if len(selection) != 1 {
				t.Fatalf("selection %v", selection)
			}
		}
	})
}
