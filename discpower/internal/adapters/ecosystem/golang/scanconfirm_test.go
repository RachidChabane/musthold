package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

// The quotes module: a test that seeds the price cache and prices other skus,
// and a test that reads the price it seeded.
const (
	quotesFixture       = "../../../../fixtures/scan/go-quotes"
	quotesSource        = "quotes.go"
	quotesAcceptAnySku  = ecosystem.TestID("quotes_test.go::TestQuotesAcceptAnySku")
	premiumRangePrice   = ecosystem.TestID("quotes_test.go::TestPriceOfThePremiumRange")
	premiumAdded        = "base += Premium"
	premiumTakenOff     = "base -= Premium"
	verifiesConfirmGo   = " [verifies SP-2144~1]"
	quotesCoverageTests = 2
)

func TestScanSelectionsOfAnOrderDependentPair(t *testing.T) {
	project := openScanModule(t, quotesFixture)
	ctx := context.Background()
	mutant, err := project.manager.Clone(ctx, project.workspace, project.layout)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(mutant.Dir, quotesSource))
	if err != nil {
		t.Fatal(err)
	}
	changed := strings.Replace(string(source), premiumAdded, premiumTakenOff, 1)
	if err := project.manager.WriteFile(mutant, project.layout, quotesSource, []byte(changed)); err != nil {
		t.Fatal(err)
	}
	runs := func(t *testing.T, tests ...ecosystem.TestID) []vacuity.Run {
		t.Helper()
		ran, err := project.suite.Run(ctx, mutant, project.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		return ran.Runs[premiumRangePrice]
	}

	t.Run("a test passes a change in a selection with the test that seeds its state and fails it alone"+verifiesConfirmGo, func(t *testing.T) {
		if together := runs(t, quotesAcceptAnySku, premiumRangePrice); !slices.Equal(together, runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)) {
			t.Fatalf("together: %v", together)
		}
		if alone := runs(t, premiumRangePrice); !slices.Equal(alone, runsOf(vacuity.RunFailed, vacuity.RunsPerOutcome)) {
			t.Fatalf("alone: %v", alone)
		}
	})
	t.Run("a recording of each test alone is the recording of the scan"+verifiesConfirmGo, func(t *testing.T) {
		cases := []ports.ScanCase{{ID: quotesAcceptAnySku, File: "quotes_test.go"}, {ID: premiumRangePrice, File: "quotes_test.go"}}
		request := ports.ScanCoverageRequest{Tests: cases, Timeout: scanRunTimeout, Workers: coverageWorkers}
		recorded, err := project.suite.LineCoverage(ctx, project.workspace, project.toolchain, request)
		if err != nil {
			t.Fatal(err)
		}
		request.Alone = true
		alone, err := project.suite.LineCoverage(ctx, project.workspace, project.toolchain, request)
		if err != nil {
			t.Fatal(err)
		}
		if len(recorded) != quotesCoverageTests || len(recorded[premiumRangePrice].Owned) == 0 || !reflect.DeepEqual(alone, recorded) {
			t.Fatalf("recorded %+v, alone %+v", recorded, alone)
		}
	})
}
