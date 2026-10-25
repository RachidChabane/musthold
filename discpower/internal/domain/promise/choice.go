package promise

import "slices"

// MaxPromises is how many promises a scan checks.
const MaxPromises = 3

// Source ranks: a lower rank is chosen first. The description and the closing
// issues state what the change intends, which the README does not.
const (
	rankChange = iota
	rankReadme
)

// Choice is the outcome of the choice among the kept promises.
type Choice struct {
	// Chosen are at most MaxPromises promises, in the order the scan keeps them.
	Chosen []Kept
	// Unchecked names each promise left out.
	Unchecked []Unchecked
}

// Choose keeps at most MaxPromises of the kept promises: those quoted from the
// description or a closing issue before those quoted from the README, then the
// order of extraction. The promises left out are reported as unchecked.
func Choose(kept []Kept) Choice {
	ordered := slices.Clone(kept)
	slices.SortStableFunc(ordered, func(a, b Kept) int { return rankOf(a.Kind) - rankOf(b.Kind) })
	var choice Choice
	for index, candidate := range ordered {
		if index < MaxPromises {
			choice.Chosen = append(choice.Chosen, candidate)
			continue
		}
		choice.Unchecked = append(choice.Unchecked, Unchecked{Promise: candidate.Promise.ID, Reason: UncheckedOverCap})
	}
	return choice
}

func rankOf(kind SourceKind) int {
	if kind == SourceReadme {
		return rankReadme
	}
	return rankChange
}

// IDs lists the ids of the promises, in order.
func IDs(kept []Kept) []string {
	ids := make([]string, len(kept))
	for index, promise := range kept {
		ids[index] = promise.Promise.ID
	}
	return ids
}
