// Package dimension holds the completeness checklist c1 (ADR 0003, "Completeness"):
// the eight dimensions a specification is held against, and the standings a
// dimension can take.
package dimension

import "slices"

// ChecklistVersion names the version of the checklist. The dimensions a request
// asks about are part of its template, so the version reaches every proof through
// the template digest.
const ChecklistVersion = "c1"

// Dimension is one aspect of behaviour a specification should address or
// explicitly declare out of scope.
type Dimension string

// The dimensions of checklist c1, in the order they are asked and reported.
const (
	ErrorHandling       Dimension = "error_handling"
	Boundaries          Dimension = "boundaries"
	Permissions         Dimension = "permissions"
	Concurrency         Dimension = "concurrency"
	Compatibility       Dimension = "compatibility"
	InputValidation     Dimension = "input_validation"
	TimeAndLocale       Dimension = "time_and_locale"
	StateAndPersistence Dimension = "state_and_persistence"
)

var checklist = []Dimension{
	ErrorHandling, Boundaries, Permissions, Concurrency, Compatibility, InputValidation, TimeAndLocale, StateAndPersistence,
}

// Checklist returns the dimensions of c1 in their fixed order.
func Checklist() []Dimension { return slices.Clone(checklist) }

// IsKnown reports whether the name is a dimension of c1.
func IsKnown(name string) bool { return slices.Contains(checklist, Dimension(name)) }

// Remaining lists the dimensions of c1 that are not declared out of scope, in
// checklist order. A declaration of a name outside the checklist excludes
// nothing.
func Remaining(declaredOutOfScope []string) []Dimension {
	remaining := make([]Dimension, 0, len(checklist))
	for _, candidate := range checklist {
		if !slices.Contains(declaredOutOfScope, string(candidate)) {
			remaining = append(remaining, candidate)
		}
	}
	return remaining
}

// Standing is what System One says about a dimension of a specification.
type Standing string

// Standings of a dimension.
const (
	Covered       Standing = "COVERED"
	NotApplicable Standing = "NOT_APPLICABLE"
	Missing       Standing = "MISSING"
)

// Standings lists the options of the dimension question.
func Standings() []Standing { return []Standing{Covered, NotApplicable, Missing} }

// Description tells a model what a dimension means.
func (d Dimension) Description() string { return descriptions[d] }

// Phrase is the dimension as it reads inside a sentence.
func (d Dimension) Phrase() string { return phrases[d] }

var descriptions = map[Dimension]string{
	ErrorHandling:       "What happens when an operation fails, an input is rejected or a dependency is unavailable.",
	Boundaries:          "Behaviour at the edges of ranges, sizes, counts and thresholds, including empty and maximal values.",
	Permissions:         "Who may perform an operation and what happens to someone who may not.",
	Concurrency:         "Behaviour when operations overlap, repeat or arrive in a different order.",
	Compatibility:       "Behaviour towards existing callers, stored data and earlier formats.",
	InputValidation:     "Which inputs are accepted, which are refused and how a refusal is reported.",
	TimeAndLocale:       "Dependence on clocks, time zones, calendars, currencies, languages and number formats.",
	StateAndPersistence: "What is kept between operations, when it is stored and what a restart or a failure leaves behind.",
}

var phrases = map[Dimension]string{
	ErrorHandling:       "error handling",
	Boundaries:          "boundary values",
	Permissions:         "permissions",
	Concurrency:         "concurrent use",
	Compatibility:       "compatibility with existing callers and data",
	InputValidation:     "input validation",
	TimeAndLocale:       "time zones, clocks and locale",
	StateAndPersistence: "state and persistence",
}
