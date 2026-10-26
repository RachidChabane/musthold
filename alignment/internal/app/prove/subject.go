package prove

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/musthold/alignment/internal/domain/audit"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/alignment/internal/domain/matrix"
	"github.com/RachidChabane/musthold/alignment/internal/domain/prompt"
	"github.com/RachidChabane/musthold/alignment/internal/domain/tournament"
	"github.com/RachidChabane/musthold/alignment/internal/domain/verdict"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

const (
	reasonHostUnresponsive = verdict.ReasonHostUnresponsive
	freshNamespace         = "fresh-alignment"
	auditNamespace         = "shadow-audit"
	keySeparator           = "\x00"
)

// subjectRun is the work of one pass on one test: its masked input, the answers
// it read from the host, and the signals it gathered, shared by every criterion
// the test is bound to.
type subjectRun struct {
	pass    *passState
	subject criterion.Subject
	masked  ports.MaskedTest
	input   prompt.TestInput
	// maskedDigest identifies the masked input: the test, the signatures and the
	// criteria as shown.
	maskedDigest string
	failure      *maskFailure
	// spent counts the tickets billed to the test in this pass.
	spent   int
	waiting bool
	answers map[hostproto.RequestID]hostbridge.Answer
	judges  map[bool]*judgeSignal
	closed  map[string]*closedSignal
}

func (p *passState) proveSubject(subject criterion.Subject) ([]TestResult, error) {
	run := &subjectRun{
		pass: p, subject: subject, answers: map[hostproto.RequestID]hostbridge.Answer{},
		judges: map[bool]*judgeSignal{}, closed: map[string]*closedSignal{},
	}
	return run.prove()
}

func (s *subjectRun) declaredNames() []string {
	names := make([]string, len(s.subject.Declared))
	for index, ref := range s.subject.Declared {
		names[index] = ref.String()
	}
	return names
}

func (s *subjectRun) prove() ([]TestResult, error) {
	s.masked, s.input, s.failure = s.pass.buildInput(s.subject.Test)
	if s.failure != nil {
		s.pass.addFinding(fmt.Sprintf("%s %s: %s", s.failure.reason, s.subject.Test.Label(), s.failure.detail))
	}
	s.maskedDigest = s.digestOfInput()
	var results []TestResult
	allWritten := true
	for _, declared := range s.subject.Declared {
		result, err := s.proveDeclared(declared)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", declared, err)
		}
		allWritten = allWritten && (result.Status == StatusProved || result.Status == StatusFresh)
		results = append(results, result)
	}
	if allWritten {
		if err := s.consume(); err != nil {
			return nil, err
		}
	}
	return results, nil
}

// digestOfInput digests what a judge is shown, or, for a test that cannot be
// shown, the reason it cannot.
func (s *subjectRun) digestOfInput() string {
	if s.failure != nil {
		return format.SHA256Hex([]byte(s.failure.reason + keySeparator + s.subject.Test.File + keySeparator + s.subject.Test.Label()))
	}
	return format.SHA256Hex([]byte(s.input.Unit + keySeparator + s.input.Signatures + keySeparator + s.input.Criteria.Text))
}

// consume records the answers the written Statements rest on, and keeps each vote
// when a recorder is attached.
func (s *subjectRun) consume() error {
	ids := make([]hostproto.RequestID, 0, len(s.answers))
	for id := range s.answers {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	for _, id := range ids {
		answer := s.answers[id]
		if err := s.pass.bridge.Consume(answer); err != nil {
			return err
		}
		if s.pass.deps.Record == nil {
			continue
		}
		for _, vote := range answer.Votes {
			name := requests.RecordingName(string(answer.ID), vote.Ticket.Vote)
			if err := s.pass.deps.Record.Record(name, vote.Output, vote.Executor); err != nil {
				return fmt.Errorf("record the answer to %s: %w", name, err)
			}
		}
	}
	return nil
}

// freshRecord remembers the Statement written for a base cache key.
type freshRecord struct {
	CacheKey string         `json:"cacheKey"`
	Verdict  format.Verdict `json:"verdict"`
	Reason   string         `json:"reason"`
}

func (s *subjectRun) proveDeclared(ref criterion.Ref) (TestResult, error) {
	declared, _ := s.pass.project.Find(ref)
	policy := s.pass.settings.Policy
	evidence, err := s.pass.evidenceFor(s.subject, declared, matrix.Thresholds{Own: policy.ThetaOwn, Other: policy.ThetaOther, None: policy.ThetaNone})
	if err != nil {
		return TestResult{}, err
	}
	base := s.baseInputs(declared, evidence)
	subject := format.RequirementSubject(declared.Ref.ID, declared.Ref.Revision, declared.StatementDigest)
	baseKey, err := format.CacheKey([]format.Subject{subject}, base)
	if err != nil {
		return TestResult{}, err
	}
	if s.pass.options.Selection == SelectChanged {
		if record, fresh := s.freshRecord(ref, baseKey); fresh {
			return TestResult{Test: s.subject.Test.Label(), Criterion: ref, Status: StatusFresh, Verdict: record.Verdict, Reason: record.Reason, CacheKey: record.CacheKey}, nil
		}
	}
	outcome, err := s.decide(declared.Ref, evidence.matrix)
	if err != nil {
		return TestResult{}, err
	}
	label := s.subject.Test.Label()
	if outcome.state == askPending {
		return TestResult{Test: label, Criterion: ref, Status: StatusAwaitingHost}, nil
	}
	if !evidence.usable && s.pass.proofPending[ref.String()] {
		return TestResult{Test: label, Criterion: ref, Status: StatusAwaitingProof, Verdict: outcome.decision.Verdict, Reason: outcome.decision.Reason}, nil
	}
	cacheKey, err := s.writeStatement(declared, evidence, base, outcome)
	if err != nil {
		return TestResult{}, err
	}
	if err := s.rememberFresh(baseKey, freshRecord{CacheKey: cacheKey, Verdict: outcome.decision.Verdict, Reason: outcome.decision.Reason}); err != nil {
		return TestResult{}, err
	}
	return TestResult{Test: label, Criterion: ref, Status: StatusProved, Verdict: outcome.decision.Verdict, Reason: outcome.decision.Reason, CacheKey: cacheKey}, nil
}

func (s *subjectRun) freshMemoKey(baseKey string) string {
	return format.SHA256Hex([]byte(freshNamespace + keySeparator + baseKey))
}

// freshRecord returns the record of the Statement written for the base cache key,
// when that Statement still exists.
func (s *subjectRun) freshRecord(ref criterion.Ref, baseKey string) (freshRecord, bool) {
	raw, found, err := s.pass.deps.Memo.Get(s.freshMemoKey(baseKey))
	if err != nil || !found {
		return freshRecord{}, false
	}
	var record freshRecord
	if json.Unmarshal(raw, &record) != nil {
		return freshRecord{}, false
	}
	exists, err := s.pass.deps.Proofs.Has(ref, record.CacheKey)
	return record, err == nil && exists
}

func (s *subjectRun) rememberFresh(baseKey string, record freshRecord) error {
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	return s.pass.deps.Memo.Put(s.freshMemoKey(baseKey), raw)
}

// decided is a verdict with the answers it rests on, or the state that stopped it.
type decided struct {
	decision verdict.Decision
	// used lists the answers the verdict rests on.
	used []hostbridge.Answer
	// closed is where the confidence of the closed answers came from.
	closed sourced
	state  askState
}

// decide gathers the signals the verdict table asks for, one at a time, until it
// has a verdict or has to wait for the host.
func (s *subjectRun) decide(declared criterion.Ref, m matrix.Result) (decided, error) {
	policy := s.pass.settings.Policy
	in := verdict.Inputs{Matrix: m}
	in.Audited = audit.Selected(s.auditKey(declared), policy.ShadowRate)
	if s.failure != nil {
		return decided{decision: failed(s.failure.reason)}, nil
	}
	var used []hostbridge.Answer
	var source sourced
	for {
		decision := verdict.Decide(in, policy)
		if decision.Done {
			return decided{decision: decision, used: used, closed: source}, nil
		}
		switch decision.Need {
		case verdict.NeedSystemOne:
			signal, err := s.systemOne(declared)
			if err != nil {
				return decided{}, err
			}
			if signal.pending {
				return decided{state: askPending}, nil
			}
			closed := s.closedAnswers()
			used, source = append(used, closed...), confidenceSource(closed, policy.DecideK)
			in.SystemOne = signal.input
			in.SystemOne.Source = source.source
		case verdict.NeedJudge:
			signal, err := s.judge(false)
			if err != nil {
				return decided{}, err
			}
			if stop, done := s.stoppedBy(signal.state); done {
				return stop, nil
			}
			result, stance, reason := s.aggregated(signal, declared.String())
			in.Judge = verdict.JudgeInput{Settled: true, Result: result, Stance: stance, IndeterminateReason: reason}
			used = append(used, signal.answer)
		case verdict.NeedTiebreak:
			signal, err := s.judge(true)
			if err != nil {
				return decided{}, err
			}
			if stop, done := s.stoppedBy(signal.state); done {
				return stop, nil
			}
			tiebreak := signal.votes[policy.AlignK].vote
			in.Tiebreak = verdict.TiebreakInput{Settled: true, Supports: aggregate.SingleStance(tiebreak, declared.String()) == aggregate.Supports}
			used = append(used, signal.answer)
		}
	}
}

// stoppedBy maps a state that is not an answer to the decision it forces: waiting
// for the host, or an UNDETERMINED verdict when the host stopped answering or the
// budget ran out.
func (s *subjectRun) stoppedBy(state askState) (decided, bool) {
	switch state {
	case askPending:
		return decided{state: askPending}, true
	case askUnresponsive:
		return decided{decision: failed(verdict.ReasonHostUnresponsive)}, true
	case askOverBudget:
		return decided{decision: failed(verdict.ReasonBudgetExhausted)}, true
	}
	return decided{}, false
}

// failed is the UNDETERMINED verdict of a test that no judge could rule on.
func failed(reason string) verdict.Decision {
	return verdict.Decision{
		Done: true, Verdict: format.VerdictUndetermined, Basis: format.BasisExecution,
		Deciders: []string{verdict.DeciderModule}, Reason: reason, Confidence: format.MinConfidence,
	}
}

func (s *subjectRun) auditKey(declared criterion.Ref) string {
	return auditNamespace + keySeparator + declared.String() + keySeparator + s.maskedDigest
}

// systemOneSignal is the two-signal check: what System One said, or that it is
// still pending.
type systemOneSignal struct {
	input   verdict.SystemOneInput
	pending bool
}

// systemOne plays the tournament over every criterion, with the answers the host
// gave so far. An answer that never comes, for lack of a responder or of budget,
// leaves the check settled without support, so that the judge decides.
func (s *subjectRun) systemOne(declared criterion.Ref) (systemOneSignal, error) {
	var failure error
	degraded, pending := false, false
	lookup := func(group []string, final bool) (tournament.Answer, bool) {
		signal, err := s.closedAnswer(group, final)
		if err != nil {
			failure = err
			return tournament.Answer{}, true
		}
		switch signal.state {
		case askPending:
			pending = true
			return tournament.Answer{}, false
		case askUnresponsive, askOverBudget:
			degraded = true
			return tournament.Answer{Choice: masking.NoneAnswer}, true
		}
		return signal.answer, true
	}
	outcome := tournament.Resolve(s.input.Criteria.Tokens, lookup)
	switch {
	case failure != nil:
		return systemOneSignal{}, failure
	case pending:
		return systemOneSignal{pending: true}, nil
	}
	input := verdict.SystemOneInput{Settled: true, Confidence: outcome.Confidence}
	if !degraded && outcome.Done {
		ref, named := s.input.Criteria.Refs[outcome.Winner]
		input.Supports = named && ref == declared && aggregate.Coverage(outcome.Coverage).Supporting()
	}
	return systemOneSignal{input: input}, nil
}

// closedAnswers lists the answers of the closed requests the tournament was
// played with, in request order.
func (s *subjectRun) closedAnswers() []hostbridge.Answer {
	var answers []hostbridge.Answer
	for _, signal := range s.closed {
		if signal.state == askAnswered {
			answers = append(answers, signal.source)
		}
	}
	slices.SortFunc(answers, func(a, b hostbridge.Answer) int { return strings.Compare(string(a.ID), string(b.ID)) })
	return answers
}

// sourced is where a confidence came from and the mapping that derived it from
// what the answerers gave.
type sourced struct {
	source  format.ConfidenceSource
	mapping string
}

// confidenceSource says where the confidence of closed answers came from: the
// relay's source when a System One endpoint answered, the modal share of the
// `decide_k` host votes otherwise.
func confidenceSource(answers []hostbridge.Answer, decideK int) sourced {
	for _, answer := range answers {
		if classified := answer.Votes[0].Classify; classified != nil {
			return sourced{source: classified.Source, mapping: classified.Mapping}
		}
		if relayed := answer.Votes[0].Executor; relayed.Channel == format.ChannelSystemOneRelay {
			return sourced{source: format.ConfidenceSource(relayed.Model.Name), mapping: hostproto.ClosedChoice.ConfidenceMapping()}
		}
	}
	return sourced{source: format.HostVotesSource(decideK), mapping: format.MappingModalShare}
}
