package power_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/format"
)

const maxScreenAnswers = 6

func screenAnswer(choice power.ClosedChoice, isolation format.Isolation, marked bool) power.ScreenAnswer {
	return power.ScreenAnswer{ClosedAnswer: hostVote(choice), Isolation: isolation, Marked: marked}
}

func TestScreenFloor(t *testing.T) {
	floor := format.IsolationRestricted
	tau := policy.DefaultTauScreen
	violates, consistent := power.ChoiceViolates, power.ChoiceConsistent
	inline := format.IsolationInline
	cases := []struct {
		name        string
		answers     []power.ScreenAnswer
		wantFlagged bool
		wantEntry   bool
	}{
		{"a screen-out below the floor does not exclude", []power.ScreenAnswer{screenAnswer(consistent, inline, false)}, true, true},
		{"a screen-out of an unrestricted executor does not exclude", []power.ScreenAnswer{screenAnswer(consistent, format.IsolationUnrestricted, false)}, true, true},
		{"a marked screen-out does not exclude", []power.ScreenAnswer{screenAnswer(consistent, format.IsolationRestrictedAttested, true)}, true, true},
		{"a screen-out at the floor excludes", []power.ScreenAnswer{screenAnswer(consistent, floor, false)}, false, false},
		{"a screen-out above the floor excludes", []power.ScreenAnswer{screenAnswer(consistent, format.IsolationRestrictedAttested, false)}, false, false},
		{"a VIOLATES answer below the floor still flags without a floor entry", []power.ScreenAnswer{screenAnswer(violates, inline, false)}, true, false},
		{"a marked VIOLATES answer still flags without a floor entry", []power.ScreenAnswer{screenAnswer(violates, floor, true)}, true, false},
		{"every answer marked VIOLATES flags without a floor entry", []power.ScreenAnswer{screenAnswer(violates, floor, true), screenAnswer(violates, floor, true)}, true, false},
		{"a VIOLATES answer at the floor flags without a floor entry", []power.ScreenAnswer{screenAnswer(violates, floor, false)}, true, false},
		{"a below-floor screen-out beside an at-floor screen-out does not change the exclusion", []power.ScreenAnswer{
			screenAnswer(consistent, floor, false), screenAnswer(consistent, inline, false),
		}, false, false},
		{"a below-floor VIOLATES answer beside an at-floor screen-out flags without a floor entry", []power.ScreenAnswer{
			screenAnswer(consistent, floor, false), screenAnswer(violates, inline, false),
		}, true, false},
		{"below-floor screen-outs cannot dilute an at-floor VIOLATES answer, which the floor keeps in the pool", []power.ScreenAnswer{
			screenAnswer(violates, floor, false), screenAnswer(consistent, inline, false), screenAnswer(consistent, inline, false),
			screenAnswer(consistent, inline, false), screenAnswer(consistent, inline, false), screenAnswer(consistent, inline, false),
		}, true, true},
		{"no answer leaves the survivor in the pool", nil, true, false},
	}
	for _, tc := range cases {
		t.Run(tc.name+" [verifies SP-252~1]", func(t *testing.T) {
			verdict := power.Screen(tc.answers, floor, tau)
			if verdict.Flagged != tc.wantFlagged || verdict.FloorEntry != tc.wantEntry {
				t.Errorf("Screen() = %+v, want flagged %v and floor entry %v", verdict, tc.wantFlagged, tc.wantEntry)
			}
		})
	}
}

func TestScreenFloorProperties(t *testing.T) {
	genAnswer := rapid.Custom(func(t *rapid.T) power.ScreenAnswer {
		return power.ScreenAnswer{
			ClosedAnswer: hostVote(rapid.SampledFrom([]power.ClosedChoice{power.ChoiceViolates, power.ChoiceConsistent}).Draw(t, "choice")),
			Isolation:    rapid.SampledFrom(format.Isolations()).Draw(t, "isolation"),
			Marked:       rapid.Bool().Draw(t, "marked"),
		}
	})

	t.Run("no set of answers below the floor or marked ever excludes a survivor [verifies SP-252~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			floor := rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
			tau := rapid.Float64Range(0, 1).Draw(t, "tau")
			answers := rapid.SliceOfN(genAnswer, 0, maxScreenAnswers).Draw(t, "answers")
			for index := range answers {
				if answers[index].AtFloor(floor) {
					answers[index].Marked = true
				}
			}
			if verdict := power.Screen(answers, floor, tau); !verdict.Flagged {
				t.Fatalf("answers without credit excluded a survivor: %+v", answers)
			}
		})
	})

	t.Run("a survivor is excluded only when an answer at the floor is unmarked and no answer reaches the threshold [verifies SP-252~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			floor := rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
			tau := rapid.Float64Range(0.01, 1).Draw(t, "tau")
			answers := rapid.SliceOfN(genAnswer, 0, maxScreenAnswers).Draw(t, "answers")
			if power.Screen(answers, floor, tau).Flagged {
				return
			}
			credited := false
			all := make([]power.ClosedAnswer, len(answers))
			for index, answer := range answers {
				credited = credited || answer.AtFloor(floor)
				all[index] = answer.ClosedAnswer
			}
			if !credited || power.FlagsSurvivor(power.ViolationProbability(all), tau) {
				t.Fatalf("excluded without credit or above the threshold: %+v", answers)
			}
		})
	})
}
