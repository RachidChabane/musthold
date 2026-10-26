package hostprototest

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// choiceProbabilities gives the picked option the probability `picked` and shares
// the rest evenly among the other options.
func choiceProbabilities(question hostproto.ClosedQuestion, choice string, picked float64) map[string]float64 {
	others := len(question.Options) - 1
	probabilities := make(map[string]float64, len(question.Options))
	for _, option := range question.OptionNames() {
		probabilities[option] = (1 - picked) / float64(others)
	}
	probabilities[choice] = picked
	return probabilities
}

// SystemOneResponse renders the response a `/v1/systemone` endpoint gives to the
// set when it picks the given answers with the given confidence: a choice names its
// option and carries the confidence, with that probability on its option, a noul carries the probability that makes the
// answer's confidence, and a score carries the index of the picked level.
func SystemOneResponse(set hostproto.ClosedSet, answers map[string]hostproto.ClosedAnswer, confidence float64) ([]byte, error) {
	rendered := map[string]map[string]any{}
	for _, question := range set.Questions {
		answer, answered := answers[question.ID]
		if !answered {
			return nil, fmt.Errorf("question %q has no answer", question.ID)
		}
		entry := map[string]any{"type": question.Type}
		switch question.Type {
		case hostproto.ClosedNoul:
			entry["noul"] = confidence
			if answer.Choice == hostproto.NoulFalse {
				entry["noul"] = 1 - confidence
			}
		case hostproto.ClosedScore:
			entry["score"] = slices.Index(question.OptionNames(), answer.Choice)
			entry["confidence"] = confidence
		default:
			entry["choice"] = answer.Choice
			entry["confidence"] = confidence
			entry["probabilities"] = choiceProbabilities(question, answer.Choice, confidence)
		}
		rendered[question.ID] = entry
	}
	return json.Marshal(map[string]any{"model": "fixture-model", "answers": rendered})
}
