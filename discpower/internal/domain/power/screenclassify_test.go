package power

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"pgregory.net/rapid"
)

func TestAnUnraisedClassificationNeverScreensASurvivorOut(t *testing.T) {
	isolations := []format.Isolation{format.IsolationRestrictedUnverified, format.IsolationUnrestricted, format.IsolationInline}

	t.Run("below the floor, whatever the answers say and whether or not they are marked, the survivor stays in the pool [verifies SP-1916~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(1, 5).Draw(t, "answers")
			answers := make([]ScreenAnswer, count)
			for index := range answers {
				answers[index] = ScreenAnswer{
					ClosedAnswer: ClosedAnswer{
						Choice:      rapid.SampledFrom([]ClosedChoice{ChoiceViolates, ChoiceConsistent}).Draw(t, "choice"),
						Violates:    rapid.Float64Range(0, 1).Draw(t, "probability"),
						HasViolates: rapid.Bool().Draw(t, "stated"),
					},
					Isolation: rapid.SampledFrom(isolations).Draw(t, "isolation"),
					Marked:    rapid.Bool().Draw(t, "marked"),
				}
			}
			tau := rapid.Float64Range(0, 1).Draw(t, "tau")
			if verdict := Screen(answers, format.IsolationRestricted, tau); !verdict.Flagged {
				t.Fatalf("%+v screened a survivor out at tau %v: %+v", answers, tau, verdict)
			}
		})
	})

	t.Run("at the floor, a classification without a threshold of its own source cannot screen a survivor out either [verifies SP-1916~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			answer := ScreenAnswer{
				ClosedAnswer:     ClosedAnswer{Choice: ChoiceConsistent, Violates: rapid.Float64Range(0, 0.19).Draw(t, "probability"), HasViolates: true},
				Isolation:        rapid.SampledFrom([]format.Isolation{format.IsolationRestricted, format.IsolationRestrictedAttested}).Draw(t, "isolation"),
				NoExactThreshold: true,
			}
			if verdict := Screen([]ScreenAnswer{answer}, format.IsolationRestricted, 0.2); !verdict.Flagged || !verdict.FloorEntry {
				t.Fatalf("verdict %+v", verdict)
			}
		})
	})
}
