package power_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
)

func TestDecideByExecution(t *testing.T) {
	restricted := format.IsolationRestricted
	identical := survivor(power.OriginTargeted, power.DiffersAt)
	identical.Identity = power.IdentityIdentical
	owner := survivor(power.OriginOperator, power.Skipped)
	owner.FailedExamples = []spec.Ref{subjectRef}
	shared := survivor(power.OriginOperator, power.Skipped)
	shared.FailedExamples = []spec.Ref{subjectRef, otherRef}
	otherExample := survivor(power.OriginTargeted, power.DiffersAt)
	otherExample.FailedExamples = []spec.Ref{otherRef}
	cases := []struct {
		name      string
		facts     power.Facts
		votes     []power.Vote
		class     power.Class
		route     power.Route
		validity  format.Validity
		evidence  []format.CorroborationKind
		offTarget *spec.Ref
	}{
		{"a sentinel is equivalent by construction", survivor(power.OriginSentinel, power.NoDiffFound), repeated(confirmedVote(fullConfidence, restricted), judgeCount), power.ClassSentinel, power.RouteNone, format.ValidityEquivalent, nil, nil},
		{"a mutant that compiles to the reference's form is excluded", failing(identical, ownWitness), nil, power.ClassExcluded, power.RouteIdentity, format.ValidityEquivalent, []format.CorroborationKind{format.CorroborationNoDiffFound}, nil},
		{"a mutant that fails an example of the criterion alone is owner-attributed", owner, nil, power.ClassOwnerAttributed, power.RouteNone, format.ValidityValid, []format.CorroborationKind{format.CorroborationExampleFailed}, nil},
		{"a mutant that fails examples of two criteria is not owner-attributed", shared, nil, power.ClassUnsettled, power.RouteNone, format.ValidityUndetermined, nil, nil},
		{"a mutant that fails another criterion's example only is excluded off target", otherExample, nil, power.ClassExcluded, power.RouteOtherCriterion, format.ValidityNotViolating, []format.CorroborationKind{format.CorroborationOtherExampleFailed}, &otherRef},
		{"a mutant that fails another criterion's admitted witness only is excluded off target", failing(survivor(power.OriginTargeted, power.DiffersAt), otherEscalateOnly), nil, power.ClassExcluded, power.RouteOtherCriterion, format.ValidityNotViolating, []format.CorroborationKind{format.CorroborationOtherExampleFailed}, &otherRef},
		{"a mutant that also fails an escalate-only witness of its own is not excluded", failing(survivor(power.OriginTargeted, power.DiffersAt), otherWitness, ownEscalateOnly), nil, power.ClassUnsettled, power.RouteNone, format.ValidityUndetermined, nil, nil},
		{"a witness the reference fails excludes nothing", failing(survivor(power.OriginTargeted, power.DiffersAt), ownConflict), nil, power.ClassUnsettled, power.RouteNone, format.ValidityUndetermined, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP164, func(t *testing.T) {
			got := decide(tc.facts, tc.votes, policy.Default())
			if got.Class != tc.class || got.Route != tc.route || got.Validity != tc.validity {
				t.Fatalf("class %q route %q validity %q", got.Class, got.Route, got.Validity)
			}
			if !slices.Equal(kinds(got.Corroboration), tc.evidence) {
				t.Errorf("evidence %v, want %v", kinds(got.Corroboration), tc.evidence)
			}
			if (got.OffTarget == nil) != (tc.offTarget == nil) || (got.OffTarget != nil && *got.OffTarget != *tc.offTarget) {
				t.Errorf("off target %v, want %v", got.OffTarget, tc.offTarget)
			}
			if got.Class != power.ClassOwnerAttributed && got.CreditBearing {
				t.Errorf("a %s mutant bears credit", got.Class)
			}
		})
	}

	t.Run("an owner-attributed mutant bears credit and rests on execution"+verifiesSP164, func(t *testing.T) {
		got := decide(owner, nil, policy.Default())
		if !got.CreditBearing || got.Basis != format.BasisSpecExample || got.Confidence != fullConfidence || got.Source != format.SourceExecution {
			t.Errorf("decision %+v", got)
		}
	})
}

func TestDecideValidE(t *testing.T) {
	t.Run("a mutant that fails a block-bearing witness of the criterion is VALID_e on executed evidence"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), nil, policy.Default())
		if got.Class != power.ClassValidE || got.Validity != format.ValidityValid || got.Basis != format.BasisExecution {
			t.Fatalf("decision %+v", got)
		}
		want := []format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}
		if !slices.Equal(kinds(got.Corroboration), want) {
			t.Errorf("evidence %v, want %v", kinds(got.Corroboration), want)
		}
		if !got.CreditBearing || got.Witness != ownWitness || !slices.Equal(got.Failing, []string{ownWitness}) {
			t.Errorf("credit %t witness %q failing %v", got.CreditBearing, got.Witness, got.Failing)
		}
	})

	t.Run("a killed VALID_e mutant carries a specific kill when it bears credit"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(killedBy(power.OriginTargeted, testOwnA), ownWitness), nil, policy.Default())
		if !hasKind(got.Corroboration, format.CorroborationSpecificKill) {
			t.Errorf("evidence %v", kinds(got.Corroboration))
		}
	})

	t.Run("a witness written below the floor blocks and earns no credit"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(killedBy(power.OriginTargeted, testOwnA), ownWitnessLow), nil, policy.Default())
		if got.Class != power.ClassValidE || got.CreditBearing || hasKind(got.Corroboration, format.CorroborationSpecificKill) {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("the credit-eligible witness is the one the claim rests on"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness, ownWitnessLow), nil, policy.Default())
		if got.Witness != ownWitness || !got.CreditBearing {
			t.Errorf("witness %q credit %t", got.Witness, got.CreditBearing)
		}
	})

	t.Run("an escalate-only witness makes no VALID_e"+verifiesSP165, func(t *testing.T) {
		if got := decide(failing(survivor(power.OriginTargeted, power.DiffersAt), ownEscalateOnly), nil, policy.Default()); got.Carries() {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("a mutant killed by another criterion's test is not isolated and stays unsettled"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(killedBy(power.OriginTargeted, testOtherA), ownWitness), nil, policy.Default())
		if got.Class != power.ClassUnsettled || got.Reason != power.ReasonWitnessedNotIsolated {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("an error on another criterion's test breaks isolation too"+verifiesSP165, func(t *testing.T) {
		facts := failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness)
		facts.Outcomes[testOtherB] = format.OutcomeError
		if got := decide(facts, nil, policy.Default()); got.Carries() {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("a mutant VALID_e for two criteria is collateral and bears no credit"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness, otherWitness), nil, policy.Default())
		if got.Class != power.ClassCollateral || got.CreditBearing || !slices.Equal(got.CollateralWith, []spec.Ref{otherRef}) {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("the executed-only basis gives VALID_e no credit"+verifiesSP165, func(t *testing.T) {
		got := decide(failing(killedBy(power.OriginTargeted, testOwnA), ownWitness), nil, withBasis(policy.ExecutedOnly))
		if got.Class != power.ClassValidE || got.CreditBearing {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("a test bound to the criterion and another one breaks isolation"+verifiesSP165, func(t *testing.T) {
		shared := bindings()
		shared[testOwnA] = []spec.Ref{subjectRef, otherRef}
		subject := power.NewSubject(subjectRef, shared, map[ecosystem.TestID]bool{})
		facts := failing(killedBy(power.OriginTargeted, testOwnA), ownWitness)
		got := power.Decide(subject, universe(policy.Default()), facts, nil, rulesOf(policy.Default()))
		if got.Carries() {
			t.Errorf("decision %+v", got)
		}
	})
}

func TestDecideVotes(t *testing.T) {
	restricted := format.IsolationRestricted
	equivalent := repeated(vote(power.AnswerEquivalent, fullConfidence, restricted), judgeCount)
	noDiff := survivor(power.OriginTargeted, power.NoDiffFound)
	cases := []struct {
		name   string
		facts  power.Facts
		votes  []power.Vote
		rules  policy.Policy
		class  power.Class
		reason power.Reason
	}{
		{"every vote EQUIVALENT on a mutant the search found no difference for excludes it", noDiff, equivalent, policy.Default(), power.ClassExcluded, power.ReasonNone},
		{"EQUIVALENT on a mutant the search found a difference for does not", survivor(power.OriginTargeted, power.DiffersAt), equivalent, policy.Default(), power.ClassUnsettled, power.ReasonNoExecutedEquivalence},
		{"EQUIVALENT where the search was skipped does not", survivor(power.OriginTargeted, power.Skipped), equivalent, policy.Default(), power.ClassUnsettled, power.ReasonNoExecutedEquivalence},
		{"EQUIVALENT under the executed-only basis does not", noDiff, equivalent, withBasis(policy.ExecutedOnly), power.ClassUnsettled, power.ReasonNoExecutedEquivalence},
		{"EQUIVALENT on a mutant that fails an escalate-only witness of the criterion does not", failing(noDiff, ownEscalateOnly), equivalent, policy.Default(), power.ClassUnsettled, power.ReasonNoExecutedEquivalence},
		{"a witness failure the reference shares is no difference from it", failing(noDiff, ownConflict), equivalent, policy.Default(), power.ClassExcluded, power.ReasonNone},
		{"EQUIVALENT below the threshold does not", noDiff, repeated(vote(power.AnswerEquivalent, lowConfidence, restricted), judgeCount), policy.Default(), power.ClassUnsettled, power.ReasonBelowThreshold},
		{"EQUIVALENT below the floor does not", noDiff, repeated(vote(power.AnswerEquivalent, fullConfidence, format.IsolationInline), judgeCount), policy.Default(), power.ClassUnsettled, power.ReasonIsolationFloor},
		{"one vote short of judge_k does not", noDiff, equivalent[:1], policy.Default(), power.ClassUnsettled, power.ReasonNoExecutor},
		{"split votes do not", noDiff, []power.Vote{equivalent[0], vote(power.AnswerUndetermined, fullConfidence, restricted)}, policy.Default(), power.ClassUnsettled, power.ReasonJudgesSplit},
		{"VALID with a confirmed witness never settles a survivor", survivor(power.OriginTargeted, power.DiffersAt), repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassUnsettled, power.ReasonJudgedValid},
		{"NOT_VIOLATING never clears a survivor, whatever the basis", survivor(power.OriginTargeted, power.DiffersAt), repeated(vote(power.AnswerNotViolating, fullConfidence, restricted), judgeCount), withBasis(policy.Judged), power.ClassUnsettled, power.ReasonJudgedNotViolating},
		{"UNDETERMINED leaves a survivor unsettled", survivor(power.OriginTargeted, power.DiffersAt), repeated(vote(power.AnswerUndetermined, fullConfidence, restricted), judgeCount), policy.Default(), power.ClassUnsettled, power.ReasonJudgedUndetermined},
		{"no vote leaves a survivor unsettled and unadjudicated", survivor(power.OriginTargeted, power.DiffersAt), nil, policy.Default(), power.ClassUnsettled, power.ReasonNotAdjudicated},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP166, func(t *testing.T) {
			got := decide(tc.facts, tc.votes, tc.rules)
			if got.Class != tc.class || got.Reason != tc.reason {
				t.Fatalf("class %q reason %q, want %q %q", got.Class, got.Reason, tc.class, tc.reason)
			}
			if got.Class == power.ClassExcluded && (got.Route != power.RouteEquivalent || !hasKind(got.Corroboration, format.CorroborationNoDiffFound)) {
				t.Errorf("exclusion %+v", got)
			}
			if got.Class == power.ClassUnsettled && (got.Validity != format.ValidityUndetermined || got.Carries()) {
				t.Errorf("unsettled %+v", got)
			}
		})
	}

	t.Run("a vote that confirms a witness keeps a mutant with no difference unsettled"+verifiesSP166, func(t *testing.T) {
		votes := []power.Vote{equivalent[0], equivalent[1]}
		votes[1].Witness = power.WitnessConfirmed
		if got := decide(noDiff, votes, policy.Default()); got.Class != power.ClassUnsettled {
			t.Errorf("decision %+v", got)
		}
	})

	t.Run("a mutant whose votes could not be asked keeps the reason why"+verifiesSP166, func(t *testing.T) {
		entry := power.Entry{Facts: survivor(power.OriginTargeted, power.DiffersAt), Unjudged: power.ReasonBudgetExhausted}
		got := power.DecideEntry(newSubject(), universe(policy.Default()), entry, rulesOf(policy.Default()))
		if got.Reason != power.ReasonBudgetExhausted {
			t.Errorf("reason %q", got.Reason)
		}
	})
}

func TestContradicts(t *testing.T) {
	restricted := format.IsolationRestricted
	cases := []struct {
		name  string
		facts power.Facts
		vote  power.Vote
		want  power.Contradiction
	}{
		{"a refused witness", survivor(power.OriginTargeted, power.DiffersAt), power.Vote{Answer: power.AnswerValid, Witness: power.WitnessRefused}, power.ContradictionWitnessRefused},
		{"EQUIVALENT on a killed mutant", killedBy(power.OriginTargeted, testOtherA), vote(power.AnswerEquivalent, fullConfidence, restricted), power.ContradictionEquivalentKilled},
		{"EQUIVALENT on a mutant that differs", survivor(power.OriginTargeted, power.DiffersAt), vote(power.AnswerEquivalent, fullConfidence, restricted), power.ContradictionEquivalentDiffers},
		{"EQUIVALENT on a mutant with no difference", survivor(power.OriginTargeted, power.NoDiffFound), vote(power.AnswerEquivalent, fullConfidence, restricted), power.ContradictionNone},
		{"VALID on a killed mutant", killedBy(power.OriginTargeted, testOwnA), vote(power.AnswerValid, fullConfidence, restricted), power.ContradictionNone},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP166, func(t *testing.T) {
			if got := power.Contradicts(tc.facts, tc.vote); got != tc.want {
				t.Errorf("contradiction %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecisionConfidenceSource(t *testing.T) {
	t.Run("an executed decision rests on execution and names no mapping"+verifiesSP406, func(t *testing.T) {
		owner := survivor(power.OriginOperator, power.Skipped)
		owner.FailedExamples = []spec.Ref{subjectRef}
		for name, decision := range map[string]power.Decision{
			"sentinel": decide(survivor(power.OriginSentinel, power.NoDiffFound), nil, policy.Default()),
			"example":  decide(owner, nil, policy.Default()),
			"witness":  decide(failing(survivor(power.OriginTargeted, power.DiffersAt), ownWitness), nil, policy.Default()),
			"unjudged": decide(survivor(power.OriginTargeted, power.DiffersAt), nil, policy.Default()),
		} {
			if decision.Source != format.SourceExecution || decision.Mapping != "" {
				t.Errorf("%s: source %q, mapping %q", name, decision.Source, decision.Mapping)
			}
		}
	})

	t.Run("a judged decision rests on the host votes and maps the weakest stated confidence"+verifiesSP406, func(t *testing.T) {
		votes := repeated(vote(power.AnswerEquivalent, judgeConfidence, format.IsolationRestricted), judgeCount)
		got := decide(survivor(power.OriginTargeted, power.NoDiffFound), votes, policy.Default())
		if got.Source != format.HostVotesSource(judgeCount) || got.Mapping != format.MappingWeakestStated || got.Confidence != judgeConfidence {
			t.Errorf("source %q, mapping %q, confidence %v", got.Source, got.Mapping, got.Confidence)
		}
	})
}

func TestScopeRuleThreshold(t *testing.T) {
	t.Run("a FIXED vote below tau_judge makes the witness escalate-only"+verifiesSP902, func(t *testing.T) {
		low := record(ownWitness, subjectRef, true, format.IsolationRestricted)
		for index := range low.Scope {
			low.Scope[index].Confidence = lowConfidence
		}
		rules := rulesOf(policy.Default())
		if got := low.Status(rules.Admission, rules.ScopeRule()); got != witness.StatusEscalateOnly {
			t.Errorf("status %q", got)
		}
	})
}
