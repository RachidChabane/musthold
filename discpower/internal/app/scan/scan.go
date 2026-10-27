// Package scan is the use case of `musthold-dp scan` (ADR 0014): it runs the
// suite of a repository three times on the reference, records each test's own
// line coverage, samples tests, draws operator mutants and sentinels in the code
// the sampled tests ran, runs each mutant against the tests that ran its
// mutated lines, gathers the executed evidence that proves a mutant changes
// behaviour, classifies every test with domain/vacuity, confirms each test
// that verifies nothing by running it alone, and records the scan as a scan/v1
// statement. It never calls a model.
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

	"github.com/RachidChabane/musthold/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
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
	// ErrRunnerFailed is returned when the runner's process on the reference
	// failed before it reported a test.
	ErrRunnerFailed = errors.New("the runner failed on the reference before it reported a test")
	// ErrNoPullRequests is returned for `scan <PR>` when no pull request can be
	// read.
	ErrNoPullRequests = errors.New("no pull request reader is wired")
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
	// PullRequests reads the pull request of `scan <PR>`; nil when none can be
	// read.
	PullRequests PullRequestReader
	// WitnessHost briefs witness writers and lays out witnesses as tests of the
	// ecosystem; nil for an ecosystem with none.
	WitnessHost ports.WitnessHost
	// Witnesses runs witness sets; nil for an ecosystem that runs none.
	Witnesses WitnessRunners
	// Host holds the ports of a scan that asks a host; nil for a scan that asks
	// none and for a replay.
	Host *HostDeps
}

// Request is one scan.
type Request struct {
	// Root is the directory of the work tree.
	Root string
	// Base, when set, makes it a change scan against that revision.
	Base string
	// PullRequest, when positive, makes it a change scan of that pull request,
	// against the merge base of the head and its base commit.
	PullRequest int
	// Resume continues the open scan session of the same binding; it applies
	// to a scan that asks a host only.
	Resume     bool
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
	// Promises is what a scan that asked its host established about the
	// promises; nil for a scan that asked none.
	Promises *PromiseReport
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
	// Confirmation covers running alone each test that verifies nothing, on
	// the changes it cites.
	Confirmation time.Duration
}

// SuiteCounts counts the tests of a suite.
type SuiteCounts struct {
	Tests   int
	Green   int
	Sampled int
}

// MutantCounts counts the mutants of a draw by what became of them.
type MutantCounts struct {
	// Drawn are the operator mutants and sentinels drawn, and the model mutants
	// a scan that asked its host added.
	Drawn int
	// AmbiguousUnits are the definitions and assignments the draw left out
	// because their file defines their name more than once.
	AmbiguousUnits int
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
	// Killers are, for each route of the mutant, the executed counter-evidence
	// it offers: a test that fails on the mutant.
	Killers []Killer
	// Witnesses and Spec are, for a witness_failure route, the witnesses laid
	// out with the failing one and the promises they were written against.
	Witnesses []format.ScanWitness
	Spec      []format.ScanPromise
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
// evidence, classification, confirmation of its claims and statement. The
// budget bounds the mutants only: the confirmation runs once they stop.
func (s *Service) Run(ctx context.Context, request Request) (Report, error) {
	started := s.deps.Clock()
	request.Workers = s.workers(ctx, request.Workers)
	commit, err := s.commitOf(ctx)
	if err != nil {
		return Report{}, err
	}
	change, err := s.changeOf(ctx, request)
	if err != nil {
		return Report{}, err
	}
	copied := s.deps.Clock()
	built, err := s.buildReference(ctx, request.Root)
	if err != nil {
		return Report{}, err
	}
	defer func() { _ = s.deps.Workspaces.Release(built.workspace) }()
	return s.scanFrom(ctx, scanInput{started: started, copied: copied, commit: commit, base: change.base, request: request, reference: built})
}

// scanInput is what the phases of a scan that runs the suite start from: the
// built reference, the commit and base, and for a scan that asks a host what
// its answers add.
type scanInput struct {
	started   time.Time
	copied    time.Time
	commit    string
	base      string
	request   Request
	reference reference
	// host is nil for a scan that asks no host.
	host *hostScan
}

// scanFrom runs the phases of a scan on a built reference, in one process: the
// suite three times, coverage, sample, draw, execution and evidence,
// classification, confirmation and statement.
func (s *Service) scanFrom(ctx context.Context, in scanInput) (Report, error) {
	request := in.request
	reference := in.reference
	if err := s.runReference(ctx, &reference, in.copied); err != nil {
		return Report{}, err
	}
	plan, err := s.plan(ctx, &reference, request, in.base)
	if err != nil {
		return Report{}, err
	}
	drawStarted := s.deps.Clock()
	drawn, err := s.draw(reference, plan, request)
	if err != nil {
		return Report{}, err
	}
	sources := s.evidenceSources(reference, drawn, request.Seed)
	if in.host != nil {
		drawn = in.host.withModels(drawn)
		if sources, err = in.host.withWitnesses(ctx, reference, sources); err != nil {
			return Report{}, err
		}
	}
	mutantsStarted := s.deps.Clock()
	deadline := time.Time{}
	if request.Budget > 0 {
		deadline = in.started.Add(request.Budget)
	}
	executed, err := s.execute(ctx, reference, plan, drawn, deadline, request, sources)
	if err != nil {
		return Report{}, err
	}
	emission := emission{commit: in.commit, base: in.base, reference: reference, plan: plan, drawn: drawn, executed: executed, request: request}
	if in.host != nil {
		if emission.promises, err = in.host.outcome(ctx, reference, drawn, executed); err != nil {
			return Report{}, err
		}
	}
	input, ran, result, err := emission.classify()
	if err != nil {
		return Report{}, err
	}
	confirmationStarted := s.deps.Clock()
	emission.ran = ran
	if emission.confirmed, err = s.confirm(ctx, reference, drawn, input, result, request); err != nil {
		return Report{}, err
	}
	confirmationFinished := s.deps.Clock()
	report, err := s.emit(ctx, emission)
	if err != nil {
		return Report{}, err
	}
	report.Elapsed = s.deps.Clock().Sub(in.started)
	report.Depth, report.Budget, report.Workers = request.Depth, request.Budget, request.Workers
	report.Phases = Phases{
		Reference: reference.referenceTime, Coverage: reference.coverageTime,
		Draw: mutantsStarted.Sub(drawStarted), Mutants: confirmationStarted.Sub(mutantsStarted),
		Confirmation: confirmationFinished.Sub(confirmationStarted),
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

// change is what a change scan is measured against: the base commit and, for
// `scan <PR>`, the pull request. A repository scan has neither.
type change struct {
	base string
	pull *PullRequest
}

// changeOf resolves the base of a change scan: the merge base of the head and
// the base commit of the pull request, or the revision the request names; a
// repository scan has none.
func (s *Service) changeOf(ctx context.Context, request Request) (change, error) {
	switch {
	case request.PullRequest > 0 && s.deps.PullRequests == nil:
		return change{}, ErrNoPullRequests
	case request.PullRequest > 0:
		pull, err := s.deps.PullRequests.Read(ctx, request.PullRequest)
		if err != nil {
			return change{}, fmt.Errorf("read pull request #%d: %w", request.PullRequest, err)
		}
		return change{base: pull.ScanBase, pull: &pull}, nil
	case request.Base == "":
		return change{}, nil
	}
	base, err := s.deps.Source.Resolve(ctx, request.Base)
	if err != nil {
		return change{}, fmt.Errorf("resolve the base %q: %w", request.Base, err)
	}
	return change{base: base}, nil
}
