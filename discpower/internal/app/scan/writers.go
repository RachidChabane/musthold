package scan

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

// Witnesses of a promise.
const (
	// promiseRefPrefix names the criterion a promise stands for in its
	// witnesses, followed by the promise id.
	promiseRefPrefix = "promise-"
	// promiseRefRevision is the revision of every promise's criterion.
	promiseRefRevision = 1
	// writerDraw is the one draw of writers a promise gets.
	writerDraw = 1
	// witnessSeedTag seeds the writers of a promise.
	witnessSeedTag = "scan-witness"
	// specLineFormat writes one promise of the witness spec under its label.
	specLineFormat = "%s: %s"
	// specJoin separates the promises of the witness spec.
	specJoin = "\n\n"
	// commitDetailSeparator separates the parts of a witness set's journal
	// detail.
	commitDetailSeparator = " "
	// contradictionUnusableWitnesses is the ground on which a writer's answer
	// is rejected: the set is not 3 to 6 examples and 1 to 3 properties.
	contradictionUnusableWitnesses = "unusable_witnesses"
)

// promiseState is what the stages of one invocation establish about the
// promises.
type promiseState struct {
	// checked is set once the extraction is answered, or when there is no
	// source to extract from.
	checked bool
	// noSources is set when the scan has no quotable source.
	noSources bool
	// extractionFailed is set when the extraction went unanswered.
	extractionFailed bool
	extracted        []promise.Extracted
	quotes           promise.QuoteCheck
	choice           promise.Choice
	// checking are the chosen promises whose writers were asked, in kept order.
	checking []promise.Kept
	// witnessesUnavailable is set for an ecosystem that writes no witness.
	witnessesUnavailable bool
	brief                briefOf
	witnesses            map[string]*promiseWitnesses
	// views are the code each promise's generation tickets show, and view the
	// code of every promise, which the labels name.
	views map[string][]viewUnit
	view  []viewUnit
	// generation are the generation tickets with their answers.
	generation []generationTicket
	labels     promise.Labels
}

func newPromiseState() *promiseState {
	return &promiseState{witnesses: map[string]*promiseWitnesses{}, views: map[string][]viewUnit{}}
}

// briefOf is the witness spec and the brief its writers are given.
type briefOf struct {
	spec     string
	language string
	style    string
	iface    []string
	helpers  []string
	labels   prompt.Labels
}

// promiseWitnesses are the witness sets of one promise and what became of each
// of its witnesses, in the order the writers answered.
type promiseWitnesses struct {
	sets    []witness.Set
	order   []string
	records map[string]*witnessRecord
}

// witnessRecord is what a scan knows of one witness.
type witnessRecord struct {
	witness witness.Witness
	imports string
	// admission are its runs on the reference that admit it.
	admission []witness.Outcome
	admitted  bool
	question  promise.QuestionReason
	// final are its runs of the final stage, and stable is set when all six
	// runs passed.
	final  []witness.Outcome
	stable bool
	votes  []witness.ScopeVote
}

// standing is the record as the domain reads it.
func (w *witnessRecord) standing(promiseID string) promise.WitnessStanding {
	return promise.WitnessStanding{ID: w.witness.ID, Promise: promiseID, Admitted: w.admitted, Stable: w.stable, Votes: w.votes}
}

// promiseRef is the criterion a promise stands for in its witnesses.
func promiseRef(id string) spec.Ref {
	return spec.Ref{ID: promiseRefPrefix + id, Revision: promiseRefRevision}
}

// witnessSpec writes the promises the writers are told of, each under the
// label of its position, as a spec of requirements.
func witnessSpec(promises []format.ScanPromise) (string, prompt.Labels) {
	refs := make([]spec.Ref, len(promises))
	for index, one := range promises {
		refs[index] = promiseRef(one.ID)
	}
	labels := prompt.NewLabels(refs)
	lines := make([]string, len(promises))
	for index, one := range promises {
		lines[index] = fmt.Sprintf(specLineFormat, labels.Of(refs[index]), one.Quote)
	}
	return strings.Join(lines, specJoin), labels
}

// write checks the extracted quotes against the saved sources, keeps at most
// three promises and asks two blind writers for each, as the budget allows.
func (r *hostRun) write(ctx context.Context) (bool, error) {
	if len(r.state.extracted) == 0 {
		return true, nil
	}
	saved := map[string]promise.Source{}
	for _, source := range r.sources {
		saved[source.Name] = source
	}
	r.state.quotes = promise.CheckQuotes(r.state.extracted, saved)
	r.state.choice = promise.Choose(r.state.quotes.Kept)
	chosen := r.state.choice.Chosen
	if len(chosen) == 0 {
		return true, nil
	}
	if r.service.deps.WitnessHost == nil || r.service.deps.Witnesses == nil {
		r.state.witnessesUnavailable, r.state.checking = true, chosen
		return true, nil
	}
	if err := r.briefWriters(ctx, chosen); err != nil {
		return false, err
	}
	asks := make([]hostAsk, len(chosen))
	for index, kept := range chosen {
		request, err := r.writerRequest(kept.Promise)
		if err != nil {
			return false, err
		}
		asks[index] = newHostAsk(request, promise.Writer(kept.Promise.ID, 0), promise.Writer(kept.Promise.ID, 1))
	}
	answers, err := r.asker.stage(asks, promise.IDs(chosen))
	if err != nil {
		return false, err
	}
	settled := true
	for index, answer := range answers {
		if answer == nil {
			continue
		}
		r.state.checking = append(r.state.checking, chosen[index])
		done, err := r.readWriters(chosen[index].Promise.ID, asks[index], answer)
		if err != nil {
			return false, err
		}
		settled = settled && done
	}
	return settled, nil
}

// briefWriters writes the witness spec of the chosen promises and asks the
// witness host for the brief of their writers. The brief is asked last before
// any witness is laid out, since an ecosystem may lay witnesses out where the
// brief places them.
func (r *hostRun) briefWriters(ctx context.Context, chosen []promise.Kept) error {
	promises := make([]format.ScanPromise, len(chosen))
	for index, kept := range chosen {
		promises[index] = kept.Promise
	}
	text, labels := witnessSpec(promises)
	code, err := r.codeOf(ctx)
	if err != nil {
		return err
	}
	brief, err := r.service.deps.WitnessHost.Brief(r.service.deps.Project, code.files, text)
	if err != nil {
		return fmt.Errorf("brief the witness writers: %w", err)
	}
	r.state.brief = briefOf{spec: text, language: brief.Language, style: brief.Conventions, iface: brief.Interface, helpers: brief.Helpers, labels: labels}
	return nil
}

// writerRequest asks two blind writers for the witnesses of a promise: they
// see the promises as a spec, the quote to check, the public signatures of the
// code and the signatures of the test helpers, never a body, a test, a diff or
// a mutant.
func (r *hostRun) writerRequest(kept format.ScanPromise) (hostproto.Request, error) {
	brief := r.state.brief
	ref := promiseRef(kept.ID)
	built := prompt.BuildWitness(prompt.WitnessInput{
		Spec: brief.spec, Target: brief.labels.Of(ref), Statement: kept.Quote,
		Language: brief.language, Conventions: brief.style, Interface: brief.iface, Helpers: brief.helpers,
	})
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: digestLines([]string{kept.Quote})},
		{Role: format.RoleSpec, Digest: digestLines([]string{brief.spec})},
		{Role: format.RoleCodeTree, Digest: digestLines(brief.iface)},
		{Role: format.RoleTestSource, Digest: digestLines(brief.helpers)},
	}
	return r.assemble(hostDraft{kind: hostproto.KindGenerateWitness, built: built, inputs: inputs, round: writerDraw,
		seed: digestLines([]string{witnessSeedTag, kept.ID})})
}

// readWriters reads every vote of a writers' answer into a set and journals
// its digest before it runs anywhere. A vote whose set is unusable is
// rejected and asked again.
func (r *hostRun) readWriters(promiseID string, ask hostAsk, answer *hostbridge.Answer) (bool, error) {
	if pending(answer) {
		return false, nil
	}
	if !answered(answer) {
		return true, nil
	}
	own := &promiseWitnesses{records: map[string]*witnessRecord{}}
	r.state.witnesses[promiseID] = own
	ref := promiseRef(promiseID)
	var unusable []hostbridge.VoteState
	for _, vote := range answer.Votes {
		set, err := prompt.ParseWitnesses(vote.Output, ref)
		if err != nil {
			unusable = append(unusable, vote)
			continue
		}
		set.Writer = witness.Writer{RequestID: string(answer.ID), Vote: vote.Ticket.Vote, Draw: writerDraw, Isolation: vote.Executor.Isolation, Marked: vote.Marked}
		detail := strings.Join([]string{ref.String(), fmt.Sprint(writerDraw), set.Digest()}, commitDetailSeparator)
		if _, err := r.bridge.Commit(hostproto.EventWitnessSetCommitted, detail); err != nil {
			return false, err
		}
		own.add(set)
	}
	if len(unusable) > 0 {
		return r.contradictVotes(ask, unusable, contradictionUnusableWitnesses)
	}
	return true, r.validate(*answer)
}

// add keeps a set and each witness it holds that no earlier set of the promise
// holds: two writers may write the same test, and it counts once.
func (p *promiseWitnesses) add(set witness.Set) {
	p.sets = append(p.sets, set)
	for _, w := range set.Witnesses {
		if _, known := p.records[w.ID]; known {
			continue
		}
		p.order = append(p.order, w.ID)
		p.records[w.ID] = &witnessRecord{witness: w, imports: set.Imports}
	}
}

// contradictVotes rejects the votes execution contradicts and asks the
// request again, which reissues them while emissions remain; the stage is
// settled only when the cap left them unasked.
func (r *hostRun) contradictVotes(ask hostAsk, votes []hostbridge.VoteState, reason string) (bool, error) {
	for _, vote := range votes {
		if err := r.bridge.Contradict(vote, reason); err != nil {
			return false, err
		}
	}
	again, err := r.asker.want(ask)
	if err != nil {
		return false, err
	}
	return !pending(&again), nil
}

// checkingIDs lists the ids of the promises whose writers were asked.
func (s *promiseState) checkingIDs() []string { return promise.IDs(s.checking) }

// recordsOf lists the witness records of a promise in the order the writers
// answered.
func (s *promiseState) recordsOf(promiseID string) []*witnessRecord {
	own, found := s.witnesses[promiseID]
	if !found {
		return nil
	}
	records := make([]*witnessRecord, len(own.order))
	for index, id := range own.order {
		records[index] = own.records[id]
	}
	return records
}

// runnableSets lists the sets of a promise that hold an admitted witness: a
// set runs whole, as its writer wrote it, so that its file builds as it did on
// the reference.
func (s *promiseState) runnableSets(promiseID string, holds func(*witnessRecord) bool) []witness.Set {
	own, found := s.witnesses[promiseID]
	if !found {
		return nil
	}
	var sets []witness.Set
	for _, set := range own.sets {
		if slices.ContainsFunc(set.Witnesses, func(w witness.Witness) bool { return holds(own.records[w.ID]) }) {
			sets = append(sets, set)
		}
	}
	return sets
}
