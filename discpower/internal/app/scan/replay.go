package scan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
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
// test that passes the change, the mutant with its edits and the route that
// proves it changes behaviour.
type FindingRecord struct {
	Commit string            `json:"commit"`
	Runner string            `json:"runner"`
	Test   string            `json:"test"`
	Mutant format.ScanMutant `json:"mutant"`
	Route  format.ScanRoute  `json:"route"`
}

// RecordOf is the replay record of a finding of a report.
func RecordOf(report Report, finding Finding) FindingRecord {
	return FindingRecord{Commit: report.Commit, Runner: string(report.Runner), Test: string(finding.Test), Mutant: finding.Mutant, Route: finding.Route}
}

// DecodeFinding reads a finding record.
func DecodeFinding(data []byte) (FindingRecord, error) {
	var record FindingRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return FindingRecord{}, fmt.Errorf("%w: %v", ErrUnreadableFinding, err)
	}
	if record.Test == "" || len(record.Mutant.Edits) == 0 || record.Route.Kind == "" {
		return FindingRecord{}, ErrUnreadableFinding
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
// route still proves the change.
func (s *Service) Replay(ctx context.Context, root string, record FindingRecord) (Replay, error) {
	commit, err := s.commitOf(ctx)
	if err != nil {
		return Replay{}, err
	}
	if commit != record.Commit {
		return Replay{}, fmt.Errorf("%w: at %s, the finding is of %s", ErrOtherCommit, commit, record.Commit)
	}
	ref, err := s.buildReference(ctx, root)
	if err != nil {
		return Replay{}, err
	}
	defer func() { _ = s.deps.Workspaces.Release(ref.workspace) }()
	tests := []ecosystem.TestID{ecosystem.TestID(record.Test)}
	if record.Route.Kind == format.ScanRouteKilledByTest {
		tests = append(tests, ecosystem.TestID(record.Route.Test))
	}
	replay := Replay{Test: record.Test}
	onReference, err := s.deps.Suite.Run(ctx, ref.workspace, ref.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: ReferenceTimeout})
	if err != nil {
		return Replay{}, fmt.Errorf("run the reference: %w", err)
	}
	for _, test := range tests {
		replay.add(fmt.Sprintf("%s passes the reference %s", test, allRuns), vacuity.Green(onReference.Runs[test]))
	}
	var onMutant ports.ScanRuns
	var differs []Check
	status, err := ref.executor.Mutate(ctx, ref.referenceOf(), mutation.EditSetOf(record.Mutant.Edits), func(ctx context.Context, mutant ecosystem.Workspace) error {
		var runErr error
		onMutant, runErr = s.deps.Suite.Run(ctx, mutant, ref.toolchain, ports.ScanRunRequest{Tests: tests, Runs: vacuity.RunsPerOutcome, Timeout: ReferenceTimeout})
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
	replay.add(fmt.Sprintf("%s passes the mutant %s", record.Test, allRuns), vacuity.Settle(onMutant.Runs[tests[0]]) == format.ScanSurvived)
	if record.Route.Kind == format.ScanRouteKilledByTest {
		replay.add(fmt.Sprintf("%s fails the mutant %s", record.Route.Test, allRuns), vacuity.Settle(onMutant.Runs[tests[1]]) == format.ScanKilled)
	}
	replay.Checks = append(replay.Checks, differs...)
	if record.Route.Kind == format.ScanRouteWitness {
		replay.add("a witness route needs the host's witness, which this replay does not hold", false)
	}
	return replay, nil
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
