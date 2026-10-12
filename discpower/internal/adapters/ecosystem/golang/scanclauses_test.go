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
	routingTests        = "routing/routing_test.go"
	routingFile         = "routing/routing.go"
	gradeIsLow          = ecosystem.TestID(routingTests + "::TestGradeIsLow")
	carryFromFirstStep  = ecosystem.TestID(routingTests + "::TestCarryFromTheFirstStep")
	bandIsHigh          = ecosystem.TestID(routingTests + "::TestBandIsHigh")
	awaitTakesReady     = ecosystem.TestID(routingTests + "::TestAwaitTakesTheReadySignal")
	verifiesClauseLines = " [verifies SP-2101~4]"
)

// The lines of routing.go: the clause headers of Grade, Carry and Await, and
// the line of Band that closes its first branch and opens its second.
var (
	gradeNegativeHeader = vacuity.Line{File: routingFile, Line: 8}
	gradeLowHeader      = vacuity.Line{File: routingFile, Line: 10}
	gradeDefaultHeader  = vacuity.Line{File: routingFile, Line: 12}
	carryFirstHeader    = vacuity.Line{File: routingFile, Line: 21}
	carrySecondHeader   = vacuity.Line{File: routingFile, Line: 24}
	bandElseIfLine      = vacuity.Line{File: routingFile, Line: 36}
	awaitReadyHeader    = vacuity.Line{File: routingFile, Line: 45}
	awaitDoneHeader     = vacuity.Line{File: routingFile, Line: 47}
)

func routingCases(ids ...ecosystem.TestID) []ports.ScanCase {
	cases := make([]ports.ScanCase, 0, len(ids))
	for _, id := range ids {
		cases = append(cases, ports.ScanCase{ID: id, File: routingTests})
	}
	return cases
}

func TestScanClauseHeaderCoverage(t *testing.T) {
	project := openMeter(t)
	covered, err := project.suite.LineCoverage(context.Background(), project.workspace, project.toolchain, ports.ScanCoverageRequest{
		Tests:   routingCases(gradeIsLow, carryFromFirstStep, bandIsHigh, awaitTakesReady),
		Timeout: scanRunTimeout,
		Workers: coverageWorkers,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Run("a header evaluated unmatched at start-up and matched in the leaf is owned, and the header that matched only at start-up is run and not owned"+verifiesClauseLines, func(t *testing.T) {
		coverage := covered[gradeIsLow]
		if !slices.Contains(coverage.Owned, gradeLowHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
		if slices.Contains(coverage.Owned, gradeNegativeHeader) || !slices.Contains(coverage.Lines, gradeDefaultHeader) || slices.Contains(coverage.Owned, gradeDefaultHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("the header a fallthrough enters is run and not owned, and the header of the clause that falls is owned"+verifiesClauseLines, func(t *testing.T) {
		coverage := covered[carryFromFirstStep]
		if !slices.Contains(coverage.Owned, carryFirstHeader) {
			t.Fatalf("owned = %v", coverage.Owned)
		}
		if !slices.Contains(coverage.Lines, carrySecondHeader) || slices.Contains(coverage.Owned, carrySecondHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("an else-if line is run and not owned when only the first branch ran"+verifiesClauseLines, func(t *testing.T) {
		coverage := covered[bandIsHigh]
		if !slices.Contains(coverage.Lines, bandElseIfLine) || slices.Contains(coverage.Owned, bandElseIfLine) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
	t.Run("a select header the test matched is owned, and the one it did not is not run"+verifiesClauseLines, func(t *testing.T) {
		coverage := covered[awaitTakesReady]
		if !slices.Contains(coverage.Owned, awaitReadyHeader) || slices.Contains(coverage.Lines, awaitDoneHeader) {
			t.Fatalf("coverage = %+v", coverage)
		}
	})
}
