package power

// ClosedChoice is the answer to one question of the screen.
type ClosedChoice string

// Choices of the screen's closed questions, one question per in-scope criterion.
const (
	ChoiceViolates   ClosedChoice = "VIOLATES"
	ChoiceConsistent ClosedChoice = "CONSISTENT"
)

// ClosedAnswer is one answer to the screen question of a criterion: a host vote
// carries no confidence, a System One answer may.
type ClosedAnswer struct {
	Choice        ClosedChoice
	Confidence    float64
	HasConfidence bool
}

// violation is the probability one answer gives to VIOLATES.
func (a ClosedAnswer) violation() float64 {
	weight := 1.0
	if a.HasConfidence {
		weight = a.Confidence
	}
	if a.Choice == ChoiceViolates {
		return weight
	}
	return 1 - weight
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
