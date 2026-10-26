package power_test

import (
	"fmt"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
)

const (
	maxVotes   = 4
	maxEntries = 5
)

var witnessIDs = []string{ownWitness, ownWitnessLow, ownEscalateOnly, ownConflict, otherWitness, otherEscalateOnly}

func genOutcomes() *rapid.Generator[map[ecosystem.TestID]format.Outcome] {
	outcome := rapid.SampledFrom([]format.Outcome{format.OutcomeKilled, format.OutcomeSurvived, format.OutcomeError})
	return rapid.Custom(func(t *rapid.T) map[ecosystem.TestID]format.Outcome {
		outcomes := map[ecosystem.TestID]format.Outcome{}
		for _, test := range allTests() {
			outcomes[test] = outcome.Draw(t, string(test))
		}
		return outcomes
	})
}

func genWitnessOutcomes(ids []string) *rapid.Generator[map[string]witness.Outcome] {
	outcome := rapid.SampledFrom([]witness.Outcome{witness.OutcomePassed, witness.OutcomeFailed, witness.OutcomeFlaky, witness.OutcomeErrored})
	return rapid.Custom(func(t *rapid.T) map[string]witness.Outcome {
		outcomes := map[string]witness.Outcome{}
		if len(ids) == 0 {
			rapid.Just(0).Draw(t, "none")
			return outcomes
		}
		for _, id := range ids {
			outcomes[id] = outcome.Draw(t, id)
		}
		return outcomes
	})
}

func genFacts(id string, witnesses []string) *rapid.Generator[power.Facts] {
	status := rapid.SampledFrom([]power.DifferentialStatus{power.DiffersAt, power.NoDiffFound, power.Skipped})
	identity := rapid.SampledFrom([]power.Identity{power.IdentityUnknown, power.IdentityDiffers, power.IdentityIdentical})
	return rapid.Custom(func(t *rapid.T) power.Facts {
		var failed []spec.Ref
		for _, ref := range []spec.Ref{subjectRef, otherRef, thirdRef} {
			if rapid.Bool().Draw(t, "example-"+ref.String()) {
				failed = append(failed, ref)
			}
		}
		return power.Facts{
			ID: id, Origin: rapid.SampledFrom([]power.Origin{power.OriginTargeted, power.OriginOperator, power.OriginSentinel}).Draw(t, "origin"),
			Slot: power.SlotDirect, Sites: []string{rapid.SampledFrom([]string{siteFirst, siteSecond, siteThird}).Draw(t, "site")},
			Outcomes: genOutcomes().Draw(t, "outcomes"), FailedExamples: failed,
			Differential:    power.Differential{Status: status.Draw(t, "status")},
			Identity:        identity.Draw(t, "identity"),
			WitnessOutcomes: genWitnessOutcomes(witnesses).Draw(t, "witnesses"),
		}
	})
}

func genVote() *rapid.Generator[power.Vote] {
	return rapid.Custom(func(t *rapid.T) power.Vote {
		return power.Vote{
			Answer:        rapid.SampledFrom([]power.Answer{power.AnswerValid, power.AnswerEquivalent, power.AnswerNotViolating, power.AnswerUndetermined}).Draw(t, "answer"),
			Confidence:    rapid.Float64Range(0, 1).Draw(t, "confidence"),
			Isolation:     rapid.SampledFrom(format.Isolations()).Draw(t, "isolation"),
			Marked:        rapid.Bool().Draw(t, "marked"),
			Witness:       rapid.SampledFrom([]power.WitnessCheck{power.WitnessAbsent, power.WitnessConfirmed}).Draw(t, "witness"),
			WitnessDigest: witnessDigest,
		}
	})
}

func genRules() *rapid.Generator[policy.Policy] {
	return rapid.Custom(func(t *rapid.T) policy.Policy {
		rules := policy.Default()
		rules.TauJudge = format.UniformThreshold(rapid.Float64Range(0, 1).Draw(t, "tauJudge"))
		rules.JudgeK = rapid.IntRange(1, maxVotes).Draw(t, "judgeK")
		rules.SignedPassBasis = rapid.SampledFrom(policy.Bases()).Draw(t, "basis")
		rules.MinIsolation = rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
		return rules
	})
}

func genEntries() *rapid.Generator[[]power.Entry] {
	return rapid.Custom(func(t *rapid.T) []power.Entry {
		count := rapid.IntRange(0, maxEntries).Draw(t, "count")
		entries := make([]power.Entry, count)
		for index := range entries {
			entries[index] = power.Entry{
				Facts: genFacts(fmt.Sprintf("m%d", index), witnessIDs).Draw(t, fmt.Sprintf("facts%d", index)),
				Votes: rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, fmt.Sprintf("votes%d", index)),
			}
		}
		return entries
	})
}

func TestDecideProperties(t *testing.T) {
	t.Run("a mutant that fails no witness and no example never blocks or earns credit, whatever the votes"+verifiesSP184, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := genRules().Draw(t, "rules")
			facts := genFacts(mutantID, nil).Draw(t, "facts")
			facts.FailedExamples = nil
			votes := rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes")
			if got := decide(facts, votes, rules); got.Carries() || got.CreditBearing {
				t.Fatalf("decision %+v", got)
			}
		})
	})

	t.Run("no judgment alone clears a survivor: without an executed exclusion, votes leave it unsettled"+verifiesSP184, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := genRules().Draw(t, "rules")
			facts := genFacts(mutantID, nil).Draw(t, "facts")
			facts.Origin, facts.FailedExamples, facts.Identity = power.OriginTargeted, nil, power.IdentityDiffers
			facts.Differential.Status = rapid.SampledFrom([]power.DifferentialStatus{power.DiffersAt, power.Skipped}).Draw(t, "status")
			votes := rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes")
			if got := decide(facts, votes, rules); got.Class != power.ClassUnsettled {
				t.Fatalf("class %q for votes %+v", got.Class, votes)
			}
		})
	})

	t.Run("a carrying decision always rests on executed evidence"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := genRules().Draw(t, "rules")
			got := decide(genFacts(mutantID, witnessIDs).Draw(t, "facts"), rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes"), rules)
			if got.Carries() && (len(got.Corroboration) == 0 || got.Source != format.SourceExecution) {
				t.Fatalf("decision %+v", got)
			}
		})
	})

	t.Run("collateral never bears credit, and credit needs a credit-eligible witness or an example"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := genRules().Draw(t, "rules")
			facts := genFacts(mutantID, witnessIDs).Draw(t, "facts")
			got := decide(facts, nil, rules)
			if got.Class == power.ClassCollateral && got.CreditBearing {
				t.Fatalf("collateral bears credit: %+v", got)
			}
			if got.Class == power.ClassValidE && got.CreditBearing && (rules.SignedPassBasis == policy.ExecutedOnly || !format.IsolationRestricted.AtLeast(rules.MinIsolation)) {
				t.Fatalf("credit without a witness at the floor: %+v under %+v", got, rules)
			}
		})
	})
}

func TestEvaluateProperties(t *testing.T) {
	t.Run("adding a reproduced failure of a block-bearing witness never turns a block into a pass"+verifiesSP184, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := rulesOf(policy.Default())
			current := standing(genEntries().Draw(t, "entries")...)
			before := power.Terminal(current, rules)
			if len(current.Entries) == 0 {
				return
			}
			index := rapid.IntRange(0, len(current.Entries)-1).Draw(t, "index")
			id := rapid.SampledFrom([]string{ownWitness, ownWitnessLow, otherWitness}).Draw(t, "witness")
			current.Entries[index].Facts.WitnessOutcomes[id] = witness.OutcomeFailed
			after := power.Terminal(current, rules)
			if before.Result == power.Block && after.Result == power.Pass {
				t.Fatalf("%+v became %+v", before, after)
			}
		})
	})

	t.Run("a pass needs a killed credit-bearing mutant, and the order of entries is irrelevant"+verifiesSP167, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			rules := rulesOf(genRules().Draw(t, "rules"))
			entries := genEntries().Draw(t, "entries")
			got := power.Evaluate(standing(entries...), rules)
			reversed := make([]power.Entry, len(entries))
			for index, item := range entries {
				reversed[len(entries)-1-index] = item
			}
			if power.Terminal(standing(reversed...), rules).Result != got.Outcome.Result {
				t.Fatal("the order of entries changed the outcome")
			}
			if got.Outcome.Result != power.Pass {
				return
			}
			subject := newSubject()
			for _, item := range entries {
				decision := got.Decisions[item.Facts.ID]
				if decision.CreditBearing && subject.Killed(item.Facts) && len(subject.SpecificKillers(item.Facts)) > 0 {
					return
				}
			}
			t.Fatal("a pass without a killed credit-bearing mutant")
		})
	})

	t.Run("adding a surviving VALID_e mutant always blocks"+verifiesSP167, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			entries := genEntries().Draw(t, "entries")
			blocker := power.Entry{Facts: withID(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), "blocker")}
			if got := power.Terminal(standing(append(entries, blocker)...), rulesOf(policy.Default())); got.Result != power.Block {
				t.Fatalf("outcome %+v", got)
			}
		})
	})
}

func TestScreenProperties(t *testing.T) {
	t.Run("the violation probability lies in [0,1] and flags are monotone in the threshold"+verifiesSP169, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			answer := rapid.Custom(func(t *rapid.T) power.ClosedAnswer {
				return power.ClosedAnswer{
					Choice:      rapid.SampledFrom([]power.ClosedChoice{power.ChoiceViolates, power.ChoiceConsistent}).Draw(t, "choice"),
					Violates:    rapid.Float64Range(0, 1).Draw(t, "violates"),
					HasViolates: rapid.Bool().Draw(t, "has"),
				}
			})
			probability := power.ViolationProbability(rapid.SliceOfN(answer, 0, maxVotes).Draw(t, "answers"))
			if probability < 0 || probability > 1 {
				t.Fatalf("probability %v", probability)
			}
			low := rapid.Float64Range(0, 1).Draw(t, "low")
			high := rapid.Float64Range(low, 1).Draw(t, "high")
			if power.FlagsSurvivor(probability, high) && !power.FlagsSurvivor(probability, low) {
				t.Fatalf("a survivor flagged at %v is not flagged at %v", high, low)
			}
		})
	})
}
