package power

import (
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

// Reason names why a mutant is unsettled or why a criterion escalates.
type Reason string

// Reasons a decision or an outcome carries.
const (
	ReasonNone                  Reason = ""
	ReasonJudgesSplit           Reason = "judges_split"
	ReasonBelowThreshold        Reason = "below_threshold"
	ReasonIsolationFloor        Reason = "below_isolation_floor"
	ReasonNoExecutedEquivalence Reason = "no_executed_equivalence"
	ReasonJudgedValid           Reason = "judged_valid"
	ReasonJudgedNotViolating    Reason = "judged_not_violating"
	ReasonJudgedUndetermined    Reason = "judged_undetermined"
	ReasonWitnessedNotIsolated  Reason = "witnessed_not_isolated"
	ReasonBudgetExhausted       Reason = "budget_exhausted"
	ReasonHostUnresponsive      Reason = "host_unresponsive"
	ReasonNoExecutor            Reason = "no_executor"
	ReasonNotAdjudicated        Reason = "not_adjudicated"
)

// Class is the class of a mutant for a criterion under G3-W.
type Class string

// Mutant classes.
const (
	// ClassSentinel is a semantic no-op drawn from the sentinel catalogue:
	// EQUIVALENT by construction.
	ClassSentinel Class = "sentinel"
	// ClassOwnerAttributed fails an example of the criterion and no example of
	// another one.
	ClassOwnerAttributed Class = "owner_attributed"
	// ClassValidE is VALID_e: it fails, reproduced, a block-bearing witness of the
	// criterion, and no test bound to another criterion killed it or errored on
	// it.
	ClassValidE Class = "valid_e"
	// ClassCollateral is VALID_e for the criterion and for another one: it blocks
	// the criterion when it survives and earns no credit.
	ClassCollateral Class = "collateral"
	// ClassExcluded is cleared by execution: see its route.
	ClassExcluded Class = "excluded"
	// ClassUnsettled is everything else; a surviving one escalates.
	ClassUnsettled Class = "unsettled"
)

// Route names how execution excluded a mutant.
type Route string

// Exclusion routes.
const (
	RouteNone Route = ""
	// RouteIdentity: it compiles to the reference's form (E0).
	RouteIdentity Route = "e0_identity"
	// RouteEquivalent: the full-resolution differential search found no
	// difference, every validity vote answered EQUIVALENT and no vote gave a
	// confirmed witness.
	RouteEquivalent Route = "f3_equivalent"
	// RouteOtherCriterion: it fails an example or an admitted witness of another
	// criterion and none of the criterion's own.
	RouteOtherCriterion Route = "other_criterion"
)

// Identities of the deciders recorded in `extensions.decision`.
const (
	DeciderSentinelCatalogue = "sentinel-catalogue"
	DeciderExamplePrefix     = "example-"
	DeciderWitnessPrefix     = "witness-"
	DeciderIdentity          = "compiled-form"
	DeciderHostJudge         = "host-judge"
)

// Evidence is one piece of executed evidence behind a claim.
type Evidence struct {
	Kind   format.CorroborationKind
	Digest string
}

// Decision is the class of one mutant for one criterion, the claim it is
// recorded as and the evidence behind it.
type Decision struct {
	Class      Class
	Route      Route
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
	// OffTarget is the criterion whose example or witness excluded the mutant.
	OffTarget *spec.Ref
	// Reason says why a mutant is unsettled.
	Reason Reason
	// Witness is the id of the block-bearing witness a VALID_e claim rests on:
	// a credit-eligible one when the mutant fails one.
	Witness string
	// Failing lists the block-bearing witnesses of the criterion the mutant
	// fails, reproduced, sorted.
	Failing []string
	// CollateralWith lists the other criteria the mutant is VALID_e for, sorted.
	CollateralWith []spec.Ref
	// CreditBearing is set for a mutant whose kill earns pass credit: an
	// owner-attributed mutant, or a VALID_e mutant that is not collateral and
	// fails a credit-eligible witness, under a basis that credits it.
	CreditBearing bool
}

// Carries reports whether the decision gives its mutant the power to block: an
// owner-attributed, VALID_e or collateral class.
func (d Decision) Carries() bool {
	return d.Class == ClassOwnerAttributed || d.Class == ClassValidE || d.Class == ClassCollateral
}

// Unsettled builds the decision of a mutant no execution settled and no vote
// could be read for.
func Unsettled(reason Reason) Decision {
	return Decision{Class: ClassUnsettled, Validity: format.ValidityUndetermined, Basis: format.BasisExecution, Confidence: format.MinConfidence, Deciders: []string{DeciderHostJudge}, Source: format.SourceExecution, Reason: reason}
}

// VoteQuestion is what a validity judge was asked about a mutant.
type VoteQuestion string

// Questions a validity ticket carries.
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

// Vote is one validity ticket's answer (V-H), with the provenance of its
// response.
type Vote struct {
	Answer     Answer           `json:"answer"`
	Confidence float64          `json:"confidence"`
	Isolation  format.Isolation `json:"isolation"`
	// Marked is set for a response journaled as leaked, exposed or conflicting.
	Marked  bool         `json:"marked,omitempty"`
	Witness WitnessCheck `json:"witness"`
	// WitnessDigest is the digest of the confirmed witness record.
	WitnessDigest string `json:"witnessDigest,omitempty"`
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

// Rules are what G3-W reads beyond the facts: the decision policy and whether
// the planted refactors take part in admitting a witness.
type Rules struct {
	Policy    policy.Policy
	Admission witness.Admission
}

// ScopeRule is what the scope judges must give for a witness to be
// block-bearing: `judge_k` votes, each FIXED at or above the threshold of the
// host votes.
func (r Rules) ScopeRule() witness.ScopeRule {
	return witness.ScopeRule{Votes: r.Policy.JudgeK, Threshold: r.Policy.TauJudge.For(format.HostVotesSource(r.Policy.JudgeK))}
}

// Universe indexes the witnesses of every criterion with their status under the
// rules.
type Universe struct {
	records  map[string]witness.Record
	statuses map[string]witness.Status
	byRef    map[spec.Ref][]string
}

// NewUniverse indexes the witness records.
func NewUniverse(records []witness.Record, rules Rules) Universe {
	universe := Universe{records: map[string]witness.Record{}, statuses: map[string]witness.Status{}, byRef: map[spec.Ref][]string{}}
	scope := rules.ScopeRule()
	for _, record := range records {
		id := record.Witness.ID
		if _, known := universe.records[id]; known {
			continue
		}
		universe.records[id] = record
		universe.statuses[id] = record.Status(rules.Admission, scope)
		universe.byRef[record.Witness.Criterion] = append(universe.byRef[record.Witness.Criterion], id)
	}
	for ref := range universe.byRef {
		slices.Sort(universe.byRef[ref])
	}
	return universe
}

// Status returns the status of a witness.
func (u Universe) Status(id string) witness.Status { return u.statuses[id] }

// Record returns the record of a witness.
func (u Universe) Record(id string) (witness.Record, bool) {
	record, known := u.records[id]
	return record, known
}

// Of lists the ids of the witnesses of a criterion, sorted.
func (u Universe) Of(ref spec.Ref) []string { return slices.Clone(u.byRef[ref]) }

// Criteria lists the criteria that have witnesses, sorted.
func (u Universe) Criteria() []spec.Ref {
	refs := make([]spec.Ref, 0, len(u.byRef))
	for ref := range u.byRef {
		refs = append(refs, ref)
	}
	slices.SortFunc(refs, spec.Ref.Compare)
	return refs
}

// BlockBearing reports whether the witness is block-bearing.
func (u Universe) BlockBearing(id string) bool { return u.statuses[id] == witness.StatusBlockBearing }

// admitted reports whether the witness is admitted: block-bearing or
// escalate-only.
func (u Universe) admitted(id string) bool {
	status := u.statuses[id]
	return status == witness.StatusBlockBearing || status == witness.StatusEscalateOnly
}

// FailingBlockBearing lists the block-bearing witnesses of the criterion the
// mutant fails, reproduced.
func (u Universe) FailingBlockBearing(f Facts, ref spec.Ref) []string {
	var failing []string
	for _, id := range u.byRef[ref] {
		if u.BlockBearing(id) && f.FailsWitness(id) {
			failing = append(failing, id)
		}
	}
	return failing
}

// FailsPassingWitness reports whether the mutant fails, reproduced, a witness of
// any criterion that the reference passes: executed evidence that the mutant
// behaves differently from the reference, whatever the witness's scope.
func (u Universe) FailsPassingWitness(f Facts) bool {
	for id, record := range u.records {
		if record.Reading() == witness.ReadingPasses && f.FailsWitness(id) {
			return true
		}
	}
	return false
}

// failsAdmitted reports whether the mutant fails, reproduced, an admitted
// witness of the criterion.
func (u Universe) failsAdmitted(f Facts, ref spec.Ref) bool {
	return slices.ContainsFunc(u.byRef[ref], func(id string) bool { return u.admitted(id) && f.FailsWitness(id) })
}

// Witnessed reports whether the pool may count the mutant towards the search's
// stop rule and the further witness draw: it is owner-attributed or fails,
// reproduced, a block-bearing witness of the criterion. Neither reads a kill
// vector or a binding, so the requests a criterion makes never depend on the
// tests.
func (u Universe) Witnessed(f Facts, ref spec.Ref) bool {
	return f.OwnerAttributed(ref) || len(u.FailingBlockBearing(f, ref)) > 0
}

// Decide classifies one pool mutant for the subject's criterion (G3-W, "Mutant
// classes"): a sentinel by construction, an identity by its compiled form, an
// owner-attributed mutant by its example, VALID_e and collateral by the
// block-bearing witnesses it fails, an exclusion by another criterion's example
// or witness, or by the full-resolution differential search with every validity
// vote EQUIVALENT; everything else is unsettled. No judgment alone settles a
// mutant.
func Decide(subject Subject, universe Universe, facts Facts, votes []Vote, rules Rules) Decision {
	switch {
	case facts.Origin == OriginSentinel:
		return sentinelDecision()
	case facts.Identity == IdentityIdentical:
		return identityDecision(facts)
	case facts.OwnerAttributed(subject.Ref):
		return ownerDecision(facts)
	}
	if failing := universe.FailingBlockBearing(facts, subject.Ref); len(failing) > 0 {
		if !subject.IsolatedFor(facts, subject.Ref) {
			return judgedOrUnsettled(universe, facts, votes, rules, ReasonWitnessedNotIsolated)
		}
		return witnessedDecision(subject, universe, facts, failing, rules)
	}
	if other, excluded := offTarget(subject.Ref, universe, facts); excluded {
		return offTargetDecision(facts, other)
	}
	return judgedOrUnsettled(universe, facts, votes, rules, ReasonNone)
}

func sentinelDecision() Decision {
	return Decision{
		Class: ClassSentinel, Validity: format.ValidityEquivalent, Basis: format.BasisConstruction,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderSentinelCatalogue}, Source: format.SourceExecution,
	}
}

func identityDecision(facts Facts) Decision {
	return Decision{
		Class: ClassExcluded, Route: RouteIdentity, Validity: format.ValidityEquivalent, Basis: format.BasisExecution,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderIdentity}, Source: format.SourceExecution,
		Corroboration: []Evidence{{Kind: format.CorroborationNoDiffFound, Digest: evidenceDigest(format.CorroborationNoDiffFound, facts.ID, RouteIdentity)}},
	}
}

func ownerDecision(facts Facts) Decision {
	return Decision{
		Class: ClassOwnerAttributed, Validity: format.ValidityValid, Basis: format.BasisSpecExample,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderExamplePrefix + facts.FailedExamples[0].String()}, Source: format.SourceExecution,
		Corroboration: []Evidence{{Kind: format.CorroborationExampleFailed, Digest: evidenceDigest(format.CorroborationExampleFailed, facts.ID, refStrings(facts.FailedExamples))}},
		CreditBearing: true,
	}
}

// witnessedDecision decides a mutant that fails, reproduced, a block-bearing
// witness of the criterion and is isolated for it: VALID_e, or collateral when it
// is VALID_e for another criterion too.
func witnessedDecision(subject Subject, universe Universe, facts Facts, failing []string, rules Rules) Decision {
	var collateral []spec.Ref
	for _, ref := range universe.Criteria() {
		if ref != subject.Ref && len(universe.FailingBlockBearing(facts, ref)) > 0 && subject.IsolatedFor(facts, ref) {
			collateral = append(collateral, ref)
		}
	}
	chosen, eligible := chooseWitness(universe, failing, rules.Policy.MinIsolation)
	decision := Decision{
		Class: ClassValidE, Validity: format.ValidityValid, Basis: format.BasisExecution,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderWitnessPrefix + shortID(chosen)}, Source: format.SourceExecution,
		Witness: chosen, Failing: failing, CollateralWith: collateral,
		CreditBearing: eligible && len(collateral) == 0 && rules.Policy.SignedPassBasis != policy.ExecutedOnly,
	}
	if len(collateral) > 0 {
		decision.Class = ClassCollateral
	}
	evidence := []Evidence{
		{Kind: format.CorroborationBuilds, Digest: evidenceDigest(format.CorroborationBuilds, facts.ID, nil)},
		{Kind: format.CorroborationOtherCriteriaPass, Digest: evidenceDigest(format.CorroborationOtherCriteriaPass, facts.ID, subject.OtherTests)},
		{Kind: format.CorroborationWitness, Digest: evidenceDigest(format.CorroborationWitness, facts.ID, chosen)},
	}
	if killers := subject.SpecificKillers(facts); decision.CreditBearing && len(killers) > 0 {
		evidence = append(evidence, Evidence{Kind: format.CorroborationSpecificKill, Digest: evidenceDigest(format.CorroborationSpecificKill, facts.ID, killers)})
	}
	decision.Corroboration = sortEvidence(evidence)
	return decision
}

// chooseWitness picks the witness a VALID_e claim rests on: the first
// credit-eligible one, else the first one.
func chooseWitness(universe Universe, failing []string, floor format.Isolation) (id string, eligible bool) {
	for _, candidate := range failing {
		if record, known := universe.Record(candidate); known && record.CreditEligible(floor) {
			return candidate, true
		}
	}
	return failing[0], false
}

func shortID(id string) string {
	const shortLength = 12
	if len(id) <= shortLength {
		return id
	}
	return id[:shortLength]
}

// otherFailure is the other criterion an off-target mutant fails, and whether it
// fails one of its examples or only one of its witnesses.
type otherFailure struct {
	ref     spec.Ref
	example bool
}

// offTarget finds the other criterion whose example or admitted witness the
// mutant fails, when it fails none of the criterion's own: the first one by
// reference, an example taking the place of a witness of the same criterion.
func offTarget(own spec.Ref, universe Universe, facts Facts) (otherFailure, bool) {
	if facts.FailsExampleOf(own) || universe.failsAdmitted(facts, own) {
		return otherFailure{}, false
	}
	var others []otherFailure
	for _, ref := range facts.FailedExamples {
		if ref != own {
			others = append(others, otherFailure{ref: ref, example: true})
		}
	}
	for _, ref := range universe.Criteria() {
		if ref != own && universe.failsAdmitted(facts, ref) {
			others = append(others, otherFailure{ref: ref})
		}
	}
	if len(others) == 0 {
		return otherFailure{}, false
	}
	slices.SortStableFunc(others, func(a, b otherFailure) int { return a.ref.Compare(b.ref) })
	return others[0], true
}

func offTargetDecision(facts Facts, other otherFailure) Decision {
	ref := other.ref
	decision := Decision{
		Class: ClassExcluded, Route: RouteOtherCriterion, Validity: format.ValidityNotViolating, Basis: format.BasisExecution,
		Confidence: format.MaxConfidence, Deciders: []string{DeciderWitnessPrefix + ref.String()}, Source: format.SourceExecution,
		Corroboration: []Evidence{{Kind: format.CorroborationOtherExampleFailed, Digest: evidenceDigest(format.CorroborationOtherExampleFailed, facts.ID, ref.String())}},
		OffTarget:     &ref,
	}
	if other.example {
		decision.Basis, decision.Deciders = format.BasisSpecExample, []string{DeciderExamplePrefix + ref.String()}
	}
	return decision
}

// judgedOrUnsettled reads the validity votes of a mutant no witness settled. Only
// one answer has an effect: EQUIVALENT from every vote, at or above the
// threshold and the floor, excludes a mutant whose full-resolution differential
// search found no difference, that no test killed, that fails no witness the
// reference passes and on which no vote confirmed a witness. Every other answer
// leaves it unsettled, with the reason the escalation shows.
func judgedOrUnsettled(universe Universe, facts Facts, votes []Vote, rules Rules, fallback Reason) Decision {
	if len(votes) == 0 {
		return Unsettled(fallbackReason(fallback, ReasonNotAdjudicated))
	}
	tally := TallyVotes(votes)
	base := Decision{
		Class: ClassUnsettled, Validity: format.ValidityUndetermined, Basis: format.BasisJudges, Confidence: tally.MinConfidence,
		Deciders: []string{DeciderHostJudge}, Source: format.HostVotesSource(len(votes)), Mapping: format.MappingWeakestStated,
	}
	reason := unsettledReason(universe, facts, votes, tally, rules)
	if reason == ReasonNone && fallback == ReasonNone {
		base.Class, base.Route, base.Validity = ClassExcluded, RouteEquivalent, format.ValidityEquivalent
		base.Corroboration = []Evidence{{Kind: format.CorroborationNoDiffFound, Digest: evidenceDigest(format.CorroborationNoDiffFound, facts.ID, RouteEquivalent)}}
		return base
	}
	base.Reason = fallbackReason(fallback, reason)
	return base
}

func fallbackReason(preferred, otherwise Reason) Reason {
	if preferred != ReasonNone {
		return preferred
	}
	return otherwise
}

// unsettledReason says why the votes do not exclude the mutant, or returns
// ReasonNone when they do.
func unsettledReason(universe Universe, facts Facts, votes []Vote, tally Tally, rules Rules) Reason {
	switch {
	case len(votes) < rules.Policy.JudgeK:
		return ReasonNoExecutor
	case !tally.Unanimous:
		return ReasonJudgesSplit
	case tally.Answer == AnswerValid:
		return ReasonJudgedValid
	case tally.Answer == AnswerNotViolating:
		return ReasonJudgedNotViolating
	case tally.Answer != AnswerEquivalent:
		return ReasonJudgedUndetermined
	case tally.MinConfidence < rules.Policy.TauJudge.For(format.HostVotesSource(len(votes))):
		return ReasonBelowThreshold
	case facts.Differential.Status != NoDiffFound || facts.KilledByAny() || universe.FailsPassingWitness(facts) ||
		confirmsWitness(votes) || rules.Policy.SignedPassBasis == policy.ExecutedOnly:
		return ReasonNoExecutedEquivalence
	case slices.ContainsFunc(votes, func(v Vote) bool { return !v.AtFloor(rules.Policy.MinIsolation) }):
		return ReasonIsolationFloor
	}
	return ReasonNone
}

func confirmsWitness(votes []Vote) bool {
	return slices.ContainsFunc(votes, func(v Vote) bool { return v.Witness == WitnessConfirmed })
}

func refStrings(refs []spec.Ref) []string {
	out := make([]string, len(refs))
	for index, ref := range refs {
		out[index] = ref.String()
	}
	return out
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
