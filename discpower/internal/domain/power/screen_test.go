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

func confidentVote(choice power.ClosedChoice, confidence float64) power.ClosedAnswer {
	return power.ClosedAnswer{Choice: choice, Confidence: confidence, HasConfidence: true}
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
		{"votes without confidence give their share of VIOLATES", []power.ClosedAnswer{hostVote(violates), hostVote(violates), hostVote(consistent)}, 2.0 / 3.0},
		{"an even split", []power.ClosedAnswer{hostVote(violates), hostVote(consistent)}, 0.5},
		{"a VIOLATES answer with confidence", []power.ClosedAnswer{confidentVote(violates, 0.8)}, 0.8},
		{"a CONSISTENT answer with confidence gives the complement", []power.ClosedAnswer{confidentVote(consistent, 0.9)}, 0.1},
		{"the mean of answers with confidence", []power.ClosedAnswer{confidentVote(violates, 0.8), confidentVote(consistent, 0.9)}, 0.45},
		{"a mix of votes with and without confidence", []power.ClosedAnswer{confidentVote(violates, 0.4), hostVote(violates), hostVote(consistent), confidentVote(consistent, 1)}, 0.35},
		{"an explicit zero confidence is not a missing one", []power.ClosedAnswer{confidentVote(violates, 0)}, 0},
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
