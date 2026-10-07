package prove

import (
	"context"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

// Grounds on which execution contradicts a vote, recorded with the rejection.
const contradictionUnreadable = "unreadable_answer"

// judge puts the validity or the equivalence of a mutant to the host and, once
// the votes are in, settles the decision.
func (r *critRun) judge(ctx context.Context, m *poolMutant) error {
	if m.decision != nil {
		return nil
	}
	question := questionOf(m)
	request, err := r.judgeRequest(m, question)
	if err != nil {
		return err
	}
	ask := hostbridge.Ask{Request: request, Votes: r.rules.JudgeK, Criteria: []string{r.c.Ref.String()}, Purpose: string(request.Kind)}
	answer, state, err := r.ask(ask, true)
	switch {
	case err != nil:
		return err
	case state == askPending:
		return nil
	case state == askUnresponsive:
		r.settleWithout(m, power.ReasonHostUnresponsive)
		return nil
	case state == askOverBudget:
		r.settleWithout(m, power.ReasonBudgetExhausted)
		return nil
	}
	votes, profiles, contradicted, err := r.readVotes(ctx, m, answer, question)
	if err != nil {
		return err
	}
	if contradicted {
		_, _, err := r.ask(ask, false)
		return err
	}
	if err := r.bridge.Validate(answer); err != nil {
		return err
	}
	r.answers = append(r.answers, answer)
	r.decide(m, votes, profiles)
	return nil
}

func (r *critRun) settleWithout(m *poolMutant, reason power.Reason) {
	decision := power.Undetermined(reason)
	m.decision = &decision
}

// questionOf chooses what a judge is asked: the equivalence of a mutant the
// search found no difference for, the validity of every other one.
func questionOf(m *poolMutant) power.VoteQuestion {
	if m.facts.Differential.Status == power.NoDiffFound {
		return power.QuestionEquivalent
	}
	return power.QuestionValidity
}

func (r *critRun) judgeRequest(m *poolMutant, question power.VoteQuestion) (hostproto.Request, error) {
	views, err := r.editViews(m)
	if err != nil {
		return hostproto.Request{}, err
	}
	input := prompt.JudgeInput{Statement: r.p.blindStatement(r.c), Edits: views}
	kind, build := hostproto.KindJudgeEquivalent, prompt.BuildEquivalence
	if question == power.QuestionValidity {
		kind, build = hostproto.KindJudgeValidity, prompt.BuildValidity
		input.Operations = prompt.OperationViews(r.st.draws.Signatures)
		input.Witnesses = prompt.WitnessViews(m.facts.Differential.Witnesses)
	}
	inputs := []hostproto.Input{
		{Role: format.RoleSpec, Digest: r.c.StatementDigest},
		{Role: format.RoleCodeTree, Digest: r.sliceDigest()},
		{Role: format.RoleMutantSet, Digest: m.spec.ID},
	}
	return r.p.assemble(requestDraft{kind: kind, built: build(input), inputs: inputs})
}

// editViews shows what a mutant changes: each edited unit before and after.
func (r *critRun) editViews(m *poolMutant) ([]prompt.EditView, error) {
	var views []prompt.EditView
	for _, edit := range m.edits.Canonical().Edits {
		before, err := r.p.unitSource(edit.File, edit.Qualname, edit.Kind())
		if err != nil {
			return nil, fmt.Errorf("show the unit %s of %s: %w", edit.Qualname, edit.File, err)
		}
		views = append(views, prompt.EditView{File: edit.File, Qualname: edit.Qualname, Before: before, After: edit.NewSource})
	}
	return views, nil
}

// readVotes turns the answered tickets into votes: it reads each output, confirms
// the witness it gives by execution (F4) and rejects any vote that execution
// contradicts. It reports whether a vote was rejected.
func (r *critRun) readVotes(ctx context.Context, m *poolMutant, answer hostbridge.Answer, question power.VoteQuestion) ([]power.Vote, []format.ExecutorProfile, bool, error) {
	var votes []power.Vote
	var profiles []format.ExecutorProfile
	contradicted := false
	for _, state := range answer.Votes {
		vote, err := r.readVote(ctx, m, state, question)
		if err != nil {
			return nil, nil, false, err
		}
		if vote == nil {
			contradicted = true
			if err := r.bridge.Contradict(state, contradictionUnreadable); err != nil {
				return nil, nil, false, err
			}
			continue
		}
		if ground := power.Contradicts(m.facts, *vote); ground != power.ContradictionNone {
			contradicted = true
			if err := r.bridge.Contradict(state, string(ground)); err != nil {
				return nil, nil, false, err
			}
			continue
		}
		votes = append(votes, *vote)
		profiles = append(profiles, state.Executor)
	}
	return votes, profiles, contradicted, nil
}

// readVote reads one ticket's output; it returns nil for an output that cannot be
// read as an answer to the question.
func (r *critRun) readVote(ctx context.Context, m *poolMutant, state hostbridge.VoteState, question power.VoteQuestion) (*power.Vote, error) {
	parse := prompt.ParseValidity
	if question == power.QuestionEquivalent {
		parse = prompt.ParseEquivalence
	}
	judgement, err := parse(state.Output)
	if err != nil {
		return nil, nil
	}
	vote := power.Vote{
		Answer: judgement.Answer, Confidence: judgement.Confidence,
		Isolation: state.Executor.Isolation, Marked: state.Marked, Witness: power.WitnessAbsent,
	}
	if judgement.Witness == nil || judgement.Answer != power.AnswerValid {
		return &vote, nil
	}
	confirmation, err := r.p.runner.Confirm(ctx, m.spec, judgement.Witness.Call, judgement.Witness.Expect)
	if err != nil {
		return nil, err
	}
	vote.Witness, vote.WitnessDigest = confirmation.Check, confirmation.Digest
	return &vote, nil
}

// screenOutcome is what the screen decided for a group of survivors.
type screenOutcome struct {
	violation float64
	flagged   bool
	pending   bool
}

// screen runs the decide.closed screen over the operator survivors: survivors
// with the same witnesses are one question, a flagged one enters the pool, and a
// screened-out one is only counted. Where the policy bypasses the screen every
// survivor enters.
func (r *critRun) screen(ctx context.Context, survivors []*poolMutant) error {
	groups := groupByBehaviour(survivors)
	for _, group := range groups {
		outcome, err := r.screenGroup(ctx, group[0])
		if err != nil {
			return err
		}
		if outcome.pending {
			continue
		}
		for _, m := range group {
			if !outcome.flagged {
				r.screenedOut++
				continue
			}
			m.flagged, m.violation = true, outcome.violation
			r.flagged++
			r.add(m)
		}
	}
	return nil
}

func (r *critRun) screenGroup(ctx context.Context, representative *poolMutant) (screenOutcome, error) {
	if r.rules.BypassesScreen() {
		return screenOutcome{violation: 1, flagged: true}, nil
	}
	scope, err := r.p.scopeOf(ctx, representative.spec.ID)
	if err != nil {
		return screenOutcome{}, err
	}
	window := windowAround(scope, r.c.Ref)
	statements := make([]string, len(window))
	for index, ref := range window {
		criterion, _ := r.p.byRef(ref)
		statements[index] = r.p.blindStatement(criterion)
	}
	views, err := r.editViews(representative)
	if err != nil {
		return screenOutcome{}, err
	}
	built, err := prompt.BuildScreen(prompt.ScreenInput{Statements: statements, Edits: views, Witnesses: prompt.WitnessViews(representative.facts.Differential.Witnesses)})
	if err != nil {
		return screenOutcome{}, err
	}
	inputs := []hostproto.Input{
		{Role: format.RoleCodeTree, Digest: r.sliceDigest()},
		{Role: format.RoleMutantSet, Digest: representative.spec.ID},
	}
	for _, ref := range window {
		criterion, _ := r.p.byRef(ref)
		inputs = append(inputs, hostproto.Input{Role: format.RoleSpec, Digest: criterion.StatementDigest})
	}
	request, err := r.p.assemble(requestDraft{kind: hostproto.KindDecideClosed, built: built, inputs: inputs})
	if err != nil {
		return screenOutcome{}, err
	}
	criteria := make([]string, len(window))
	for index, ref := range window {
		criteria[index] = ref.String()
	}
	answer, state, err := r.ask(hostbridge.Ask{Request: request, Votes: r.rules.DecideK, Criteria: criteria, Purpose: string(hostproto.KindDecideClosed)}, false)
	switch {
	case err != nil:
		return screenOutcome{}, err
	case state == askPending:
		return screenOutcome{pending: true}, nil
	case state == askUnresponsive:
		return screenOutcome{violation: 1, flagged: true}, nil
	}
	if err := r.bridge.Validate(answer); err != nil {
		return screenOutcome{}, err
	}
	r.answers = append(r.answers, answer)
	answers, source := closedAnswers(answer, prompt.QuestionID(indexOf(window, r.c.Ref)))
	violation := power.ViolationProbability(answers)
	return screenOutcome{violation: violation, flagged: power.FlagsSurvivor(violation, r.rules.TauScreen.For(source))}, nil
}

// closedAnswers collects the answers of the votes to one question and says where
// their probabilities came from: the model of the relay for a System One answer,
// which replaces the host votes when there is one and whose stated probability of
// VIOLATES is kept, and the host votes otherwise, each of which counts by its
// choice whatever probability it states.
func closedAnswers(answer hostbridge.Answer, question string) ([]power.ClosedAnswer, format.ConfidenceSource) {
	var relayed, voted []power.ClosedAnswer
	var relaySource format.ConfidenceSource
	for _, vote := range answer.Votes {
		parsed, err := prompt.ParseScreen(vote.Output)
		if err != nil {
			continue
		}
		given, answered := parsed[question]
		if !answered {
			continue
		}
		if vote.Executor.Channel == format.ChannelSystemOneRelay {
			relayed = append(relayed, given)
			relaySource = format.ConfidenceSource(vote.Executor.Model.Name)
		} else {
			voted = append(voted, power.ClosedAnswer{Choice: given.Choice})
		}
	}
	if len(relayed) > 0 {
		return relayed, relaySource
	}
	return voted, format.HostVotesSource(max(len(answer.Votes), 1))
}

// maxScreenWindow is the most criteria one screen request asks about.
const maxScreenWindow = prompt.MaxQuestions

// windowAround lists at most maxScreenWindow criteria of the scope, the one
// asked about included, in the order of the scope.
func windowAround(scope []spec.Ref, own spec.Ref) []spec.Ref {
	if len(scope) <= maxScreenWindow {
		return scope
	}
	start := max(0, min(indexOf(scope, own)-maxScreenWindow/2, len(scope)-maxScreenWindow))
	return scope[start : start+maxScreenWindow]
}

func indexOf(refs []spec.Ref, ref spec.Ref) int {
	for index, candidate := range refs {
		if candidate == ref {
			return index
		}
	}
	return 0
}
