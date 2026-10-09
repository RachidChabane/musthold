package prove

import (
	"cmp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/tournament"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/verdict"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

// askState is where an ask of the host stands.
type askState int

const (
	askAnswered askState = iota
	askPending
	askUnresponsive
	askOverBudget
)

// reasonClassificationUncredited is why a classification is rejected for the
// host's deciders to answer its request.
const reasonClassificationUncredited = "classification_uncredited"

// ask puts a request to the host and reports where it stands. A reasoning request,
// one the judge answers, is billed against the budget of the test the first time
// it is asked, in the order the pass asks, so the budget never depends on what
// earlier runs left in the store. Closed questions, which System One or the
// deciders answer, are not reasoning calls and cost nothing.
func (s *subjectRun) ask(ask hostbridge.Ask, cost int) (hostbridge.Answer, askState, error) {
	if cost > 0 {
		if s.spent+cost > s.pass.settings.Policy.MaxReasoningCalls {
			return hostbridge.Answer{}, askOverBudget, nil
		}
		s.spent += cost
	}
	answer, err := s.pass.bridge.Want(ask)
	if err != nil {
		return answer, askAnswered, err
	}
	switch answer.Standing {
	case hostbridge.Pending:
		s.waiting = true
		return answer, askPending, nil
	case hostbridge.Unresponsive:
		return answer, askUnresponsive, nil
	}
	return answer, askAnswered, nil
}

// credited reports whether a vote may speak for alignment: its executor reaches
// the isolation floor and the journal does not mark the response.
func (s *subjectRun) credited(vote hostbridge.VoteState) bool {
	return !vote.Marked && vote.Executor.Isolation.AtLeast(s.pass.settings.Policy.MinIsolation)
}

// standsForTheVotes reports whether the first vote of a closed request replaces
// the host votes: the answer of a System One relay, or a classification.
func standsForTheVotes(vote hostbridge.VoteState) bool {
	return vote.Executor.Channel == format.ChannelSystemOneRelay || vote.Executor.Channel == format.ChannelHostClassify
}

// classificationCredited reports whether the classification the vote is may count
// as the closed signal: it is credited like any vote, and its exact source has a
// threshold of its own. Otherwise it has no power, and the host's deciders answer
// the request.
func (s *subjectRun) classificationCredited(vote hostbridge.VoteState) bool {
	return s.credited(vote) && vote.Classify != nil && s.pass.settings.Policy.TauRoute.HasExact(vote.Classify.Source)
}

// remember keeps an answer the pass read, validated, so that its votes are
// consumed once the Statements they rest on are written.
func (s *subjectRun) remember(answer hostbridge.Answer) error {
	if err := s.pass.bridge.Validate(answer); err != nil {
		return err
	}
	s.answers[answer.ID] = answer
	return nil
}

// judgeVote is one vote of the judge request as the module reads it.
type judgeVote struct {
	vote    aggregate.Vote
	profile format.ExecutorProfile
}

// judgeSignal is the judge's answers: the votes in ticket order and the answer
// they come from.
type judgeSignal struct {
	state  askState
	votes  []judgeVote
	answer hostbridge.Answer
}

// judge asks the judge request, with one vote more when extra is set, and reads
// the votes. The request is the same for every criterion the test is bound to.
func (s *subjectRun) judge(extra bool) (*judgeSignal, error) {
	if signal, known := s.judges[extra]; known {
		return signal, nil
	}
	policy := s.pass.settings.Policy
	built, err := prompt.BuildJudge(s.input)
	if err != nil {
		return nil, err
	}
	request, err := s.assemble(hostproto.KindJudgeAlignment, built, s.allStatementDigests())
	if err != nil {
		return nil, err
	}
	votes, cost := policy.AlignK, policy.AlignK
	if extra {
		votes, cost = policy.AlignK+1, 1
	}
	answer, state, err := s.ask(hostbridge.Ask{Request: request, Votes: votes, Criteria: s.declaredNames(), Purpose: string(hostproto.KindJudgeAlignment)}, cost)
	signal := &judgeSignal{state: state, answer: answer}
	s.judges[extra] = signal
	if err != nil || state != askAnswered {
		return signal, err
	}
	for _, voted := range answer.Votes {
		read, err := prompt.ParseJudge(voted.Output)
		if err != nil {
			return nil, err
		}
		reference := masking.NoneAnswer
		if ref, named := s.input.Criteria.Refs[read.Token]; named {
			reference = ref.String()
		}
		signal.votes = append(signal.votes, judgeVote{
			vote:    aggregate.Vote{Verifies: reference, Coverage: read.Coverage, Confidence: read.Confidence, Credit: s.credited(voted)},
			profile: voted.Executor,
		})
	}
	return signal, s.remember(answer)
}

// aggregated reads the first AlignK votes against the declared criterion. When
// the votes would support the criterion but for those that lose their credit, the
// stance is indeterminate for want of isolation, not a rejection.
func (s *subjectRun) aggregated(signal *judgeSignal, declared string) (aggregate.Result, aggregate.Stance, string) {
	total := s.pass.settings.Policy.AlignK
	modalMin := s.pass.settings.Policy.ModalMin.For(format.HostVotesSource(total))
	counted := signal.votes[:min(total, len(signal.votes))]
	demoted, credited := make([]aggregate.Vote, 0, total), make([]aggregate.Vote, 0, total)
	for _, read := range counted {
		demoted = append(demoted, read.vote.Demote(declared))
		granted := read.vote
		granted.Credit = true
		credited = append(credited, granted)
	}
	result, ok := aggregate.Aggregate(demoted, total)
	if !ok {
		return result, aggregate.Indeterminate, ""
	}
	stance := result.StanceFor(declared, modalMin)
	if stance != aggregate.Supports {
		if full, _ := aggregate.Aggregate(credited, total); full.StanceFor(declared, modalMin) == aggregate.Supports {
			return result, aggregate.Indeterminate, verdict.ReasonIsolationBelowFloor
		}
	}
	return result, stance, ""
}

// closedSignal is the answer to the closed questions about one group of criteria.
type closedSignal struct {
	state  askState
	answer tournament.Answer
	// source is the host's answer the signal was read from.
	source hostbridge.Answer
}

func groupKey(group []string, final bool) string {
	key := strings.Join(group, ",")
	if final {
		return key + "!"
	}
	return key
}

// closed asks the closed questions about a group of criteria. The first ticket is
// the one a System One relay answers; only when a host decider answered it do the
// other `decide_k` tickets follow, because a relay answer replaces the host votes.
func (s *subjectRun) closedAnswer(group []string, final bool) (*closedSignal, error) {
	key := groupKey(group, final)
	if signal, known := s.closed[key]; known {
		return signal, nil
	}
	built, err := prompt.BuildClosed(prompt.ClosedInput{Test: s.input, Group: group, Final: final})
	if err != nil {
		return nil, err
	}
	request, err := s.assemble(hostproto.KindDecideClosed, built, s.groupStatementDigests(group))
	if err != nil {
		return nil, err
	}
	signal := &closedSignal{}
	s.closed[key] = signal
	decideK := s.pass.settings.Policy.DecideK
	ask := hostbridge.Ask{Request: request, Votes: 1, Criteria: s.declaredNames(), Purpose: string(hostproto.KindDecideClosed)}
	answer, state, err := s.ask(ask, 0)
	if err != nil || state != askAnswered {
		signal.state = state
		return signal, err
	}
	if first := answer.Votes[0]; first.Executor.Channel == format.ChannelHostClassify && !s.classificationCredited(first) {
		if err := s.pass.bridge.Contradict(first, reasonClassificationUncredited); err != nil {
			return signal, err
		}
		ask.Votes = max(decideK, 1)
		answer, state, err = s.ask(ask, 0)
		if err != nil || state != askAnswered {
			signal.state = state
			return signal, err
		}
	}
	if !standsForTheVotes(answer.Votes[0]) && decideK > 1 {
		ask.Votes = decideK
		if answer, state, err = s.ask(ask, 0); err != nil || state != askAnswered {
			signal.state = state
			return signal, err
		}
	}
	signal.answer, signal.source = s.readClosed(answer, final), answer
	return signal, s.remember(answer)
}

// readClosed turns the votes of a closed request into one answer. A System One
// relay answer, the first vote, stands for the votes and carries the confidence
// the endpoint gave, none counting as zero. Host votes give the modal choice of
// each question with its share of the `decide_k` tickets as the confidence; a
// vote without credit counts for no choice.
func (s *subjectRun) readClosed(answer hostbridge.Answer, final bool) tournament.Answer {
	if relayed := answer.Votes[0]; standsForTheVotes(relayed) {
		if s.credited(relayed) {
			return relayedAnswer(relayed, final)
		}
		return tournament.Answer{Choice: masking.NoneAnswer}
	}
	total := len(answer.Votes)
	criterionChoices, coverageChoices := map[string]int{}, map[string]int{}
	for _, vote := range answer.Votes {
		if !s.credited(vote) {
			continue
		}
		parsed, err := hostproto.ParseClosedAnswers(vote.Output)
		if err != nil {
			continue
		}
		criterionChoices[parsed[prompt.QuestionCriterion].Choice]++
		if final {
			coverageChoices[parsed[prompt.QuestionCoverage].Choice]++
		}
	}
	choice, count := modalOf(criterionChoices)
	result := tournament.Answer{Choice: choice, Confidence: float64(count) / float64(total)}
	if choice == "" {
		result.Choice = masking.NoneAnswer
	}
	if final {
		coverage, covered := modalOf(coverageChoices)
		result.Coverage = coverage
		result.Confidence = min(result.Confidence, float64(covered)/float64(total))
	}
	return result
}

func relayedAnswer(vote hostbridge.VoteState, final bool) tournament.Answer {
	parsed, err := hostproto.ParseClosedAnswers(vote.Output)
	if err != nil {
		return tournament.Answer{Choice: masking.NoneAnswer}
	}
	criterion := parsed[prompt.QuestionCriterion]
	result := tournament.Answer{Choice: criterion.Choice, Confidence: statedConfidence(criterion)}
	if final {
		coverage := parsed[prompt.QuestionCoverage]
		result.Coverage = coverage.Choice
		result.Confidence = min(result.Confidence, statedConfidence(coverage))
	}
	return result
}

func statedConfidence(answer hostproto.ClosedAnswer) float64 {
	if answer.HasConfidence {
		return answer.Confidence
	}
	return 0
}

// modalOf returns the commonest choice and its count, ties going to the
// lexicographically smaller choice.
func modalOf(counts map[string]int) (string, int) {
	choices := make([]string, 0, len(counts))
	for choice := range counts {
		choices = append(choices, choice)
	}
	if len(choices) == 0 {
		return "", 0
	}
	slices.SortFunc(choices, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(a, b)) })
	return choices[0], counts[choices[0]]
}
