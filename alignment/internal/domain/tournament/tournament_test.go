package tournament_test

import (
	"fmt"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/tournament"
)

func options(count int) []string {
	list := make([]string, count)
	for index := range list {
		list[index] = fmt.Sprintf("K-%04d", index)
	}
	return list
}

// chooses answers every question with the option at the given position of its
// group, or NONE when the group is shorter, at the given confidence.
func chooses(position int, confidence float64) tournament.Lookup {
	return func(group []string, _ bool) (tournament.Answer, bool) {
		if position >= len(group) {
			return tournament.Answer{Choice: masking.NoneAnswer, Confidence: confidence}, true
		}
		return tournament.Answer{Choice: group[position], Confidence: confidence}, true
	}
}

func TestGroups(t *testing.T) {
	t.Run("up to 23 criteria are one group [verifies SP-309~1]", func(t *testing.T) {
		for _, count := range []int{1, 22, tournament.MaxGroup} {
			if groups := tournament.Groups(options(count)); len(groups) != 1 || len(groups[0]) != count {
				t.Errorf("%d options gave %d groups", count, len(groups))
			}
		}
	})

	t.Run("more criteria are split into the fewest groups of at most 23, as even as possible [verifies SP-309~1]", func(t *testing.T) {
		groups := tournament.Groups(options(24))
		if len(groups) != 2 || len(groups[0]) != 12 || len(groups[1]) != 12 {
			t.Fatalf("groups %v", groups)
		}
		groups = tournament.Groups(options(47))
		if len(groups) != 3 || len(groups[0]) != 16 || len(groups[1]) != 16 || len(groups[2]) != 15 {
			t.Fatalf("groups of 47: %d %d %d", len(groups[0]), len(groups[1]), len(groups[2]))
		}
	})

	t.Run("the groups partition the options in order, whatever their number [verifies SP-309~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 600).Draw(t, "count")
			var joined []string
			for _, group := range tournament.Groups(options(count)) {
				if len(group) > tournament.MaxGroup {
					t.Fatalf("group of %d", len(group))
				}
				joined = append(joined, group...)
			}
			if !slices.Equal(joined, options(count)) && count != 0 {
				t.Fatal("the groups do not partition the options in order")
			}
		})
	})
}

func TestResolve(t *testing.T) {
	t.Run("a single group is the final round and its answer is the winner [verifies SP-309~1]", func(t *testing.T) {
		outcome := tournament.Resolve(options(10), chooses(3, 0.95))
		if !outcome.Done || !outcome.Final || outcome.Winner != "K-0003" || outcome.Confidence != 0.95 {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("the coverage of the final round is the coverage of the tournament [verifies SP-309~1]", func(t *testing.T) {
		outcome := tournament.Resolve(options(5), func(group []string, final bool) (tournament.Answer, bool) {
			return tournament.Answer{Choice: group[1], Confidence: 0.9, Coverage: "PARTIAL"}, true
		})
		if !outcome.Done || outcome.Winner != "K-0001" || outcome.Coverage != "PARTIAL" {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("without an answer the first round is pending [verifies SP-309~1]", func(t *testing.T) {
		outcome := tournament.Resolve(options(30), func([]string, bool) (tournament.Answer, bool) { return tournament.Answer{}, false })
		if outcome.Done || len(outcome.Pending) != 2 || outcome.Final {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("the winners of the groups meet in a final round and the confidence is the minimum over rounds [verifies SP-309~1]", func(t *testing.T) {
		lookup := func(group []string, _ bool) (tournament.Answer, bool) {
			if len(group) > 2 {
				return tournament.Answer{Choice: group[0], Confidence: 0.99}, true
			}
			return tournament.Answer{Choice: group[len(group)-1], Confidence: 0.91}, true
		}
		outcome := tournament.Resolve(options(40), lookup)
		if !outcome.Done || !outcome.Final || outcome.Winner != "K-0020" || outcome.Confidence != 0.91 {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("a round answered only in part stays pending for the groups left [verifies SP-309~1]", func(t *testing.T) {
		groups := tournament.Groups(options(40))
		lookup := func(group []string, _ bool) (tournament.Answer, bool) {
			if slices.Equal(group, groups[0]) {
				return tournament.Answer{Choice: group[1], Confidence: 0.9}, true
			}
			return tournament.Answer{}, false
		}
		outcome := tournament.Resolve(options(40), lookup)
		if outcome.Done || len(outcome.Pending) != 1 || !slices.Equal(outcome.Pending[0], groups[1]) {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("NONE in every group ends the tournament with NONE [verifies SP-309~1]", func(t *testing.T) {
		outcome := tournament.Resolve(options(50), chooses(99, 0.8))
		if !outcome.Done || outcome.Winner != masking.NoneAnswer || outcome.Confidence != 0.8 {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("an answer that names no option of its group counts as NONE [verifies SP-309~1]", func(t *testing.T) {
		outcome := tournament.Resolve(options(5), func([]string, bool) (tournament.Answer, bool) {
			return tournament.Answer{Choice: "K-9999", Confidence: 1}, true
		})
		if outcome.Winner != masking.NoneAnswer {
			t.Fatalf("outcome %+v", outcome)
		}
	})

	t.Run("a tournament always ends, whatever the number of criteria and the answers [verifies SP-309~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 700).Draw(t, "count")
			position := rapid.IntRange(0, 30).Draw(t, "position")
			outcome := tournament.Resolve(options(count), chooses(position, 0.9))
			if !outcome.Done {
				t.Fatalf("the tournament of %d options did not end", count)
			}
			if outcome.Winner != masking.NoneAnswer && !slices.Contains(options(count), outcome.Winner) {
				t.Fatalf("the winner %q is not an option", outcome.Winner)
			}
		})
	})
}
