package power_test

import (
	"math"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
)

func hostVote(choice power.ClosedChoice) power.ClosedAnswer {
	return power.ClosedAnswer{Choice: choice}
}

// stated is the answer of an endpoint that picks a choice and states the
// probability it gives to VIOLATES.
func stated(choice power.ClosedChoice, violates float64) power.ClosedAnswer {
	return power.ClosedAnswer{Choice: choice, Violates: violates, HasViolates: true}
}

func TestViolationProbability(t *testing.T) {
	violates, consistent := power.ChoiceViolates, power.ChoiceConsistent
	cases := []struct {
		name    string
		answers []power.ClosedAnswer
		want    float64
	}{
		{"no answer", nil, 0},
		{"one VIOLATES vote", []power.ClosedAnswer{hostVote(violates)}, 1},
		{"one CONSISTENT vote", []power.ClosedAnswer{hostVote(consistent)}, 0},
		{"votes give their share of VIOLATES", []power.ClosedAnswer{hostVote(violates), hostVote(violates), hostVote(consistent)}, 2.0 / 3.0},
		{"an even split", []power.ClosedAnswer{hostVote(violates), hostVote(consistent)}, 0.5},
		{"an endpoint's probability of VIOLATES", []power.ClosedAnswer{stated(violates, 0.8)}, 0.8},
		{"the probability of VIOLATES of a CONSISTENT answer is its own, not a complement of anything else", []power.ClosedAnswer{stated(consistent, 0.1)}, 0.1},
		{"the mean of the probabilities of VIOLATES", []power.ClosedAnswer{stated(violates, 0.8), stated(consistent, 0.1)}, 0.45},
		{"a mix of stated probabilities and votes", []power.ClosedAnswer{stated(violates, 0.4), hostVote(violates), hostVote(consistent), stated(consistent, 0)}, 0.35},
		{"an explicit zero probability is not a missing one", []power.ClosedAnswer{stated(violates, 0)}, 0},
		{"a VIOLATES answer just above one half with no concentration is that probability, not zero", []power.ClosedAnswer{stated(violates, 0.55)}, 0.55},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP169, func(t *testing.T) {
			if got := power.ViolationProbability(tc.answers); math.Abs(got-tc.want) > floatTolerance {
				t.Errorf("ViolationProbability() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestFlagsSurvivor(t *testing.T) {
	cases := []struct {
		name        string
		probability float64
		want        bool
	}{
		{"above the threshold", 0.5, true},
		{"exactly at the threshold", policy.DefaultTauScreen, true},
		{"just below the threshold", 0.19, false},
		{"zero", 0, false},
		{"one", 1, true},
	}
	for _, tc := range cases {
		t.Run(tc.name+verifiesSP169, func(t *testing.T) {
			if got := power.FlagsSurvivor(tc.probability, policy.DefaultTauScreen); got != tc.want {
				t.Errorf("FlagsSurvivor(%v) = %v", tc.probability, got)
			}
		})
	}
	t.Run("a threshold of zero flags every survivor"+verifiesSP169, func(t *testing.T) {
		if !power.FlagsSurvivor(0, 0) {
			t.Error("a zero probability was not flagged at tau_screen 0")
		}
	})
}
