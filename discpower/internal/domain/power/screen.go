package power

import "github.com/RachidChabane/musthold/format"

// ClosedChoice is the answer to one question of the screen.
type ClosedChoice string

// Choices of the screen's closed questions, one question per in-scope criterion.
const (
	ChoiceViolates   ClosedChoice = "VIOLATES"
	ChoiceConsistent ClosedChoice = "CONSISTENT"
)

// ClosedAnswer is one answer to the screen question of a criterion. A host vote
// names a choice; a System One answer also gives the probability it assigns to
// each option. Its `confidence` is how concentrated those probabilities are and
// has no place here.
type ClosedAnswer struct {
	Choice ClosedChoice
	// Violates is the probability the answerer gave to VIOLATES, when it gave one.
	Violates    float64
	HasViolates bool
}

// violation is the probability one answer gives to VIOLATES: the probability the
// answerer stated, or, for a vote that states none, one for a VIOLATES vote and
// zero for any other.
func (a ClosedAnswer) violation() float64 {
	switch {
	case a.HasViolates:
		return a.Violates
	case a.Choice == ChoiceViolates:
		return 1
	}
	return 0
}

// ViolationProbability is P(VIOLATES): the mean of the answers' probabilities,
// which for host votes is the share of VIOLATES votes. No answer gives 0.
func ViolationProbability(answers []ClosedAnswer) float64 {
	if len(answers) == 0 {
		return 0
	}
	total := 0.0
	for _, answer := range answers {
		total += answer.violation()
	}
	return total / float64(len(answers))
}

// FlagsSurvivor reports whether the probability flags the survivor into the
// criterion's pool: at or above `tau_screen`.
func FlagsSurvivor(probability, tauScreen float64) bool { return probability >= tauScreen }

// ScreenAnswer is a ClosedAnswer with the provenance of the response it came
// from.
type ScreenAnswer struct {
	ClosedAnswer
	Isolation format.Isolation
	// Marked is set for a response journaled as leaked, exposed or conflicting.
	Marked bool
	// NoExactThreshold is set for a classification whose exact source has no
	// threshold of its own: it may flag a survivor and never screen one out.
	NoExactThreshold bool
}

// AtFloor reports whether the answer keeps exclusion power under the isolation
// floor.
func (a ScreenAnswer) AtFloor(floor format.Isolation) bool {
	return !a.Marked && !a.NoExactThreshold && a.Isolation.AtLeast(floor)
}

// ScreenVerdict is what the screen decided for one survivor.
type ScreenVerdict struct {
	Violation float64
	Flagged   bool
	// FloorEntry is set when the survivor is flagged only because the floor kept
	// answers below it or marked from screening it out: counted without the floor,
	// the same answers would have let it go.
	FloorEntry bool
}

// Screen decides one survivor. Only answers at the floor and unmarked can screen
// it out; any answer keeps its power to flag. A survivor with no such answer, or
// whose such answers reach `tau_screen`, enters the pool.
func Screen(answers []ScreenAnswer, floor format.Isolation, tauScreen float64) ScreenVerdict {
	var credited []ClosedAnswer
	all := make([]ClosedAnswer, len(answers))
	for index, answer := range answers {
		all[index] = answer.ClosedAnswer
		if answer.AtFloor(floor) {
			credited = append(credited, answer.ClosedAnswer)
		}
	}
	violation := max(ViolationProbability(all), ViolationProbability(credited))
	if len(credited) == 0 {
		violation = 1
	}
	flagged := len(credited) == 0 || FlagsSurvivor(violation, tauScreen)
	screenedOutWithoutFloor := !FlagsSurvivor(ViolationProbability(all), tauScreen)
	return ScreenVerdict{
		Violation: violation, Flagged: flagged,
		FloorEntry: flagged && len(credited) < len(answers) && screenedOutWithoutFloor,
	}
}
