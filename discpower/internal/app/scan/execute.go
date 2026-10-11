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
	"github.com/RachidChabane/spec-proof/format"
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
}

// EvidenceSource gathers evidence on a built mutant, in its live workspace,
// after the suite ran on it. The differential search is one; a host's
// witnesses are another.
type EvidenceSource interface {
	Gather(ctx context.Context, candidate Candidate, mutant ecosystem.Workspace, runs map[ecosystem.TestID][]vacuity.Run) (Evidence, error)
}

// execute runs every candidate on the workers until the deadline, if any,
// passes. The outcomes are indexed like the candidates.
func (s *Service) execute(ctx context.Context, ref reference, drawn drawn, deadline time.Time, request Request) ([]outcome, error) {
	runner := mutantRunner{service: s, ref: ref, forms: map[string]string{}, sources: s.evidenceSources(ref, drawn, request.Seed), covered: map[ecosystem.TestID]scanscope.LineSet{}}
	for _, test := range ref.green {
		runner.covered[test] = scanscope.NewLineSet(ref.coverage[test])
	}
	outcomes := make([]outcome, len(drawn.candidates))
	jobs := make(chan int)
	errs := make(chan error, max(request.Workers, 1))
	var wait sync.WaitGroup
	for range max(request.Workers, 1) {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for index := range jobs {
				result, err := runner.run(ctx, drawn.candidates[index])
				if err != nil {
					errs <- err
					return
				}
				outcomes[index] = result
			}
		}()
	}
	var failure error
dispatch:
	for index := range drawn.candidates {
		if !deadline.IsZero() && !s.deps.Clock().Before(deadline) {
			break
		}
		select {
		case jobs <- index:
		case failure = <-errs:
			break dispatch
		}
	}
	close(jobs)
	wait.Wait()
	close(errs)
	if failure == nil {
		failure = <-errs
	}
	return outcomes, failure
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
	// covered holds the lines each green test ran.
	covered map[ecosystem.TestID]scanscope.LineSet
}

// run builds the mutant, compares its compiled form, runs the tests that ran
// its mutated lines three times, and gathers the evidence of a mutant no test
// killed.
func (r *mutantRunner) run(ctx context.Context, candidate Candidate) (outcome, error) {
	result := outcome{state: stateRan}
	tests := r.covering(candidate)
	status, err := r.ref.executor.Mutate(ctx, r.ref.referenceOf(), candidate.Edits, func(ctx context.Context, mutant ecosystem.Workspace) error {
		identical, err := r.identical(ctx, candidate.Edits, mutant)
		if err != nil || identical {
			result.identity = identical
			return err
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

func (r *mutantRunner) referenceForms(ctx context.Context, compiler ports.FormCompiler, sites []string) (differential.CompiledForms, error) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	var missing []string
	for _, site := range sites {
		if _, known := r.forms[site]; !known {
			missing = append(missing, site)
		}
	}
	if len(missing) > 0 {
		compiled, err := compiler.Forms(ctx, r.ref.workspace, r.ref.toolchain, missing)
		if err != nil {
			return nil, fmt.Errorf("compile the reference: %w", err)
		}
		for _, site := range missing {
			r.forms[site] = compiled[site]
		}
	}
	forms := differential.CompiledForms{}
	for _, site := range sites {
		forms[site] = r.forms[site]
	}
	return forms, nil
}

// killedByOne reports whether a test failed every run of the mutant.
func killedByOne(runs map[ecosystem.TestID][]vacuity.Run) bool {
	for _, testRuns := range runs {
		if vacuity.Settle(testRuns) == format.ScanKilled {
			return true
		}
	}
	return false
}
