package prove_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const claimWitness = "claim-witness"

// claimUniverse holds one block-bearing witness of the criterion, written at the
// strongest isolation.
func claimUniverse(rules power.Rules) power.Universe {
	passed := []witness.Outcome{witness.OutcomePassed, witness.OutcomePassed, witness.OutcomePassed}
	fixed := witness.ScopeVote{Answer: witness.ScopeFixed, Confidence: format.MaxConfidence, Isolation: format.IsolationRestrictedAttested}
	record := witness.Record{
		Witness:   witness.Witness{ID: claimWitness, Criterion: criterion, Kind: witness.KindExample},
		Writer:    witness.Writer{Draw: 1, Isolation: format.IsolationRestrictedAttested},
		Reference: passed, Scope: []witness.ScopeVote{fixed, fixed},
	}
	return power.NewUniverse([]witness.Record{record}, rules)
}

func votesOf(t *rapid.T, checks []power.WitnessCheck) []power.Vote {
	votes := make([]power.Vote, rapid.IntRange(1, maxVotes).Draw(t, "votes"))
	for index := range votes {
		votes[index] = vote(t, fmt.Sprint("vote", index), checks)
	}
	return votes
}

func TestClaimPowerG3W(t *testing.T) {
	every := []power.WitnessCheck{power.WitnessAbsent, power.WitnessConfirmed, power.WitnessRefused}

	t.Run("votes alone never give a mutant the power to block or to earn credit [verifies SP-184~2]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := power.Rules{Policy: rules(t), Admission: witness.AdmissionReferenceOnly}
			decision := power.Decide(subject(), claimUniverse(rules), facts(t), votesOf(t, every), rules)
			if decision.Carries() || decision.CreditBearing {
				t.Fatalf("votes alone gave power: %+v", decision)
			}
		})
	})

	t.Run("the most pass-favourable votes never clear a survivor the search found a difference for [verifies SP-184~2]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := power.Rules{Policy: rules(t), Admission: witness.AdmissionReferenceOnly}
			drawn := facts(t)
			for test := range drawn.Outcomes {
				drawn.Outcomes[test] = format.OutcomeSurvived
			}
			drawn.Differential.Status = power.DiffersAt
			for _, answer := range []power.Answer{power.AnswerEquivalent, power.AnswerNotViolating, power.AnswerValid} {
				favourable := power.Vote{Answer: answer, Confidence: format.MaxConfidence, Isolation: format.IsolationRestrictedAttested, Witness: power.WitnessAbsent}
				entry := power.Entry{Facts: drawn, Votes: []power.Vote{favourable, favourable, favourable, favourable}}
				outcome := power.Terminal(power.Standing{Subject: subject(), CorrectCodePasses: true, Entries: []power.Entry{entry}}, rules)
				if outcome.Result == power.Pass || outcome.Result == power.Block {
					t.Fatalf("%s votes alone gave %+v", answer, outcome)
				}
			}
		})
	})

	t.Run("adding a surviving mutant, witnessed or not, never lowers the severity [verifies SP-184~2]", func(t *testing.T) {
		rules := power.Rules{Policy: policy.Default(), Admission: witness.AdmissionReferenceOnly}
		severity := map[power.Result]hostprototest.Severity{power.Pass: hostprototest.SeverityPass, power.Escalate: hostprototest.SeverityEscalate, power.Block: hostprototest.SeverityBlock}
		decide := func(entries []power.Entry) hostprototest.Severity {
			standing := power.Standing{Subject: subject(), CorrectCodePasses: true, Witnesses: claimRecords(rules), Entries: entries}
			return severity[power.Terminal(standing, rules).Result]
		}
		survivor := rapid.Custom(func(t *rapid.T) power.Entry {
			outcomes := map[ecosystem.TestID]format.Outcome{}
			for _, test := range append(subject().Tests, subject().OtherTests...) {
				outcomes[test] = format.OutcomeSurvived
			}
			facts := power.Facts{ID: rapid.StringMatching(`[a-f0-9]{8}`).Draw(t, "id"), Origin: power.OriginTargeted, Outcomes: outcomes, Differential: power.Differential{Status: power.DiffersAt}}
			if rapid.Bool().Draw(t, "witnessed") {
				facts.WitnessOutcomes = map[string]witness.Outcome{claimWitness: witness.OutcomeFailed}
			}
			return power.Entry{Facts: facts}
		})
		hardens := func(power.Entry) bool { return true }
		rapid.Check(t, func(t *rapid.T) { hostprototest.CheckMonotone(t, decide, hardens, survivor) })
	})
}

func claimRecords(rules power.Rules) []witness.Record {
	universe := claimUniverse(rules)
	record, _ := universe.Record(claimWitness)
	return []witness.Record{record}
}
