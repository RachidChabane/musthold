package power_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const maxVotes = 4

func genOrigin() *rapid.Generator[power.Origin] {
	return rapid.SampledFrom([]power.Origin{power.OriginTargeted, power.OriginOperator, power.OriginSentinel})
}

func genNonSentinelOrigin() *rapid.Generator[power.Origin] {
	return rapid.SampledFrom([]power.Origin{power.OriginTargeted, power.OriginOperator})
}

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

func genFailedExamples() *rapid.Generator[[]spec.Ref] {
	return rapid.Custom(func(t *rapid.T) []spec.Ref {
		var failed []spec.Ref
		for _, ref := range []spec.Ref{subjectRef, otherRef, thirdRef} {
			if rapid.Bool().Draw(t, ref.String()) {
				failed = append(failed, ref)
			}
		}
		return failed
	})
}

func genFacts(origin *rapid.Generator[power.Origin], failed *rapid.Generator[[]spec.Ref]) *rapid.Generator[power.Facts] {
	status := rapid.SampledFrom([]power.DifferentialStatus{power.DiffersAt, power.NoDiffFound, power.Skipped})
	return rapid.Custom(func(t *rapid.T) power.Facts {
		return power.Facts{
			ID: mutantID, Origin: origin.Draw(t, "origin"), Slot: power.SlotDirect, Sites: []string{siteFirst},
			Outcomes:       genOutcomes().Draw(t, "outcomes"),
			FailedExamples: failed.Draw(t, "failed"),
			Differential:   power.Differential{Status: status.Draw(t, "status")},
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
			Witness:       rapid.SampledFrom([]power.WitnessCheck{power.WitnessAbsent, power.WitnessConfirmed, power.WitnessRefused}).Draw(t, "witness"),
			WitnessDigest: witnessDigest,
		}
	})
}

func genRules() *rapid.Generator[policy.Policy] {
	return rapid.Custom(func(t *rapid.T) policy.Policy {
		rules := policy.Default()
		rules.TauJudge = rapid.Float64Range(0, 1).Draw(t, "tauJudge")
		rules.JudgeK = rapid.IntRange(1, maxVotes).Draw(t, "judgeK")
		rules.SignedPassBasis = rapid.SampledFrom(policy.Bases()).Draw(t, "basis")
		rules.MinIsolation = rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
		return rules
	})
}

func genSubject() *rapid.Generator[power.Subject] {
	return rapid.Custom(func(t *rapid.T) power.Subject {
		subject := newSubject()
		for _, test := range allTests() {
			if rapid.Bool().Draw(t, "sentinelKiller-"+string(test)) {
				subject.SentinelKillers[test] = true
			}
		}
		return subject
	})
}

func TestDecideProperties(t *testing.T) {
	t.Run("a VALID decision always carries corroboration, and a sentinel never carries power"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			subject := genSubject().Draw(t, "subject")
			facts := genFacts(genOrigin(), genFailedExamples()).Draw(t, "facts")
			votes := rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes")
			got := power.Decide(subject, facts, votes, genRules().Draw(t, "rules"))
			if got.Carries() && len(got.Corroboration) == 0 {
				t.Fatalf("a %s decision has no corroboration: %+v", got.Class, got)
			}
			if got.Class == power.ClassValid && len(got.Corroboration) == 0 && len(facts.FailedExamples) == 0 {
				t.Fatalf("a VALID decision is neither corroborated nor attributed by examples")
			}
			if facts.Origin == power.OriginSentinel && got.Carries() {
				t.Fatalf("a sentinel carries power")
			}
			if got.Class == power.ClassUndetermined && (got.Reason == power.ReasonNone || got.Carries()) {
				t.Fatalf("an undetermined decision without reason: %+v", got)
			}
		})
	})

	t.Run("the most favourable votes never give power to a mutant without a confirmed witness"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			subject := genSubject().Draw(t, "subject")
			facts := genFacts(genNonSentinelOrigin(), rapid.Just([]spec.Ref(nil))).Draw(t, "facts")
			favourable := power.Vote{
				Answer: power.AnswerValid, Confidence: fullConfidence, Isolation: format.IsolationRestrictedAttested,
				Witness: rapid.SampledFrom([]power.WitnessCheck{power.WitnessAbsent, power.WitnessRefused}).Draw(t, "witness"),
			}
			rules := genRules().Draw(t, "rules")
			votes := repeated(favourable, rapid.IntRange(rules.JudgeK, maxVotes+rules.JudgeK).Draw(t, "count"))
			if got := power.Decide(subject, facts, votes, rules); got.Carries() {
				t.Fatalf("a mutant without a confirmed witness carries power: %+v", got)
			}
		})
	})

	t.Run("raising tau_judge never turns an undetermined decision into a carrying one"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			subject := genSubject().Draw(t, "subject")
			facts := genFacts(genNonSentinelOrigin(), rapid.Just([]spec.Ref(nil))).Draw(t, "facts")
			votes := rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes")
			rules := genRules().Draw(t, "rules")
			higher := rules
			higher.TauJudge = rapid.Float64Range(rules.TauJudge, 1).Draw(t, "higherTau")
			if power.Decide(subject, facts, votes, rules).Class != power.ClassUndetermined {
				return
			}
			if got := power.Decide(subject, facts, votes, higher); got.Carries() {
				t.Fatalf("raising tau_judge from %v to %v produced %+v", rules.TauJudge, higher.TauJudge, got)
			}
		})
	})

	t.Run("a decision from judges depends on tau_judge only through the confidence check"+verifiesSP165, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			subject := genSubject().Draw(t, "subject")
			facts := genFacts(genNonSentinelOrigin(), rapid.Just([]spec.Ref(nil))).Draw(t, "facts")
			votes := rapid.SliceOfN(genVote(), 0, maxVotes).Draw(t, "votes")
			rules := genRules().Draw(t, "rules")
			lowest := power.TallyVotes(votes).MinConfidence
			if len(votes) == 0 || lowest < rules.TauJudge {
				return
			}
			zero := rules
			zero.TauJudge = 0
			got, baseline := power.Decide(subject, facts, votes, rules), power.Decide(subject, facts, votes, zero)
			if got.Class != baseline.Class || got.Reason != baseline.Reason {
				t.Fatalf("decision %+v differs from the one at tau_judge 0: %+v", got, baseline)
			}
		})
	})
}

type entryKind int

const (
	kindValidKilledSpecific entryKind = iota
	kindEquivalentSurvivor
	kindOffTargetSurvivor
	kindUnattributedSurvivor
	kindUndeterminedSurvivor
	kindValidSurvivor
	kindSentinel
	kindKilledUndetermined
)

func entryFor(kind entryKind, site string) power.Entry {
	var entry power.Entry
	switch kind {
	case kindValidKilledSpecific:
		entry = validKilled()
	case kindEquivalentSurvivor:
		entry = entryOf(survivor(power.OriginTargeted, power.NoDiffFound), power.ClassEquivalent)
	case kindOffTargetSurvivor:
		entry = entryOf(survivor(power.OriginOperator, power.Skipped), power.ClassOffTarget)
	case kindUnattributedSurvivor:
		entry = entryOf(survivor(power.OriginTargeted, power.Skipped), power.ClassUnattributed)
	case kindUndeterminedSurvivor:
		entry = undeterminedSurvivor()
	case kindValidSurvivor:
		entry = validSurviving()
	case kindSentinel:
		entry = entryOf(survivor(power.OriginSentinel, power.NoDiffFound), power.ClassEquivalent)
	default:
		entry = killedUndetermined()
	}
	entry.Facts.Sites = []string{site}
	return entry
}

func killedUndetermined() power.Entry {
	return entryOf(killedBy(power.OriginTargeted, testOwnB), power.ClassUndetermined)
}

func genStanding() *rapid.Generator[power.Standing] {
	kind := rapid.SampledFrom([]entryKind{
		kindValidKilledSpecific, kindEquivalentSurvivor, kindOffTargetSurvivor, kindUnattributedSurvivor,
		kindUndeterminedSurvivor, kindValidSurvivor, kindSentinel, kindKilledUndetermined,
	})
	site := rapid.SampledFrom([]string{siteFirst, siteSecond, siteThird})
	return rapid.Custom(func(t *rapid.T) power.Standing {
		count := rapid.IntRange(0, maxVotes+maxVotes).Draw(t, "entries")
		entries := make([]power.Entry, count)
		for index := range entries {
			entries[index] = entryFor(kind.Draw(t, "kind"), site.Draw(t, "site"))
		}
		return power.Standing{
			Subject: genSubject().Draw(t, "subject"), CorrectCodePasses: rapid.Bool().Draw(t, "reference"), ExampleFailed: rapid.Bool().Draw(t, "example"),
			Entries: entries, BudgetExhausted: rapid.Bool().Draw(t, "budget"), HostUnresponsive: rapid.Bool().Draw(t, "host"),
		}
	})
}

func TestTerminalProperties(t *testing.T) {
	worse := []entryKind{kindValidSurvivor, kindUndeterminedSurvivor}
	t.Run("adding a worse entry never improves the outcome"+verifiesSP167, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standing := genStanding().Draw(t, "standing")
			before := power.Terminal(standing).Result
			added := entryFor(rapid.SampledFrom(worse).Draw(t, "worse"), siteFirst)
			standing.Entries = append(append([]power.Entry(nil), standing.Entries...), added)
			after := power.Terminal(standing).Result
			if after.Severity() < before.Severity() {
				t.Fatalf("the outcome improved from %s to %s", before, after)
			}
		})
	})
	t.Run("adding a surviving VALID entry always blocks"+verifiesSP167, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standing := genStanding().Draw(t, "standing")
			standing.Entries = append(append([]power.Entry(nil), standing.Entries...), validSurviving())
			if got := power.Terminal(standing); got.Result != power.Block || !got.Has(power.FindingSurvivingValid) {
				t.Fatalf("outcome %+v", got)
			}
		})
	})
	t.Run("a pass needs a credited mutant and no finding, and the order of entries is irrelevant"+verifiesSP167, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standing := genStanding().Draw(t, "standing")
			got := power.Terminal(standing)
			if (got.Result == power.Pass) != (len(got.Findings) == 0) {
				t.Fatalf("result %s with findings %v", got.Result, got.Findings)
			}
			reversed := standing
			reversed.Entries = make([]power.Entry, len(standing.Entries))
			for index, entry := range standing.Entries {
				reversed.Entries[len(standing.Entries)-1-index] = entry
			}
			if other := power.Terminal(reversed); other.Result != got.Result || len(other.Findings) != len(got.Findings) {
				t.Fatalf("the outcome depends on the order: %+v against %+v", got, other)
			}
		})
	})
	t.Run("the search stops whenever the outcome blocks on an entry"+verifiesSP168, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			standing := genStanding().Draw(t, "standing")
			if power.Terminal(standing).Result == power.Block && !power.StopSearching(standing, policy.Default()) {
				t.Fatalf("the search continues past a block")
			}
		})
	})
}

func TestScreenProperties(t *testing.T) {
	t.Run("the violation probability lies in [0,1] and flags are monotone in the threshold"+verifiesSP169, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			answer := rapid.Custom(func(t *rapid.T) power.ClosedAnswer {
				return power.ClosedAnswer{
					Choice:        rapid.SampledFrom([]power.ClosedChoice{power.ChoiceViolates, power.ChoiceConsistent}).Draw(t, "choice"),
					Confidence:    rapid.Float64Range(0, 1).Draw(t, "confidence"),
					HasConfidence: rapid.Bool().Draw(t, "has"),
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
