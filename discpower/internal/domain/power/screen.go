package power

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
