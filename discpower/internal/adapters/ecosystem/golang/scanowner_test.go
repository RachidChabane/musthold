package golang_test

import (
	"context"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

const (
	tariffTests       = "tariff/tariff_test.go"
	tariffFile        = "tariff/tariff.go"
	priceNear         = ecosystem.TestID(tariffTests + "::TestPrice/near")
	priceFar          = ecosystem.TestID(tariffTests + "::TestPrice/far")
	surchargeSeed     = ecosystem.TestID(tariffTests + "::FuzzSurcharge/seed#0")
	zonesNear         = ecosystem.TestID(tariffTests + "::TestZones/near")
	quotesNear        = ecosystem.TestID(tariffTests + "::TestQuotes/near")
	quotesFar         = ecosystem.TestID(tariffTests + "::TestQuotes/far")
	samplesAreWarm    = ecosystem.TestID(tariffTests + "::TestSamplesAreWarm")
	verifiesBaselines = " [verifies SP-2101~3]"
	verifiesAncestry  = " [verifies SP-2141~1]"
	// verifiesSelectionRuns names the requirement of how a scan selects tests.
	verifiesSelectionRuns = " [verifies SP-2102~4]"
)

// The lines of tariff.go and samples.go: the body of Standard, the line of
// Price that reads the table, the body of Surcharge and the body of Sample.
var (
	standardBody  = vacuity.Line{File: tariffFile, Line: 8}
	priceLookup   = vacuity.Line{File: tariffFile, Line: 13}
	surchargeBody = vacuity.Line{File: tariffFile, Line: 22}
	sampleBody    = vacuity.Line{File: "tariff/samples.go", Line: 21}
)

// farPricedHigher prices the far zone above the standard table.
const farPricedHigher = `package tariff

// Table holds the price of each zone.
type Table map[string]int

// Standard is the table every tariff starts from.
func Standard() Table {
	return Table{"near": 3, "far": 9}
}

// Price is the price of a zone in the table, zero for an unknown zone.
func Price(table Table, zone string) int {
	price, known := table[zone]
	if !known {
		return 0
	}
	return price
}

// Surcharge adds the handling fee to a price.
func Surcharge(price int) int {
	return price + 2
}
`

func tariffCases(ids ...ecosystem.TestID) []ports.ScanCase {
	cases := make([]ports.ScanCase, 0, len(ids))
	for _, id := range ids {
		cases = append(cases, ports.ScanCase{ID: id, File: tariffTests})
	}
	return cases
}

func TestScanLineCoverageAgainstBaselines(t *testing.T) {
	project := openMeter(t)
	covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
		Tests:   tariffCases(priceNear, surchargeSeed, zonesNear, samplesAreWarm),
		Timeout: scanRunTimeout,
		Workers: coverageWorkers,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a subtest runs its parent's setup and does not own it, and owns what its body runs"+verifiesBaselines, func(t *testing.T) {
		coverage := covered[priceNear]
		if !slices.Contains(coverage.Lines, standardBody) || slices.Contains(coverage.Owned, standardBody) || !slices.Contains(coverage.Owned, priceLookup) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a fuzz seed does not own the seeds its target adds, and owns what its body runs"+verifiesBaselines, func(t *testing.T) {
		coverage := covered[surchargeSeed]
		if !slices.Contains(coverage.Lines, priceLookup) || slices.Contains(coverage.Owned, priceLookup) || slices.Contains(coverage.Owned, standardBody) {
			t.Fatalf("coverage = %+v", coverage)
		}
		if !slices.Contains(coverage.Owned, surchargeBody) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
	})
	t.Run("a function a start-up goroutine runs a varying number of times is run and owned by no test"+verifiesBaselines, func(t *testing.T) {
		coverage := covered[samplesAreWarm]
		if !slices.Contains(coverage.Lines, sampleBody) || slices.Contains(coverage.Owned, sampleBody) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a subtest whose parent fails with no subtest owns no line"+verifiesBaselines, func(t *testing.T) {
		coverage := covered[zonesNear]
		if !slices.Contains(coverage.Lines, priceLookup) || coverage.Owned != nil {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
}

func TestScanRunsAgainstAncestors(t *testing.T) {
	project := openMeter(t)

	t.Run("a subtest that passes while its parent fails after it is errored"+verifiesAncestry, func(t *testing.T) {
		runs := project.run(t, project.workspace, scanRunTimeout, zonesNear)
		if !slices.Equal(runs[zonesNear], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) {
			t.Fatalf("runs = %v", runs[zonesNear])
		}
	})
	t.Run("a subtest whose sibling is killed passes once it runs again without it"+verifiesAncestry, func(t *testing.T) {
		runs := project.run(t, project.withFile(t, tariffFile, farPricedHigher), scanRunTimeout, priceNear, priceFar)
		if !slices.Equal(runs[priceNear], runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)) {
			t.Fatalf("surviving subtest: %v", runs[priceNear])
		}
		if !slices.Equal(runs[priceFar], runsOf(vacuity.RunFailed, vacuity.RunsPerOutcome)) {
			t.Fatalf("killed subtest: %v", runs[priceFar])
		}
	})
	t.Run("a subtest whose sibling is killed and whose parent checks its subtests on cleanup is errored"+verifiesAncestry, func(t *testing.T) {
		runs := project.run(t, project.withFile(t, tariffFile, farPricedHigher), scanRunTimeout, quotesNear, quotesFar)
		if !slices.Equal(runs[quotesNear], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) {
			t.Fatalf("surviving subtest: %v", runs[quotesNear])
		}
		if !slices.Equal(runs[quotesFar], runsOf(vacuity.RunFailed, vacuity.RunsPerOutcome)) {
			t.Fatalf("killed subtest: %v", runs[quotesFar])
		}
	})
	t.Run("a selection runs no subtest it does not name, so a parent checks only the named ones"+verifiesSelectionRuns, func(t *testing.T) {
		runs := project.run(t, project.workspace, scanRunTimeout, zonesNear, priceFar)
		if !slices.Equal(runs[zonesNear], runsOf(vacuity.RunErrored, vacuity.RunsPerOutcome)) {
			t.Fatalf("runs = %v", runs[zonesNear])
		}
		if !slices.Equal(runs[priceFar], runsOf(vacuity.RunPassed, vacuity.RunsPerOutcome)) {
			t.Fatalf("runs = %v", runs[priceFar])
		}
	})
}
