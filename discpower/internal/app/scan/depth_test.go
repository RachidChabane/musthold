package scan_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	verifiesComposition = " [verifies SP-2129~2]"
	verifiesWorkers     = " [verifies SP-2130~1]"
	verifiesDepth       = " [verifies SP-2131~1]"

	gibibyte = 1 << 30
)

// fakeCapacity is a machine of the given capacity, or one that cannot be read.
type fakeCapacity struct {
	capacity ports.Capacity
	err      error
}

func (c fakeCapacity) Capacity(context.Context) (ports.Capacity, error) { return c.capacity, c.err }

func TestDepth(t *testing.T) {
	t.Run("a quick scan samples 12 tests, draws 6 mutants for each and stops starting them after 90 seconds"+verifiesDepth, func(t *testing.T) {
		quick := scan.PresetOf(scan.DepthQuick)
		if quick != (scan.Preset{SampleSize: 12, Mutants: 12 * scanscope.CandidatesPerTest, Budget: scan.QuickBudget, Coverage: scan.CoverSampled}) || scan.QuickBudget.Seconds() != 90 {
			t.Fatalf("quick preset %+v", quick)
		}
	})
	t.Run("a full scan samples 40 tests, draws 150 mutants, records every green test's coverage and has no budget"+verifiesDepth, func(t *testing.T) {
		if full := scan.PresetOf(scan.DepthFull); full != (scan.Preset{SampleSize: 40, Mutants: 150, Coverage: scan.CoverGreen}) {
			t.Fatalf("full preset %+v", full)
		}
	})
	t.Run("a scan of the sampled tests' coverage records only theirs and runs only them against a mutant"+verifiesDepth, func(t *testing.T) {
		f := newFixture()
		req := request()
		req.SampleSize, req.Coverage, req.Depth = 1, scan.CoverSampled, scan.DepthQuick
		report := run(t, f, req)
		sampled := report.Statement.Predicate.Tests
		if len(sampled) != 1 || !slices.Equal(f.suite.covered, []ecosystem.TestID{ecosystem.TestID(sampled[0].ID)}) {
			t.Fatalf("sampled %+v, covered %v", sampled, f.suite.covered)
		}
		for _, selection := range f.suite.selections {
			if !slices.Equal(selection, []ecosystem.TestID{ecosystem.TestID(sampled[0].ID)}) {
				t.Fatalf("selection %v", selection)
			}
		}
		if report.Depth != scan.DepthQuick {
			t.Fatalf("depth %v", report.Depth)
		}
	})
	t.Run("a quick scan of a suite that records every test in one run records every green test"+verifiesDepth, func(t *testing.T) {
		f := newFixture()
		f.suite.oneRun = true
		req := request()
		req.SampleSize, req.Coverage = 1, scan.CoverSampled
		run(t, f, req)
		if len(f.suite.covered) != 3 {
			t.Fatalf("covered %v", f.suite.covered)
		}
	})
	t.Run("a full scan records the coverage of every green test"+verifiesDepth, func(t *testing.T) {
		f := newFixture()
		run(t, f, request())
		if !slices.Equal(slices.Sorted(slices.Values(f.suite.covered)), []ecosystem.TestID{vacuousTest, exactTest, feeTest}) {
			t.Fatalf("covered %v", f.suite.covered)
		}
	})
}

func TestWorkers(t *testing.T) {
	cases := []struct {
		name     string
		capacity ports.Capacity
		want     int
	}{
		{"a large machine is capped at four", ports.Capacity{Processors: 18, TotalMemory: 24 * gibibyte, FreePercent: 72}, scan.MaxAutoWorkers},
		{"two processors give one worker", ports.Capacity{Processors: 2, TotalMemory: 64 * gibibyte, FreePercent: 90}, 1},
		{"the memory above the guard's free share bounds the workers", ports.Capacity{Processors: 16, TotalMemory: 10 * gibibyte, FreePercent: 60}, 2},
		{"a machine below the guard's free share still gets one worker", ports.Capacity{Processors: 16, TotalMemory: 64 * gibibyte, FreePercent: 30}, 1},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesWorkers, func(t *testing.T) {
			if got := scan.AutoWorkers(c.capacity); got != c.want {
				t.Fatalf("workers %d", got)
			}
		})
	}
	t.Run("a scan with no requested workers takes those of the machine, or the default when it cannot be read"+verifiesWorkers, func(t *testing.T) {
		req := request()
		req.Workers = 0
		f := newFixture()
		service := f.serviceWith(t, fakeCapacity{capacity: ports.Capacity{Processors: 6, TotalMemory: 64 * gibibyte, FreePercent: 80}})
		if report, err := service.Run(context.Background(), req); err != nil || report.Workers != 3 {
			t.Fatalf("workers %d, %v", report.Workers, err)
		}
		f = newFixture()
		service = f.serviceWith(t, fakeCapacity{err: errors.New("unreadable")})
		if report, err := service.Run(context.Background(), req); err != nil || report.Workers != scan.DefaultWorkers {
			t.Fatalf("workers %d, %v", report.Workers, err)
		}
	})
	t.Run("requested workers are kept"+verifiesWorkers, func(t *testing.T) {
		if report := run(t, newFixture(), request()); report.Workers != request().Workers {
			t.Fatalf("workers %d", report.Workers)
		}
	})
}

func TestUnprovenComposition(t *testing.T) {
	f := newFixture()
	f.prober.flaky = true
	report := run(t, f, request())
	t.Run("an unproven test lists each unproven mutant on its lines with why it proves nothing"+verifiesComposition, func(t *testing.T) {
		index := slices.IndexFunc(report.Unproven, func(test scan.UnprovenTest) bool { return test.Test == vacuousTest })
		if index < 0 {
			t.Fatalf("unproven %+v", report.Unproven)
		}
		explained := report.Unproven[index]
		if explained.Reason != format.ScanTooFewChanges || len(explained.Gaps) == 0 {
			t.Fatalf("explained %+v", explained)
		}
		mutants := map[string]format.ScanMutant{}
		for _, mutant := range report.Statement.Predicate.Mutants {
			mutants[mutant.ID] = mutant
		}
		for id, gap := range explained.Gaps {
			mutant, ran := mutants[id]
			if !ran || mutant.Sentinel || mutant.Proven() || !slices.Contains(scan.Gaps(), gap) {
				t.Fatalf("gap %s of %s: %+v", gap, id, mutant)
			}
		}
		if !slices.ContainsFunc(sortedGaps(explained.Gaps), func(gap scan.Gap) bool { return gap == scan.GapNoDifference }) {
			t.Fatalf("gaps %v", explained.Gaps)
		}
	})
	t.Run("only unproven tests are explained"+verifiesComposition, func(t *testing.T) {
		for _, explained := range report.Unproven {
			if classOf(report, explained.Test) != format.ScanUnproven {
				t.Fatalf("%s is %s", explained.Test, classOf(report, explained.Test))
			}
		}
	})
	t.Run("the phases of the scan add up to no more than its time"+verifiesComposition, func(t *testing.T) {
		phases := report.Phases
		if phases.Reference <= 0 || phases.Coverage <= 0 || phases.Draw <= 0 || phases.Mutants <= 0 ||
			phases.Reference+phases.Coverage+phases.Draw+phases.Mutants > report.Elapsed {
			t.Fatalf("phases %+v of %s", phases, report.Elapsed)
		}
	})
}

func sortedGaps(gaps map[string]scan.Gap) []scan.Gap {
	var list []scan.Gap
	for _, gap := range gaps {
		list = append(list, gap)
	}
	slices.Sort(list)
	return list
}
