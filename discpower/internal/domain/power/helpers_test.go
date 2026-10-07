package power_test

import (
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
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
	floatTolerance  = 1e-9
)

var (
	subjectRef = spec.Ref{ID: "SP-1", Revision: 1}
	otherRef   = spec.Ref{ID: "SP-2", Revision: 1}
	thirdRef   = spec.Ref{ID: "SP-3", Revision: 2}
)

const (
	tagOpen  = "["
	tagVerb  = "verifies"
	tagClose = "]"
)

// verifies renders the suffix that ties a subtest to a requirement.
func verifies(requirement string) string {
	return " " + tagOpen + tagVerb + " " + requirement + "~1" + tagClose
}

var (
	verifiesSP164 = verifies("SP-164")
	verifiesSP165 = verifies("SP-165")
	verifiesSP166 = verifies("SP-166")
	verifiesSP167 = verifies("SP-167")
	verifiesSP168 = verifies("SP-168")
	verifiesSP169 = " [verifies SP-169~2]"
	verifiesSP902 = " [verifies SP-902~1]"
)

func newSubject(sentinelKillers ...ecosystem.TestID) power.Subject {
	killers := map[ecosystem.TestID]bool{}
	for _, test := range sentinelKillers {
		killers[test] = true
	}
	return power.Subject{
		Ref:             subjectRef,
		Tests:           []ecosystem.TestID{testOwnA, testOwnB},
		OtherTests:      []ecosystem.TestID{testOtherA, testOtherB},
		SentinelKillers: killers,
	}
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
