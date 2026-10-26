package scan

import (
	"context"
	"fmt"
	"strconv"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// Requests of the admission stage.
const (
	// generationSeedTag seeds the generation tickets.
	generationSeedTag = "scan-generation"
	// suiteKeyTag opens the scan suite key, which names the reference copy the
	// witness facts were observed on.
	suiteKeyTag = "scan-suite"
	// contradictionUnansweredScope is the ground on which a scope answer is
	// rejected: it leaves a witness without a verdict.
	contradictionUnansweredScope = "unanswered_scope"
	// contradictionUnusableLabels is the ground on which a labels answer is
	// rejected: it names a promise or a function the request did not show.
	contradictionUnusableLabels = "unusable_labels"
)

// generationTicket is one generation ticket of a promise with its answer, nil
// when the budget cut it.
type generationTicket struct {
	promise string
	ask     hostAsk
	answer  *hostbridge.Answer
}

// admit runs every witness set on the reference to admit its witnesses, then
// asks two scope votes per promise about its admitted witnesses, the four
// generation tickets, round-robin over the promises, and the labels ticket, as
// the budget allows.
func (r *hostRun) admit(ctx context.Context) (bool, error) {
	if len(r.state.checking) == 0 {
		return true, nil
	}
	if err := r.admitWitnesses(ctx); err != nil {
		return false, err
	}
	if err := r.buildViews(ctx); err != nil {
		return false, err
	}
	scopes, scopeOf, err := r.scopeAsks()
	if err != nil {
		return false, err
	}
	generation, err := r.generationAsks()
	if err != nil {
		return false, err
	}
	labels, labelled, err := r.labelsAsk()
	if err != nil {
		return false, err
	}
	asks := append(append(scopes, generation...), labels...)
	answers, err := r.asker.stage(asks, r.state.checkingIDs())
	if err != nil {
		return false, err
	}
	settled := true
	for index, ask := range scopes {
		done, err := r.readScope(scopeOf[index], ask, answers[index])
		if err != nil {
			return false, err
		}
		settled = settled && done
	}
	for index, ask := range generation {
		answer := answers[len(scopes)+index]
		r.state.generation = append(r.state.generation, generationTicket{promise: ask.asks[0].Promise, ask: ask, answer: answer})
		settled = settled && !pending(answer)
	}
	if len(labels) > 0 {
		done, err := r.readLabels(labels[0], answers[len(answers)-1], labelled)
		if err != nil {
			return false, err
		}
		settled = settled && done
	}
	return settled, nil
}

// suiteKey names the reference copy the witness facts are observed on: the
// file-set digest of the tracked files, the scan's folder aside, and the
// ecosystem.
func (r *hostRun) suiteKey() string {
	return digestLines([]string{suiteKeyTag, r.tree, string(r.service.deps.Ecosystem.Name())})
}

// witnessRunner is the witness runner of the built reference: memoized for the
// admission and the coverage, whose facts the request ids derive from.
func (r *hostRun) witnessRunner(ctx context.Context) (WitnessRunner, error) {
	built, err := r.reference(ctx)
	if err != nil {
		return nil, err
	}
	var memo ports.ExecutionMemo
	if r.service.deps.Host.Memo != nil {
		memo = r.service.deps.Host.Memo
	}
	return r.service.deps.Witnesses.For(WitnessReference{Executor: built.executor, Reference: built.referenceOf()}, memo, r.suiteKey()), nil
}

// admitWitnesses runs each set of each promise alone on the reference three
// times: a witness that passes every run is admitted, one the reference fails
// is a question about its promise.
func (r *hostRun) admitWitnesses(ctx context.Context) error {
	if r.state.witnessesUnavailable {
		return nil
	}
	runner, err := r.witnessRunner(ctx)
	if err != nil {
		return err
	}
	for _, kept := range r.state.checking {
		own, found := r.state.witnesses[kept.Promise.ID]
		if !found {
			continue
		}
		for _, set := range own.sets {
			runs, err := runner.Reference(ctx, set)
			if err != nil {
				return fmt.Errorf("admit the witnesses of promise %s: %w", kept.Promise.ID, err)
			}
			for _, w := range set.Witnesses {
				record := own.records[w.ID]
				if record.admission != nil {
					continue
				}
				record.admission = runs[w.ID]
				decision := promise.Admit(record.admission)
				record.admitted, record.question = decision.Holds, decision.Question
			}
		}
	}
	return nil
}

// admitted reports whether the reference passed every admission run.
func admitted(record *witnessRecord) bool { return record != nil && record.admitted }

// buildViews takes, for each promise, the code its admitted witnesses run on
// the reference, and the code of every promise together, each bounded on its
// own by the size bound of a view.
func (r *hostRun) buildViews(ctx context.Context) error {
	seen := map[string]bool{}
	var union []viewUnit
	r.state.view, r.state.truncated = nil, nil
	for _, kept := range r.state.checking {
		entries, err := r.entriesOf(ctx, kept.Promise.ID)
		if err != nil {
			return err
		}
		full, err := r.viewOf(ctx, entries)
		if err != nil {
			return err
		}
		r.state.views[kept.Promise.ID] = r.state.bound(kept.Promise.ID, full)
		for _, unit := range full {
			if !seen[unit.key()] {
				seen[unit.key()] = true
				union = append(union, unit)
			}
		}
	}
	r.state.view = r.state.bound(labelsView, union)
	return nil
}

// labelsView names the view of the labels ticket among the truncated views.
const labelsView = ""

// TruncatedView is a view the size bound cut: the promise whose generation
// tickets show it, empty for the view the labels ticket shows, and how many
// of its units it shows out of how many.
type TruncatedView struct {
	Promise string
	Shown   int
	Total   int
}

// OfLabels reports whether the view is the one the labels ticket shows.
func (v TruncatedView) OfLabels() bool { return v.Promise == labelsView }

// bound keeps a view within the size bound and records it when the bound cut
// it.
func (s *promiseState) bound(promiseID string, full []viewUnit) []viewUnit {
	view, omitted := boundedView(full)
	if omitted > 0 {
		s.truncated = append(s.truncated, TruncatedView{Promise: promiseID, Shown: len(view), Total: len(full)})
	}
	return view
}

// omittedFrom counts the units the size bound left out of a view.
func (s *promiseState) omittedFrom(promiseID string) int {
	for _, truncated := range s.truncated {
		if truncated.Promise == promiseID {
			return truncated.Total - truncated.Shown
		}
	}
	return 0
}

// entriesOf are the units the admitted witnesses of a promise run on the
// reference: none when no witness was admitted or the ecosystem records no
// coverage.
func (r *hostRun) entriesOf(ctx context.Context, promiseID string) ([]footprint.Ref, error) {
	sets := r.state.runnableSets(promiseID, admitted)
	if len(sets) == 0 || r.state.witnessesUnavailable {
		return nil, nil
	}
	runner, err := r.witnessRunner(ctx)
	if err != nil {
		return nil, err
	}
	entries, err := runner.Entries(ctx, sets)
	if err != nil {
		return nil, fmt.Errorf("record the coverage of the witnesses of promise %s: %w", promiseID, err)
	}
	return entries, nil
}

// scopeAsks asks two scope votes per promise about its admitted witnesses.
func (r *hostRun) scopeAsks() ([]hostAsk, []string, error) {
	var asks []hostAsk
	var promises []string
	for _, kept := range r.state.checking {
		shown := r.admittedRecords(kept.Promise.ID)
		if len(shown) == 0 {
			continue
		}
		request, err := r.scopeRequest(kept.Promise, shown)
		if err != nil {
			return nil, nil, err
		}
		asks = append(asks, newHostAsk(request, promise.Scope(kept.Promise.ID, 0), promise.Scope(kept.Promise.ID, 1)))
		promises = append(promises, kept.Promise.ID)
	}
	return asks, promises, nil
}

// admittedRecords lists the admitted witnesses of a promise, in the order the
// writers answered.
func (r *hostRun) admittedRecords(promiseID string) []*witnessRecord {
	var shown []*witnessRecord
	for _, record := range r.state.recordsOf(promiseID) {
		if record.admitted {
			shown = append(shown, record)
		}
	}
	return shown
}

// scopeRequest shows the promises as a spec, the quote and every admitted
// witness of the promise, and asks whether the quote alone fixes the output
// each witness asserts.
func (r *hostRun) scopeRequest(kept format.ScanPromise, shown []*witnessRecord) (hostproto.Request, error) {
	brief := r.state.brief
	items := make([]prompt.ScopeWitness, len(shown))
	ids := make([]string, len(shown))
	for index, record := range shown {
		items[index] = prompt.ScopeWitness{Clause: record.witness.Clause, Code: record.witness.Code}
		ids[index] = record.witness.ID
	}
	built := prompt.BuildScope(prompt.ScopeInput{Spec: brief.spec, Target: brief.labels.Of(promiseRef(kept.ID)), Statement: kept.Quote, Witnesses: items})
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: digestLines([]string{kept.Quote})},
		{Role: format.RoleSpec, Digest: digestLines([]string{brief.spec})},
		{Role: format.RoleCodeTree, Digest: digestLines(brief.iface)},
		{Role: format.RoleHostResponse, Digest: digestLines(ids)},
	}
	return r.assemble(hostDraft{kind: hostproto.KindJudgeScope, built: built, inputs: inputs})
}

// readScope records the votes of a scope answer on the witnesses it judged. An
// answer that leaves a witness without a verdict is rejected and asked again.
func (r *hostRun) readScope(promiseID string, ask hostAsk, answer *hostbridge.Answer) (bool, error) {
	if pending(answer) {
		return false, nil
	}
	if !answered(answer) {
		return true, nil
	}
	shown := r.admittedRecords(promiseID)
	labels := make([]string, len(shown))
	for index := range shown {
		labels[index] = prompt.ScopeLabel(index)
	}
	var unusable []hostbridge.VoteState
	votes := make([]map[string]witness.ScopeVote, 0, len(answer.Votes))
	for _, vote := range answer.Votes {
		parsed, err := prompt.ParseScope(vote.Output, labels)
		if err != nil {
			unusable = append(unusable, vote)
			continue
		}
		for label, scope := range parsed {
			scope.Isolation, scope.Marked = vote.Executor.Isolation, vote.Marked
			parsed[label] = scope
		}
		votes = append(votes, parsed)
	}
	if len(unusable) > 0 {
		return r.contradictVotes(ask, unusable, contradictionUnansweredScope)
	}
	for _, parsed := range votes {
		for index, record := range shown {
			record.votes = append(record.votes, parsed[labels[index]])
		}
	}
	return true, r.validate(*answer)
}

// generationAsks gives the generation tickets to the promises round-robin;
// each shows the quote of its promise and the code its admitted witnesses run.
func (r *hostRun) generationAsks() ([]hostAsk, error) {
	quotes := map[string]string{}
	for _, kept := range r.state.checking {
		quotes[kept.Promise.ID] = kept.Promise.Quote
	}
	var asks []hostAsk
	for _, ticket := range promise.AssignGeneration(r.state.checkingIDs()) {
		view := r.state.views[ticket.Promise]
		round := ticket.Slot + 1
		built := prompt.BuildGeneration(prompt.GenerationInput{Statement: quotes[ticket.Promise], Slice: sliceOf(view), Omitted: r.state.omittedFrom(ticket.Promise)})
		inputs := []hostproto.Input{
			{Role: format.RoleSpec, Digest: digestLines([]string{quotes[ticket.Promise]})},
			{Role: format.RoleCodeTree, Digest: digestUnits(view)},
		}
		seed := digestLines([]string{generationSeedTag, strconv.FormatUint(r.request.Seed, seedBase), ticket.Promise, strconv.Itoa(ticket.Slot)})
		request, err := r.assemble(hostDraft{kind: hostproto.KindGenerateTargeted, built: built, inputs: inputs, round: round, seed: seed})
		if err != nil {
			return nil, err
		}
		asks = append(asks, newHostAsk(request, ticket))
	}
	return asks, nil
}

// labelling is what the labels request showed: the promises and the functions,
// each under its opaque id.
type labelling struct {
	promises  map[string]string
	functions map[string]string
}

// labelsAsk asks the one labels ticket: the kept promises and the functions of
// the generation view in, promise to functions out. A scan whose view is empty
// asks none.
func (r *hostRun) labelsAsk() ([]hostAsk, labelling, error) {
	shown := labelling{promises: map[string]string{}, functions: map[string]string{}}
	if len(r.state.view) == 0 {
		return nil, shown, nil
	}
	input := prompt.LabelsInput{Omitted: r.state.omittedFrom(labelsView)}
	for index, kept := range r.state.checking {
		id := prompt.PromiseID(index)
		shown.promises[id] = kept.Promise.ID
		input.Promises = append(input.Promises, prompt.LabelPromise{ID: id, Quote: kept.Promise.Quote})
	}
	var quotes []string
	for _, kept := range r.state.checking {
		quotes = append(quotes, kept.Promise.Quote)
	}
	for index, unit := range r.state.view {
		id := prompt.FunctionID(index)
		shown.functions[id] = unit.key()
		input.Functions = append(input.Functions, prompt.LabelFunction{ID: id, Path: unit.file, Signature: unit.signature(), Code: unit.source})
	}
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: digestLines(quotes)},
		{Role: format.RoleCodeTree, Digest: digestUnits(r.state.view)},
	}
	request, err := r.assemble(hostDraft{kind: hostproto.KindJudgeLabels, built: prompt.BuildLabels(input), inputs: inputs})
	if err != nil {
		return nil, shown, err
	}
	return []hostAsk{newHostAsk(request, promise.LabelTicket())}, shown, nil
}

// readLabels reads the labels answer back into promise ids and function keys;
// a label can only name a kept promise and a function of the view. An answer
// that names anything else is rejected and asked again.
func (r *hostRun) readLabels(ask hostAsk, answer *hostbridge.Answer, shown labelling) (bool, error) {
	if pending(answer) {
		return false, nil
	}
	if !answered(answer) {
		return true, nil
	}
	promiseIDs := make([]string, len(r.state.checking))
	for index := range r.state.checking {
		promiseIDs[index] = prompt.PromiseID(index)
	}
	functionIDs := make([]string, len(r.state.view))
	for index := range r.state.view {
		functionIDs[index] = prompt.FunctionID(index)
	}
	parsed, err := prompt.ParseLabels(answer.Votes[0].Output, promiseIDs, functionIDs)
	if err != nil {
		return r.contradictVotes(ask, answer.Votes, contradictionUnusableLabels)
	}
	raw := map[string][]string{}
	for promiseLabel, functions := range parsed {
		for _, function := range functions {
			raw[shown.promises[promiseLabel]] = append(raw[shown.promises[promiseLabel]], shown.functions[function])
		}
	}
	keys := make([]string, len(r.state.view))
	for index, unit := range r.state.view {
		keys[index] = unit.key()
	}
	r.state.labels = promise.NormalizeLabels(raw, r.state.checkingIDs(), keys)
	return true, r.validate(*answer)
}
