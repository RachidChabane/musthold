package power_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/format"
)

func TestTallyVotes(t *testing.T) {
	restricted := format.IsolationRestricted
	cases := []struct {
		name       string
		votes      []power.Vote
		answer     power.Answer
		unanimous  bool
		confidence float64
	}{
		{"no votes", nil, power.AnswerUndetermined, false, 0},
		{"one vote", []power.Vote{vote(power.AnswerValid, judgeConfidence, restricted)}, power.AnswerValid, true, judgeConfidence},
		{"agreeing votes take the lowest confidence", []power.Vote{vote(power.AnswerEquivalent, fullConfidence, restricted), vote(power.AnswerEquivalent, 0.75, restricted), vote(power.AnswerEquivalent, 0.8, restricted)}, power.AnswerEquivalent, true, 0.75},
		{"split votes", []power.Vote{vote(power.AnswerValid, fullConfidence, restricted), vote(power.AnswerNotViolating, 0.6, restricted)}, power.AnswerValid, false, 0.6},
		{"a dissenter last", []power.Vote{vote(power.AnswerValid, fullConfidence, restricted), vote(power.AnswerValid, fullConfidence, restricted), vote(power.AnswerUndetermined, fullConfidence, restricted)}, power.AnswerValid, false, fullConfidence},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP165, func(t *testing.T) {
			got := power.TallyVotes(tc.votes)
			if got.Answer != tc.answer || got.Unanimous != tc.unanimous || got.MinConfidence != tc.confidence {
				t.Errorf("TallyVotes() = %+v", got)
			}
		})
	}
}

func TestVoteAtFloor(t *testing.T) {
	cases := []struct {
		name      string
		isolation format.Isolation
		marked    bool
		floor     format.Isolation
		want      bool
	}{
		{"the floor itself", format.IsolationRestricted, false, format.IsolationRestricted, true},
		{"a stronger response", format.IsolationRestrictedAttested, false, format.IsolationRestricted, true},
		{"a weaker response", format.IsolationRestrictedUnverified, false, format.IsolationRestricted, false},
		{"an unrestricted response", format.IsolationUnrestricted, false, format.IsolationRestricted, false},
		{"an inline response", format.IsolationInline, false, format.IsolationRestricted, false},
		{"a marked response at the floor", format.IsolationRestricted, true, format.IsolationRestricted, false},
		{"a marked response above the floor", format.IsolationRestrictedAttested, true, format.IsolationRestricted, false},
		{"an unknown isolation", "sandboxed", false, format.IsolationInline, false},
		{"any isolation against the weakest floor", format.IsolationInline, false, format.IsolationInline, true},
		{"a restricted response against the attested floor", format.IsolationRestricted, false, format.IsolationRestrictedAttested, false},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP165, func(t *testing.T) {
			judged := power.Vote{Isolation: tc.isolation, Marked: tc.marked}
			if got := judged.AtFloor(tc.floor); got != tc.want {
				t.Errorf("AtFloor() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFactsQueries(t *testing.T) {
	t.Run("KilledBy keeps the order of the candidates"+verifiesSP165, func(t *testing.T) {
		facts := killedBy(power.OriginTargeted, testOwnA, testOtherA)
		got := facts.KilledBy([]ecosystem.TestID{testOtherA, testOwnB, testOwnA})
		if !slices.Equal(got, []ecosystem.TestID{testOtherA, testOwnA}) {
			t.Errorf("KilledBy() = %v", got)
		}
	})
	t.Run("KilledBy ignores tests the facts do not know"+verifiesSP165, func(t *testing.T) {
		if got := killedBy(power.OriginTargeted, testOwnA).KilledBy([]ecosystem.TestID{"unknown"}); len(got) != 0 {
			t.Errorf("KilledBy() = %v", got)
		}
	})
	t.Run("an error outcome is not a kill"+verifiesSP165, func(t *testing.T) {
		facts := survivor(power.OriginTargeted, power.Skipped)
		facts.Outcomes[testOwnA] = format.OutcomeError
		if facts.KilledByAny() || len(facts.KilledBy(allTests())) != 0 {
			t.Error("an error counted as a kill")
		}
	})
	killedAny := []struct {
		name  string
		facts power.Facts
		want  bool
	}{
		{"a survivor", survivor(power.OriginTargeted, power.Skipped), false},
		{"a mutant killed by a test of another criterion", killedBy(power.OriginTargeted, testOtherB), true},
		{"a mutant with no outcome", power.Facts{}, false},
	}
	for _, tc := range killedAny {
		t.Run("KilledByAny on "+tc.name+verifiesSP165, func(t *testing.T) {
			if got := tc.facts.KilledByAny(); got != tc.want {
				t.Errorf("KilledByAny() = %v", got)
			}
		})
	}
	t.Run("AllSurvived is true for no test"+verifiesSP165, func(t *testing.T) {
		if !killedBy(power.OriginTargeted, testOwnA).AllSurvived(nil) {
			t.Error("AllSurvived(nil) = false")
		}
	})
	t.Run("AllSurvived is false for a killed test and for an error"+verifiesSP165, func(t *testing.T) {
		facts := killedBy(power.OriginTargeted, testOwnA)
		facts.Outcomes[testOwnB] = format.OutcomeError
		if facts.AllSurvived([]ecosystem.TestID{testOtherA, testOwnA}) || facts.AllSurvived([]ecosystem.TestID{testOwnB}) || !facts.AllSurvived([]ecosystem.TestID{testOtherA, testOtherB}) {
			t.Error("AllSurvived gave a wrong answer")
		}
	})
	t.Run("AllSurvived is false for a test with no outcome"+verifiesSP165, func(t *testing.T) {
		if survivor(power.OriginTargeted, power.Skipped).AllSurvived([]ecosystem.TestID{"unknown"}) {
			t.Error("a test without outcome counted as survived")
		}
	})
}

func TestSubjectQueries(t *testing.T) {
	t.Run("SpecificKillers drops the tests that killed a sentinel"+verifiesSP165, func(t *testing.T) {
		subject := newSubject(testOwnA)
		got := subject.SpecificKillers(killedBy(power.OriginTargeted, testOwnA, testOwnB, testOtherA))
		if !slices.Equal(got, []ecosystem.TestID{testOwnB}) {
			t.Errorf("SpecificKillers() = %v", got)
		}
	})
	t.Run("SpecificKillers only considers the tests of the criterion"+verifiesSP165, func(t *testing.T) {
		if got := newSubject().SpecificKillers(killedBy(power.OriginTargeted, testOtherA)); len(got) != 0 {
			t.Errorf("SpecificKillers() = %v", got)
		}
	})
	t.Run("Killed looks at the tests of the criterion only"+verifiesSP165, func(t *testing.T) {
		subject := newSubject()
		if !subject.Killed(killedBy(power.OriginTargeted, testOwnB)) || subject.Killed(killedBy(power.OriginTargeted, testOtherA)) || subject.Killed(survivor(power.OriginTargeted, power.Skipped)) {
			t.Error("Killed gave a wrong answer")
		}
	})
	t.Run("OtherCriteriaPass needs every other test to pass"+verifiesSP165, func(t *testing.T) {
		subject := newSubject()
		if !subject.OtherCriteriaPass(killedBy(power.OriginTargeted, testOwnA)) || subject.OtherCriteriaPass(killedBy(power.OriginTargeted, testOtherB)) {
			t.Error("OtherCriteriaPass gave a wrong answer")
		}
	})
}

func TestSentinelKillers(t *testing.T) {
	t.Run("only sentinels contribute killers"+verifiesSP165, func(t *testing.T) {
		got := power.SentinelKillers([]power.Facts{
			killedBy(power.OriginSentinel, testOwnA),
			killedBy(power.OriginTargeted, testOwnB),
			killedBy(power.OriginOperator, testOtherA),
			killedBy(power.OriginSentinel, testOtherB),
		})
		want := map[ecosystem.TestID]bool{testOwnA: true, testOtherB: true}
		if len(got) != len(want) || !got[testOwnA] || !got[testOtherB] {
			t.Errorf("SentinelKillers() = %v", got)
		}
	})
	t.Run("no sentinel gives an empty set"+verifiesSP165, func(t *testing.T) {
		if got := power.SentinelKillers([]power.Facts{killedBy(power.OriginTargeted, testOwnA)}); len(got) != 0 {
			t.Errorf("SentinelKillers() = %v", got)
		}
	})
	t.Run("a surviving sentinel gives no killer"+verifiesSP165, func(t *testing.T) {
		if got := power.SentinelKillers([]power.Facts{survivor(power.OriginSentinel, power.NoDiffFound)}); len(got) != 0 {
			t.Errorf("SentinelKillers() = %v", got)
		}
	})
}

func TestSlots(t *testing.T) {
	t.Run("the slots are listed in order"+verifiesSP165, func(t *testing.T) {
		if got := power.Slots(); !slices.Equal(got, []power.Slot{power.SlotDirect, power.SlotBoundary, power.SlotPartial}) {
			t.Errorf("Slots() = %v", got)
		}
	})
	t.Run("the list of slots is a copy"+verifiesSP165, func(t *testing.T) {
		power.Slots()[0] = power.SlotPartial
		if power.Slots()[0] != power.SlotDirect {
			t.Error("mutating the returned list changed the slots")
		}
	})
	for _, tc := range []struct {
		slot  power.Slot
		valid bool
	}{{power.SlotDirect, true}, {power.SlotBoundary, true}, {power.SlotPartial, true}, {"S4", false}, {"", false}, {"s1", false}} {
		t.Run("the slot "+string(tc.slot)+" validity"+verifiesSP165, func(t *testing.T) {
			if tc.slot.Valid() != tc.valid {
				t.Errorf("Valid() = %v", tc.slot.Valid())
			}
		})
	}
}
