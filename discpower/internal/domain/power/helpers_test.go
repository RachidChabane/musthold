package power_test

import (
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	testOwnA   ecosystem.TestID = "tests/test_sut.py::test_own_a"
	testOwnB   ecosystem.TestID = "tests/test_sut.py::test_own_b"
	testOtherA ecosystem.TestID = "tests/test_other.py::test_other_a"
	testOtherB ecosystem.TestID = "tests/test_other.py::test_other_b"

	mutantID      = "mutant-1"
	witnessDigest = "witness-digest"
	siteFirst     = "src/m.py::first"
	siteSecond    = "src/m.py::second"
	siteThird     = "src/m.py::third"

	judgeConfidence = 0.9
	fullConfidence  = 1.0
	lowConfidence   = 0.1
	floatTolerance  = 1e-9
	judgeCount      = 2
)

var (
	subjectRef = spec.Ref{ID: "SP-1", Revision: 1}
	otherRef   = spec.Ref{ID: "SP-2", Revision: 1}
	thirdRef   = spec.Ref{ID: "SP-3", Revision: 2}
)

var (
	verifiesSP164  = " [verifies SP-164~2]"
	verifiesSP165  = " [verifies SP-165~2]"
	verifiesSP166  = " [verifies SP-166~2]"
	verifiesSP167  = " [verifies SP-167~2]"
	verifiesSP168  = " [verifies SP-168~2]"
	verifiesSP169  = " [verifies SP-169~2]"
	verifiesSP184  = " [verifies SP-184~2]"
	verifiesSP406  = " [verifies SP-406~1]"
	verifiesSP902  = " [verifies SP-902~1]"
	verifiesSP1976 = " [verifies SP-1976~1]"
	verifiesSP1977 = " [verifies SP-1977~1]"
)

// bindings binds the own tests to the subject's criterion and the other tests to
// another criterion.
func bindings() map[ecosystem.TestID][]spec.Ref {
	return map[ecosystem.TestID][]spec.Ref{
		testOwnA: {subjectRef}, testOwnB: {subjectRef},
		testOtherA: {otherRef}, testOtherB: {otherRef},
	}
}

func newSubject(sentinelKillers ...ecosystem.TestID) power.Subject {
	killers := map[ecosystem.TestID]bool{}
	for _, test := range sentinelKillers {
		killers[test] = true
	}
	return power.NewSubject(subjectRef, bindings(), killers)
}

func allTests() []ecosystem.TestID {
	return []ecosystem.TestID{testOwnA, testOwnB, testOtherA, testOtherB}
}

// outcomesWith marks the given tests killed and every other test of the suite
// survived.
func outcomesWith(killed ...ecosystem.TestID) map[ecosystem.TestID]format.Outcome {
	outcomes := map[ecosystem.TestID]format.Outcome{}
	for _, test := range allTests() {
		outcomes[test] = format.OutcomeSurvived
	}
	for _, test := range killed {
		outcomes[test] = format.OutcomeKilled
	}
	return outcomes
}

func survivor(origin power.Origin, status power.DifferentialStatus) power.Facts {
	return power.Facts{
		ID: mutantID, Origin: origin, Slot: power.SlotDirect, Sites: []string{siteFirst},
		Outcomes:     outcomesWith(),
		Differential: power.Differential{Status: status},
	}
}

func killedBy(origin power.Origin, killers ...ecosystem.TestID) power.Facts {
	facts := survivor(origin, power.Skipped)
	facts.Outcomes = outcomesWith(killers...)
	return facts
}

// failing marks the mutant failing the witnesses, reproduced.
func failing(facts power.Facts, witnesses ...string) power.Facts {
	facts.WitnessOutcomes = map[string]witness.Outcome{}
	for _, id := range witnesses {
		facts.WitnessOutcomes[id] = witness.OutcomeFailed
	}
	return facts
}

func vote(answer power.Answer, confidence float64, isolation format.Isolation) power.Vote {
	return power.Vote{Answer: answer, Confidence: confidence, Isolation: isolation, Witness: power.WitnessAbsent}
}

func confirmedVote(confidence float64, isolation format.Isolation) power.Vote {
	judged := vote(power.AnswerValid, confidence, isolation)
	judged.Witness, judged.WitnessDigest = power.WitnessConfirmed, witnessDigest
	return judged
}

func repeated(judged power.Vote, count int) []power.Vote {
	votes := make([]power.Vote, count)
	for index := range votes {
		votes[index] = judged
	}
	return votes
}

func kinds(evidence []power.Evidence) []format.CorroborationKind {
	out := make([]format.CorroborationKind, len(evidence))
	for index, item := range evidence {
		out[index] = item.Kind
	}
	return out
}

func hasKind(evidence []power.Evidence, kind format.CorroborationKind) bool {
	for _, item := range evidence {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func withBasis(basis policy.SignedPassBasis) policy.Policy {
	rules := policy.Default()
	rules.SignedPassBasis = basis
	return rules
}

func rulesOf(rules policy.Policy) power.Rules {
	return power.Rules{Policy: rules, Admission: witness.AdmissionReferenceOnly}
}

// Witness ids of the fixtures.
const (
	ownWitness        = "own-witness"
	ownWitnessLow     = "own-witness-below-floor"
	ownEscalateOnly   = "own-escalate-only"
	ownConflict       = "own-reading-conflict"
	otherWitness      = "other-witness"
	otherEscalateOnly = "other-escalate-only"
)

// record builds the record of a witness of a criterion: passing the reference
// and approved by both scope judges when blockBearing, with a writer at the
// given isolation.
func record(id string, criterion spec.Ref, blockBearing bool, isolation format.Isolation) witness.Record {
	answer := witness.ScopeFixed
	if !blockBearing {
		answer = witness.ScopeNotFixed
	}
	passed := []witness.Outcome{witness.OutcomePassed, witness.OutcomePassed, witness.OutcomePassed}
	scope := witness.ScopeVote{Answer: answer, Confidence: fullConfidence, Isolation: format.IsolationRestricted}
	return witness.Record{
		Witness:   witness.Witness{ID: id, Criterion: criterion, Name: id, Clause: id, Kind: witness.KindExample},
		Writer:    witness.Writer{Draw: 1, Isolation: isolation},
		Reference: passed,
		Scope:     []witness.ScopeVote{scope, scope},
	}
}

// universeRecords are the witnesses of the subject's criterion and of another
// one used across the tests.
func universeRecords() []witness.Record {
	conflict := record(ownConflict, subjectRef, true, format.IsolationRestricted)
	conflict.Reference = []witness.Outcome{witness.OutcomeFailed, witness.OutcomeFailed, witness.OutcomeFailed}
	return []witness.Record{
		record(ownWitness, subjectRef, true, format.IsolationRestricted),
		record(ownWitnessLow, subjectRef, true, format.IsolationInline),
		record(ownEscalateOnly, subjectRef, false, format.IsolationRestricted),
		conflict,
		record(otherWitness, otherRef, true, format.IsolationRestricted),
		record(otherEscalateOnly, otherRef, false, format.IsolationRestricted),
	}
}

func universe(rules policy.Policy) power.Universe {
	return power.NewUniverse(universeRecords(), rulesOf(rules))
}

func decide(facts power.Facts, votes []power.Vote, rules policy.Policy) power.Decision {
	return power.Decide(newSubject(), universe(rules), facts, votes, rulesOf(rules))
}

func standing(entries ...power.Entry) power.Standing {
	return power.Standing{Subject: newSubject(), CorrectCodePasses: true, Witnesses: universeRecords(), Entries: entries}
}
