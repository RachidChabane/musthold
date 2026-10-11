package scan

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// DefaultWorkers is how many mutants a scan runs at once.
const DefaultWorkers = 2

// siteSeparator joins the file and the qualified name of a compiled site.
const siteSeparator = "::"

// state is what became of one mutant.
type state int

// States of a mutant.
const (
	// stateNotRun: the budget passed before the mutant started.
	stateNotRun state = iota
	// stateDidNotBuild: the mutant did not splice or build; it is dropped.
	stateDidNotBuild
	// stateRan: the mutant built; E0 held it identical or its tests ran.
	stateRan
)

// outcome is what one mutant did.
type outcome struct {
	state    state
	identity bool
	// runs holds the runs of every test that ran a mutated line.
	runs     map[ecosystem.TestID][]vacuity.Run
	evidence Evidence
}

// Evidence is executed evidence on a mutant besides the runs of the suite.
type Evidence struct {
	Differences     []vacuity.Difference
	WitnessFailures []vacuity.WitnessFailure
	// Search is how far the differential search went.
	Search SearchOutcome
}

// EvidenceSource gathers evidence on a built mutant, in its live workspace,
// after the suite ran on it. The differential search is one; a host's
// witnesses are another.
type EvidenceSource interface {
	Gather(ctx context.Context, candidate Candidate, mutant ecosystem.Workspace, runs map[ecosystem.TestID][]vacuity.Run) (Evidence, error)
}

// execute runs the candidates on the workers until the deadline, if any,
// passes: no candidate starts after it, and a candidate still running when it
// passes is stopped and counted as not run. One coordinator starts each
// candidate on an idle worker, the next one by need (scanscope.Queue), and
// records each finished candidate in the progress of the scan
// (vacuity.Progress), which no worker reads. The outcomes are indexed like the
// candidates.
func (s *Service) execute(ctx context.Context, ref reference, plan plan, drawn drawn, deadline time.Time, request Request) ([]outcome, error) {
	budgeted, cancel := s.budgeted(ctx, deadline)
	defer cancel()
	runner := mutantRunner{service: s, ref: ref, forms: map[string]string{}, sources: s.evidenceSources(ref, drawn, request.Seed), covered: map[ecosystem.TestID]scanscope.LineSet{}}
	for _, test := range ref.green {
		if coverage, recorded := ref.coverage[test]; recorded {
			runner.covered[test] = scanscope.NewLineSet(coverage.Lines)
		}
	}
	workers := max(request.Workers, 1)
	jobs := make(chan int)
	finished := make(chan finishedRun)
	var wait sync.WaitGroup
	for range workers {
		wait.Go(func() {
			for index := range jobs {
				result, err := runner.run(budgeted, drawn.candidates[index])
				stopped := budgeted.Err() != nil && ctx.Err() == nil
				finished <- finishedRun{index: index, result: result, err: err, stopped: stopped}
			}
		})
	}
	coordinator := coordinator{
		service: s, deadline: deadline, drawn: drawn,
		queue: scanscope.NewQueue(drawn.order), progress: vacuity.NewProgress(progressInput(ref, plan, drawn)),
		outcomes: make([]outcome, len(drawn.candidates)),
	}
	failure := coordinator.dispatch(workers, jobs, finished)
	close(jobs)
	wait.Wait()
	return coordinator.outcomes, failure
}

// finishedRun is one candidate a worker finished, with what it did.
type finishedRun struct {
	index  int
	result outcome
	err    error
	// stopped is set when the deadline passed while the candidate ran.
	stopped bool
}

// coordinator starts the candidates and gathers their outcomes; it alone reads
// and writes the progress and the queue.
type coordinator struct {
	service  *Service
	deadline time.Time
	drawn    drawn
	queue    *scanscope.Queue
	progress *vacuity.Progress
	outcomes []outcome
}

// dispatch keeps every worker busy until no candidate is left, the deadline
// passes or a candidate fails, and returns the first failure once every
// started candidate has finished.
func (c *coordinator) dispatch(workers int, jobs chan<- int, finished <-chan finishedRun) error {
	var failure error
	running := 0
	for {
		for running < workers && failure == nil && c.mayStart() {
			index, left := c.queue.Next(c.needed)
			if !left {
				break
			}
			jobs <- index
			running++
		}
		if running == 0 {
			return failure
		}
		done := <-finished
		running--
		if err := c.gather(done); err != nil && failure == nil {
			failure = err
		}
	}
}

// mayStart reports whether the deadline, read on the scan's clock, has not
// passed.
func (c *coordinator) mayStart() bool {
	return c.deadline.IsZero() || c.service.deps.Clock().Before(c.deadline)
}

// needed reports whether the candidate can still move a verdict: a sentinel
// always can, and another mutant can when a sampled test whose verdict is
// still open ran one of its mutated lines.
func (c *coordinator) needed(index int) bool {
	candidate := c.drawn.candidates[index]
	return candidate.Sentinel || c.progress.Open(candidate.Lines)
}

// gather keeps the outcome of a finished candidate and records it in the
// progress: a candidate the deadline stopped is not run, and one that failed
// is the failure of the scan.
func (c *coordinator) gather(done finishedRun) error {
	switch {
	case done.stopped:
		c.outcomes[done.index] = outcome{state: stateNotRun}
		return nil
	case done.err != nil:
		return done.err
	}
	c.outcomes[done.index] = done.result
	candidate := c.drawn.candidates[done.index]
	c.progress.Record(vacuity.Mutant{ID: candidate.ID, Lines: candidate.Lines, Sentinel: candidate.Sentinel, CompiledIdentical: done.result.identity}, done.result.runs)
	return nil
}

// progressInput is what the progress of a scan follows: the reference runs,
// the sample, the coverage and every drawn candidate.
func progressInput(ref reference, plan plan, drawn drawn) vacuity.Input {
	input := vacuity.Input{Reference: ref.runs, Sampled: plan.sampled, Coverage: ref.coverage}
	for _, candidate := range drawn.candidates {
		input.Mutants = append(input.Mutants, vacuity.Mutant{ID: candidate.ID, Lines: candidate.Lines, Sentinel: candidate.Sentinel})
	}
	return input
}

// budgeted is the context of the mutant runs: it ends when the deadline, read
// on the scan's clock, passes; with no deadline it ends with the scan.
func (s *Service) budgeted(ctx context.Context, deadline time.Time) (context.Context, context.CancelFunc) {
	if deadline.IsZero() {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, deadline.Sub(s.deps.Clock()))
}

// evidenceSources are the evidence a scan gathers without a host: the
// spec-free differential search, where the ecosystem can probe.
func (s *Service) evidenceSources(ref reference, drawn drawn, seed uint64) []EvidenceSource {
	if s.deps.Prober == nil {
		return nil
	}
	return []EvidenceSource{newDifferentialSearch(s.deps.Prober, s.deps.Suite, ref, s.deps.Project, drawn, seed)}
}

// mutantRunner runs one mutant at a time on a worker.
type mutantRunner struct {
	service *Service
	ref     reference
	sources []EvidenceSource
	mutex   sync.Mutex
	// forms memoizes the compiled forms of the reference, by site.
	forms map[string]string
	// covered holds the lines each green test ran, start-up included.
	covered map[ecosystem.TestID]scanscope.LineSet
}

// run builds the mutant, compares the compiled form of a mutant that is not a
// sentinel, runs the tests that ran its mutated lines three times, and gathers
// the evidence of a mutant no test killed. A sentinel always runs: it keeps
// behaviour by construction, often in its compiled form too, and a test that
// fails on it proves nothing.
func (r *mutantRunner) run(ctx context.Context, candidate Candidate) (outcome, error) {
	result := outcome{state: stateRan}
	tests := r.covering(candidate)
	status, err := r.ref.executor.Mutate(ctx, r.ref.referenceOf(), candidate.Edits, func(ctx context.Context, mutant ecosystem.Workspace) error {
		if !candidate.Sentinel {
			identical, err := r.identical(ctx, candidate.Edits, mutant)
			if err != nil || identical {
				result.identity = identical
				return err
			}
		}
		if len(tests) > 0 {
			ran, err := r.service.deps.Suite.Run(ctx, mutant, r.ref.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: r.ref.runTimeout})
			if err != nil {
				return err
			}
			result.runs = ran.Runs
		}
		if candidate.Sentinel || killedByOne(result.runs) {
			return nil
		}
		for _, source := range r.sources {
			found, err := source.Gather(ctx, candidate, mutant, result.runs)
			if err != nil {
				return err
			}
			result.evidence.Differences = append(result.evidence.Differences, found.Differences...)
			result.evidence.WitnessFailures = append(result.evidence.WitnessFailures, found.WitnessFailures...)
			result.evidence.Search = max(result.evidence.Search, found.Search)
		}
		return nil
	})
	if err != nil {
		return outcome{}, fmt.Errorf("run mutant %s: %w", candidate.ID, err)
	}
	if !status.Built {
		return outcome{state: stateDidNotBuild}, nil
	}
	return result, nil
}

// covering lists the green tests whose coverage holds a mutated line: the
// same match the classification reads.
func (r *mutantRunner) covering(candidate Candidate) []ecosystem.TestID {
	var tests []ecosystem.TestID
	for _, test := range r.ref.green {
		if r.covered[test].Intersects(candidate.Lines) {
			tests = append(tests, test)
		}
	}
	slices.SortFunc(tests, func(a, b ecosystem.TestID) int { return strings.Compare(string(a), string(b)) })
	return tests
}

// identical reports whether every site the mutant edits compiles to the
// reference's form (E0); without a compiler nothing is identical.
func (r *mutantRunner) identical(ctx context.Context, edits mutation.EditSet, mutant ecosystem.Workspace) (bool, error) {
	compiler := r.service.deps.Compiler
	if compiler == nil {
		return false, nil
	}
	var sites []string
	for _, edit := range edits.Canonical().Edits {
		sites = append(sites, edit.File+siteSeparator+edit.Qualname)
	}
	reference, err := r.referenceForms(ctx, compiler, sites)
	if err != nil {
		return false, err
	}
	forms, err := compiler.Forms(ctx, mutant, r.ref.toolchain, sites)
	if err != nil {
		return false, fmt.Errorf("compile a mutant: %w", err)
	}
	return differential.CompareForms(sites, reference, forms) == power.IdentityIdentical, nil
}

// referenceForms are the compiled forms of the sites on the reference,
// memoized by site. The lock guards the memo only: workers that need forms of
// other sites compile them at the same time, and two that need the same new
// site may both compile it, to the same form.
func (r *mutantRunner) referenceForms(ctx context.Context, compiler ports.FormCompiler, sites []string) (differential.CompiledForms, error) {
	missing := r.unknownSites(sites)
	if len(missing) > 0 {
		compiled, err := compiler.Forms(ctx, r.ref.workspace, r.ref.toolchain, missing)
		if err != nil {
			return nil, fmt.Errorf("compile the reference: %w", err)
		}
		r.mutex.Lock()
		for _, site := range missing {
			r.forms[site] = compiled[site]
		}
		r.mutex.Unlock()
	}
	r.mutex.Lock()
	defer r.mutex.Unlock()
	forms := differential.CompiledForms{}
	for _, site := range sites {
		forms[site] = r.forms[site]
	}
	return forms, nil
}

// unknownSites are the sites whose reference form is not memoized yet.
func (r *mutantRunner) unknownSites(sites []string) []string {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	var missing []string
	for _, site := range sites {
		if _, known := r.forms[site]; !known {
			missing = append(missing, site)
		}
	}
	return missing
}

// killedByOne reports whether a test failed every run of the mutant.
func killedByOne(runs map[ecosystem.TestID][]vacuity.Run) bool {
	for _, testRuns := range runs {
		if vacuity.Settle(testRuns) == vacuity.SettledKilled {
			return true
		}
	}
	return false
}
