package prove

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// discriminatingPowerModule is the module name whose pending file says which
// criteria the discriminating-power proof still waits for.
const discriminatingPowerModule = "discpower"

// UseCase is the `prove` use case over its ports and settings.
type UseCase struct {
	Deps     Deps
	Settings Settings
}

// Run proves the tests one pass at a time. Each pass recomputes the deterministic
// stages, asks the host for what it needs and writes a Statement for every test
// it can finish. With a replayer the passes follow one another in this
// invocation until nothing is pending.
func (u UseCase) Run(ctx context.Context, options Options) (Result, error) {
	if err := u.Settings.Policy.Validate(); err != nil {
		return Result{}, err
	}
	for {
		result, err := u.pass(ctx, options)
		if err != nil {
			return result, err
		}
		if err := u.writePending(result); err != nil {
			return result, err
		}
		if result.PendingTickets == 0 || u.Deps.Replay == nil {
			return result, nil
		}
		if err := u.Deps.Replay.Serve(result.pending); errors.Is(err, requests.ErrNothingServed) {
			return result, nil
		} else if err != nil {
			return result, err
		}
	}
}

// pass is one wave: every selected test is advanced as far as the host's answers
// allow.
func (u UseCase) pass(ctx context.Context, options Options) (Result, error) {
	report, err := u.Deps.Criteria.Load(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("load the criteria: %w", err)
	}
	wave, err := u.Deps.Store.AdvanceWave(hostproto.SessionProve)
	if err != nil {
		return Result{}, fmt.Errorf("advance the wave: %w", err)
	}
	policyDigest, err := u.Settings.Policy.Digest()
	if err != nil {
		return Result{}, err
	}
	pass := &passState{
		deps: u.Deps, settings: u.Settings, project: report.Project, policyDigest: policyDigest, options: options,
		bridge: hostbridge.New(u.Deps.Store, u.Deps.Clock, u.Deps.Entropy, hostproto.SessionProve, wave), proofs: map[criterion.Ref]*chosenProof{},
		files: map[string]fileDigest{},
	}
	if err := pass.loadProofPending(); err != nil {
		return Result{}, err
	}
	result := Result{Wave: wave}
	for _, subject := range report.Project.Subjects() {
		proved, err := pass.proveSubject(subject)
		if err != nil {
			return Result{}, fmt.Errorf("prove %s: %w", subject.Test.Label(), err)
		}
		result.Tests = append(result.Tests, proved...)
	}
	result.Findings = slices.Clone(pass.findings)
	pending, err := pass.bridge.FinishWave()
	if err != nil {
		return Result{}, err
	}
	result.PendingTickets = len(pending)
	for _, ticket := range pending {
		result.pending = append(result.pending, requests.PendingTicket{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce})
	}
	return result, nil
}

// writePending tells Shallnot what the module still waits for: the criteria whose
// tests have tickets in flight, those whose verdict waits for the
// discriminating-power proof, and those whose host stopped answering.
func (u UseCase) writePending(result Result) error {
	reasons := map[string]map[format.PendingReason]bool{}
	mark := func(ref criterion.Ref, reason format.PendingReason) {
		key := ref.String()
		if reasons[key] == nil {
			reasons[key] = map[format.PendingReason]bool{}
		}
		reasons[key][reason] = true
	}
	for _, test := range result.Tests {
		switch {
		case test.Status == StatusAwaitingHost:
			mark(test.Criterion, format.PendingAwaitingHost)
		case test.Status == StatusAwaitingProof:
			mark(test.Criterion, format.PendingAwaitingProof)
		case test.Reason == reasonHostUnresponsive:
			mark(test.Criterion, format.PendingHostUnresponsive)
		}
	}
	pending := format.Pending{Module: u.Settings.PendingModule}
	for _, criterionName := range slices.Sorted(maps.Keys(reasons)) {
		for _, reason := range format.PendingReasons() {
			if reasons[criterionName][reason] {
				pending.Pending = append(pending.Pending, format.PendingEntry{Criterion: criterionName, Reason: reason})
			}
		}
	}
	if err := u.Deps.Pending.Write(pending); err != nil {
		return fmt.Errorf("write the pending file: %w", err)
	}
	return nil
}
