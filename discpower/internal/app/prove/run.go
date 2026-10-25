package prove

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
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
		if err := u.Deps.Replay.Serve(result.pending); errors.Is(err, requests.ErrNothingServed) {
			return result, nil
		} else if err != nil {
			return result, err
		}
	}
}

// pass is one wave. Every criterion first draws its witnesses; while one of them
// still waits for its writers or its scope judges, no criterion goes further, so
// that every footprint and every search sees the witnesses of every criterion.
// Then each criterion searches its pool. Once every search ended, the pools are
// run against the witnesses of every criterion and settled, and the criteria
// whose survivors are judged are proved.
func (u UseCase) pass(ctx context.Context, p *project, options Options) (Result, error) {
	wave, err := u.Deps.Store.AdvanceWave(hostproto.SessionProve)
	if err != nil {
		return Result{}, fmt.Errorf("advance the wave: %w", err)
	}
	bridge := hostbridge.New(u.Deps.Store, u.Deps.Clock, u.Deps.Entropy, hostproto.SessionProve, wave)
	p.scopes = nil
	runs := make([]*critRun, len(p.criteria))
	settled := true
	for index, criterion := range p.criteria {
		runs[index] = p.newCritRun(bridge, criterion)
		if err := runs[index].firstDraw(ctx); err != nil {
			return Result{}, fmt.Errorf("prove %s: %w", criterion.Ref, err)
		}
		settled = settled && runs[index].witnessesSettled()
	}
	result := Result{Wave: wave}
	if settled {
		if result.Criteria, err = p.proveAll(ctx, runs, options); err != nil {
			return Result{}, err
		}
	} else {
		for _, run := range runs {
			result.Criteria = append(result.Criteria, CriterionResult{Ref: run.c.Ref, Status: StatusAwaitingHost})
		}
	}
	result.Findings = slices.Clone(p.findings)
	for _, run := range runs {
		result.Findings = append(result.Findings, run.readingConflicts()...)
	}
	pending, err := bridge.FinishWave()
	if err != nil {
		return Result{}, err
	}
	result.PendingTickets = len(pending)
	for _, ticket := range pending {
		result.pending = append(result.pending, requests.PendingTicket{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce})
	}
	return result, nil
}

// freshRecord remembers the Statement written for a freshness key.
type freshRecord struct {
	CacheKey  string     `json:"cacheKey"`
	Result    string     `json:"result"`
	Survivors []Survivor `json:"survivors,omitempty"`
}

// proveAll runs the searches of every criterion, then settles the pools against
// the witnesses of every criterion and proves the criteria that need no more
// answers. A criterion whose Statement of the current inputs exists is reported
// fresh under `--changed`; its search still runs, so that its witnesses take
// their part in every other criterion's proof.
func (p *project) proveAll(ctx context.Context, runs []*critRun, options Options) ([]CriterionResult, error) {
	for _, run := range runs {
		entries, err := run.entriesOf(ctx)
		if err != nil {
			return nil, err
		}
		if run.st, err = p.staticOf(ctx, run.c, entries); err != nil {
			return nil, err
		}
	}
	p.searchSets, p.searchRecords = universeOf(runs, (*critRun).searchDraws)
	searching := false
	for _, run := range runs {
		if err := run.search(ctx); err != nil {
			return nil, fmt.Errorf("prove %s: %w", run.c.Ref, err)
		}
		searching = searching || run.waiting
	}
	results := make([]CriterionResult, len(runs))
	if searching {
		for index, run := range runs {
			results[index] = CriterionResult{Ref: run.c.Ref, Status: StatusAwaitingHost}
		}
		return results, nil
	}
	sets, records := universeOf(runs, (*critRun).allDraws)
	for index, run := range runs {
		result, err := run.conclude(ctx, sets, records, options)
		if err != nil {
			return nil, fmt.Errorf("prove %s: %w", run.c.Ref, err)
		}
		results[index] = result
	}
	return results, nil
}

// universeOf gathers the witnesses of the chosen draws of every criterion: the
// sets a pool mutant runs and the records its classification reads.
func universeOf(runs []*critRun, drawsOf func(*critRun) []*witnessDraw) ([]witness.Set, []witness.Record) {
	var sets []witness.Set
	var records []witness.Record
	for _, run := range runs {
		draws := drawsOf(run)
		sets = append(sets, run.runnableSetsAmong(draws)...)
		records = append(records, recordsOf(draws)...)
	}
	return sets, records
}

// conclude settles the pool of a criterion against the witnesses of every
// criterion, asks the validity votes its unwitnessed survivors need and, once
// they are in, writes its Statement.
func (r *critRun) conclude(ctx context.Context, sets []witness.Set, records []witness.Record, options Options) (CriterionResult, error) {
	if err := r.settle(ctx, sets, records); err != nil {
		return CriterionResult{}, err
	}
	base, err := r.baseInputs()
	if err != nil {
		return CriterionResult{}, err
	}
	freshness, err := r.freshnessKey(base)
	if err != nil {
		return CriterionResult{}, err
	}
	if options.Selection == SelectChanged {
		if record, fresh := r.p.freshRecord(r.c.Ref, freshness); fresh {
			return CriterionResult{Ref: r.c.Ref, Status: StatusFresh, CacheKey: record.CacheKey, Outcome: power.Outcome{Result: power.Result(record.Result)}, Survivors: record.Survivors}, nil
		}
	}
	if r.waiting || !r.creditSettled() {
		return CriterionResult{Ref: r.c.Ref, Status: StatusAwaitingHost}, nil
	}
	return r.finalize(base, freshness)
}

// finalize writes the Statement of a criterion whose pool is settled and consumes
// the answers it rests on.
func (r *critRun) finalize(base []format.Input, freshness string) (CriterionResult, error) {
	correct, exampleFailed := r.p.referencePasses(r.c), r.p.exampleFailsOnReference(r.c.Ref)
	standing := r.standing(correct, exampleFailed)
	evaluation := power.Evaluate(standing, r.g3w())
	outcome := evaluation.Outcome
	inputs := r.finalInputs(base)
	slices.SortFunc(inputs, format.CompareInputs)
	cacheKey, err := format.CacheKey(r.statementSubject(), inputs)
	if err != nil {
		return CriterionResult{}, err
	}
	predicate, err := r.predicate(inputs, cacheKey, correct, evaluation)
	if err != nil {
		return CriterionResult{}, err
	}
	if err := r.writeFacts(standing, cacheKey); err != nil {
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
	survivors := r.survivors(predicate)
	if err := r.p.rememberFresh(freshness, freshRecord{CacheKey: cacheKey, Result: string(outcome.Result), Survivors: survivors}); err != nil {
		return CriterionResult{}, err
	}
	return CriterionResult{Ref: r.c.Ref, Status: StatusProved, Outcome: outcome, CacheKey: cacheKey, Survivors: survivors}, nil
}

// writeFacts keeps the facts record of the Statement: every executed fact and
// answer its outcome rests on, so that the outcome of any test set can be
// recomputed from it.
func (r *critRun) writeFacts(standing power.Standing, cacheKey string) error {
	if r.p.deps.Facts == nil {
		return nil
	}
	reference := make(map[ecosystem.TestID]ecosystem.TestOutcome, len(r.p.referenceOutcomes))
	for test, outcome := range r.p.referenceOutcomes {
		reference[test] = outcome
	}
	encoded, err := power.NewRecord(standing, r.g3w(), reference).Encode()
	if err != nil {
		return fmt.Errorf("encode the facts of %s: %w", r.c.Ref, err)
	}
	if err := r.p.deps.Facts.Write(r.c.Ref, cacheKey, encoded); err != nil {
		return fmt.Errorf("write the facts of %s: %w", r.c.Ref, err)
	}
	return nil
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
			name := requests.RecordingName(string(answer.ID), vote.Ticket.Vote)
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
