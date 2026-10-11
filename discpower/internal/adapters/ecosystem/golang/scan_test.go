package golang_test

import (
	"context"
	"os/exec"
	"path/filepath"
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
	verifiesScanRuns  = " [verifies SP-2102~1]"
	verifiesLines     = " [verifies SP-2101~1]"
	verifiesBinding   = " [verifies SP-2103~1]"
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
	prepared, err := manager.Prepare(ctx, filepath.Clean(scanFixture), strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Fatal(err)
	}
	return scanProject{manager: manager, suite: strategy.Scan(), workspace: prepared, toolchain: toolchain, layout: strategy.Layout()}
}

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

func TestScanLineCoverage(t *testing.T) {
	project := openScanProject(t)
	covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, []ports.ScanCase{
		{ID: acceptsAnyPercent, File: "discount_test.go"}, {ID: takesPercentOff, File: "discount_test.go"}, {ID: chargedBelowFifty, File: "tally_test.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ran := func(test ecosystem.TestID, line int) bool {
		return slices.Contains(covered[test], vacuity.Line{File: "tally.go", Line: line})
	}
	t.Run("a test that returns early ran its branches and not the arithmetic line"+verifiesLines, func(t *testing.T) {
		if !ran(acceptsAnyPercent, 20) || !ran(acceptsAnyPercent, 23) || ran(acceptsAnyPercent, 25) || ran(acceptsAnyPercent, 13) {
			t.Fatalf("lines = %v", covered[acceptsAnyPercent])
		}
	})
	t.Run("a test of the computed discount ran the arithmetic line and no early return"+verifiesLines, func(t *testing.T) {
		if !ran(takesPercentOff, 25) || ran(takesPercentOff, 20) || ran(takesPercentOff, 23) {
			t.Fatalf("lines = %v", covered[takesPercentOff])
		}
	})
	t.Run("a subtest ran only the lines of its own case and never a line of a test file"+verifiesLines, func(t *testing.T) {
		if !ran(chargedBelowFifty, 33) || ran(chargedBelowFifty, 31) {
			t.Fatalf("lines = %v", covered[chargedBelowFifty])
		}
		for _, line := range covered[chargedBelowFifty] {
			if line.File != "tally.go" {
				t.Fatalf("line of %s", line.File)
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
