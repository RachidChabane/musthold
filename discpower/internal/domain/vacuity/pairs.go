package vacuity

import "slices"

// Coverage is what one test ran on the reference.
type Coverage struct {
	// Lines are every line the test's run executed, the start-up of its
	// process included; a mutant with a mutated line among them runs against
	// the test.
	Lines []Line
	// Owned are the lines the test ran itself, beyond what its process runs
	// before any test starts and what it shares with other tests. Only a line
	// the test owns lets a survival count against it; a test with no owned line
	// owns none.
	Owned []Line
}

// OwnedLines are the owned lines among the lines the test ran: a line outside
// Lines is never owned, whatever Owned says.
func (c Coverage) OwnedLines() []Line {
	ran := setOf(c.Lines)
	return slices.DeleteFunc(slices.Clone(c.Owned), func(line Line) bool { return !ran[line] })
}

// lineSet is a set of lines.
type lineSet map[Line]bool

func setOf(lines []Line) lineSet {
	set := make(lineSet, len(lines))
	for _, line := range lines {
		set[line] = true
	}
	return set
}

// holdsAny reports whether one of the lines is in the set.
func (s lineSet) holdsAny(lines []Line) bool {
	return slices.ContainsFunc(lines, func(line Line) bool { return s[line] })
}

// pairKind is what a pair of a test and a proven mutant it ran becomes.
type pairKind int

// Kinds of a pair.
const (
	// pairIgnored: the pair counts neither way and is not listed.
	pairIgnored pairKind = iota
	// pairChange: the pair is one of the test's changes.
	pairChange
	// pairNotStarted: the pair is listed apart, as a change that stopped the
	// test from starting.
	pairNotStarted
)

// pairKindOf is the rule of a pair, by its settled outcome and whether the
// test owns a mutated line of the mutant or ran them only at start-up:
//
//	settled       owned line     start-up-only line
//	killed        change         change
//	inconclusive  change         change
//	survived      change         ignored
//	not_started   listed apart   ignored
//
// Only a survival needs ownership, so a line the test ran only at start-up
// never feeds the claim that it verifies nothing, yet still records a kill and
// still blocks that claim when inconclusive.
func pairKindOf(settled Settled, owned bool) pairKind {
	switch settled {
	case SettledKilled, SettledInconclusive:
		return pairChange
	case SettledSurvived:
		if owned {
			return pairChange
		}
	case SettledNotStarted:
		if owned {
			return pairNotStarted
		}
	}
	return pairIgnored
}
