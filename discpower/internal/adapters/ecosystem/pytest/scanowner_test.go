package pytest_test

import (
	"context"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	ledgerFixture       = "../../../../fixtures/scan/pytest-ledger"
	ledgerTests         = "tests/test_ledger.py"
	chargesTests        = "tests/test_charges.py"
	tickerTests         = "tests/test_ticker.py"
	invoicingTests      = "tests/billing/test_invoicing.py"
	postAddsTheAmount   = ecosystem.TestID(ledgerTests + "::test_post_adds_the_amount")
	dayBookStartsEmpty  = ecosystem.TestID(ledgerTests + "::test_day_book_starts_empty")
	ledgerIsNamed       = ecosystem.TestID(ledgerTests + "::test_ledger_is_named")
	feeOfStandard       = ecosystem.TestID(chargesTests + "::test_fee_of_standard")
	farBeyondAHundred   = ecosystem.TestID(chargesTests + "::test_far_beyond_a_hundred")
	loyaltyTakesATenth  = ecosystem.TestID(chargesTests + "::test_loyalty_takes_a_tenth")
	tickerKeepsItsBook  = ecosystem.TestID(tickerTests + "::test_ticker_keeps_its_own_book")
	expressCostsMore    = ecosystem.TestID(invoicingTests + "::test_express_costs_more")
	verifiesOwnedLines  = " [verifies SP-2100~3]"
	verifiesSessionRuns = " [verifies SP-2142~1]"
	verifiesSelection   = " [verifies SP-2102~4]"
)

// The lines of the ledger, the rates and the zones of the fixture.
var (
	ledgerOpens   = vacuity.Line{File: "invoice/ledger.py", Line: 3}
	ledgerPosts   = vacuity.Line{File: "invoice/ledger.py", Line: 7}
	ledgerCloses  = vacuity.Line{File: "invoice/ledger.py", Line: 11}
	loyaltyBase   = vacuity.Line{File: "invoice/loyalty.py", Line: 1}
	loyaltyShare  = vacuity.Line{File: "invoice/loyalty.py", Line: 5}
	zonesTable    = vacuity.Line{File: "invoice/zones.py", Line: 1}
	zoneCondition = vacuity.Line{File: "invoice/zones.py", Line: 5}
	zoneFar       = vacuity.Line{File: "invoice/zones.py", Line: 6}
)

func openLedgerProject(t *testing.T) scanProject {
	t.Helper()
	return openScanFixture(t, ledgerFixture)
}

func TestScanLineCoverageOwnership(t *testing.T) {
	for _, tracer := range []string{"", settraceTracer} {
		name := "sys.monitoring"
		if tracer != "" {
			name = tracer
			t.Setenv(tracerVariable, tracer)
		}
		t.Run(name, func(t *testing.T) {
			project := openLedgerProject(t)
			var cases []ports.ScanCase
			for _, id := range []ecosystem.TestID{postAddsTheAmount, dayBookStartsEmpty, ledgerIsNamed, feeOfStandard, loyaltyTakesATenth, farBeyondAHundred, tickerKeepsItsBook} {
				cases = append(cases, ports.ScanCase{ID: id})
			}
			covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{Tests: cases, Timeout: scanRunTimeout})
			if err != nil {
				t.Fatal(err)
			}
			ranNotOwned := func(t *testing.T, test ecosystem.TestID, lines ...vacuity.Line) {
				t.Helper()
				for _, line := range lines {
					if !slices.Contains(covered[test].Lines, line) || slices.Contains(covered[test].Owned, line) {
						t.Fatalf("%s, %v: %+v", test, line, covered[test])
					}
				}
			}
			owned := func(t *testing.T, test ecosystem.TestID, lines ...vacuity.Line) {
				t.Helper()
				for _, line := range lines {
					if !slices.Contains(covered[test].Owned, line) {
						t.Fatalf("%s does not own %v: %+v", test, line, covered[test])
					}
				}
			}
			t.Run("the setup of a module fixture is run and not owned by the test it is set up for, which owns its own call"+verifiesOwnedLines, func(t *testing.T) {
				ranNotOwned(t, postAddsTheAmount, ledgerOpens)
				owned(t, postAddsTheAmount, ledgerPosts)
			})
			t.Run("the teardown of a module fixture is run and not owned by the test that never asked for it"+verifiesOwnedLines, func(t *testing.T) {
				ranNotOwned(t, ledgerIsNamed, ledgerCloses)
			})
			t.Run("a test owns the setup of its own fixture and not its teardown"+verifiesOwnedLines, func(t *testing.T) {
				owned(t, dayBookStartsEmpty, ledgerOpens)
				ranNotOwned(t, dayBookStartsEmpty, ledgerCloses)
			})
			t.Run("a module a test imports is run and not owned, while the call after the import is"+verifiesOwnedLines, func(t *testing.T) {
				ranNotOwned(t, loyaltyTakesATenth, loyaltyBase)
				owned(t, loyaltyTakesATenth, loyaltyShare)
			})
			t.Run("a project module a third-party module imports is not owned, while the calls after that import are"+verifiesOwnedLines, func(t *testing.T) {
				ranNotOwned(t, farBeyondAHundred, zonesTable)
				owned(t, farBeyondAHundred, zoneCondition, zoneFar)
			})
			t.Run("a line a thread of an earlier test runs is not owned"+verifiesOwnedLines, func(t *testing.T) {
				owned(t, tickerKeepsItsBook, ledgerOpens)
				if slices.Contains(covered[tickerKeepsItsBook].Owned, ledgerPosts) {
					t.Fatalf("owned = %v", covered[tickerKeepsItsBook].Owned)
				}
			})
		})
	}
}

func TestScanRunsOfABatchPastABrokenModule(t *testing.T) {
	project := openLedgerProject(t)
	ctx := context.Background()
	run := func(t *testing.T, file, content string, tests ...ecosystem.TestID) map[ecosystem.TestID][]vacuity.Run {
		t.Helper()
		on := project.workspace
		if file != "" {
			changed, err := project.manager.Clone(ctx, project.workspace, project.layout)
			if err != nil {
				t.Fatal(err)
			}
			if err := project.manager.WriteFile(changed, project.layout, file, []byte(content)); err != nil {
				t.Fatal(err)
			}
			on = changed
		}
		ran, err := project.suite.Run(ctx, on, project.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		return ran.Runs
	}
	passed := runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)
	notStarted := runsOf(vacuity.RunNotStarted, vacuity.RunsPerOutcome)

	t.Run("a test module that does not import leaves its batch-mates running"+verifiesSelection, func(t *testing.T) {
		runs := run(t, chargesTests, "from invoice.rates import fee_of_kind\n\n\ndef test_fee_of_standard():\n    assert fee_of_kind(\"standard\") == 5\n", feeOfStandard, ledgerIsNamed)
		if !slices.Equal(runs[feeOfStandard], notStarted) || !slices.Equal(runs[ledgerIsNamed], passed) {
			t.Fatalf("runs = %v", runs)
		}
	})
	t.Run("a nested conftest that raises leaves the tests outside its directory running"+verifiesSessionRuns, func(t *testing.T) {
		runs := run(t, "tests/billing/conftest.py", "import os\n\nCURRENCY = os.environ[\"BILLING_CURRENCY_THAT_IS_NOT_SET\"]\n", expressCostsMore, feeOfStandard)
		if !slices.Equal(runs[expressCostsMore], notStarted) || !slices.Equal(runs[feeOfStandard], passed) {
			t.Fatalf("runs = %v", runs)
		}
	})
	t.Run("a test absent from a module that collected errored, and its batch-mates run"+verifiesSessionRuns, func(t *testing.T) {
		absent := ecosystem.TestID(chargesTests + "::test_fee_of_express")
		runs := run(t, "", "", absent, feeOfStandard)
		if !slices.Equal(runs[absent], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) || !slices.Equal(runs[feeOfStandard], passed) {
			t.Fatalf("runs = %v", runs)
		}
	})
	t.Run("a failure does not stop the session, whatever the project's options say"+verifiesSelection, func(t *testing.T) {
		runs := run(t, "invoice/rates.py", "FEES = {\"standard\": 6, \"express\": 9}\n\n\ndef fee_of(kind):\n    return FEES[kind]\n", feeOfStandard, expressCostsMore)
		if !slices.Equal(runs[feeOfStandard], runsOf(vacuity.RunFailed, vacuity.RunsPerOutcome)) || !slices.Equal(runs[expressCostsMore], passed) {
			t.Fatalf("runs = %v", runs)
		}
	})
	t.Run("a session of selected tests runs only them"+verifiesSelection, func(t *testing.T) {
		ran, err := project.suite.Run(ctx, project.workspace, project.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{feeOfStandard}, Runs: 1, Timeout: scanRunTimeout})
		if err != nil || len(ran.Runs) != 1 || !slices.Equal(ran.Runs[feeOfStandard], []vacuity.Run{vacuity.RunPassed}) {
			t.Fatalf("runs = %v, %v", ran.Runs, err)
		}
	})
}
