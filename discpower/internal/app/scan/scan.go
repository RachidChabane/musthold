// Package scan is the use case of `spec-proof-dp scan` (ADR 0014): it runs the
// suite of a repository three times on the reference, records each test's own
// line coverage, samples tests, draws operator mutants and sentinels in the code
// the sampled tests ran, runs each mutant against the tests that ran its
// mutated lines, gathers the executed evidence that proves a mutant changes
// behaviour, classifies every test with domain/vacuity and records the scan as
// a scan/v1 statement. It never calls a model.
//
// The phases are seams: a host adds model mutants to the draw and witnesses to
// the evidence without changing the others.
package scan

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/format"
)

// Errors of a scan.
var (
	// ErrDirtyTree is returned when a tracked file differs from the head commit:
	// a scan names the commit it ran on, so it runs on a clean work tree.
	ErrDirtyTree = errors.New("tracked files differ from the head commit; commit or stash them so the scan names the code it ran")
	// ErrReferenceBuild is returned when the reference does not build.
	ErrReferenceBuild = errors.New("the reference does not build")
	// ErrNoTests is returned when the runner collects no test.
	ErrNoTests = errors.New("the runner collected no test")
)

// Deps are the ports a scan works through.
type Deps struct {
	// Ecosystem is the detected strategy: layout, toolchain, build check and
	// definition locator.
	Ecosystem ports.Ecosystem
	// Suite runs the tests of the ecosystem for a scan.
	Suite      ports.ScanSuite
	Workspaces ports.Workspaces
	Engines    *operatorgen.Registry
	// Compiler compares compiled forms (E0); nil for an ecosystem with none.
	Compiler ports.FormCompiler
	// Prober calls functions for the differential search; nil for an ecosystem
	// that cannot probe.
	Prober ports.Prober
	Source ports.SourceControl
	// Project is the work tree of the scanned repository.
	Project fs.FS
	Clock   func() time.Time
	// Capacity reads the machine a scan chooses its workers on; nil chooses
	// DefaultWorkers.
	Capacity ports.CapacityReader
}

// Request is one scan.
type Request struct {
	// Root is the directory of the work tree.
	Root string
	// Base, when set, makes it a change scan against that revision.
	Base       string
	Seed       uint64
	SampleSize int
	// Mutants is how many operator mutants are drawn, sentinels aside.
	Mutants int
	// Budget, when positive, stops the execution of mutants once it has passed
	// since the scan began; the scan is then partial.
	Budget time.Duration
	// Workers is how many mutants, or tests under coverage, run at once;
	// below one, as many as the capacity of the machine allows.
	Workers int
	// Depth names the preset the other fields come from, for the report.
	Depth Depth
	// CoverageTimeout, when positive, bounds each run of tests under coverage
	// in place of the bound derived from the reference runs.
	CoverageTimeout time.Duration
	// Coverage is whose line coverage a repository scan records, and so which
	// tests may run a mutant.
	Coverage CoverageScope
	Module   format.Module
}

// CoverageScope is whose line coverage a repository scan records. Only a test
// whose coverage is recorded runs a mutant or a sentinel, so it also bounds
// the tests whose kills prove a change.
type CoverageScope int

// Scopes of the coverage of a repository scan.
const (
	// CoverGreen records the coverage of every green test.
	CoverGreen CoverageScope = iota
	// CoverSampled records the coverage of the sampled tests only, unless
	// the suite records every test in one run.
	CoverSampled
)

// of lists the tests whose coverage the scope records: the sampled tests
// alone only when recording fewer tests saves time.
func (c CoverageScope) of(green, sampled []ecosystem.TestID, oneRun bool) []ecosystem.TestID {
	if c == CoverSampled && !oneRun {
		return sampled
	}
	return green
}

// Report is what a scan established and what its renderers show.
type Report struct {
	// Statement is the scan statement and Encoded its canonical bytes.
	Statement format.ScanStatement
	Encoded   []byte
	// Repository names the repository and Commit the scanned commit.
	Repository string
	Commit     string
	Runner     ecosystem.Name
	Mode       format.ScanMode
	Base       string
	Seed       uint64
	// Suite counts the tests of the suite.
	Suite SuiteCounts
	// Mutants counts the mutants of the draw.
	Mutants MutantCounts
	// Candidates counts, for each sampled test with an operator mutant among
	// the lines it ran, the operator mutants drawn with a mutated line among
	// them; the scan statement does not record it.
	Candidates map[ecosystem.TestID]int
	// Unproven explains each unproven test, by test id: the proven changes it
	// ran and why each other drawn mutant on its lines proves nothing; the scan
	// statement does not record it.
	Unproven []UnprovenTest
	// Findings are the tests that verify nothing, ranked, each with one change.
	Findings []Finding
	Partial  bool
	Elapsed  time.Duration
	// Depth, Budget and Workers are those the scan ran with.
	Depth   Depth
	Budget  time.Duration
	Workers int
	// Phases splits the elapsed time by phase.
	Phases Phases
}

// Phases is how long each phase of a scan took.
type Phases struct {
	// Reference covers copying, building and the three runs of the suite.
	Reference time.Duration
	// Coverage is the recording of the line coverage of the tests.
	Coverage time.Duration
	// Draw is the generation and allocation of the mutants.
	Draw time.Duration
	// Mutants covers running the mutants and gathering their evidence.
	Mutants time.Duration
}

// SuiteCounts counts the tests of a suite.
type SuiteCounts struct {
	Tests   int
	Green   int
	Sampled int
}

// MutantCounts counts the mutants of a draw by what became of them.
type MutantCounts struct {
	// Drawn are the operator mutants and sentinels drawn.
	Drawn int
	// Ran are the mutants that built and ran.
	Ran int
	// DidNotBuild are the mutants dropped because they did not splice or
	// failed the build check, which in Go also starts the test binaries of
	// the packages they edit.
	DidNotBuild int
	// StoppedSentinels are the sentinels on which a run of a test did not
	// start: a sentinel keeps behaviour, so the harness, not the test, is at
	// fault.
	StoppedSentinels int
	// CompiledIdentical are the mutants E0 found identical to the reference.
	CompiledIdentical int
	// NotRun are the mutants the budget left unrun.
	NotRun int
	// Proven are the mutants executed evidence proves change behaviour.
	Proven int
}

// Finding is one test that verifies nothing, with one of its changes, the
// route that proves it and how to reproduce it.
type Finding struct {
	// Rank is the place of the test among the findings, from 1.
	Rank int
	Test ecosystem.TestID
	// Changes is how many proven changes in the lines it ran the test passed.
	Changes int
	// NotStarted is how many more proven changes in the lines it ran stopped
	// it from starting; they count neither way.
	NotStarted int
	Mutant     format.ScanMutant
	// Site is the first mutated line of the shown change.
	Site  format.ScanLine
	Route format.ScanRoute
	// Files are the files the change edits, before and after.
	Files []FileChange
	// Repro is the command line that runs the test from the project root.
	Repro []string
}

// FileChange is one file a mutant edits, before and after.
type FileChange struct {
	Path   string
	Before []byte
	After  []byte
}

// Service runs scans.
type Service struct {
	deps Deps
}

// New returns a scan service over its ports.
func New(deps Deps) *Service {
	if deps.Clock == nil {
		deps.Clock = time.Now
	}
	return &Service{deps: deps}
}

// Run scans the repository: reference, coverage, sample, draw, execute,
// evidence, classification and statement.
func (s *Service) Run(ctx context.Context, request Request) (Report, error) {
	started := s.deps.Clock()
	request.Workers = s.workers(ctx, request.Workers)
	commit, err := s.commitOf(ctx)
	if err != nil {
		return Report{}, err
	}
	base, err := s.baseOf(ctx, request.Base)
	if err != nil {
		return Report{}, err
	}
	reference, err := s.prepareReference(ctx, request)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = s.deps.Workspaces.Release(reference.workspace) }()
	plan, err := s.plan(ctx, &reference, request, base)
	if err != nil {
		return Report{}, err
	}
	drawStarted := s.deps.Clock()
	drawn, err := s.draw(reference, plan, request)
	if err != nil {
		return Report{}, err
	}
	mutantsStarted := s.deps.Clock()
	deadline := time.Time{}
	if request.Budget > 0 {
		deadline = started.Add(request.Budget)
	}
	executed, err := s.execute(ctx, reference, drawn, deadline, request)
	if err != nil {
		return Report{}, err
	}
	report, err := s.emit(ctx, emission{commit: commit, base: base, reference: reference, plan: plan, drawn: drawn, executed: executed, request: request})
	if err != nil {
		return Report{}, err
	}
	finished := s.deps.Clock()
	report.Elapsed = finished.Sub(started)
	report.Depth, report.Budget, report.Workers = request.Depth, request.Budget, request.Workers
	report.Phases = Phases{
		Reference: reference.referenceTime, Coverage: reference.coverageTime,
		Draw: mutantsStarted.Sub(drawStarted), Mutants: finished.Sub(mutantsStarted),
	}
	return report, nil
}

// commitOf is the head commit of a clean work tree.
func (s *Service) commitOf(ctx context.Context) (string, error) {
	dirty, err := s.deps.Source.Dirty(ctx)
	if err != nil {
		return "", fmt.Errorf("read the work tree: %w", err)
	}
	if dirty {
		return "", ErrDirtyTree
	}
	commit, err := s.deps.Source.Head(ctx)
	if err != nil {
		return "", fmt.Errorf("read the head commit: %w", err)
	}
	return commit, nil
}

// baseOf resolves the base revision of a change scan; a repository scan has
// none.
func (s *Service) baseOf(ctx context.Context, revision string) (string, error) {
	if revision == "" {
		return "", nil
	}
	base, err := s.deps.Source.Resolve(ctx, revision)
	if err != nil {
		return "", fmt.Errorf("resolve the base %q: %w", revision, err)
	}
	return base, nil
}
