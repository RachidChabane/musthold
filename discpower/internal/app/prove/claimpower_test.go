package prove_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	ownTestCount   = 2
	otherTestCount = 2
	maxVotes       = 4
)

var criterion = spec.Ref{ID: "PAR-3", Revision: 1}

func testsNamed(prefix string, count int) []ecosystem.TestID {
	tests := make([]ecosystem.TestID, count)
	for index := range tests {
		tests[index] = ecosystem.TestID(fmt.Sprintf("%s%d", prefix, index))
	}
	return tests
}

func subject() power.Subject {
	return power.Subject{Ref: criterion, Tests: testsNamed("own", ownTestCount), OtherTests: testsNamed("other", otherTestCount), SentinelKillers: map[ecosystem.TestID]bool{}}
}

func outcomes() *rapid.Generator[format.Outcome] {
	return rapid.SampledFrom([]format.Outcome{format.OutcomeKilled, format.OutcomeSurvived, format.OutcomeError})
}

// facts draws the executed facts of a judged mutant: no example failed.
func facts(t *rapid.T) power.Facts {
	results := map[ecosystem.TestID]format.Outcome{}
	for _, test := range append(subject().Tests, subject().OtherTests...) {
		results[test] = outcomes().Draw(t, string(test))
	}
	status := rapid.SampledFrom([]power.DifferentialStatus{power.DiffersAt, power.NoDiffFound, power.Skipped}).Draw(t, "differential")
	return power.Facts{ID: "mutant", Origin: power.OriginTargeted, Sites: []string{"parcel/rates.py::shipping_fee"}, Outcomes: results, Differential: power.Differential{Status: status}}
}

func rules(t *rapid.T) policy.Policy {
	rules := policy.Default()
	rules.SignedPassBasis = rapid.SampledFrom(policy.Bases()).Draw(t, "basis")
	rules.MinIsolation = rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
	rules.TauJudge = rapid.Float64Range(0, 1).Draw(t, "tau")
	return rules
}

// vote draws a judge's vote; the witness checks allowed are the given ones.
func vote(t *rapid.T, label string, witnesses []power.WitnessCheck) power.Vote {
	return power.Vote{
		Answer:        rapid.SampledFrom([]power.Answer{power.AnswerValid, power.AnswerEquivalent, power.AnswerNotViolating, power.AnswerUndetermined}).Draw(t, label+"answer"),
		Confidence:    rapid.Float64Range(0, 1).Draw(t, label+"confidence"),
		Isolation:     rapid.SampledFrom(format.Isolations()).Draw(t, label+"isolation"),
		Marked:        rapid.Bool().Draw(t, label+"marked"),
		Witness:       rapid.SampledFrom(witnesses).Draw(t, label+"witness"),
		WitnessDigest: format.SHA256Hex([]byte(label)),
	}
}

func TestClaimPower(t *testing.T) {
	unconfirmed := []power.WitnessCheck{power.WitnessAbsent, power.WitnessRefused}

	t.Run("a judged claim never carries power without executed corroboration [verifies SP-184~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			votes := make([]power.Vote, rapid.IntRange(1, maxVotes).Draw(t, "votes"))
			for index := range votes {
				votes[index] = vote(t, fmt.Sprint("vote", index), unconfirmed)
			}
			decision := power.Decide(subject(), facts(t), votes, rules(t))
			if decision.Carries() {
				t.Fatalf("judges with no confirmed witness gave a decision with power: %+v", decision)
			}
		})
	})

	t.Run("a decision with power always lists its corroboration [verifies SP-184~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			votes := make([]power.Vote, rapid.IntRange(1, maxVotes).Draw(t, "votes"))
			for index := range votes {
				votes[index] = vote(t, fmt.Sprint("vote", index), []power.WitnessCheck{power.WitnessAbsent, power.WitnessConfirmed, power.WitnessRefused})
			}
			decision := power.Decide(subject(), facts(t), votes, rules(t))
			if decision.Carries() && len(decision.Corroboration) == 0 {
				t.Fatalf("a decision with power has no corroboration: %+v", decision)
			}
		})
	})

	t.Run("the most pass-favourable judges never block or credit without corroboration [verifies SP-184~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			favourable := power.Vote{Answer: power.AnswerValid, Confidence: format.MaxConfidence, Isolation: format.IsolationRestrictedAttested, Witness: power.WitnessAbsent}
			votes := make([]power.Vote, rapid.IntRange(1, maxVotes).Draw(t, "votes"))
			for index := range votes {
				votes[index] = favourable
			}
			drawn := facts(t)
			standing := power.Standing{Subject: subject(), CorrectCodePasses: true}
			standing.Entries = []power.Entry{{Facts: drawn, Decision: power.Decide(subject(), drawn, votes, rules(t))}}
			if outcome := power.Terminal(standing); outcome.Result == power.Block || outcome.Has(power.FindingSurvivingValid) {
				t.Fatalf("judged-only evidence blocked: %+v", outcome)
			}
		})
	})

	t.Run("adding a surviving mutant that is VALID or undetermined never lowers the severity [verifies SP-184~1]", func(t *testing.T) {
		severity := map[power.Result]hostprototest.Severity{power.Pass: hostprototest.SeverityPass, power.Escalate: hostprototest.SeverityEscalate, power.Block: hostprototest.SeverityBlock}
		decide := func(entries []power.Entry) hostprototest.Severity {
			return severity[power.Terminal(power.Standing{Subject: subject(), CorrectCodePasses: true, Entries: entries}).Result]
		}
		survivor := rapid.Custom(func(t *rapid.T) power.Entry {
			outcomes := map[ecosystem.TestID]format.Outcome{}
			for _, test := range append(subject().Tests, subject().OtherTests...) {
				outcomes[test] = format.OutcomeSurvived
			}
			class := rapid.SampledFrom([]power.Class{power.ClassValid, power.ClassUndetermined, power.ClassEquivalent, power.ClassOffTarget}).Draw(t, "class")
			return power.Entry{
				Facts:    power.Facts{ID: rapid.StringMatching(`[a-f0-9]{8}`).Draw(t, "id"), Origin: power.OriginTargeted, Outcomes: outcomes},
				Decision: power.Decision{Class: class},
			}
		})
		hardens := func(entry power.Entry) bool {
			return entry.Decision.Class == power.ClassValid || entry.Decision.Class == power.ClassUndetermined
		}
		rapid.Check(t, func(t *rapid.T) { hostprototest.CheckMonotone(t, decide, hardens, survivor) })
	})
}
