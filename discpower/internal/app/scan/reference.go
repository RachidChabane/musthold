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
	// coverage holds the lines each green test ran.
	coverage map[ecosystem.TestID][]vacuity.Line
	// runTimeout bounds one run of a mutant's tests.
	runTimeout time.Duration
}

// referenceOf is the reference as the mutant executor takes it.
func (r reference) referenceOf() mutantrun.Reference {
	return mutantrun.Reference{Workspace: r.workspace, Toolchain: r.toolchain}
}

// prepareReference copies the work tree into a workspace outside it, builds it,
// runs the whole suite three times and records the line coverage of every
// green test.
func (s *Service) prepareReference(ctx context.Context, root string) (reference, error) {
	ref, err := s.buildReference(ctx, root)
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
	ref.runTimeout = runTimeout(s.deps.Clock().Sub(started))
	ref.cases, ref.runs = ran.Cases, ran.Runs
	var greenCases []ports.ScanCase
	for _, scanCase := range ran.Cases {
		if vacuity.Green(ran.Runs[scanCase.ID]) {
			ref.green = append(ref.green, scanCase.ID)
			greenCases = append(greenCases, scanCase)
		}
	}
	ref.coverage, err = s.deps.Suite.LineCoverage(ctx, ref.workspace, ref.toolchain, greenCases)
	if err != nil {
		return reference{}, fmt.Errorf("record the line coverage: %w", err)
	}
	return ref, nil
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

// layoutOverride is the ecosystem strategy with the layout of a scan, which
// also keeps scan reports out of every workspace.
type layoutOverride struct {
	ports.Ecosystem
	layout ecosystem.Layout
}

// Layout is the layout of the scan.
func (l layoutOverride) Layout() ecosystem.Layout { return l.layout }
