package power_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

const judgeCount = 2

func TestDecideSentinel(t *testing.T) {
	cases := []struct {
		name  string
		facts power.Facts
		votes []power.Vote
	}{
		{"a surviving sentinel", survivor(power.OriginSentinel, power.NoDiffFound), nil},
		{"a sentinel that a test killed", killedBy(power.OriginSentinel, testOwnA), nil},
		{"a sentinel whatever the judges say", survivor(power.OriginSentinel, power.NoDiffFound), repeated(confirmedVote(fullConfidence, format.IsolationRestrictedAttested), judgeCount)},
		{"a sentinel that fails an example", func() power.Facts {
			facts := survivor(power.OriginSentinel, power.NoDiffFound)
			facts.FailedExamples = []spec.Ref{subjectRef}
			return facts
		}(), nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+" is equivalent by construction"+verifiesSP164, func(t *testing.T) {
			got := power.Decide(newSubject(), tc.facts, tc.votes, policy.Default())
			if got.Class != power.ClassEquivalent || got.Validity != format.ValidityEquivalent {
				t.Errorf("class %q validity %q", got.Class, got.Validity)
			}
			if got.Basis != format.BasisConstruction || got.Confidence != fullConfidence {
				t.Errorf("basis %q confidence %v", got.Basis, got.Confidence)
			}
			if !slices.Equal(got.Deciders, []string{power.DeciderSentinelCatalogue}) {
				t.Errorf("deciders %v", got.Deciders)
			}
			if len(got.Corroboration) != 0 || got.Carries() || got.OffTarget != nil || got.HostExclusion {
				t.Errorf("unexpected decision %+v", got)
			}
		})
	}
}

func TestDecideAttributedByExamples(t *testing.T) {
	cases := []struct {
		name         string
		failed       []spec.Ref
		class        power.Class
		validity     format.Validity
		offTarget    *spec.Ref
		evidenceKind format.CorroborationKind
		deciders     []string
	}{
		{"examples of the criterion alone", []spec.Ref{subjectRef}, power.ClassValid, format.ValidityValid, nil, format.CorroborationExampleFailed, []string{"example-SP-1~1"}},
		{"examples of several criteria with the criterion among them", []spec.Ref{subjectRef, otherRef}, power.ClassCollateral, format.ValidityValid, nil, format.CorroborationExampleFailed, []string{"example-SP-1~1", "example-SP-2~1"}},
		{"examples of several criteria with the criterion last", []spec.Ref{otherRef, thirdRef, subjectRef}, power.ClassCollateral, format.ValidityValid, nil, format.CorroborationExampleFailed, []string{"example-SP-1~1", "example-SP-2~1", "example-SP-3~2"}},
		{"examples of another criterion only", []spec.Ref{otherRef}, power.ClassOffTarget, format.ValidityNotViolating, &otherRef, format.CorroborationOtherExampleFailed, []string{"example-SP-2~1"}},
		{"examples of several other criteria", []spec.Ref{otherRef, thirdRef}, power.ClassOffTarget, format.ValidityNotViolating, &otherRef, format.CorroborationOtherExampleFailed, []string{"example-SP-2~1", "example-SP-3~2"}},
		{"another revision of the criterion counts as another criterion", []spec.Ref{{ID: subjectRef.ID, Revision: subjectRef.Revision + 1}}, power.ClassOffTarget, format.ValidityNotViolating, &spec.Ref{ID: subjectRef.ID, Revision: subjectRef.Revision + 1}, format.CorroborationOtherExampleFailed, []string{"example-SP-1~2"}},
	}
	for _, tc := range cases {
		t.Run("a mutant failing "+tc.name+verifiesSP164, func(t *testing.T) {
			facts := killedBy(power.OriginOperator, testOwnA)
			facts.FailedExamples = tc.failed
			got := power.Decide(newSubject(), facts, nil, policy.Default())
			if got.Class != tc.class || got.Validity != tc.validity {
				t.Fatalf("class %q validity %q, want %q %q", got.Class, got.Validity, tc.class, tc.validity)
			}
			if got.Basis != format.BasisSpecExample || got.Confidence != fullConfidence {
				t.Errorf("basis %q confidence %v", got.Basis, got.Confidence)
			}
			if !slices.Equal(got.Deciders, tc.deciders) {
				t.Errorf("deciders %v, want %v", got.Deciders, tc.deciders)
			}
			if len(got.Corroboration) != 1 || got.Corroboration[0].Kind != tc.evidenceKind || got.Corroboration[0].Digest == "" {
				t.Errorf("corroboration %+v, want one %s", got.Corroboration, tc.evidenceKind)
			}
			if (got.OffTarget == nil) != (tc.offTarget == nil) || (tc.offTarget != nil && *got.OffTarget != *tc.offTarget) {
				t.Errorf("off target %v, want %v", got.OffTarget, tc.offTarget)
			}
			wantCarries := tc.class == power.ClassValid || tc.class == power.ClassCollateral
			if got.Carries() != wantCarries {
				t.Errorf("Carries() = %v, want %v", got.Carries(), wantCarries)
			}
		})
	}
	t.Run("examples attribute a mutant without any judge vote"+verifiesSP164, func(t *testing.T) {
		facts := survivor(power.OriginTargeted, power.Skipped)
		facts.FailedExamples = []spec.Ref{subjectRef}
		got := power.Decide(newSubject(), facts, nil, policy.Default())
		if got.Class != power.ClassValid {
			t.Errorf("class %q", got.Class)
		}
	})
	t.Run("examples take precedence over the judges"+verifiesSP164, func(t *testing.T) {
		facts := survivor(power.OriginTargeted, power.NoDiffFound)
		facts.FailedExamples = []spec.Ref{otherRef}
		votes := repeated(vote(power.AnswerValid, fullConfidence, format.IsolationRestrictedAttested), judgeCount)
		got := power.Decide(newSubject(), facts, votes, policy.Default())
		if got.Class != power.ClassOffTarget {
			t.Errorf("class %q", got.Class)
		}
	})
	t.Run("the evidence digest depends on the failed examples and the mutant"+verifiesSP164, func(t *testing.T) {
		digestOf := func(id string, failed ...spec.Ref) string {
			facts := survivor(power.OriginTargeted, power.Skipped)
			facts.ID, facts.FailedExamples = id, failed
			return power.Decide(newSubject(), facts, nil, policy.Default()).Corroboration[0].Digest
		}
		base := digestOf(mutantID, subjectRef)
		if base != digestOf(mutantID, subjectRef) {
			t.Error("the digest is not stable")
		}
		if base == digestOf("mutant-2", subjectRef) || base == digestOf(mutantID, subjectRef, otherRef) {
			t.Error("the digest ignores its facts")
		}
	})
}

func TestDecideJudgedUndetermined(t *testing.T) {
	atFloor := format.IsolationRestricted
	cases := []struct {
		name   string
		votes  []power.Vote
		rules  policy.Policy
		reason power.Reason
	}{
		{"no vote", nil, policy.Default(), power.ReasonNoExecutor},
		{"fewer votes than judges", []power.Vote{confirmedVote(fullConfidence, atFloor)}, policy.Default(), power.ReasonNoExecutor},
		{"judges that disagree", []power.Vote{confirmedVote(fullConfidence, atFloor), vote(power.AnswerNotViolating, fullConfidence, atFloor)}, policy.Default(), power.ReasonJudgesSplit},
		{"one judge below the threshold", []power.Vote{confirmedVote(fullConfidence, atFloor), confirmedVote(0.69, atFloor)}, policy.Default(), power.ReasonBelowThreshold},
		{"judges that answer UNDETERMINED", repeated(vote(power.AnswerUndetermined, fullConfidence, atFloor), judgeCount), policy.Default(), power.ReasonJudgedUndetermined},
		{"judges that answer outside the vocabulary", repeated(vote("MAYBE", fullConfidence, atFloor), judgeCount), policy.Default(), power.ReasonJudgedUndetermined},
	}
	for _, tc := range cases {
		t.Run(tc.name+" leave the mutant undetermined"+verifiesSP165, func(t *testing.T) {
			got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.NoDiffFound), tc.votes, tc.rules)
			if got.Class != power.ClassUndetermined || got.Validity != format.ValidityUndetermined || got.Reason != tc.reason {
				t.Errorf("got %q %q %q, want undetermined for %q", got.Class, got.Validity, got.Reason, tc.reason)
			}
			if got.Carries() || len(got.Corroboration) != 0 {
				t.Errorf("an undetermined decision carries or corroborates: %+v", got)
			}
		})
	}
	t.Run("a confidence exactly at the threshold is enough"+verifiesSP165, func(t *testing.T) {
		votes := repeated(confirmedVote(policy.DefaultTauJudge, format.IsolationRestricted), judgeCount)
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, policy.Default())
		if got.Class != power.ClassValid {
			t.Errorf("class %q reason %q", got.Class, got.Reason)
		}
	})
	t.Run("the threshold of host votes applies to the judges, whatever the default is"+verifiesSP902, func(t *testing.T) {
		rules := policy.Default()
		rules.TauJudge = format.Threshold{Default: 0.5, BySource: map[format.ConfidenceSource]float64{format.SourceHostVotes: 0.95}}
		votes := repeated(confirmedVote(judgeConfidence, format.IsolationRestricted), judgeCount)
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, rules)
		if got.Class != power.ClassUndetermined || got.Reason != power.ReasonBelowThreshold {
			t.Fatalf("got %q %q", got.Class, got.Reason)
		}
		if got.Source != format.HostVotesSource(rules.JudgeK) || got.Confidence != judgeConfidence || got.Mapping != format.MappingWeakestStated {
			t.Errorf("the decision does not keep the claim that was held to the threshold: %+v", got)
		}
	})
	t.Run("the threshold of the exact source beats the one of its kind"+verifiesSP902, func(t *testing.T) {
		rules := policy.Default()
		rules.TauJudge = format.Threshold{Default: 0.95, BySource: map[format.ConfidenceSource]float64{
			format.SourceHostVotes: 0.95, format.HostVotesSource(rules.JudgeK): 0.5,
		}}
		votes := repeated(confirmedVote(judgeConfidence, format.IsolationRestricted), judgeCount)
		if got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, rules); got.Class != power.ClassValid {
			t.Fatalf("class %q reason %q", got.Class, got.Reason)
		}
	})
	t.Run("a threshold of another source leaves the judges on the default"+verifiesSP902, func(t *testing.T) {
		rules := policy.Default()
		rules.TauJudge = format.Threshold{Default: 0.5, BySource: map[format.ConfidenceSource]float64{format.SourceSystemOne: 1}}
		votes := repeated(confirmedVote(judgeConfidence, format.IsolationRestricted), judgeCount)
		if got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, rules); got.Class != power.ClassValid {
			t.Fatalf("class %q reason %q", got.Class, got.Reason)
		}
	})
	t.Run("the decision confidence is the lowest of the judges"+verifiesSP165, func(t *testing.T) {
		votes := []power.Vote{confirmedVote(fullConfidence, format.IsolationRestricted), confirmedVote(judgeConfidence, format.IsolationRestricted)}
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, policy.Default())
		if got.Confidence != judgeConfidence {
			t.Errorf("confidence %v, want %v", got.Confidence, judgeConfidence)
		}
	})
	t.Run("votes beyond the judge count all take part"+verifiesSP165, func(t *testing.T) {
		votes := []power.Vote{confirmedVote(fullConfidence, format.IsolationRestricted), confirmedVote(fullConfidence, format.IsolationRestricted), vote(power.AnswerNotViolating, fullConfidence, format.IsolationRestricted)}
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), votes, policy.Default())
		if got.Reason != power.ReasonJudgesSplit {
			t.Errorf("reason %q", got.Reason)
		}
	})
}

func TestDecideJudgedValid(t *testing.T) {
	restricted, attested, weak := format.IsolationRestricted, format.IsolationRestrictedAttested, format.IsolationUnrestricted
	killedSpecific := killedBy(power.OriginTargeted, testOwnA)
	otherFails := survivor(power.OriginTargeted, power.Skipped)
	otherFails.Outcomes = outcomesWith(testOtherA)
	killedAlsoOther := killedBy(power.OriginTargeted, testOwnA, testOtherB)
	unconfirmed := repeated(vote(power.AnswerValid, fullConfidence, attested), judgeCount)
	refused := repeated(vote(power.AnswerValid, fullConfidence, attested), judgeCount)
	refused[0].Witness = power.WitnessRefused
	marked := repeated(confirmedVote(fullConfidence, attested), judgeCount)
	marked[1].Marked = true
	oneConfirmed := []power.Vote{vote(power.AnswerValid, fullConfidence, restricted), confirmedVote(fullConfidence, restricted)}

	cases := []struct {
		name     string
		subject  power.Subject
		facts    power.Facts
		votes    []power.Vote
		rules    policy.Policy
		class    power.Class
		reason   power.Reason
		evidence []format.CorroborationKind
	}{
		{"a survivor with a confirmed witness, a build and passing other criteria", newSubject(), survivor(power.OriginTargeted, power.Skipped), repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}},
		{"a survivor with one confirmed witness among the judges", newSubject(), survivor(power.OriginTargeted, power.Skipped), oneConfirmed, policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}},
		{"a survivor without a witness", newSubject(), survivor(power.OriginTargeted, power.Skipped), unconfirmed, policy.Default(), power.ClassUndetermined, power.ReasonUncorroborated, nil},
		{"a survivor whose witness execution refused", newSubject(), survivor(power.OriginTargeted, power.Skipped), refused, policy.Default(), power.ClassUndetermined, power.ReasonUncorroborated, nil},
		{"a survivor that breaks the tests of another criterion", newSubject(), otherFails, repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassUndetermined, power.ReasonUncorroborated, nil},
		{"a survivor judged below the isolation floor keeps its power to block", newSubject(), survivor(power.OriginTargeted, power.Skipped), repeated(confirmedVote(fullConfidence, weak), judgeCount), policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}},
		{"a survivor judged by a marked response keeps its power to block", newSubject(), survivor(power.OriginTargeted, power.Skipped), marked, policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}},
		{"a survivor under the executed-only basis keeps its power to block", newSubject(), survivor(power.OriginTargeted, power.Skipped), repeated(confirmedVote(fullConfidence, attested), judgeCount), withBasis(policy.ExecutedOnly), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass}},
		{"a killed mutant with a specific kill at the floor", newSubject(), killedSpecific, repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass, format.CorroborationSpecificKill}},
		{"a killed mutant whose only killer also killed a sentinel", newSubject(testOwnA), killedSpecific, repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassUndetermined, power.ReasonUncorroborated, nil},
		{"a killed mutant with one killer clear of the sentinels among others", newSubject(testOwnA), killedBy(power.OriginTargeted, testOwnA, testOwnB), repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass, format.CorroborationSpecificKill}},
		{"a killed mutant judged below the floor", newSubject(), killedSpecific, repeated(confirmedVote(fullConfidence, weak), judgeCount), policy.Default(), power.ClassUndetermined, power.ReasonIsolationFloor, nil},
		{"a killed mutant with one judge below the floor", newSubject(), killedSpecific, []power.Vote{confirmedVote(fullConfidence, attested), confirmedVote(fullConfidence, weak)}, policy.Default(), power.ClassUndetermined, power.ReasonIsolationFloor, nil},
		{"a killed mutant with a marked response", newSubject(), killedSpecific, marked, policy.Default(), power.ClassUndetermined, power.ReasonIsolationFloor, nil},
		{"a killed mutant under the executed-only basis", newSubject(), killedSpecific, repeated(confirmedVote(fullConfidence, attested), judgeCount), withBasis(policy.ExecutedOnly), power.ClassUndetermined, power.ReasonIsolationFloor, nil},
		{"a killed mutant under the judged basis", newSubject(), killedSpecific, repeated(confirmedVote(fullConfidence, restricted), judgeCount), withBasis(policy.Judged), power.ClassValid, power.ReasonNone,
			[]format.CorroborationKind{format.CorroborationBuilds, format.CorroborationWitness, format.CorroborationOtherCriteriaPass, format.CorroborationSpecificKill}},
		{"a killed mutant that also fails another criterion's tests", newSubject(), killedAlsoOther, repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default(), power.ClassUndetermined, power.ReasonUncorroborated, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP165, func(t *testing.T) {
			got := power.Decide(tc.subject, tc.facts, tc.votes, tc.rules)
			if got.Class != tc.class || got.Reason != tc.reason {
				t.Fatalf("got %q (%q), want %q (%q)", got.Class, got.Reason, tc.class, tc.reason)
			}
			if tc.class != power.ClassValid {
				if got.Carries() {
					t.Error("a decision that is not VALID carries power")
				}
				return
			}
			if got.Validity != format.ValidityValid || got.Basis != format.BasisJudges || !slices.Equal(got.Deciders, []string{power.DeciderHostJudge}) {
				t.Errorf("unexpected decision %+v", got)
			}
			gotKinds := kinds(got.Corroboration)
			if len(gotKinds) != len(tc.evidence) {
				t.Fatalf("corroboration %v, want kinds %v", gotKinds, tc.evidence)
			}
			for _, want := range tc.evidence {
				if !hasKind(got.Corroboration, want) {
					t.Errorf("corroboration %v lacks %s", gotKinds, want)
				}
			}
			if !slices.IsSortedFunc(got.Corroboration, func(a, b power.Evidence) int {
				if a.Kind != b.Kind {
					return compareText(string(a.Kind), string(b.Kind))
				}
				return compareText(a.Digest, b.Digest)
			}) {
				t.Errorf("corroboration is not sorted: %v", got.Corroboration)
			}
		})
	}
	t.Run("the witness evidence carries the digest of the confirmed witness"+verifiesSP165, func(t *testing.T) {
		got := power.Decide(newSubject(), survivor(power.OriginTargeted, power.Skipped), repeated(confirmedVote(fullConfidence, restricted), judgeCount), policy.Default())
		for _, item := range got.Corroboration {
			if item.Kind == format.CorroborationWitness && item.Digest != witnessDigest {
				t.Errorf("witness digest %q", item.Digest)
			}
		}
	})
	t.Run("a stricter isolation floor withdraws the credit of a restricted response"+verifiesSP165, func(t *testing.T) {
		rules := policy.Default()
		rules.MinIsolation = format.IsolationRestrictedAttested
		got := power.Decide(newSubject(), killedSpecific, repeated(confirmedVote(fullConfidence, restricted), judgeCount), rules)
		if got.Reason != power.ReasonIsolationFloor {
			t.Errorf("reason %q", got.Reason)
		}
	})
}

func compareText(a, b string) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

func TestDecideJudgedEquivalent(t *testing.T) {
	restricted, weak := format.IsolationRestricted, format.IsolationUnrestricted
	equivalentVotes := func(isolation format.Isolation) []power.Vote {
		return repeated(vote(power.AnswerEquivalent, fullConfidence, isolation), judgeCount)
	}
	cases := []struct {
		name   string
		facts  power.Facts
		votes  []power.Vote
		rules  policy.Policy
		class  power.Class
		reason power.Reason
	}{
		{"no difference found under the corroborated basis", survivor(power.OriginTargeted, power.NoDiffFound), equivalentVotes(restricted), policy.Default(), power.ClassEquivalent, power.ReasonNone},
		{"no difference found under the judged basis", survivor(power.OriginTargeted, power.NoDiffFound), equivalentVotes(restricted), withBasis(policy.Judged), power.ClassEquivalent, power.ReasonNone},
		{"no difference found under the executed-only basis", survivor(power.OriginTargeted, power.NoDiffFound), equivalentVotes(restricted), withBasis(policy.ExecutedOnly), power.ClassUndetermined, power.ReasonNoExecutedEquivalence},
		{"a search that was skipped", survivor(power.OriginTargeted, power.Skipped), equivalentVotes(restricted), policy.Default(), power.ClassUndetermined, power.ReasonNoExecutedEquivalence},
		{"a search that found a difference", survivor(power.OriginTargeted, power.DiffersAt), equivalentVotes(restricted), policy.Default(), power.ClassUndetermined, power.ReasonJudgedUndetermined},
		{"a mutant that a test killed", killedBy(power.OriginTargeted, testOtherA), equivalentVotes(restricted), policy.Default(), power.ClassUndetermined, power.ReasonJudgedUndetermined},
		{"a mutant killed with no difference found", func() power.Facts {
			facts := killedBy(power.OriginTargeted, testOwnA)
			facts.Differential.Status = power.NoDiffFound
			return facts
		}(), equivalentVotes(restricted), policy.Default(), power.ClassUndetermined, power.ReasonJudgedUndetermined},
		{"judges below the isolation floor", survivor(power.OriginTargeted, power.NoDiffFound), equivalentVotes(weak), policy.Default(), power.ClassUndetermined, power.ReasonIsolationFloor},
	}
	for _, tc := range cases {
		t.Run("judges ruling EQUIVALENT on "+tc.name+verifiesSP166, func(t *testing.T) {
			got := power.Decide(newSubject(), tc.facts, tc.votes, tc.rules)
			if got.Class != tc.class || got.Reason != tc.reason {
				t.Fatalf("got %q (%q), want %q (%q)", got.Class, got.Reason, tc.class, tc.reason)
			}
			if tc.class != power.ClassEquivalent {
				return
			}
			if got.Validity != format.ValidityEquivalent || got.Basis != format.BasisJudges || got.Carries() || got.HostExclusion {
				t.Errorf("unexpected decision %+v", got)
			}
			if len(got.Corroboration) != 1 || got.Corroboration[0].Kind != format.CorroborationNoDiffFound {
				t.Errorf("corroboration %+v", got.Corroboration)
			}
		})
	}
}

func TestDecideJudgedNotViolating(t *testing.T) {
	restricted, weak := format.IsolationRestricted, format.IsolationUnrestricted
	notViolating := func(isolation format.Isolation) []power.Vote {
		return repeated(vote(power.AnswerNotViolating, fullConfidence, isolation), judgeCount)
	}
	cases := []struct {
		name   string
		facts  power.Facts
		votes  []power.Vote
		rules  policy.Policy
		class  power.Class
		reason power.Reason
	}{
		{"the judged basis at the floor", survivor(power.OriginTargeted, power.Skipped), notViolating(restricted), withBasis(policy.Judged), power.ClassUnattributed, power.ReasonNone},
		{"the corroborated basis", survivor(power.OriginTargeted, power.Skipped), notViolating(restricted), policy.Default(), power.ClassUndetermined, power.ReasonNoExecutedExclusion},
		{"the executed-only basis", survivor(power.OriginTargeted, power.Skipped), notViolating(restricted), withBasis(policy.ExecutedOnly), power.ClassUndetermined, power.ReasonNoExecutedExclusion},
		{"the judged basis below the floor", survivor(power.OriginTargeted, power.Skipped), notViolating(weak), withBasis(policy.Judged), power.ClassUndetermined, power.ReasonIsolationFloor},
		{"a mutant that a test of the criterion killed", killedBy(power.OriginTargeted, testOwnB), notViolating(restricted), withBasis(policy.Judged), power.ClassUndetermined, power.ReasonJudgedUndetermined},
		{"a mutant that only another criterion's test killed", killedBy(power.OriginTargeted, testOtherA), notViolating(restricted), withBasis(policy.Judged), power.ClassUnattributed, power.ReasonNone},
	}
	for _, tc := range cases {
		t.Run("judges ruling NOT_VIOLATING under "+tc.name+verifiesSP166, func(t *testing.T) {
			got := power.Decide(newSubject(), tc.facts, tc.votes, tc.rules)
			if got.Class != tc.class || got.Reason != tc.reason {
				t.Fatalf("got %q (%q), want %q (%q)", got.Class, got.Reason, tc.class, tc.reason)
			}
			wantExclusion := tc.class == power.ClassUnattributed
			if got.HostExclusion != wantExclusion {
				t.Errorf("host exclusion %v, want %v", got.HostExclusion, wantExclusion)
			}
			if wantExclusion && (got.Validity != format.ValidityNotViolating || got.Basis != format.BasisJudges || got.Carries() || got.OffTarget != nil) {
				t.Errorf("unexpected decision %+v", got)
			}
		})
	}
}

func TestContradicts(t *testing.T) {
	restricted := format.IsolationRestricted
	equivalent := vote(power.AnswerEquivalent, fullConfidence, restricted)
	refusedEquivalent := equivalent
	refusedEquivalent.Witness = power.WitnessRefused
	refusedValid := confirmedVote(fullConfidence, restricted)
	refusedValid.Witness = power.WitnessRefused
	notViolating := vote(power.AnswerNotViolating, fullConfidence, restricted)
	differs := survivor(power.OriginTargeted, power.DiffersAt)
	cases := []struct {
		name  string
		facts power.Facts
		vote  power.Vote
		want  power.Contradiction
	}{
		{"EQUIVALENT for a killed mutant", killedBy(power.OriginTargeted, testOtherB), equivalent, power.ContradictionEquivalentKilled},
		{"EQUIVALENT for a mutant whose behaviour differs", differs, equivalent, power.ContradictionEquivalentDiffers},
		{"EQUIVALENT for a killed mutant whose behaviour also differs", func() power.Facts {
			facts := killedBy(power.OriginTargeted, testOwnA)
			facts.Differential.Status = power.DiffersAt
			return facts
		}(), equivalent, power.ContradictionEquivalentKilled},
		{"EQUIVALENT for a surviving mutant with no difference", survivor(power.OriginTargeted, power.NoDiffFound), equivalent, power.ContradictionNone},
		{"EQUIVALENT for a mutant whose search was skipped", survivor(power.OriginTargeted, power.Skipped), equivalent, power.ContradictionNone},
		{"a witness that execution refused", survivor(power.OriginTargeted, power.Skipped), refusedValid, power.ContradictionWitnessRefused},
		{"a refused witness before any other ground", killedBy(power.OriginTargeted, testOwnA), refusedEquivalent, power.ContradictionWitnessRefused},
		{"a confirmed witness", survivor(power.OriginTargeted, power.Skipped), confirmedVote(fullConfidence, restricted), power.ContradictionNone},
		{"VALID for a killed mutant", killedBy(power.OriginTargeted, testOwnA), vote(power.AnswerValid, fullConfidence, restricted), power.ContradictionNone},
		{"NOT_VIOLATING for a mutant whose behaviour differs", differs, notViolating, power.ContradictionNone},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP166, func(t *testing.T) {
			if got := power.Contradicts(tc.facts, tc.vote); got != tc.want {
				t.Errorf("Contradicts() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDecisionCarries(t *testing.T) {
	for _, class := range []power.Class{power.ClassValid, power.ClassCollateral, power.ClassOffTarget, power.ClassUnattributed, power.ClassEquivalent, power.ClassUndetermined} {
		want := class == power.ClassValid || class == power.ClassCollateral
		t.Run("the class "+string(class)+verifiesSP167, func(t *testing.T) {
			if got := (power.Decision{Class: class}).Carries(); got != want {
				t.Errorf("Carries() = %v, want %v", got, want)
			}
		})
	}
	t.Run("an undetermined decision records its reason and no evidence"+verifiesSP165, func(t *testing.T) {
		got := power.Undetermined(power.ReasonBudgetExhausted)
		if got.Class != power.ClassUndetermined || got.Validity != format.ValidityUndetermined || got.Reason != power.ReasonBudgetExhausted || got.Basis != format.BasisJudges {
			t.Errorf("unexpected decision %+v", got)
		}
		if got.Confidence != format.MinConfidence || len(got.Corroboration) != 0 {
			t.Errorf("unexpected confidence or evidence %+v", got)
		}
	})
}
