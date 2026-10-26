package advice_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/completeness/internal/domain/advice"
	"github.com/RachidChabane/musthold/completeness/internal/domain/closedq"
	"github.com/RachidChabane/musthold/completeness/internal/domain/dimension"
	"github.com/RachidChabane/musthold/completeness/internal/domain/prompt"
	"github.com/RachidChabane/musthold/completeness/internal/domain/tournament"
)

const tau = 0.5

func decided(choice string, confidence float64) closedq.Decided {
	return closedq.Decided{Choice: choice, Confidence: confidence}
}

func TestDimensionGaps(t *testing.T) {
	asked := dimension.Checklist()

	t.Run("MISSING at or above tau_adv is a gap and MISSING below it a low-confidence gap [verifies SP-503~1]", func(t *testing.T) {
		got := advice.DimensionGaps(asked, map[string]closedq.Decided{
			"boundaries":       decided("MISSING", 0.5),
			"error_handling":   decided("MISSING", 0.49),
			"permissions":      decided("MISSING", 1),
			"concurrency":      decided("MISSING", 0),
			"compatibility":    decided("COVERED", 1),
			"input_validation": decided("NOT_APPLICABLE", 1),
		}, tau)
		want := []advice.Gap{
			{Dimension: dimension.ErrorHandling, LowConfidence: true},
			{Dimension: dimension.Boundaries},
			{Dimension: dimension.Permissions},
			{Dimension: dimension.Concurrency, LowConfidence: true},
		}
		if !slices.Equal(got, want) {
			t.Fatalf("gaps %+v, want %+v", got, want)
		}
	})

	t.Run("an unanswered dimension is no gap [verifies SP-503~1]", func(t *testing.T) {
		if got := advice.DimensionGaps(asked, map[string]closedq.Decided{}, tau); len(got) != 0 {
			t.Fatalf("gaps %+v", got)
		}
	})

	t.Run("a dimension that was not asked never yields a gap, whatever System One says [verifies SP-501~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			declared := rapid.SliceOf(rapid.SampledFrom(dimension.Checklist())).Draw(t, "declared")
			names := make([]string, len(declared))
			for index, d := range declared {
				names[index] = string(d)
			}
			readings := map[string]closedq.Decided{}
			for _, d := range dimension.Checklist() {
				readings[string(d)] = decided("MISSING", rapid.Float64Range(0, 1).Draw(t, string(d)))
			}
			remaining := dimension.Remaining(names)
			for _, gap := range advice.DimensionGaps(remaining, readings, rapid.Float64Range(0, 1).Draw(t, "tau")) {
				if slices.Contains(declared, gap.Dimension) {
					t.Fatalf("%s is declared out of scope and still yields a gap", gap.Dimension)
				}
			}
		})
	})

	t.Run("raising tau_adv never removes a gap and only flags more of them [verifies SP-503~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			readings := map[string]closedq.Decided{}
			for _, d := range dimension.Checklist() {
				readings[string(d)] = decided(rapid.SampledFrom([]string{"MISSING", "COVERED"}).Draw(t, string(d)), rapid.Float64Range(0, 1).Draw(t, "c"+string(d)))
			}
			low := rapid.Float64Range(0, 1).Draw(t, "low")
			high := rapid.Float64Range(low, 1).Draw(t, "high")
			before, after := advice.DimensionGaps(asked, readings, low), advice.DimensionGaps(asked, readings, high)
			if len(before) != len(after) {
				t.Fatalf("%d gaps at %v, %d at %v", len(before), low, len(after), high)
			}
			for index := range before {
				if before[index].LowConfidence && !after[index].LowConfidence {
					t.Fatalf("%s lost its flag when tau rose", before[index].Dimension)
				}
			}
		})
	})
}

func TestUnjustified(t *testing.T) {
	t.Run("a change is unjustified when the tournament ended on NONE at or above tau_adv [verifies SP-510~1]", func(t *testing.T) {
		cases := []struct {
			name    string
			outcome tournament.Outcome
			want    bool
		}{
			{"none, sure", tournament.Outcome{Done: true, Winner: "NONE", Confidence: 0.9}, true},
			{"none at the threshold", tournament.Outcome{Done: true, Winner: "NONE", Confidence: 0.5}, true},
			{"none, unsure", tournament.Outcome{Done: true, Winner: "NONE", Confidence: 0.49}, false},
			{"a requirement", tournament.Outcome{Done: true, Winner: "K-0001", Confidence: 1}, false},
			{"unfinished", tournament.Outcome{Done: false, Winner: "NONE", Confidence: 1}, false},
		}
		for _, c := range cases {
			if got := advice.Unjustified(c.outcome, tau); got != c.want {
				t.Errorf("%s: %v, want %v", c.name, got, c.want)
			}
		}
	})
}

func TestUnclear(t *testing.T) {
	tokens := []string{"K-a", "K-b", "K-c", "K-d"}

	t.Run("AMBIGUOUS and UNTESTABLE are unclear at any confidence and TESTABLE is not [verifies SP-512~1]", func(t *testing.T) {
		got := advice.Unclear(tokens, map[string]closedq.Decided{
			"K-a": decided("AMBIGUOUS", 0),
			"K-b": decided("UNTESTABLE", 0.1),
			"K-c": decided("TESTABLE", 1),
		})
		want := map[string]prompt.Testability{"K-a": prompt.Ambiguous, "K-b": prompt.Untestable}
		if len(got) != 2 || got["K-a"] != want["K-a"] || got["K-b"] != want["K-b"] {
			t.Fatalf("unclear %v", got)
		}
	})

	t.Run("an answer for a token outside the group is ignored [verifies SP-512~1]", func(t *testing.T) {
		if got := advice.Unclear(tokens[:1], map[string]closedq.Decided{"K-z": decided("AMBIGUOUS", 1)}); len(got) != 0 {
			t.Fatalf("unclear %v", got)
		}
	})
}
