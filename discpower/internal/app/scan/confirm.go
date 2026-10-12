package scan

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// confirmed is a classification once its claims are confirmed, with the
// killers the confirmation upheld, by mutant id.
type confirmed struct {
	result  vacuity.Result
	killers map[string][]ecosystem.TestID
}

// confirm re-executes, alone, each test the classification says verifies
// nothing, and reclassifies it on the changes it upholds (vacuity.Confirm):
//
//  1. where the runner records every test in one run, it records the test's
//     coverage in a run that selects it alone; elsewhere the recording of the
//     scan already ran it alone;
//  2. for each change left on a line it owns in both recordings that only
//     kills prove, each killer runs alone on the reference, then on the
//     mutant, until one passes the first and fails the second every run;
//  3. the test runs alone on each change left.
//
// Every run selects one test. The confirmation reads the scan's context, never
// its budget, and is bounded by the changes the tests cite. A run that fails
// leaves the change it was for unconfirmed; only the end of the scan's context
// fails the confirmation.
func (s *Service) confirm(ctx context.Context, ref reference, drawn drawn, input vacuity.Input, result vacuity.Result, request Request) (confirmed, error) {
	claims, err := vacuity.Claims(input, result)
	if err != nil || len(claims) == 0 {
		return confirmed{result: result}, err
	}
	confirmation := vacuity.Confirmation{Tests: map[ecosystem.TestID]vacuity.Alone{}, Killers: map[string][]ecosystem.TestID{}}
	var standing []vacuity.Claim
	for _, claim := range claims {
		coverage, recorded, err := s.recordAlone(ctx, ref, claim.Test, request)
		if err != nil {
			return confirmed{}, err
		}
		if recorded {
			confirmation.Tests[claim.Test] = vacuity.Alone{Coverage: coverage, Runs: map[string][]vacuity.Run{}}
			standing = append(standing, claim.OwnedAlone(coverage))
		}
	}
	confirmer := aloneRunner{service: s, ref: ref, confirmation: confirmation}
	if err := confirmer.killersOnReference(ctx, standing); err != nil {
		return confirmed{}, err
	}
	if err := confirmer.onMutants(ctx, jobsOf(standing, drawn), request.Workers); err != nil {
		return confirmed{}, err
	}
	reclassified, err := vacuity.Confirm(input, result, confirmation)
	if err != nil {
		return confirmed{}, fmt.Errorf("confirm the claims: %w", err)
	}
	return confirmed{result: reclassified, killers: confirmation.Killers}, nil
}

// recordAlone is the coverage of the test recorded in a run that selects it
// alone: the scan's own recording where the runner records each test alone.
// A recording that fails or leaves nothing for the test is no recording.
func (s *Service) recordAlone(ctx context.Context, ref reference, test ecosystem.TestID, request Request) (vacuity.Coverage, bool, error) {
	if !s.deps.Suite.CoverageOfOneRun() {
		coverage, recorded := ref.coverage[test]
		return coverage, recorded, nil
	}
	index := slices.IndexFunc(ref.cases, func(scanCase ports.ScanCase) bool { return scanCase.ID == test })
	if index < 0 {
		return vacuity.Coverage{}, false, nil
	}
	limit := coverageTimeout(ref.referenceRuns, request.CoverageTimeout)
	covered, err := s.deps.Suite.LineCoverage(ctx, ref.workspace, ref.toolchain, ports.ScanCoverageRequest{Tests: ref.cases[index : index+1], Timeout: limit, Workers: 1, Alone: true})
	if ctx.Err() != nil {
		return vacuity.Coverage{}, false, ctx.Err()
	}
	if err != nil {
		return vacuity.Coverage{}, false, nil
	}
	coverage, recorded := covered[test]
	return coverage, recorded, nil
}

// confirmationJob is one mutant the confirmation runs: the killers that, alone,
// passed the reference, when only kills prove it, and the tests whose claims
// cite it.
type confirmationJob struct {
	candidate Candidate
	killsOnly bool
	killers   []ecosystem.TestID
	tests     []ecosystem.TestID
}

// jobsOf groups the changes the claims cite by mutant, in mutant id order, so
// that each mutant is built once and each killer runs once on it.
func jobsOf(claims []vacuity.Claim, drawn drawn) []confirmationJob {
	byID := map[string]*confirmationJob{}
	for _, claim := range claims {
		for _, change := range claim.Changes {
			job, known := byID[change.Mutant]
			if !known {
				index := slices.IndexFunc(drawn.candidates, func(candidate Candidate) bool { return candidate.ID == change.Mutant })
				if index < 0 {
					continue
				}
				job = &confirmationJob{candidate: drawn.candidates[index], killsOnly: change.KillsOnly(), killers: change.Killers}
				byID[change.Mutant] = job
			}
			job.tests = append(job.tests, claim.Test)
		}
	}
	jobs := make([]confirmationJob, 0, len(byID))
	for _, job := range byID {
		jobs = append(jobs, *job)
	}
	slices.SortFunc(jobs, func(a, b confirmationJob) int { return strings.Compare(a.candidate.ID, b.candidate.ID) })
	return jobs
}

// aloneRunner runs single tests for a confirmation and records what they did.
type aloneRunner struct {
	service      *Service
	ref          reference
	confirmation vacuity.Confirmation
	mutex        sync.Mutex
	// greenAlone holds, for each killer, whether it passed the reference
	// alone on every run.
	greenAlone map[ecosystem.TestID]bool
}

// killersOnReference runs once, alone on the reference, each killer of a
// change that only kills prove.
func (r *aloneRunner) killersOnReference(ctx context.Context, claims []vacuity.Claim) error {
	r.greenAlone = map[ecosystem.TestID]bool{}
	for _, claim := range claims {
		for _, change := range claim.Changes {
			for _, killer := range change.Killers {
				if _, known := r.greenAlone[killer]; known {
					continue
				}
				ran, err := r.runAlone(ctx, r.ref.workspace, killer)
				if err != nil {
					return err
				}
				r.greenAlone[killer] = vacuity.Green(ran)
			}
		}
	}
	return nil
}

// onMutants runs the jobs on the workers.
func (r *aloneRunner) onMutants(ctx context.Context, jobs []confirmationJob, workers int) error {
	pending := make(chan confirmationJob)
	failures := make([]error, max(workers, 1))
	var wait sync.WaitGroup
	for worker := range failures {
		wait.Go(func() {
			for job := range pending {
				if err := r.onMutant(ctx, job); err != nil && failures[worker] == nil {
					failures[worker] = err
				}
			}
		})
	}
	for _, job := range jobs {
		pending <- job
	}
	close(pending)
	wait.Wait()
	for _, err := range failures {
		if err != nil {
			return err
		}
	}
	return nil
}

// onMutant builds the job's mutant and runs the job in it, with the killers
// that passed the reference alone. A mutant that does not build confirms
// nothing.
func (r *aloneRunner) onMutant(ctx context.Context, job confirmationJob) error {
	killers := slices.DeleteFunc(slices.Clone(job.killers), func(killer ecosystem.TestID) bool { return !r.greenAlone[killer] })
	if job.killsOnly && len(killers) == 0 {
		return nil
	}
	_, err := r.ref.executor.Mutate(ctx, r.ref.referenceOf(), job.candidate.Edits, func(ctx context.Context, mutant ecosystem.Workspace) error {
		return r.inMutant(ctx, job, killers, mutant)
	})
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

// inMutant runs, in the built mutant, the killers until one fails it every
// run, then, unless only kills prove the mutant and none did, each test whose
// claim cites it.
func (r *aloneRunner) inMutant(ctx context.Context, job confirmationJob, killers []ecosystem.TestID, mutant ecosystem.Workspace) error {
	for _, killer := range killers {
		ran, err := r.runAlone(ctx, mutant, killer)
		if err != nil {
			return err
		}
		if vacuity.Settle(ran) == vacuity.SettledKilled {
			r.record(func() { r.confirmation.Killers[job.candidate.ID] = []ecosystem.TestID{killer} })
			break
		}
	}
	if job.killsOnly && len(r.confirmedKillers(job.candidate.ID)) == 0 {
		return nil
	}
	for _, test := range job.tests {
		ran, err := r.runAlone(ctx, mutant, test)
		if err != nil {
			return err
		}
		r.record(func() { r.confirmation.Tests[test].Runs[job.candidate.ID] = ran })
	}
	return nil
}

func (r *aloneRunner) confirmedKillers(mutant string) []ecosystem.TestID {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	return r.confirmation.Killers[mutant]
}

// runAlone runs the test, and no other, three times in the workspace. A run
// that fails gives no runs; only the end of the context is an error.
func (r *aloneRunner) runAlone(ctx context.Context, workspace ecosystem.Workspace, test ecosystem.TestID) ([]vacuity.Run, error) {
	ran, err := r.service.deps.Suite.Run(ctx, workspace, r.ref.toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{test}, Runs: vacuity.RunsPerOutcome, Timeout: r.ref.runTimeout})
	switch {
	case ctx.Err() != nil:
		return nil, ctx.Err()
	case err != nil:
		return nil, nil
	}
	return ran.Runs[test], nil
}

func (r *aloneRunner) record(write func()) {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	write()
}
