package prove

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/criterion"
	"github.com/RachidChabane/musthold/completeness/internal/domain/justify"
	"github.com/RachidChabane/musthold/completeness/internal/domain/reach"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// UseCase is the `prove` use case over its ports and settings.
type UseCase struct {
	Deps     Deps
	Settings Settings
}

// Run proves the specs one pass at a time. Each pass recomputes the deterministic
// stages, asks the host for what it needs and writes a Statement for every spec
// it can finish. With a replayer the passes follow one another in this invocation
// until nothing is pending.
func (u UseCase) Run(ctx context.Context, options Options) (Result, error) {
	if err := u.Settings.Policy.Validate(); err != nil {
		return Result{}, err
	}
	options, err := u.resumedOptions(options)
	if err != nil {
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

// baseMemoKey addresses, in the memo, the base the last run that was not a resume
// measured the change from.
var baseMemoKey = format.SHA256Hex([]byte("completeness:resume-base"))

// rememberedBase is what the memo keeps of the base of a run.
type rememberedBase struct {
	Base string `json:"base"`
}

// resumedOptions makes a resume continue with the base of the run it continues,
// so that `prove --base X` followed by a bare `prove --resume` does not measure the
// change from another base and leave the first run's tickets behind. A run that is
// not a resume records its base, empty for the default.
func (u UseCase) resumedOptions(options Options) (Options, error) {
	if !options.Resume {
		raw, err := json.Marshal(rememberedBase{Base: options.Base})
		if err != nil {
			return options, err
		}
		return options, u.Deps.Memo.Put(baseMemoKey, raw)
	}
	if options.Base != "" {
		return options, nil
	}
	raw, found, err := u.Deps.Memo.Get(baseMemoKey)
	if err != nil || !found {
		return options, nil
	}
	var remembered rememberedBase
	if json.Unmarshal(raw, &remembered) == nil {
		options.Base = remembered.Base
	}
	return options, nil
}

// passState is what one pass shares across its specs.
type passState struct {
	deps         Deps
	settings     Settings
	options      Options
	project      criterion.Project
	policyDigest string
	bridge       *hostbridge.Bridge
	findings     []string
	files        map[string]fileDigest

	graph reach.Graph
	// units are the units of every source file of the project.
	units   []code.Unit
	changes changeSummary
	// powers maps a criterion to what its discriminating-power Statement says.
	powers map[criterion.Ref]powerEvidence
	// evidence lists the probes and tests that justify changed units.
	evidence []justify.Evidence
	// justifiers maps a unit key to the criteria that justify it.
	justifiers map[string][]criterion.Ref
	// proofInputs are the `proof` inputs of the Statements the pass read.
	proofInputs []format.Input
	// closed memoizes the answers to the closed requests of the pass.
	closed map[string]*closedSignal
}

func (p *passState) addFinding(text string) { p.findings = append(p.findings, text) }

// pass is one wave: every selected spec is advanced as far as the host's answers
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
		deps: u.Deps, settings: u.Settings, options: options, project: report.Project, policyDigest: policyDigest,
		bridge: hostbridge.New(u.Deps.Store, u.Deps.Clock, u.Deps.Entropy, hostproto.SessionProve, wave),
		files:  map[string]fileDigest{}, closed: map[string]*closedSignal{},
	}
	if err := pass.analyse(ctx); err != nil {
		return Result{}, err
	}
	result := Result{Wave: wave}
	for _, spec := range report.Project.Specs {
		proved, err := pass.proveSpec(spec)
		if err != nil {
			return Result{}, fmt.Errorf("prove %s: %w", spec.Path, err)
		}
		result.Specs = append(result.Specs, proved)
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

// analyse runs the deterministic stages that do not depend on the host: the
// units of the project, the change since the base, the Statements of the other
// modules and what they justify.
func (p *passState) analyse(ctx context.Context) error {
	if err := p.loadSources(); err != nil {
		return err
	}
	if err := p.loadChanges(ctx); err != nil {
		return err
	}
	if err := p.loadEvidence(); err != nil {
		return err
	}
	p.justifiers = justify.Justifiers(p.graph, p.evidence)
	return nil
}

// writePending tells Shallnot what the module still waits for: the criteria of
// the specs that have tickets in flight, and those whose host stopped answering.
func (u UseCase) writePending(result Result) error {
	reasons := map[string]map[format.PendingReason]bool{}
	for _, spec := range result.Specs {
		for _, name := range spec.criteria {
			if reasons[name] == nil {
				reasons[name] = map[format.PendingReason]bool{}
			}
			switch {
			case spec.Status == StatusAwaitingHost:
				reasons[name][format.PendingAwaitingHost] = true
			case spec.Unresponsive:
				reasons[name][format.PendingHostUnresponsive] = true
			}
		}
	}
	pending := format.Pending{Module: u.Settings.PendingModule}
	for _, name := range slices.Sorted(maps.Keys(reasons)) {
		for _, reason := range format.PendingReasons() {
			if reasons[name][reason] {
				pending.Pending = append(pending.Pending, format.PendingEntry{Criterion: name, Reason: reason})
			}
		}
	}
	if err := u.Deps.Pending.Write(pending); err != nil {
		return fmt.Errorf("write the pending file: %w", err)
	}
	return nil
}
