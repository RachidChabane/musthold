// Package power holds the pure decision rules of discriminating power: how the
// executed facts about a mutant and the host's judgments become a validity with
// its corroboration (ADR 0003 validity mapping, ADR 0005 claim-power table), how
// the decisions of a pool become the terminal outcome of a criterion, when the
// search for mutants stops, and how a screen answer flags a survivor.
package power

import (
	"maps"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
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

// Identity is the result of comparing the compiled forms of a mutant and the
// reference (E0).
type Identity string

// Identities.
const (
	// IdentityUnknown: the ecosystem compiles no comparable form, or the
	// comparison could not run.
	IdentityUnknown Identity = ""
	// IdentityIdentical: the mutant compiles to the reference's form.
	IdentityIdentical Identity = "identical"
	// IdentityDiffers: the compiled forms differ.
	IdentityDiffers Identity = "differs"
)

// Facts are the executed facts about one mutant that built: where it edits, what
// every test of the suite did against it, which criteria's examples it fails,
// what the differential search found, whether it compiles to the reference's
// form and what every witness did on it.
type Facts struct {
	ID     string `json:"id"`
	Origin Origin `json:"origin"`
	Slot   Slot   `json:"slot,omitempty"`
	// Sites lists the units the mutant edits, as `file::qualname`.
	Sites []string `json:"sites"`
	// Outcomes holds the outcome of every test of the suite against the mutant.
	Outcomes map[ecosystem.TestID]format.Outcome `json:"outcomes"`
	// FailedExamples lists the criteria one of whose examples the mutant fails,
	// sorted.
	FailedExamples []spec.Ref `json:"failedExamples,omitempty"`
	// ExampleFailures names each example the mutant fails, sorted by criterion
	// and index.
	ExampleFailures []ExampleFailure `json:"exampleFailures,omitempty"`
	Differential    Differential     `json:"differential"`
	Identity        Identity         `json:"identity,omitempty"`
	// WitnessOutcomes holds, by witness id, what each witness the mutant ran
	// against did: a failure is reproduced, a failure the rerun did not repeat is
	// flaky.
	WitnessOutcomes map[string]witness.Outcome `json:"witnessOutcomes,omitempty"`
}

// ExampleFailure is one example of a criterion that a mutant fails: the
// criterion and the example's index among its examples.
type ExampleFailure struct {
	Owner spec.Ref `json:"owner"`
	Index int      `json:"index"`
}

// Compare orders example failures by criterion, then by index.
func (e ExampleFailure) Compare(other ExampleFailure) int {
	if order := e.Owner.Compare(other.Owner); order != 0 {
		return order
	}
	return e.Index - other.Index
}

// FirstFailedExample returns the lowest index of an example of the criterion
// that the mutant fails.
func (f Facts) FirstFailedExample(ref spec.Ref) (int, bool) {
	for _, failure := range f.ExampleFailures {
		if failure.Owner == ref {
			return failure.Index, true
		}
	}
	return 0, false
}

// FailsWitness reports whether the mutant failed the witness, reproduced.
func (f Facts) FailsWitness(id string) bool { return f.WitnessOutcomes[id] == witness.OutcomeFailed }

// FailsExampleOf reports whether the mutant fails an example of the criterion.
func (f Facts) FailsExampleOf(ref spec.Ref) bool { return slices.Contains(f.FailedExamples, ref) }

// OwnerAttributed reports whether the mutant fails an example of the criterion
// and no example of another one.
func (f Facts) OwnerAttributed(ref spec.Ref) bool {
	return len(f.FailedExamples) == 1 && f.FailedExamples[0] == ref
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
	// Tests are the tests bound to the criterion: the set S the outcome is about.
	Tests []ecosystem.TestID
	// OtherTests are the tests bound to some other criterion, a test bound to the
	// criterion and to another one included.
	OtherTests []ecosystem.TestID
	// SentinelKillers are the tests of the suite that killed a sentinel.
	SentinelKillers map[ecosystem.TestID]bool
	// Bindings names, for every bound test of the suite, the criteria it is bound
	// to; it decides whether a mutant is isolated for another criterion.
	Bindings map[ecosystem.TestID][]spec.Ref
}

// NewSubject builds the subject of a criterion from the bindings of the suite and
// the tests that killed a sentinel.
func NewSubject(ref spec.Ref, bindings map[ecosystem.TestID][]spec.Ref, sentinelKillers map[ecosystem.TestID]bool) Subject {
	subject := Subject{Ref: ref, SentinelKillers: sentinelKillers, Bindings: bindings}
	for _, test := range slices.Sorted(maps.Keys(bindings)) {
		criteria := bindings[test]
		if slices.Contains(criteria, ref) {
			subject.Tests = append(subject.Tests, test)
		}
		if boundElsewhere(criteria, ref) {
			subject.OtherTests = append(subject.OtherTests, test)
		}
	}
	return subject
}

func boundElsewhere(criteria []spec.Ref, ref spec.Ref) bool {
	return slices.ContainsFunc(criteria, func(other spec.Ref) bool { return other != ref })
}

// IsolatedFor reports whether no test bound to a criterion other than ref killed
// the mutant or errored on it. Without bindings, the other tests of the subject
// stand for those of the subject's own criterion.
func (s Subject) IsolatedFor(f Facts, ref spec.Ref) bool {
	if s.Bindings == nil {
		return ref == s.Ref && s.OtherCriteriaPass(f)
	}
	for test, criteria := range s.Bindings {
		if !boundElsewhere(criteria, ref) {
			continue
		}
		if outcome := f.Outcomes[test]; outcome == format.OutcomeKilled || outcome == format.OutcomeError {
			return false
		}
	}
	return true
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
