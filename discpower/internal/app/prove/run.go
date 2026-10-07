package prove

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/hostbridge"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const freshNamespace = "fresh-proof"

// UseCase is the `prove` use case over its ports and settings.
type UseCase struct {
	Deps     Deps
	Settings Settings
}

// Run proves the criteria one pass at a time. Each pass recomputes the
// deterministic stages, asks the host for what it needs and writes a Statement
// for every criterion it can finish. With a replayer the passes follow one
// another in this invocation until nothing is pending.
func (u UseCase) Run(ctx context.Context, options Options) (Result, error) {
	if err := u.Settings.Policy.Validate(); err != nil {
		return Result{}, err
	}
	p, err := prepare(ctx, u.Deps, u.Settings)
	if err != nil {
		return Result{}, err
	}
	defer p.release()
	for {
		result, err := u.pass(ctx, p, options)
		if err != nil || result.PendingTickets == 0 || u.Deps.Replay == nil {
			return result, err
		}
		if err := u.Deps.Replay.Serve(result.pending); errors.Is(err, ErrNothingServed) {
			return result, nil
		} else if err != nil {
			return result, err
		}
	}
}

// pass is one wave: every selected criterion is advanced as far as the host's
// answers allow.
func (u UseCase) pass(ctx context.Context, p *project, options Options) (Result, error) {
	wave, err := u.Deps.Store.AdvanceWave()
	if err != nil {
		return Result{}, fmt.Errorf("advance the wave: %w", err)
	}
	bridge := hostbridge.New(u.Deps.Store, u.Deps.Clock, u.Deps.Entropy, wave)
	result := Result{Wave: wave}
	for _, criterion := range p.criteria {
		proved, err := p.proveCriterion(ctx, bridge, criterion, options)
		if err != nil {
			return Result{}, err
		}
		result.Criteria = append(result.Criteria, proved)
	}
	result.Findings = slices.Clone(p.findings)
	if err := bridge.SupersedeUnwanted(); err != nil {
		return Result{}, err
	}
	pending, err := bridge.PendingTickets()
	if err != nil {
		return Result{}, err
	}
	result.PendingTickets = len(pending)
	for _, ticket := range pending {
		result.pending = append(result.pending, PendingTicket{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce})
	}
	return result, nil
}

// freshRecord remembers the Statement written for a freshness key.
type freshRecord struct {
	CacheKey string `json:"cacheKey"`
	Result   string `json:"result"`
}

func (p *project) proveCriterion(ctx context.Context, bridge *hostbridge.Bridge, criterion spec.Criterion, options Options) (CriterionResult, error) {
	analysis, err := p.staticOf(ctx, criterion)
	if err != nil {
		return CriterionResult{}, err
	}
	run := p.newCritRun(bridge, criterion, analysis)
	base, err := run.baseInputs()
	if err != nil {
		return CriterionResult{}, err
	}
	freshness, err := run.freshnessKey(base)
	if err != nil {
		return CriterionResult{}, err
	}
	if options.Selection == SelectChanged {
		if record, fresh := p.freshRecord(criterion.Ref, freshness); fresh {
			return CriterionResult{Ref: criterion.Ref, Status: StatusFresh, CacheKey: record.CacheKey, Outcome: power.Outcome{Result: power.Result(record.Result)}}, nil
		}
	}
	if err := run.search(ctx); err != nil {
		return CriterionResult{}, fmt.Errorf("prove %s: %w", criterion.Ref, err)
	}
	if run.waiting {
		return CriterionResult{Ref: criterion.Ref, Status: StatusAwaitingHost}, nil
	}
	return run.finalize(base, freshness)
}

// finalize writes the Statement of a criterion whose pool is settled and consumes
// the answers it rests on.
func (r *critRun) finalize(base []format.Input, freshness string) (CriterionResult, error) {
	correct, exampleFailed := r.p.referencePasses(r.c), r.p.exampleFailsOnReference(r.c.Ref)
	outcome := power.Terminal(r.standing(correct, exampleFailed))
	inputs := r.finalInputs(base)
	slices.SortFunc(inputs, func(a, b format.Input) int {
		return cmp.Or(strings.Compare(string(a.Role), string(b.Role)), strings.Compare(a.Name, b.Name))
	})
	cacheKey, err := format.CacheKey(r.statementSubject(), inputs)
	if err != nil {
		return CriterionResult{}, err
	}
	predicate, err := r.predicate(inputs, cacheKey, correct)
	if err != nil {
		return CriterionResult{}, err
	}
	encoded, err := format.NewStatement(r.statementSubject(), predicate).Encode()
	if err != nil {
		return CriterionResult{}, fmt.Errorf("encode the Statement of %s: %w", r.c.Ref, err)
	}
	if err := r.p.deps.Proofs.Write(r.c.Ref, cacheKey, encoded); err != nil {
		return CriterionResult{}, fmt.Errorf("write the Statement of %s: %w", r.c.Ref, err)
	}
	if err := r.consume(); err != nil {
		return CriterionResult{}, err
	}
	if err := r.p.rememberFresh(freshness, freshRecord{CacheKey: cacheKey, Result: string(outcome.Result)}); err != nil {
		return CriterionResult{}, err
	}
	return CriterionResult{Ref: r.c.Ref, Status: StatusProved, Outcome: outcome, CacheKey: cacheKey}, nil
}

// consume records the answers a written Statement used, and keeps each vote when
// a recorder is attached.
func (r *critRun) consume() error {
	for _, answer := range r.answers {
		if err := r.bridge.Consume(answer); err != nil {
			return err
		}
		if r.p.deps.Record == nil {
			continue
		}
		for _, vote := range answer.Votes {
			name := ports.RecordingName(string(answer.ID), vote.Ticket.Vote)
			if err := r.p.deps.Record.Record(name, vote.Output, vote.Executor); err != nil {
				return fmt.Errorf("record the answer to %s: %w", name, err)
			}
		}
	}
	return nil
}

func (p *project) freshMemoKey(freshness string) string {
	return digestText(freshNamespace + seedSeparator + freshness)
}

// freshRecord returns the record of the Statement written for the freshness key,
// when that Statement still exists.
func (p *project) freshRecord(ref spec.Ref, freshness string) (freshRecord, bool) {
	raw, found, err := p.deps.Memo.Get(p.freshMemoKey(freshness))
	if err != nil || !found {
		return freshRecord{}, false
	}
	var record freshRecord
	if json.Unmarshal(raw, &record) != nil {
		return freshRecord{}, false
	}
	exists, err := p.deps.Proofs.Has(ref, record.CacheKey)
	return record, err == nil && exists
}

func (p *project) rememberFresh(freshness string, record freshRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return p.deps.Memo.Put(p.freshMemoKey(freshness), raw)
}
