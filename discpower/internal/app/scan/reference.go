package scan

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// Timeouts of the runs of a scan.
const (
	// ReferenceTimeout bounds one run of the whole suite on the reference.
	ReferenceTimeout = 10 * time.Minute
	// MinRunTimeout is the least time one run of a mutant's tests gets.
	MinRunTimeout = 30 * time.Second
	// RunTimeoutFactor is how many times longer than one reference run of the
	// whole suite one run of a mutant's tests may take.
	RunTimeoutFactor = 3
	// CoverageTimeoutFactor is how many times longer than one reference run of
	// the whole suite one run of tests under coverage may take: recording every
	// line a test runs slows it several times.
	CoverageTimeoutFactor = 10
	// MinCoverageTimeout is the least time one run of tests under coverage
	// gets.
	MinCoverageTimeout = 2 * time.Minute
	// MaxCoverageTimeout is the most time one run of tests under coverage gets
	// unless the request sets its own.
	MaxCoverageTimeout = 30 * time.Minute
)

// outputDirectory is where scans write their reports in the work tree; it is
// never copied into a workspace.
const outputDirectory = ".spec-proof"

// wholeProject is the source root of a scan: every file of the project, tests
// and the files a scan never mutates aside.
const wholeProject = "."

// reference is the built reference workspace and what the suite did on it.
type reference struct {
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
	layout    ecosystem.Layout
	executor  mutantrun.Executor
	cases     []ports.ScanCase
	// runs holds the reference runs of every test of the suite.
	runs  map[ecosystem.TestID][]vacuity.Run
	green []ecosystem.TestID
	// coverage holds the lines each green test whose coverage was recorded
	// ran.
	coverage map[ecosystem.TestID][]vacuity.Line
	// runTimeout bounds one run of a mutant's tests.
	runTimeout time.Duration
	// referenceRuns is how long the three runs of the whole suite took.
	referenceRuns time.Duration
	// referenceTime is how long copying, building and running the reference
	// took, and coverageTime how long recording its coverage took.
	referenceTime time.Duration
	coverageTime  time.Duration
}

// referenceOf is the reference as the mutant executor takes it.
func (r reference) referenceOf() mutantrun.Reference {
	return mutantrun.Reference{Workspace: r.workspace, Toolchain: r.toolchain}
}

// prepareReference copies the work tree into a workspace outside it, builds it
// and runs the whole suite three times.
func (s *Service) prepareReference(ctx context.Context, request Request) (reference, error) {
	copied := s.deps.Clock()
	ref, err := s.buildReference(ctx, request.Root)
	if err != nil {
		return reference{}, err
	}
	started := s.deps.Clock()
	ran, err := s.deps.Suite.Run(ctx, ref.workspace, ref.toolchain, ports.ScanRunRequest{Runs: vacuity.RunsPerOutcome, Timeout: ReferenceTimeout})
	if err != nil {
		return reference{}, fmt.Errorf("run the suite on the reference: %w", err)
	}
	if len(ran.Cases) == 0 {
		return reference{}, ErrNoTests
	}
	finished := s.deps.Clock()
	ref.referenceRuns = finished.Sub(started)
	ref.referenceTime = finished.Sub(copied)
	ref.runTimeout = runTimeout(ref.referenceRuns)
	ref.cases, ref.runs = ran.Cases, ran.Runs
	for _, scanCase := range ran.Cases {
		if vacuity.Green(ran.Runs[scanCase.ID]) {
			ref.green = append(ref.green, scanCase.ID)
		}
	}
	return ref, nil
}

// recordCoverage records the line coverage of the given green tests, each run
// under coverage bounded by the request's timeout or by one derived from the
// reference runs, on the request's workers.
func (s *Service) recordCoverage(ctx context.Context, ref *reference, tests []ecosystem.TestID, request Request) error {
	started := s.deps.Clock()
	wanted := make(map[ecosystem.TestID]bool, len(tests))
	for _, test := range tests {
		wanted[test] = true
	}
	var cases []ports.ScanCase
	for _, scanCase := range ref.cases {
		if wanted[scanCase.ID] {
			cases = append(cases, scanCase)
		}
	}
	limit := coverageTimeout(ref.referenceRuns, request.CoverageTimeout)
	coverage, err := s.deps.Suite.LineCoverage(ctx, ref.workspace, ref.toolchain, ports.ScanCoverageRequest{Tests: cases, Timeout: limit, Workers: request.Workers})
	if err != nil {
		return fmt.Errorf("record the line coverage, each run bounded by %s: %w", limit, err)
	}
	ref.coverage = coverage
	ref.coverageTime = s.deps.Clock().Sub(started)
	return nil
}

// buildReference copies the work tree, scan reports aside, into a workspace
// outside it and builds it.
func (s *Service) buildReference(ctx context.Context, root string) (reference, error) {
	layout := s.deps.Ecosystem.Layout()
	layout.ExcludedDirs = append(slices.Clone(layout.ExcludedDirs), outputDirectory)
	executor := mutantrun.Executor{
		Ecosystem:  layoutOverride{Ecosystem: s.deps.Ecosystem, layout: layout},
		Workspaces: s.deps.Workspaces,
		Policy:     mutation.Policy{SourceRoots: []string{wholeProject}, Layout: layout},
	}
	prepared, err := executor.PrepareReference(ctx, root)
	if err != nil {
		return reference{}, err
	}
	built, err := executor.BuildReference(ctx, prepared, nil)
	if err != nil {
		return reference{}, fmt.Errorf("build the reference: %w", err)
	}
	if !built.OK {
		_ = s.deps.Workspaces.Release(prepared.Workspace)
		return reference{}, fmt.Errorf("%w: %s", ErrReferenceBuild, built.Diagnostics)
	}
	return reference{workspace: prepared.Workspace, toolchain: prepared.Toolchain, layout: layout, executor: executor}, nil
}

// runTimeout bounds one run of a mutant's tests by the time one reference run
// of the whole suite took.
func runTimeout(referenceRuns time.Duration) time.Duration {
	perRun := referenceRuns / vacuity.RunsPerOutcome
	return max(MinRunTimeout, perRun*RunTimeoutFactor)
}

// coverageTimeout bounds one run of tests under coverage: the requested time
// when set, otherwise the time one reference run of the whole suite took,
// CoverageTimeoutFactor times over, between MinCoverageTimeout and
// MaxCoverageTimeout.
func coverageTimeout(referenceRuns, requested time.Duration) time.Duration {
	if requested > 0 {
		return requested
	}
	perRun := referenceRuns / vacuity.RunsPerOutcome
	return min(MaxCoverageTimeout, max(MinCoverageTimeout, perRun*CoverageTimeoutFactor))
}

// layoutOverride is the ecosystem strategy with the layout of a scan, which
// also keeps scan reports out of every workspace.
type layoutOverride struct {
	ports.Ecosystem
	layout ecosystem.Layout
}

// Layout is the layout of the scan.
func (l layoutOverride) Layout() ecosystem.Layout { return l.layout }
