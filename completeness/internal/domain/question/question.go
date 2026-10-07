// Package question holds the deterministic templates that turn a completeness
// signal into the advisory question a spec owner reads, and the normal form of a
// list of questions. A question is a pure function of its signal: no model writes
// it.
package question

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
)

// Dimensions of the questions that do not come from the checklist: they name the
// signal that raised the question.
const (
	DimensionUnjustifiedChange      = "unjustified_change"
	DimensionUnconstrainedBehaviour = "unconstrained_behaviour"
	DimensionClarity                = "clarity"
)

// LowConfidenceMarker starts the text of a question System One raised without the
// confidence the threshold asks for.
const LowConfidenceMarker = "(low confidence) "

// MutantsShown is the number of mutant digests an unconstrained-behaviour question
// names, and DigestPrefix the number of digest characters it shows of each.
const (
	MutantsShown = 3
	DigestPrefix = 12
)

// Templates of the questions.
const (
	missingTemplate = "The specification does not say how %s is handled. State the expected behaviour, " +
		"or declare the dimension out of scope with `Out of scope (%s): <reason>`."
	unjustifiedTemplate = "The change to %s in %s is stated by no requirement and no example. " +
		"Which requirement states this behaviour, or should the change be removed?"
	unconstrainedTemplate = "In %s, the changed %s can be altered in %d ways that violate no stated requirement (mutants %s). " +
		"Which requirement should constrain this behaviour?"
	ambiguousTemplate  = "Requirement %s can be read in more than one way. State which reading is meant."
	untestableTemplate = "Requirement %s states no outcome a test could observe. " +
		"Rewrite it with an observable condition and outcome, or mark it non-testable."
	listSeparator = ", "
)

// Question is one advisory question: the dimension or signal it belongs to, its
// text and the requirements it concerns.
type Question struct {
	Dimension      string
	Text           string
	RequirementIDs []string
}

// Missing asks for the behaviour of a checklist dimension the spec is silent about.
func Missing(d dimension.Dimension, lowConfidence bool) Question {
	text := fmt.Sprintf(missingTemplate, d.Phrase(), d)
	if lowConfidence {
		text = LowConfidenceMarker + text
	}
	return Question{Dimension: string(d), Text: text}
}

// UnjustifiedChange asks which requirement states a change no requirement and no
// example covers.
func UnjustifiedChange(file, function string) Question {
	return Question{Dimension: DimensionUnjustifiedChange, Text: fmt.Sprintf(unjustifiedTemplate, function, file)}
}

// UnconstrainedBehaviour asks which requirement should constrain behaviour that
// mutants of changed code can alter without violating any: the file, the changed
// functions in it, the requirement whose tests execute the file and the digests of
// the mutants.
func UnconstrainedBehaviour(file string, functions []string, requirementID string, mutants []string) Question {
	sortedFunctions := slices.Sorted(slices.Values(functions))
	sortedMutants := slices.Sorted(slices.Values(mutants))
	shown := make([]string, 0, MutantsShown)
	for _, digest := range sortedMutants[:min(MutantsShown, len(sortedMutants))] {
		shown = append(shown, digest[:min(DigestPrefix, len(digest))])
	}
	text := fmt.Sprintf(unconstrainedTemplate, file, strings.Join(sortedFunctions, listSeparator), len(sortedMutants), strings.Join(shown, listSeparator))
	return Question{Dimension: DimensionUnconstrainedBehaviour, Text: text, RequirementIDs: []string{requirementID}}
}

// Unclear asks for a clearer statement of a requirement System One finds
// ambiguous or untestable.
func Unclear(requirementID string, standing prompt.Testability) Question {
	template := untestableTemplate
	if standing == prompt.Ambiguous {
		template = ambiguousTemplate
	}
	return Question{Dimension: DimensionClarity, Text: fmt.Sprintf(template, requirementID), RequirementIDs: []string{requirementID}}
}

// Normalize returns the questions in their normal form: identical questions merged
// with the union of their requirements, requirements sorted and without repeats,
// questions sorted by dimension, then text. The result is the same for any order
// of the input.
func Normalize(questions []Question) []Question {
	merged := map[[2]string]*Question{}
	for _, q := range questions {
		key := [2]string{q.Dimension, q.Text}
		if existing, found := merged[key]; found {
			existing.RequirementIDs = append(existing.RequirementIDs, q.RequirementIDs...)
			continue
		}
		copied := q
		copied.RequirementIDs = slices.Clone(q.RequirementIDs)
		merged[key] = &copied
	}
	result := make([]Question, 0, len(merged))
	for _, q := range merged {
		slices.Sort(q.RequirementIDs)
		q.RequirementIDs = slices.Compact(q.RequirementIDs)
		if len(q.RequirementIDs) == 0 {
			q.RequirementIDs = nil
		}
		result = append(result, *q)
	}
	slices.SortFunc(result, func(a, b Question) int {
		return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Text, b.Text))
	})
	return result
}
