// Package tournament plans the System One `verifies` question when a project has
// more criteria than one choice question holds (ADR 0003, "System One
// questions"): groups of at most MaxGroup criteria plus NONE, then a final round
// among the winners. The plan is a pure function of the options and of the
// answers recorded so far, so a resumed run recomputes it rather than storing it.
package tournament

import (
	"slices"

	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

// MaxGroup is the largest number of criteria one question holds, besides NONE.
const MaxGroup = 23

// Answer is the choice a closed question was answered with and the confidence of
// the answer.
type Answer struct {
	Choice     string
	Confidence float64
	// Coverage is the answer to the coverage question of the final round; it is
	// empty in the other rounds.
	Coverage string
}

// Lookup returns the answer to the question about a group, when there is one. A
// group is addressed by its options in order, without NONE; final tells whether
// the group is the only one of its round.
type Lookup func(group []string, final bool) (Answer, bool)

// Outcome is where the tournament stands.
type Outcome struct {
	// Done is true when the final round was answered.
	Done bool
	// Winner is the criterion the final round chose, or masking.NoneAnswer.
	Winner string
	// Coverage is the coverage the final round gave, empty when the final round was
	// not reached or chose NONE.
	Coverage string
	// Confidence is the minimum confidence over every answer of every round.
	Confidence float64
	// Pending lists the groups the next round asks about, when not Done.
	Pending [][]string
	// Final is true when Pending, or the answered round, is the final round.
	Final bool
}

// Groups splits the options in order into the fewest groups of at most MaxGroup,
// as even as possible, the earlier groups taking the extra option.
func Groups(options []string) [][]string {
	if len(options) <= MaxGroup {
		return [][]string{slices.Clone(options)}
	}
	count := (len(options) + MaxGroup - 1) / MaxGroup
	size, extra := len(options)/count, len(options)%count
	groups := make([][]string, 0, count)
	start := 0
	for index := range count {
		length := size
		if index < extra {
			length++
		}
		groups = append(groups, slices.Clone(options[start:start+length]))
		start += length
	}
	return groups
}

// Resolve plays the tournament over the options as far as the lookup's answers
// go. With no option the winner is NONE.
func Resolve(options []string, lookup Lookup) Outcome {
	confidence := 1.0
	lastCoverage := ""
	candidates := slices.Clone(options)
	for {
		groups := Groups(candidates)
		final := len(groups) == 1
		var winners []string
		for _, group := range groups {
			answer, answered := lookup(group, final)
			if !answered {
				return Outcome{Pending: unanswered(groups, final, lookup), Final: final, Confidence: confidence}
			}
			confidence = min(confidence, answer.Confidence)
			lastCoverage = answer.Coverage
			if answer.Choice != masking.NoneAnswer && slices.Contains(group, answer.Choice) {
				winners = append(winners, answer.Choice)
			}
		}
		switch {
		case final && len(winners) == 0:
			return Outcome{Done: true, Winner: masking.NoneAnswer, Confidence: confidence, Final: true}
		case final:
			return Outcome{Done: true, Winner: winners[0], Coverage: lastCoverage, Confidence: confidence, Final: true}
		case len(winners) == 0:
			return Outcome{Done: true, Winner: masking.NoneAnswer, Confidence: confidence}
		}
		candidates = winners
	}
}

func unanswered(groups [][]string, final bool, lookup Lookup) [][]string {
	var pending [][]string
	for _, group := range groups {
		if _, answered := lookup(group, final); !answered {
			pending = append(pending, group)
		}
	}
	return pending
}
