package hostprototest

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// SystemOneResponse renders the response a `/v1/systemone` endpoint gives to the
// set when it picks the given answers with the given confidence: a choice names its
// option and carries the confidence, a noul carries the probability that makes the
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
		}
		rendered[question.ID] = entry
	}
	return json.Marshal(map[string]any{"model": "fixture-model", "answers": rendered})
}
