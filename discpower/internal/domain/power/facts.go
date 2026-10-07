// Package power holds the pure decision rules of discriminating power: how the
// executed facts about a mutant and the host's judgments become a validity with
// its corroboration (ADR 0003 validity mapping, ADR 0005 claim-power table), how
// the decisions of a pool become the terminal outcome of a criterion, when the
// search for mutants stops, and how a screen answer flags a survivor.
package power

import (
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
)

// Origin tells where a pool mutant comes from.
type Origin string

// Origins of pool mutants.
const (
	// OriginTargeted is a mutant a host generation ticket returned.
	OriginTargeted Origin = "targeted"
	// OriginOperator is a deterministic operator mutant.
	OriginOperator Origin = "operator"
	// OriginSentinel is a semantic no-op drawn from the sentinel catalogue.
	OriginSentinel Origin = "sentinel"
)

// Slot is one of the three mutant slots of a generation round.
type Slot string

// Generation slots (ADR 0003, S0 targeted).
const (
	SlotDirect   Slot = "S1"
	SlotBoundary Slot = "S2"
	SlotPartial  Slot = "S3"
)

var slots = []Slot{SlotDirect, SlotBoundary, SlotPartial}

// Slots lists the generation slots in order.
func Slots() []Slot { return slices.Clone(slots) }

// Valid reports whether the slot is one of the three.
func (s Slot) Valid() bool { return slices.Contains(slots, s) }

// DifferentialStatus is the result of the differential search (F3).
type DifferentialStatus string

// Differential results.
const (
	// DiffersAt: the search found inputs on which mutant and reference differ.
	DiffersAt DifferentialStatus = "differs"
	// NoDiffFound: the search drew inputs and found no difference.
	NoDiffFound DifferentialStatus = "no_diff_found"
	// Skipped: the search could not run; the reason says why.
	Skipped DifferentialStatus = "skipped"
)

// Witness is an input on which the mutant and the reference behave differently,
// with what each did.
type Witness struct {
	Call      spec.Call        `json:"call"`
	Reference spec.Observation `json:"reference"`
	Mutant    spec.Observation `json:"mutant"`
}

// Differential is the outcome of F3 for one mutant.
type Differential struct {
	Status     DifferentialStatus `json:"status"`
	Witnesses  []Witness          `json:"witnesses,omitempty"`
	SkipReason string             `json:"skipReason,omitempty"`
}

// Facts are the executed facts about one mutant that built: where it edits, what
// every test of the suite did against it, which criteria's examples it fails and
// what the differential search found.
type Facts struct {
	ID     string
	Origin Origin
	Slot   Slot
	// Sites lists the units the mutant edits, as `file::qualname`.
	Sites []string
	// Outcomes holds the outcome of every test of the suite against the mutant.
	Outcomes map[ecosystem.TestID]format.Outcome
	// FailedExamples lists the criteria one of whose examples the mutant fails,
	// sorted.
	FailedExamples []spec.Ref
	Differential   Differential
}

// KilledBy lists, in the given order, the tests among the candidates that the
// mutant failed.
func (f Facts) KilledBy(tests []ecosystem.TestID) []ecosystem.TestID {
	var killers []ecosystem.TestID
	for _, test := range tests {
		if f.Outcomes[test] == format.OutcomeKilled {
			killers = append(killers, test)
		}
	}
	return killers
}

// KilledByAny reports whether any test of the suite failed against the mutant.
func (f Facts) KilledByAny() bool {
	for _, outcome := range f.Outcomes {
		if outcome == format.OutcomeKilled {
			return true
		}
	}
	return false
}

// AllSurvived reports whether every given test passed against the mutant.
func (f Facts) AllSurvived(tests []ecosystem.TestID) bool {
	for _, test := range tests {
		if f.Outcomes[test] != format.OutcomeSurvived {
			return false
		}
	}
	return true
}

// Subject is the criterion a pool is judged for and the suite around it.
type Subject struct {
	Ref spec.Ref
	// Tests are the tests bound to the criterion.
	Tests []ecosystem.TestID
	// OtherTests are the tests bound to every other criterion.
	OtherTests []ecosystem.TestID
	// SentinelKillers are the tests of the suite that killed a sentinel.
	SentinelKillers map[ecosystem.TestID]bool
}

// SpecificKillers lists the criterion's tests that killed the mutant and killed
// no sentinel.
func (s Subject) SpecificKillers(f Facts) []ecosystem.TestID {
	var killers []ecosystem.TestID
	for _, test := range f.KilledBy(s.Tests) {
		if !s.SentinelKillers[test] {
			killers = append(killers, test)
		}
	}
	return killers
}

// Killed reports whether a test bound to the criterion failed against the mutant.
func (s Subject) Killed(f Facts) bool { return len(f.KilledBy(s.Tests)) > 0 }

// OtherCriteriaPass reports whether every test of every other criterion passed.
func (s Subject) OtherCriteriaPass(f Facts) bool { return f.AllSurvived(s.OtherTests) }

// SentinelKillers lists the tests that killed any of the sentinels.
func SentinelKillers(all []Facts) map[ecosystem.TestID]bool {
	killers := map[ecosystem.TestID]bool{}
	for _, facts := range all {
		if facts.Origin != OriginSentinel {
			continue
		}
		for test, outcome := range facts.Outcomes {
			if outcome == format.OutcomeKilled {
				killers[test] = true
			}
		}
	}
	return killers
}
