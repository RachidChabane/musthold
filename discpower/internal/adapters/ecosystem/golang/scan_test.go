package golang_test

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	scanFixture       = "../../../../fixtures/scan/go"
	takesPercentOff   = "discount_test.go::TestDiscountTakesThePercentOff"
	acceptsAnyPercent = "discount_test.go::TestDiscountAcceptsAnyPercent"
	chargedBelowFifty = "tally_test.go::TestShipping/charged_below_fifty"
	scanRunTimeout    = 3 * time.Minute
	verifiesScanRuns  = " [verifies SP-2102~4]"
	verifiesLines     = " [verifies SP-2101~4]"
	verifiesBinding   = " [verifies SP-2103~1]"
	verifiesEvents    = " [verifies SP-2128~1]"
	verifiesWorkers   = " [verifies SP-2131~2]"
	// coverageWorkers runs the tests under coverage two at a time.
	coverageWorkers = 2
	verifiesBound   = " [verifies SP-2127~1]"
	// overrunBound is a coverage bound no run can keep.
	overrunBound = time.Nanosecond
)

type scanProject struct {
	manager   *workspace.Manager
	suite     ports.ScanSuite
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
	layout    ecosystem.Layout
}

func openScanProject(t *testing.T) scanProject {
	t.Helper()
	return openScanModule(t, scanFixture)
}

// openScanModule prepares a workspace of the Go module in the directory.
func openScanModule(t *testing.T, dir string) scanProject {
	t.Helper()
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go command: %v", err)
	}
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := golang.New(golang.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: goparser.NewLocator()})
	ctx := context.Background()
	prepared, err := manager.Prepare(ctx, filepath.Clean(dir), strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	return scanProject{manager: manager, suite: strategy.Scan(), workspace: prepared, toolchain: toolchain, layout: strategy.Layout()}
}

// noisyTest logs more than a process keeps of a standard output it captures.
const noisyTest = `package tally

import (
	"strings"
	"testing"
)

func TestNoisy(t *testing.T) {
	line := strings.Repeat("x", 1000)
	for range 6000 {
		t.Log(line)
	}
}
`

func runsOf(run vacuity.Run, count int) []vacuity.Run {
	return slices.Repeat([]vacuity.Run{run}, count)
}

func TestScanRuns(t *testing.T) {
	project := openScanProject(t)
	ctx := context.Background()
	whole, err := project.suite.Run(ctx, project.workspace, project.toolchain, ports.ScanRunRequest{Runs: 2, Timeout: scanRunTimeout})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a run of the whole suite lists the leaf tests by file and subtest path, never a parent"+verifiesScanRuns, func(t *testing.T) {
		want := []ports.ScanCase{
			{ID: acceptsAnyPercent, File: "discount_test.go"}, {ID: takesPercentOff, File: "discount_test.go"},
			{ID: "tally_test.go::TestLineTotalMultipliesPriceByQuantity", File: "tally_test.go"},
			{ID: chargedBelowFifty, File: "tally_test.go"}, {ID: "tally_test.go::TestShipping/waived_from_fifty", File: "tally_test.go"},
		}
		if !slices.Equal(whole.Cases, want) {
			t.Fatalf("cases = %v", whole.Cases)
		}
		for _, scanCase := range whole.Cases {
			if runs := whole.Runs[scanCase.ID]; !slices.Equal(runs, runsOf(vacuity.RunPassed, 2)) {
				t.Errorf("%s: %v", scanCase.ID, runs)
			}
		}
	})
	t.Run("a selected subtest runs alone the requested number of times"+verifiesScanRuns, func(t *testing.T) {
		ran, err := project.suite.Run(ctx, project.workspace, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{chargedBelowFifty}, Runs: 3, Timeout: scanRunTimeout})
		if err != nil || len(ran.Runs) != 1 || !slices.Equal(ran.Runs[chargedBelowFifty], runsOf(vacuity.RunPassed, 3)) {
			t.Fatalf("runs = %v, %v", ran.Runs, err)
		}
	})
	t.Run("every test is read however much the tests log"+verifiesEvents, func(t *testing.T) {
		noisy, err := project.manager.Clone(ctx, project.workspace, project.layout)
		if err != nil {
			t.Fatal(err)
		}
		if err := project.manager.WriteFile(noisy, project.layout, "noisy_test.go", []byte(noisyTest)); err != nil {
			t.Fatal(err)
		}
		ran, err := project.suite.Run(ctx, noisy, project.toolchain, ports.ScanRunRequest{Runs: 1, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		for _, test := range []ecosystem.TestID{"noisy_test.go::TestNoisy", "tally_test.go::TestShipping/waived_from_fifty"} {
			if !slices.Equal(ran.Runs[test], runsOf(vacuity.RunPassed, 1)) {
				t.Fatalf("%s: %v", test, ran.Runs[test])
			}
		}
	})
	t.Run("a failing test fails each run and a test no run reported is errored"+verifiesScanRuns, func(t *testing.T) {
		mutant, err := project.manager.Clone(ctx, project.workspace, project.layout)
		if err != nil {
			t.Fatal(err)
		}
		source := "package tally\n\nconst FullPercent = 100\n\nfunc LineTotal(unitPrice, quantity int) int { return unitPrice * quantity }\n\nfunc Discount(total, percent int) int { return total }\n\nfunc Shipping(total int) int { return 0 }\n"
		if err := project.manager.WriteFile(mutant, project.layout, "tally.go", []byte(source)); err != nil {
			t.Fatal(err)
		}
		missing := ecosystem.TestID("discount_test.go::TestRemoved")
		ran, err := project.suite.Run(ctx, mutant, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{takesPercentOff, acceptsAnyPercent, missing}, Runs: 3, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ran.Runs[takesPercentOff], runsOf(vacuity.RunFailed, 3)) || !slices.Equal(ran.Runs[acceptsAnyPercent], runsOf(vacuity.RunPassed, 3)) ||
			!slices.Equal(ran.Runs[missing], runsOf(vacuity.RunErrored, 3)) {
			t.Fatalf("runs = %v", ran.Runs)
		}
	})
}

func TestScanCoverageOfOneRun(t *testing.T) {
	t.Run("a Go test runs alone under coverage"+verifiesWorkers, func(t *testing.T) {
		if openScanProject(t).suite.CoverageOfOneRun() {
			t.Fatal("go records every test in one run")
		}
	})
}

func TestScanLineCoverageBound(t *testing.T) {
	t.Run("a test run under coverage past its bound is a coverage timeout"+verifiesBound, func(t *testing.T) {
		project := openScanProject(t)
		_, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
			Tests:   []ports.ScanCase{{ID: acceptsAnyPercent, File: "discount_test.go"}},
			Timeout: overrunBound,
		})
		if !errors.Is(err, ports.ErrCoverageTimedOut) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestScanLineCoverage(t *testing.T) {
	project := openScanProject(t)
	covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
		Tests: []ports.ScanCase{
			{ID: acceptsAnyPercent, File: "discount_test.go"}, {ID: takesPercentOff, File: "discount_test.go"}, {ID: chargedBelowFifty, File: "tally_test.go"},
		},
		Timeout: scanRunTimeout,
		Workers: coverageWorkers,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("the tests run under coverage on the workers each give their own lines"+verifiesWorkers, func(t *testing.T) {
		if len(covered) != 3 || len(covered[acceptsAnyPercent].Lines) == 0 || len(covered[chargedBelowFifty].Lines) == 0 {
			t.Fatalf("covered %v", covered)
		}
	})
	ran := func(test ecosystem.TestID, line int) bool {
		return slices.Contains(covered[test].Lines, vacuity.Line{File: "tally.go", Line: line})
	}
	t.Run("a test that returns early ran its branches and not the arithmetic line"+verifiesLines, func(t *testing.T) {
		if !ran(acceptsAnyPercent, 20) || !ran(acceptsAnyPercent, 23) || ran(acceptsAnyPercent, 25) || ran(acceptsAnyPercent, 13) {
			t.Fatalf("lines = %v", covered[acceptsAnyPercent].Lines)
		}
	})
	t.Run("a test of the computed discount ran the arithmetic line and no early return"+verifiesLines, func(t *testing.T) {
		if !ran(takesPercentOff, 25) || ran(takesPercentOff, 20) || ran(takesPercentOff, 23) {
			t.Fatalf("lines = %v", covered[takesPercentOff].Lines)
		}
	})
	t.Run("a subtest ran only the lines of its own case and never a line of a test file"+verifiesLines, func(t *testing.T) {
		if !ran(chargedBelowFifty, 33) || ran(chargedBelowFifty, 31) {
			t.Fatalf("lines = %v", covered[chargedBelowFifty].Lines)
		}
		for _, line := range covered[chargedBelowFifty].Lines {
			if line.File != "tally.go" {
				t.Fatalf("line of %s", line.File)
			}
		}
	})
}

func TestScanLineCoverageOfPackages(t *testing.T) {
	project := openScanModule(t, ratesModule)
	tests := []ports.ScanCase{
		{ID: "billing/billing_test.go::TestTotalAddsTheStandardRate", File: "billing/billing_test.go"},
		{ID: "ledger/ledger_test.go::TestPostSumsTheEntries", File: "ledger/ledger_test.go"},
		{ID: "quote/quote_test.go::TestRateQuotesTheBounds", File: "quote/quote_test.go"},
		{ID: "quote/quote_test.go::TestRateRefusesOutOfRange", File: "quote/quote_test.go"},
	}
	coverageOn := func(workers int) map[ecosystem.TestID]vacuity.Coverage {
		covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{Tests: tests, Timeout: scanRunTimeout, Workers: workers})
		if err != nil {
			t.Fatal(err)
		}
		return covered
	}
	one, several := coverageOn(1), coverageOn(len(tests))
	t.Run("the coverage binaries of several packages built on the workers give each test the lines one worker gives"+verifiesWorkers, func(t *testing.T) {
		if !reflect.DeepEqual(one, several) || len(several) != len(tests) {
			t.Fatalf("one worker %v, several %v", one, several)
		}
		for _, test := range tests {
			if len(several[test.ID].Owned) == 0 {
				t.Fatalf("%s owns no line: %v", test.ID, several[test.ID])
			}
		}
	})
}

func TestScanBinding(t *testing.T) {
	project := openScanProject(t)
	cases := []struct {
		file, qualname, target string
	}{
		{"tally.go", "tally.Discount", "example.com/tally:Discount"},
		{"tally.go", "tally.discount", ""},
		{"tally.go", "(*Ledger).Add", ""},
		{"tally.go", "Ledger.Add", ""},
		{"tally_test.go", "tally.TestShipping", ""},
	}
	for _, c := range cases {
		t.Run(c.file+" "+c.qualname+verifiesBinding, func(t *testing.T) {
			target, bound := project.suite.Operation(project.workspace, c.file, c.qualname)
			if target != c.target || bound != (c.target != "") {
				t.Fatalf("target %q, bound %v", target, bound)
			}
		})
	}
	t.Run("a test is reproduced by go test anchored to its subtest path"+verifiesBinding, func(t *testing.T) {
		want := []string{"go", "test", "-count=1", "-run", "^TestShipping$/^charged_below_fifty$", "."}
		if got := project.suite.Repro(chargedBelowFifty); !slices.Equal(got, want) {
			t.Fatalf("repro = %v", got)
		}
	})
}
