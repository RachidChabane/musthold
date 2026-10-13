package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

// Errors of a replay.
var (
	// ErrOtherCommit is returned when the work tree is not at the commit the
	// finding was made on.
	ErrOtherCommit = errors.New("the work tree is not at the scanned commit")
	// ErrUnreadableFinding is returned for a finding record that does not decode
	// or names no test, mutant or route.
	ErrUnreadableFinding = errors.New("unreadable finding")
)

// FindingRecord is what a finding folder records for a replay: the commit, the
// test that passes the change, or the promise it breaks, the mutant with its
// edits and the route that proves it changes behaviour. A witness_failure
// route carries the witnesses of its test file and the promises they were
// written against, so that a replay needs neither the host nor the statement.
type FindingRecord struct {
	Commit string            `json:"commit"`
	Runner string            `json:"runner"`
	Test   string            `json:"test"`
	Mutant format.ScanMutant `json:"mutant"`
	Route  format.ScanRoute  `json:"route"`
	// Promise is set for a broken promise: the promise the mutant breaks.
	Promise   *format.ScanPromise  `json:"promise,omitempty"`
	Witnesses []format.ScanWitness `json:"witnesses,omitempty"`
	Promises  []format.ScanPromise `json:"promises,omitempty"`
}

// RecordOf is the replay record of a finding of a report.
func RecordOf(report Report, finding Finding) FindingRecord {
	return FindingRecord{
		Commit: report.Commit, Runner: string(report.Runner), Test: string(finding.Test), Mutant: finding.Mutant, Route: finding.Route,
		Witnesses: finding.Witnesses, Promises: finding.Spec,
	}
}

// BrokenRecordOf is the replay record of a broken promise of a report.
func BrokenRecordOf(report Report, broken BrokenPromise) FindingRecord {
	brokenPromise := broken.Promise
	return FindingRecord{
		Commit: report.Commit, Runner: string(report.Runner), Mutant: broken.Mutant, Route: broken.Killer.Route,
		Promise: &brokenPromise, Witnesses: broken.File, Promises: broken.Spec,
	}
}

// DecodeFinding reads a finding record.
func DecodeFinding(data []byte) (FindingRecord, error) {
	var record FindingRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return FindingRecord{}, fmt.Errorf("%w: %v", ErrUnreadableFinding, err)
	}
	if (record.Test == "" && record.Promise == nil) || len(record.Mutant.Edits) == 0 || record.Route.Kind == "" {
		return FindingRecord{}, ErrUnreadableFinding
	}
	if record.Route.Kind == format.ScanRouteWitness && !slices.ContainsFunc(record.Witnesses, func(w format.ScanWitness) bool { return w.ID == record.Route.Witness }) {
		return FindingRecord{}, fmt.Errorf("%w: the witness of its route is not recorded", ErrUnreadableFinding)
	}
	return record, nil
}

// Check is one claim of a finding, re-executed.
type Check struct {
	Claim string
	Held  bool
}

// Replay is the outcome of re-executing every claim of a finding.
type Replay struct {
	// Test names what was replayed: the test of a finding, or the broken
	// promise.
	Test   string
	Checks []Check
}

// Held reports whether every claim held.
func (r Replay) Held() bool {
	for _, check := range r.Checks {
		if !check.Held {
			return false
		}
	}
	return len(r.Checks) > 0
}

// Replay re-executes a finding on the work tree at its commit, with no model:
// the test passes the reference and the mutant three runs out of three, and the
// route still proves the change. The test and a killing test each run alone,
// and a witness runs laid out as the scan laid it out. A broken promise
// replays in full: its witness passes the reference and fails the mutant three
// runs out of three, and the whole suite passes the mutant three times.
func (s *Service) Replay(ctx context.Context, root string, record FindingRecord) (Replay, error) {
	commit, err := s.commitOf(ctx)
	if err != nil {
		return Replay{}, err
	}
	if commit != record.Commit {
		return Replay{}, fmt.Errorf("%w: at %s, the finding is of %s", ErrOtherCommit, commit, record.Commit)
	}
	copied := s.deps.Clock()
	ref, err := s.buildReference(ctx, root)
	if err != nil {
		return Replay{}, err
	}
	defer func() { _ = s.deps.Workspaces.Release(ref.workspace) }()
	if record.Promise != nil {
		return s.replayBroken(ctx, ref, copied, record)
	}
	tests := []ecosystem.TestID{ecosystem.TestID(record.Test)}
	if record.Route.Kind == format.ScanRouteKilledByTest {
		tests = append(tests, ecosystem.TestID(record.Route.Test))
	}
	replay := Replay{Test: record.Test}
	onReference, err := s.eachAlone(ctx, ref.workspace, ref.toolchain, tests)
	if err != nil {
		return Replay{}, fmt.Errorf("run the reference: %w", err)
	}
	for _, test := range tests {
		replay.add(fmt.Sprintf("%s passes the reference %s", test, allRuns), vacuity.Green(onReference[test]))
	}
	var onMutant map[ecosystem.TestID][]vacuity.Run
	var differs []Check
	status, err := ref.executor.Mutate(ctx, ref.referenceOf(), mutation.EditSetOf(record.Mutant.Edits), func(ctx context.Context, mutant ecosystem.Workspace) error {
		var runErr error
		onMutant, runErr = s.eachAlone(ctx, mutant, ref.toolchain, tests)
		if runErr != nil || record.Route.Kind != format.ScanRouteDifferential {
			return runErr
		}
		differs = s.replayDifference(ctx, ref, mutant, record.Route)
		return nil
	})
	if err != nil {
		return Replay{}, fmt.Errorf("run the mutant: %w", err)
	}
	replay.add("the mutant builds", status.Built)
	if !status.Built {
		return replay, nil
	}
	replay.add(fmt.Sprintf("%s passes the mutant %s", record.Test, allRuns), vacuity.Settle(onMutant[tests[0]]) == vacuity.SettledSurvived)
	if record.Route.Kind == format.ScanRouteKilledByTest {
		replay.add(fmt.Sprintf("%s fails the mutant %s", record.Route.Test, allRuns), vacuity.Settle(onMutant[tests[1]]) == vacuity.SettledKilled)
	}
	replay.Checks = append(replay.Checks, differs...)
	if record.Route.Kind == format.ScanRouteWitness {
		replay.Checks = append(replay.Checks, s.replayWitness(ctx, ref, record)...)
	}
	return replay, nil
}

// replayBroken replays a broken promise: the mutant builds, its witness passes
// the reference and fails the mutant every run, and every test green on the
// reference passes the mutant three times.
func (s *Service) replayBroken(ctx context.Context, ref reference, copied time.Time, record FindingRecord) (Replay, error) {
	replay := Replay{Test: fmt.Sprintf(brokenPromiseTitle, record.Promise.Quote)}
	replay.Checks = append(replay.Checks, s.replayWitness(ctx, ref, record)...)
	if err := s.runReference(ctx, &ref, copied); err != nil {
		return Replay{}, err
	}
	suite, err := s.wholeSuite(ctx, ref, Candidate{ID: record.Mutant.ID, Edits: mutation.EditSetOf(record.Mutant.Edits)})
	if err != nil {
		return Replay{}, err
	}
	replay.add("the mutant builds", suite != nil)
	replay.add(fmt.Sprintf("the whole suite passes the mutant %s", allRuns), passedAll(suite))
	return replay, nil
}

// brokenPromiseTitle names a replayed broken promise.
const brokenPromiseTitle = "broken promise %q"

// replayWitness lays out the witness file of a witness_failure route as the
// scan did, and runs it alone three times on the reference, which it must
// pass, and three times on the mutant, which it must fail.
func (s *Service) replayWitness(ctx context.Context, ref reference, record FindingRecord) []Check {
	id := record.Route.Witness
	short := id[:min(len(id), witnessShortLength)]
	if s.deps.Witnesses == nil {
		return []Check{{Claim: "the ecosystem runs witnesses", Held: false}}
	}
	set, err := s.briefedSet(ctx, record.Promises, record.Witnesses)
	if err != nil {
		return []Check{{Claim: fmt.Sprintf("witness %s lays out: %v", short, err), Held: false}}
	}
	runner := s.deps.Witnesses.For(WitnessReference{Executor: ref.executor, Reference: ref.referenceOf()}, nil, "")
	onReference, referenceErr := runner.Repeat(ctx, emptyEdits, set)
	onMutant, mutantErr := runner.Repeat(ctx, mutation.EditSetOf(record.Mutant.Edits), set)
	return []Check{
		{Claim: fmt.Sprintf("witness %s passes the reference %s", short, allRuns), Held: referenceErr == nil && every(onReference[id], witness.OutcomePassed)},
		{Claim: fmt.Sprintf("witness %s fails the mutant %s", short, allRuns), Held: mutantErr == nil && every(onMutant[id], witness.OutcomeFailed)},
	}
}

// witnessShortLength is how many characters of a witness id a replay shows.
const witnessShortLength = 12

// every reports whether there are as many runs as a settled outcome takes and
// each has the outcome.
func every(runs []witness.Outcome, want witness.Outcome) bool {
	return len(runs) == vacuity.RunsPerOutcome && !slices.ContainsFunc(runs, func(run witness.Outcome) bool { return run != want })
}

// passedAll reports whether every run of the whole suite passed.
func passedAll(runs []witness.Outcome) bool { return every(runs, witness.OutcomePassed) }

// eachAlone runs each test three times in a selection of its own, so that no
// test of a replay runs beside another.
func (s *Service) eachAlone(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ecosystem.TestID) (map[ecosystem.TestID][]vacuity.Run, error) {
	runs := make(map[ecosystem.TestID][]vacuity.Run, len(tests))
	for _, test := range tests {
		ran, err := s.deps.Suite.Run(ctx, workspace, toolchain, ports.ScanRunRequest{Tests: []ecosystem.TestID{test}, Runs: vacuity.RunsPerOutcome, Timeout: ReferenceTimeout})
		if err != nil {
			return nil, err
		}
		runs[test] = ran.Runs[test]
	}
	return runs, nil
}

func (r *Replay) add(claim string, held bool) {
	r.Checks = append(r.Checks, Check{Claim: claim, Held: held})
}

// replayDifference calls the route's operation on its input twice on the
// reference and twice on the mutant: each must give the result the route
// recorded.
func (s *Service) replayDifference(ctx context.Context, ref reference, mutant ecosystem.Workspace, route format.ScanRoute) []Check {
	var args map[string]spec.Literal
	var reference, changed spec.Observation
	decoded := json.Unmarshal([]byte(route.Input), &args) == nil &&
		json.Unmarshal([]byte(route.Reference), &reference) == nil &&
		json.Unmarshal([]byte(route.Mutant), &changed) == nil
	if !decoded || s.deps.Prober == nil {
		return []Check{{Claim: "the difference can be called", Held: false}}
	}
	probe := []ports.ProbeCall{{Target: route.Operation, Args: args}}
	return []Check{
		{Claim: "the reference gives the recorded result twice", Held: s.observesTwice(ctx, ref.workspace, ref.toolchain, probe, reference)},
		{Claim: "the mutant gives the recorded result twice", Held: s.observesTwice(ctx, mutant, ref.toolchain, probe, changed)},
	}
}

func (s *Service) observesTwice(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, probe []ports.ProbeCall, want spec.Observation) bool {
	for range differenceRuns {
		observed, err := s.deps.Prober.Invoke(ctx, workspace, toolchain, probe)
		if err != nil || len(observed) != 1 || !observed[0].Same(want) {
			return false
		}
	}
	return true
}

// differenceRuns is how many times each side of a difference runs.
const differenceRuns = 2

// allRuns writes "every run of a settled outcome", as in 3/3.
var allRuns = fmt.Sprintf("%d/%d", vacuity.RunsPerOutcome, vacuity.RunsPerOutcome)
