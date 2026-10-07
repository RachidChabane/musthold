package power

import (
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

// Reason names why a claim was not accepted or why a criterion escalates.
type Reason string

// Reasons a decision or an outcome carries.
const (
	ReasonNone                  Reason = ""
	ReasonJudgesSplit           Reason = "judges_split"
	ReasonBelowThreshold        Reason = "below_threshold"
	ReasonUncorroborated        Reason = "uncorroborated"
	ReasonIsolationFloor        Reason = "below_isolation_floor"
	ReasonNoExecutedEquivalence Reason = "no_executed_equivalence"
	ReasonNoExecutedExclusion   Reason = "no_executed_exclusion"
	ReasonJudgedUndetermined    Reason = "judged_undetermined"
	ReasonBudgetExhausted       Reason = "budget_exhausted"
	ReasonHostUnresponsive      Reason = "host_unresponsive"
	ReasonNoExecutor            Reason = "no_executor"
	ReasonNotAdjudicated        Reason = "not_adjudicated"
)

// Class is the pilot's mutant class (ADR 0003, validity mapping).
type Class string

// Mutant classes.
const (
	// ClassValid is VALID_c: the mutant violates the criterion.
	ClassValid Class = "valid"
	// ClassCollateral is VALID that violates several criteria; counted by the
	// gate, excluded from the matrix.
	ClassCollateral Class = "collateral"
	// ClassOffTarget is NOT_VIOLATING, re-homed to the criterion it violates.
	ClassOffTarget Class = "off_target"
	// ClassUnattributed is NOT_VIOLATING with no criterion to blame.
	ClassUnattributed Class = "unattributed"
	// ClassEquivalent is EQUIVALENT: no behavioural difference.
	ClassEquivalent Class = "equivalent"
	// ClassUndetermined is everything else.
	ClassUndetermined Class = "undetermined"
)

// Identities of the deciders recorded in `extensions.decision`.
const (
	DeciderSentinelCatalogue = "sentinel-catalogue"
	DeciderExamplePrefix     = "example-"
	DeciderHostJudge         = "host-judge"
)

// Evidence is one piece of executed evidence behind a judged claim.
type Evidence struct {
	Kind   format.CorroborationKind
	Digest string
}

// Decision is the validity of one mutant for one criterion and what backs it.
type Decision struct {
	Class      Class
	Validity   format.Validity
	Basis      format.DecisionBasis
	Confidence float64
	Deciders   []string
	// Source says where Confidence came from, and Mapping how it was derived from
	// what the answerers gave; the Statement records both.
	Source  format.ConfidenceSource
	Mapping string
	// Corroboration is the executed evidence, sorted and without duplicates.
	Corroboration []Evidence
	// OffTarget is the criterion an off-target mutant violates.
	OffTarget *spec.Ref
	// Reason says why a claim is undetermined.
	Reason Reason
	// HostExclusion is set when a survivor was cleared on judges alone.
	HostExclusion bool
}

// Carries reports whether the decision gives its mutant the power to block or to
// earn pass credit: a VALID class.
func (d Decision) Carries() bool { return d.Class == ClassValid || d.Class == ClassCollateral }

// Undetermined builds the decision of a mutant no stage could classify.
func Undetermined(reason Reason) Decision {
	return Decision{Class: ClassUndetermined, Validity: format.ValidityUndetermined, Basis: format.BasisJudges, Confidence: format.MinConfidence, Deciders: []string{DeciderHostJudge}, Source: format.SourceExecution, Reason: reason}
}

// VoteQuestion is what a judge was asked about a mutant.
type VoteQuestion string

// Questions a judge ticket carries.
const (
	QuestionValidity   VoteQuestion = "validity"
	QuestionEquivalent VoteQuestion = "equivalent"
)

// Answer is a judge's answer to a validity or an equivalence question.
type Answer string

// Judge answers; the first four are the validities of the format.
const (
	AnswerValid        Answer = "VALID"
	AnswerEquivalent   Answer = "EQUIVALENT"
	AnswerNotViolating Answer = "NOT_VIOLATING"
	AnswerUndetermined Answer = "UNDETERMINED"
)

// WitnessCheck is the result of confirming the witness a judge gave (F4).
type WitnessCheck string

// Witness checks.
const (
	// WitnessAbsent: the judge gave no witness.
	WitnessAbsent WitnessCheck = "absent"
	// WitnessConfirmed: the reference output equals the expected value and the
	// mutant's output differs or raises.
	WitnessConfirmed WitnessCheck = "confirmed"
	// WitnessRefused: execution contradicts the witness.
	WitnessRefused WitnessCheck = "refused"
)

// Vote is one judge ticket's answer, with the provenance of its response.
type Vote struct {
	Answer     Answer
	Confidence float64
	Isolation  format.Isolation
	// Marked is set for a response journaled as leaked, exposed or conflicting.
	Marked  bool
	Witness WitnessCheck
	// WitnessDigest is the digest of the confirmed witness record.
	WitnessDigest string
}

// AtFloor reports whether the vote's response keeps judged pass credit under the
// isolation floor.
func (v Vote) AtFloor(floor format.Isolation) bool { return !v.Marked && v.Isolation.AtLeast(floor) }

// Tally is the aggregate of the judge votes of one mutant.
type Tally struct {
	Answer        Answer
	Unanimous     bool
	MinConfidence float64
}

// TallyVotes aggregates votes: the answer is the common one when every vote
// agrees, the confidence is the lowest of them.
func TallyVotes(votes []Vote) Tally {
	if len(votes) == 0 {
		return Tally{Answer: AnswerUndetermined}
	}
	tally := Tally{Answer: votes[0].Answer, Unanimous: true, MinConfidence: votes[0].Confidence}
	for _, vote := range votes[1:] {
		tally.Unanimous = tally.Unanimous && vote.Answer == tally.Answer
		tally.MinConfidence = min(tally.MinConfidence, vote.Confidence)
	}
	return tally
}

// Decide applies the validity mapping and the claim-power table to one mutant of
// a pool. Sentinels are EQUIVALENT by construction, a failed example attributes
// the mutant to the criterion it belongs to, and everything else rests on the
// judge votes together with the executed evidence the table names.
func Decide(subject Subject, facts Facts, votes []Vote, rules policy.Policy) Decision {
	if facts.Origin == OriginSentinel {
		return sentinelDecision(facts)
	}
	if len(facts.FailedExamples) > 0 {
		return attributedDecision(subject, facts)
	}
	return judgedDecision(subject, facts, votes, rules)
}

func sentinelDecision(facts Facts) Decision {
	return Decision{
		Class: ClassEquivalent, Validity: format.ValidityEquivalent, Basis: format.BasisConstruction,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderSentinelCatalogue}, Source: format.SourceExecution,
		Corroboration: nil, Reason: ReasonNone,
	}
}

func attributedDecision(subject Subject, facts Facts) Decision {
	failed := facts.FailedExamples
	deciders := make([]string, 0, len(failed))
	for _, ref := range failed {
		deciders = append(deciders, DeciderExamplePrefix+ref.String())
	}
	slices.Sort(deciders)
	evidence := []Evidence{{Kind: format.CorroborationExampleFailed, Digest: evidenceDigest(format.CorroborationExampleFailed, facts.ID, refStrings(failed))}}
	decision := Decision{Basis: format.BasisSpecExample, Confidence: format.MaxConfidence, Deciders: deciders, Source: format.SourceExecution, Corroboration: evidence}
	inSubject := slices.ContainsFunc(failed, func(ref spec.Ref) bool { return ref == subject.Ref })
	switch {
	case inSubject && len(failed) == 1:
		decision.Class, decision.Validity = ClassValid, format.ValidityValid
	case inSubject:
		decision.Class, decision.Validity = ClassCollateral, format.ValidityValid
	default:
		other := firstOther(failed, subject.Ref)
		decision.Class, decision.Validity, decision.OffTarget = ClassOffTarget, format.ValidityNotViolating, &other
		decision.Corroboration = []Evidence{{Kind: format.CorroborationOtherExampleFailed, Digest: evidenceDigest(format.CorroborationOtherExampleFailed, facts.ID, refStrings(failed))}}
	}
	return decision
}

func firstOther(failed []spec.Ref, own spec.Ref) spec.Ref {
	for _, ref := range failed {
		if ref != own {
			return ref
		}
	}
	return own
}

func refStrings(refs []spec.Ref) []string {
	out := make([]string, len(refs))
	for index, ref := range refs {
		out[index] = ref.String()
	}
	return out
}

func judgedDecision(subject Subject, facts Facts, votes []Vote, rules policy.Policy) Decision {
	if len(votes) < rules.JudgeK {
		return Undetermined(ReasonNoExecutor)
	}
	tally := TallyVotes(votes)
	switch {
	case !tally.Unanimous:
		return Undetermined(ReasonJudgesSplit)
	case tally.MinConfidence < rules.TauJudge:
		return Undetermined(ReasonBelowThreshold)
	}
	base := Decision{
		Basis: format.BasisJudges, Confidence: tally.MinConfidence, Deciders: []string{DeciderHostJudge},
		Source: format.HostVotesSource(rules.JudgeK), Mapping: format.MappingWeakestStated,
	}
	atFloor := allAtFloor(votes, rules.MinIsolation)
	switch tally.Answer {
	case AnswerValid:
		return validDecision(subject, facts, votes, rules, base, atFloor)
	case AnswerEquivalent:
		return equivalentDecision(subject, facts, rules, base, atFloor)
	case AnswerNotViolating:
		return notViolatingDecision(subject, facts, rules, base, atFloor)
	default:
		return Undetermined(ReasonJudgedUndetermined)
	}
}

func allAtFloor(votes []Vote, floor format.Isolation) bool {
	return !slices.ContainsFunc(votes, func(v Vote) bool { return !v.AtFloor(floor) })
}

// validDecision applies the VALID rows of the claim-power table. A survivor that
// is judged VALID blocks with a confirmed witness, a build and the passing tests
// of the other criteria; a killed one earns credit with a specific kill on top,
// and only from responses that keep judged credit.
func validDecision(subject Subject, facts Facts, votes []Vote, rules policy.Policy, base Decision, atFloor bool) Decision {
	witness, hasWitness := confirmedWitness(votes)
	if !hasWitness || !subject.OtherCriteriaPass(facts) {
		return Undetermined(ReasonUncorroborated)
	}
	evidence := []Evidence{
		{Kind: format.CorroborationBuilds, Digest: evidenceDigest(format.CorroborationBuilds, facts.ID, nil)},
		{Kind: format.CorroborationOtherCriteriaPass, Digest: evidenceDigest(format.CorroborationOtherCriteriaPass, facts.ID, subject.OtherTests)},
		{Kind: format.CorroborationWitness, Digest: witness},
	}
	if subject.Killed(facts) {
		killers := subject.SpecificKillers(facts)
		switch {
		case len(killers) == 0:
			return Undetermined(ReasonUncorroborated)
		case !atFloor || rules.SignedPassBasis == policy.ExecutedOnly:
			return Undetermined(ReasonIsolationFloor)
		}
		evidence = append(evidence, Evidence{Kind: format.CorroborationSpecificKill, Digest: evidenceDigest(format.CorroborationSpecificKill, facts.ID, killers)})
	}
	base.Class, base.Validity, base.Corroboration = ClassValid, format.ValidityValid, sortEvidence(evidence)
	return base
}

func confirmedWitness(votes []Vote) (digest string, found bool) {
	for _, vote := range votes {
		if vote.Witness == WitnessConfirmed {
			return vote.WitnessDigest, true
		}
	}
	return "", false
}

func equivalentDecision(subject Subject, facts Facts, rules policy.Policy, base Decision, atFloor bool) Decision {
	switch {
	case facts.KilledByAny() || facts.Differential.Status == DiffersAt:
		return Undetermined(ReasonJudgedUndetermined)
	case facts.Differential.Status != NoDiffFound || rules.SignedPassBasis == policy.ExecutedOnly:
		return Undetermined(ReasonNoExecutedEquivalence)
	case !atFloor:
		return Undetermined(ReasonIsolationFloor)
	}
	base.Class, base.Validity = ClassEquivalent, format.ValidityEquivalent
	base.Corroboration = []Evidence{{Kind: format.CorroborationNoDiffFound, Digest: evidenceDigest(format.CorroborationNoDiffFound, facts.ID, nil)}}
	return base
}

func notViolatingDecision(subject Subject, facts Facts, rules policy.Policy, base Decision, atFloor bool) Decision {
	switch {
	case subject.Killed(facts):
		return Undetermined(ReasonJudgedUndetermined)
	case rules.SignedPassBasis != policy.Judged:
		return Undetermined(ReasonNoExecutedExclusion)
	case !atFloor:
		return Undetermined(ReasonIsolationFloor)
	}
	base.Class, base.Validity, base.HostExclusion = ClassUnattributed, format.ValidityNotViolating, true
	return base
}

func sortEvidence(evidence []Evidence) []Evidence {
	slices.SortFunc(evidence, func(a, b Evidence) int {
		if byKind := compareStrings(string(a.Kind), string(b.Kind)); byKind != 0 {
			return byKind
		}
		return compareStrings(a.Digest, b.Digest)
	})
	return slices.CompactFunc(evidence, func(a, b Evidence) bool { return a == b })
}

func compareStrings(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// evidenceDigest is the digest of one evidence record: its kind, the mutant and
// the facts the evidence names.
func evidenceDigest(kind format.CorroborationKind, mutant string, subject any) string {
	canonical, err := format.Canonicalize(struct {
		Kind    format.CorroborationKind `json:"kind"`
		Mutant  string                   `json:"mutant"`
		Subject any                      `json:"subject,omitempty"`
	}{Kind: kind, Mutant: mutant, Subject: subject})
	if err != nil {
		return format.SHA256Hex([]byte(string(kind) + mutant))
	}
	return format.SHA256Hex(canonical)
}
