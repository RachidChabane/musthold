package prove

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/operator"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/prompt"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// Witness draws of a criterion and the votes each asks for.
const (
	// firstDraw is asked of every criterion before its search: two writers.
	firstDraw       = 1
	firstDrawVotes  = 2
	furtherDraw     = 2
	furtherDrawVote = 1
	// creditDraw is asked in CI only, of two fresh writers seeded by the CI
	// seed: the only witnesses that earn pass credit there.
	creditDraw      = 3
	creditDrawVotes = 2
	witnessSeedTag  = "witness"
	// Grounds on which a witness or scope answer is rejected.
	contradictionUnusableWitnesses = "unusable_witnesses"
	contradictionUnansweredScope   = "unanswered_scope"
	commitDetailSeparator          = " "
)

// Module findings about witnesses, reported next to the outcomes.
const (
	FindingWitnessRunnerUnavailable = "witness_runner_unavailable"
	FindingReadingConflict          = "reading_conflict"
)

// witnessDraw is one witness request of a criterion: the sets its writers
// returned and what became of their witnesses.
type witnessDraw struct {
	number  int
	sets    []witness.Set
	records map[string]*witness.Record
	// profiles are the executors of the writer of each witness and of the scope
	// judges that answered about it, by witness id.
	profiles map[string][]format.ExecutorProfile
	// settled is set when the writers and the scope judges answered or were
	// abandoned.
	settled bool
}

func newWitnessDraw(number int) *witnessDraw {
	return &witnessDraw{number: number, records: map[string]*witness.Record{}, profiles: map[string][]format.ExecutorProfile{}}
}

// witnessSeed is the seed of a witness draw: the criterion's statement, the draw
// and, for the credit draw alone, the run seed. The first and the further draws
// never depend on the run seed, so CI asks the same requests the developer's host
// answered and the pushed answers serve them; the credit draw a CI run asks is
// fresh.
func (r *critRun) witnessSeed(draw int) string {
	seed := uint64(operator.LocalSeed)
	if draw == creditDraw {
		seed = r.p.settings.Seed
	}
	return digestText(strings.Join([]string{witnessSeedTag, fmt.Sprint(seed), r.c.StatementDigest, fmt.Sprint(draw)}, seedSeparator))
}

// redrawsForCredit reports whether the run asks a credit draw: a run that
// produces its Statements in CI.
func (p *project) redrawsForCredit() bool { return p.producedBy() == format.ProducedByCI }

// drawWitnesses asks the writers of a draw, commits the digest of each set they
// return to the journal, runs the sets on the reference, asks the scope judges
// about every witness the reference passes, then runs the witnesses the
// reference passes on the planted refactors. It returns the draw, settled or
// not.
func (r *critRun) drawWitnesses(ctx context.Context, number, votes int) (*witnessDraw, error) {
	draw := newWitnessDraw(number)
	request, err := r.witnessRequest(number)
	if err != nil {
		return nil, err
	}
	ask := hostbridge.Ask{Request: request, Votes: votes, Criteria: []string{r.c.Ref.String()}, Purpose: string(hostproto.KindGenerateWitness)}
	answer, state, err := r.ask(ask, billSchedule)
	if err != nil || state == askPending {
		return draw, err
	}
	if state != askAnswered {
		draw.settled = true
		return draw, nil
	}
	contradicted, err := r.readSets(draw, answer)
	if err != nil || contradicted {
		if err == nil {
			_, _, err = r.ask(ask, billNone)
		}
		return draw, err
	}
	if err := r.bridge.Validate(answer); err != nil {
		return nil, err
	}
	r.answers = append(r.answers, answer)
	if err := r.runReference(ctx, draw); err != nil {
		return nil, err
	}
	settled, err := r.judgeScope(draw)
	if err != nil || !settled {
		return draw, err
	}
	if err := r.runRefactors(ctx, draw); err != nil {
		return nil, err
	}
	draw.settled = true
	return draw, nil
}

func (r *critRun) witnessRequest(draw int) (hostproto.Request, error) {
	brief := r.p.brief
	built := prompt.BuildWitness(prompt.WitnessInput{
		Spec: r.p.specText, Target: r.p.labels.Of(r.c.Ref), Statement: r.p.labels.Relabel(r.c.Statement),
		Language: brief.Language, Conventions: brief.Conventions, Interface: brief.Interface, Helpers: brief.Helpers,
	})
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: r.c.StatementDigest},
		{Role: format.RoleSpec, Digest: digestText(r.p.specText)},
		{Role: format.RoleCodeTree, Digest: digestText(strings.Join(brief.Interface, "\n"))},
		{Role: format.RoleTestSource, Digest: digestText(strings.Join(brief.Helpers, "\n"))},
	}
	return r.p.assemble(requestDraft{kind: hostproto.KindGenerateWitness, built: built, inputs: inputs, round: draw, seed: r.witnessSeed(draw)})
}

// readSets reads every vote of a witness answer into a set and commits its
// digest. A vote whose set is unusable is rejected, and the answer waits for
// its reissue.
func (r *critRun) readSets(draw *witnessDraw, answer hostbridge.Answer) (contradicted bool, err error) {
	for _, vote := range answer.Votes {
		set, err := prompt.ParseWitnesses(vote.Output, r.c.Ref)
		if err != nil {
			contradicted = true
			if err := r.bridge.Contradict(vote, contradictionUnusableWitnesses); err != nil {
				return false, err
			}
			continue
		}
		set.Writer = witness.Writer{
			RequestID: string(answer.ID), Vote: vote.Ticket.Vote, Draw: draw.number, Isolation: vote.Executor.Isolation, Marked: vote.Marked,
			CreditWithheld: r.p.redrawsForCredit() && draw.number != creditDraw,
		}
		if _, err := r.bridge.Commit(hostproto.EventWitnessSetCommitted, commitDetail(r.c.Ref, draw.number, set)); err != nil {
			return false, err
		}
		if !slices.ContainsFunc(set.Witnesses, func(w witness.Witness) bool { return !r.knownWitness(draw, w.ID) }) {
			continue
		}
		draw.sets = append(draw.sets, set)
		for _, w := range set.Witnesses {
			if r.knownWitness(draw, w.ID) {
				continue
			}
			draw.records[w.ID] = &witness.Record{Witness: w, Writer: set.Writer}
			draw.profiles[w.ID] = []format.ExecutorProfile{vote.Executor}
		}
	}
	return contradicted, nil
}

// knownWitness reports whether a witness of the same content is already a
// witness of the criterion: two writers may write the same test, and it counts
// once, credited to the first.
func (r *critRun) knownWitness(current *witnessDraw, id string) bool {
	if _, known := current.records[id]; known {
		return true
	}
	return slices.ContainsFunc(r.allDraws(), func(draw *witnessDraw) bool {
		_, known := draw.records[id]
		return known
	})
}

func commitDetail(ref spec.Ref, draw int, set witness.Set) string {
	return strings.Join([]string{ref.String(), fmt.Sprint(draw), set.Digest()}, commitDetailSeparator)
}

// runReference runs every set of the draw on the reference.
func (r *critRun) runReference(ctx context.Context, draw *witnessDraw) error {
	for _, set := range draw.sets {
		runs, err := r.p.runner.ReferenceWitnesses(ctx, set)
		if err != nil {
			return err
		}
		for _, w := range set.Witnesses {
			if record, own := draw.records[w.ID]; own {
				record.Reference = runs[w.ID]
			}
		}
	}
	return nil
}

// runRefactors runs the sets that hold a witness the reference passes on every
// planted refactor of the admission input, after the scope verdicts, and records
// the result per witness.
func (r *critRun) runRefactors(ctx context.Context, draw *witnessDraw) error {
	runnable := r.runnableSetsOf(draw)
	if len(runnable) == 0 {
		return nil
	}
	for _, refactor := range r.p.settings.Refactors {
		program, err := refactorProgram(refactor)
		if err != nil {
			return err
		}
		outcomes, err := r.p.runner.EditWitnesses(ctx, program, refactor.Edits, runnable)
		if err != nil {
			return err
		}
		for _, w := range r.passingWitnesses(draw) {
			record := draw.records[w.ID]
			if record.Refactors == nil {
				record.Refactors = map[string]witness.Outcome{}
			}
			record.Refactors[refactor.Name] = outcomes[w.ID]
		}
	}
	return nil
}

// refactorProgram names a planted refactor by its content, so that a refactor
// that changes is run again.
func refactorProgram(refactor Refactor) (string, error) {
	id, err := refactor.Edits.ID()
	if err != nil {
		return "", fmt.Errorf("the planted refactor %s: %w", refactor.Name, err)
	}
	return refactorProgramPrefix + id, nil
}

const refactorProgramPrefix = "refactor:"

// runnableSetsOf lists, in the order the writers answered, the whole sets of the
// draw that hold a witness the reference passes: a set runs as its writer wrote
// it, so that its file builds as it did on the reference, and only its admitted
// witnesses are read.
func (r *critRun) runnableSetsOf(draw *witnessDraw) []witness.Set {
	var sets []witness.Set
	for _, set := range draw.sets {
		if slices.ContainsFunc(set.Witnesses, func(w witness.Witness) bool { return r.passes(draw, w.ID) }) {
			sets = append(sets, set)
		}
	}
	return sets
}

// passes reports whether the reference passes a witness of the criterion, in
// whichever draw it was first written.
func (r *critRun) passes(current *witnessDraw, id string) bool {
	for _, draw := range append([]*witnessDraw{current}, r.allDraws()...) {
		if record, known := draw.records[id]; known {
			return record.Reading() == witness.ReadingPasses
		}
	}
	return false
}

// passingWitnesses lists the witnesses of the draw the reference passes, in the
// order the writers answered.
func (r *critRun) passingWitnesses(draw *witnessDraw) []witness.Witness {
	var passing []witness.Witness
	for _, set := range draw.sets {
		for _, w := range set.Witnesses {
			record, own := draw.records[w.ID]
			if own && record.Reading() == witness.ReadingPasses && !slices.ContainsFunc(passing, func(seen witness.Witness) bool { return seen.ID == w.ID }) {
				passing = append(passing, w)
			}
		}
	}
	return passing
}

// judgeScope asks the scope judges about every witness of the draw the reference
// passes and records their votes. It reports whether the draw is settled.
func (r *critRun) judgeScope(draw *witnessDraw) (bool, error) {
	shown := r.passingWitnesses(draw)
	if len(shown) == 0 {
		return true, nil
	}
	request, labels, err := r.scopeRequest(shown)
	if err != nil {
		return false, err
	}
	ask := hostbridge.Ask{Request: request, Votes: r.rules.JudgeK, Criteria: []string{r.c.Ref.String()}, Purpose: string(hostproto.KindJudgeScope)}
	answer, state, err := r.ask(ask, billSchedule)
	switch {
	case err != nil || state == askPending:
		return false, err
	case state != askAnswered:
		return true, nil
	}
	for _, vote := range answer.Votes {
		parsed, err := prompt.ParseScope(vote.Output, labels)
		if err != nil {
			if err := r.bridge.Contradict(vote, contradictionUnansweredScope); err != nil {
				return false, err
			}
			_, _, err := r.ask(ask, billNone)
			return false, err
		}
		for index, w := range shown {
			scope := parsed[labels[index]]
			scope.Isolation, scope.Marked = vote.Executor.Isolation, vote.Marked
			draw.records[w.ID].Scope = append(draw.records[w.ID].Scope, scope)
			draw.profiles[w.ID] = append(draw.profiles[w.ID], vote.Executor)
		}
	}
	if err := r.bridge.Validate(answer); err != nil {
		return false, err
	}
	r.answers = append(r.answers, answer)
	return true, nil
}

func (r *critRun) scopeRequest(shown []witness.Witness) (hostproto.Request, []string, error) {
	labels := make([]string, len(shown))
	items := make([]prompt.ScopeWitness, len(shown))
	ids := make([]string, len(shown))
	for index, w := range shown {
		labels[index] = prompt.ScopeLabel(index)
		items[index] = prompt.ScopeWitness{Clause: w.Clause, Code: w.Code}
		ids[index] = w.ID
	}
	built := prompt.BuildScope(prompt.ScopeInput{
		Spec: r.p.specText, Target: r.p.labels.Of(r.c.Ref), Statement: r.p.labels.Relabel(r.c.Statement), Witnesses: items,
	})
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: r.c.StatementDigest},
		{Role: format.RoleSpec, Digest: digestText(r.p.specText)},
		{Role: format.RoleCodeTree, Digest: digestText(strings.Join(r.p.brief.Interface, "\n"))},
		{Role: format.RoleHostResponse, Digest: digestText(strings.Join(ids, "\n"))},
	}
	request, err := r.p.assemble(requestDraft{kind: hostproto.KindJudgeScope, built: built, inputs: inputs})
	return request, labels, err
}

// allDraws lists the draws the search reads and, in CI, the credit draw.
func (r *critRun) allDraws() []*witnessDraw { return slices.Concat(r.draws, r.creditDraws) }

// recordsOf lists the records of the draws, sorted by witness id.
func recordsOf(draws []*witnessDraw) []witness.Record {
	var records []witness.Record
	for _, draw := range draws {
		for _, record := range draw.records {
			records = append(records, *record)
		}
	}
	slices.SortFunc(records, func(a, b witness.Record) int { return strings.Compare(a.Witness.ID, b.Witness.ID) })
	return records
}

// witnessRecords lists the records of every draw of the criterion, the credit
// draw included, sorted by witness id.
func (r *critRun) witnessRecords() []witness.Record { return recordsOf(r.allDraws()) }

// runnableSetsAmong lists the runnable sets of the draws.
func (r *critRun) runnableSetsAmong(draws []*witnessDraw) []witness.Set {
	var sets []witness.Set
	for _, draw := range draws {
		sets = append(sets, r.runnableSetsOf(draw)...)
	}
	return sets
}

// searchDraws lists the draws the search reads: never the credit draw.
func (r *critRun) searchDraws() []*witnessDraw { return r.draws }

// searchSets lists the runnable sets the search reads.
func (r *critRun) searchSets() []witness.Set { return r.runnableSetsAmong(r.searchDraws()) }

// witnessesSettled reports whether every draw the search reads is settled.
func (r *critRun) witnessesSettled() bool { return allSettled(r.draws) }

// creditSettled reports whether the credit draw, when the run asks one, is
// settled.
func (r *critRun) creditSettled() bool { return allSettled(r.creditDraws) }

func allSettled(draws []*witnessDraw) bool {
	return !slices.ContainsFunc(draws, func(draw *witnessDraw) bool { return !draw.settled })
}

// profilesOf returns the executors a witness rests on: its writer and the scope
// judges.
func (r *critRun) profilesOf(id string) []format.ExecutorProfile {
	for _, draw := range r.allDraws() {
		if profiles, known := draw.profiles[id]; known {
			return profiles
		}
	}
	return nil
}

// readingConflicts lists the witnesses of the criterion the reference fails in
// every run: questions about the spec, reported beside the outcome.
func (r *critRun) readingConflicts() []string {
	var conflicts []string
	for _, record := range r.witnessRecords() {
		if record.Reading() == witness.ReadingConflict {
			conflicts = append(conflicts, fmt.Sprintf("%s: %s witness %s (%s)", FindingReadingConflict, r.c.Ref, record.Witness.ShortID(), record.Witness.Clause))
		}
	}
	return conflicts
}

// ownUniverse indexes the criterion's own witnesses the search reads: never the
// credit draw's, so that the search asks the same requests in CI as on the
// developer's machine.
func (r *critRun) ownUniverse() power.Universe {
	return power.NewUniverse(recordsOf(r.draws), r.g3w())
}
