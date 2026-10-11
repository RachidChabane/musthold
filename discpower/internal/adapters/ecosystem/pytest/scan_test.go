package pytest_test

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	scanFixture      = "../../../../fixtures/scan/pytest"
	discountTests    = "tests/test_discount.py"
	takesPercentOff  = discountTests + "::test_discount_takes_the_percent_off"
	acceptsAnyPrice  = discountTests + "::test_discount_accepts_any_percent"
	waivedFromFifty  = "tests/test_totals.py::test_shipping_is_waived_from_fifty[50-0]"
	totalsFile       = "invoice/totals.py"
	tracerVariable   = "SPEC_PROOF_COVERAGE_TRACER"
	settraceTracer   = "settrace"
	scanRunTimeout   = 2 * time.Minute
	verifiesScanRuns = " [verifies SP-2102~2]"
	verifiesLines    = " [verifies SP-2100~1]"
	verifiesBinding  = " [verifies SP-2103~1]"
	verifiesBound    = " [verifies SP-2127~1]"
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
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := pytest.New(pytest.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	ctx := context.Background()
	prepared, err := manager.Prepare(ctx, filepath.Clean(scanFixture), strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Skipf("no python interpreter: %v", err)
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
	t.Run("a run of the whole suite lists every collected test with its file, by node id"+verifiesScanRuns, func(t *testing.T) {
		if len(whole.Cases) != 5 || !slices.Contains(whole.Cases, ports.ScanCase{ID: waivedFromFifty, File: "tests/test_totals.py"}) {
			t.Fatalf("cases = %v", whole.Cases)
		}
	})
	t.Run("each test has one result per run"+verifiesScanRuns, func(t *testing.T) {
		for _, scanCase := range whole.Cases {
			if runs := whole.Runs[scanCase.ID]; !slices.Equal(runs, runsOf(vacuity.RunPassed, 2)) {
				t.Errorf("%s: %v", scanCase.ID, runs)
			}
		}
	})

	t.Run("a failing test fails each run and a test the run never reported is errored"+verifiesScanRuns, func(t *testing.T) {
		mutant, err := project.manager.Clone(ctx, project.workspace, project.layout)
		if err != nil {
			t.Fatal(err)
		}
		source := "def discount(total: int, percent: int) -> int:\n    return total\n"
		if err := project.manager.WriteFile(mutant, project.layout, totalsFile, []byte("FREE_SHIPPING_FROM = 50\nSHIPPING_FEE = 5\n\n\ndef line_total(unit_price, quantity):\n    return unit_price * quantity\n\n\n"+source+"\n\ndef shipping(total):\n    return 0 if total >= 50 else 5\n")); err != nil {
			t.Fatal(err)
		}
		missing := ecosystem.TestID(discountTests + "::test_that_was_removed")
		ran, err := project.suite.Run(ctx, mutant, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{takesPercentOff, acceptsAnyPrice}, Runs: 3, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(ran.Runs[takesPercentOff], runsOf(vacuity.RunFailed, 3)) || !slices.Equal(ran.Runs[acceptsAnyPrice], runsOf(vacuity.RunPassed, 3)) {
			t.Fatalf("runs = %v", ran.Runs)
		}
		absent, err := project.suite.Run(ctx, mutant, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{missing}, Runs: 1, Timeout: scanRunTimeout})
		if err != nil || !slices.Equal(absent.Runs[missing], []vacuity.Run{vacuity.RunErrored}) {
			t.Fatalf("runs = %v, %v", absent.Runs, err)
		}
	})

	t.Run("a test module that does not import leaves the other tests running"+verifiesScanRuns, func(t *testing.T) {
		broken, err := project.manager.Clone(ctx, project.workspace, project.layout)
		if err != nil {
			t.Fatal(err)
		}
		if err := project.manager.WriteFile(broken, project.layout, "tests/test_ledger.py", []byte("from invoice.ledger import post\n")); err != nil {
			t.Fatal(err)
		}
		ran, err := project.suite.Run(ctx, broken, project.toolchain, ports.ScanRunRequest{Runs: 1, Timeout: scanRunTimeout})
		if err != nil || len(ran.Cases) != 5 || !slices.Equal(ran.Runs[takesPercentOff], []vacuity.Run{vacuity.RunPassed}) {
			t.Fatalf("cases %v, runs %v, %v", ran.Cases, ran.Runs, err)
		}
	})

	t.Run("a run the timeout stops leaves its tests and every later run timed out"+verifiesScanRuns, func(t *testing.T) {
		slow, err := project.manager.Clone(ctx, project.workspace, project.layout)
		if err != nil {
			t.Fatal(err)
		}
		if err := project.manager.WriteFile(slow, project.layout, discountTests, []byte("import time\n\n\ndef test_discount_takes_the_percent_off():\n    time.sleep(30)\n")); err != nil {
			t.Fatal(err)
		}
		ran, err := project.suite.Run(ctx, slow, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{takesPercentOff}, Runs: 3, Timeout: 2 * time.Second})
		if err != nil || !slices.Equal(ran.Runs[takesPercentOff], runsOf(vacuity.RunTimedOut, 3)) {
			t.Fatalf("runs = %v, %v", ran.Runs, err)
		}
	})
}

func TestScanLineCoverageBound(t *testing.T) {
	t.Run("a coverage run past its bound is a coverage timeout"+verifiesBound, func(t *testing.T) {
		project := openScanProject(t)
		_, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
			Tests:   []ports.ScanCase{{ID: acceptsAnyPrice, File: discountTests}},
			Timeout: overrunBound,
		})
		if !errors.Is(err, ports.ErrCoverageTimedOut) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestScanLineCoverage(t *testing.T) {
	for _, tracer := range []string{"", settraceTracer} {
		name := "sys.monitoring"
		if tracer != "" {
			name = tracer
			t.Setenv(tracerVariable, tracer)
		}
		t.Run(name, func(t *testing.T) {
			project := openScanProject(t)
			covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
				Tests:   []ports.ScanCase{{ID: acceptsAnyPrice, File: discountTests}, {ID: takesPercentOff, File: discountTests}},
				Timeout: scanRunTimeout,
			})
			if err != nil {
				t.Fatal(err)
			}
			lines := func(test ecosystem.TestID) []int {
				var numbers []int
				for _, line := range covered[test] {
					if line.File != totalsFile {
						t.Fatalf("%s ran a line of %s", test, line.File)
					}
					numbers = append(numbers, line.Line)
				}
				return numbers
			}
			t.Run("a test that returns early ran the lines of its branches and not the arithmetic line"+verifiesLines, func(t *testing.T) {
				if got := lines(acceptsAnyPrice); !slices.Equal(got, []int{11, 12, 13, 14}) {
					t.Fatalf("lines = %v", got)
				}
			})
			t.Run("a test of the computed discount ran the arithmetic line and not the early returns"+verifiesLines, func(t *testing.T) {
				if got := lines(takesPercentOff); !slices.Equal(got, []int{11, 13, 15}) {
					t.Fatalf("lines = %v", got)
				}
			})
		})
	}
}

func TestScanBinding(t *testing.T) {
	suite := pytest.New(pytest.Dependencies{}).Scan()
	cases := []struct {
		file, qualname, target string
	}{
		{"invoice/totals.py", "discount", "invoice.totals:discount"},
		{"src/invoice/totals.py", "discount", "invoice.totals:discount"},
		{"invoice/__init__.py", "make", "invoice:make"},
		{"invoice/totals.py", "Ledger.add", ""},
		{"invoice/totals.py", "outer.inner", ""},
		{"invoice/my-totals.py", "discount", ""},
		{"invoice/totals.go", "discount", ""},
	}
	for _, c := range cases {
		t.Run(c.file+" "+c.qualname+verifiesBinding, func(t *testing.T) {
			target, bound := suite.Operation(ecosystem.Workspace{}, c.file, c.qualname)
			if target != c.target || bound != (c.target != "") {
				t.Fatalf("target %q, bound %v", target, bound)
			}
		})
	}
	t.Run("a test is reproduced by its node id"+verifiesBinding, func(t *testing.T) {
		if got := suite.Repro(acceptsAnyPrice); !slices.Equal(got, []string{"python3", "-m", "pytest", "-p", "no:cacheprovider", string(acceptsAnyPrice)}) {
			t.Fatalf("repro = %v", got)
		}
	})
}
