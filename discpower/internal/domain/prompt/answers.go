package prompt

import (
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// GeneratedMutant is one mutant a generation response proposes.
type GeneratedMutant struct {
	Slot      power.Slot
	Mechanism string
	Edits     mutation.EditSet
}

// ParseGeneration reads a response that satisfies the generation schema.
func ParseGeneration(output []byte) ([]GeneratedMutant, error) {
	var document struct {
		Mutants []struct {
			Slot      power.Slot      `json:"slot"`
			Mechanism string          `json:"mechanism"`
			Edits     []mutation.Edit `json:"edits"`
		} `json:"mutants"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("read the generation response: %w", err)
	}
	mutants := make([]GeneratedMutant, len(document.Mutants))
	for index, mutant := range document.Mutants {
		mutants[index] = GeneratedMutant{Slot: mutant.Slot, Mechanism: mutant.Mechanism, Edits: mutation.EditSet{Edits: mutant.Edits}}
	}
	return mutants, nil
}

// WitnessClaim is the witness a judge gives: a call and what the criterion
// demands it returns.
type WitnessClaim struct {
	Call   spec.Call
	Expect spec.Expectation
}

// Judgement is one judge's answer to a validity or an equivalence question.
type Judgement struct {
	Answer     power.Answer
	Confidence float64
	Witness    *WitnessClaim
}

// FullConfidence is the confidence of a vote that states none.
const FullConfidence = format.MaxConfidence

type wireWitness struct {
	Operation string                  `json:"operation"`
	Args      map[string]spec.Literal `json:"args"`
	Expected  spec.Observation        `json:"expected"`
}

// ParseValidity reads a response that satisfies the validity schema.
func ParseValidity(output []byte) (Judgement, error) {
	var document struct {
		Validity   power.Answer `json:"validity"`
		Confidence *float64     `json:"confidence"`
		Witness    *wireWitness `json:"witness"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return Judgement{}, fmt.Errorf("read the validity response: %w", err)
	}
	judgement := Judgement{Answer: document.Validity, Confidence: confidenceOf(document.Confidence)}
	if document.Witness != nil {
		judgement.Witness = &WitnessClaim{
			Call:   spec.Call{Operation: document.Witness.Operation, Args: document.Witness.Args},
			Expect: document.Witness.Expected,
		}
	}
	return judgement, nil
}

// Answers of an equivalence response besides EQUIVALENT.
const (
	equivalentAnswer = "EQUIVALENT"
)

// ParseEquivalence reads a response that satisfies the equivalence schema. Any
// answer but EQUIVALENT is undetermined: a judge's claim that the mutant differs
// settles nothing without an input.
func ParseEquivalence(output []byte) (Judgement, error) {
	var document struct {
		Equivalent string   `json:"equivalent"`
		Confidence *float64 `json:"confidence"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return Judgement{}, fmt.Errorf("read the equivalence response: %w", err)
	}
	answer := power.AnswerUndetermined
	if document.Equivalent == equivalentAnswer {
		answer = power.AnswerEquivalent
	}
	return Judgement{Answer: answer, Confidence: confidenceOf(document.Confidence)}, nil
}

func confidenceOf(stated *float64) float64 {
	if stated == nil {
		return FullConfidence
	}
	return *stated
}

// ParseScreen reads a response that satisfies the closed schema into the answers
// by question id.
func ParseScreen(output []byte) (map[string]power.ClosedAnswer, error) {
	parsed, err := hostproto.ParseClosedAnswers(output)
	if err != nil {
		return nil, err
	}
	answers := make(map[string]power.ClosedAnswer, len(parsed))
	for id, answer := range parsed {
		answers[id] = power.ClosedAnswer{Choice: power.ClosedChoice(answer.Choice), Confidence: answer.Confidence, HasConfidence: answer.HasConfidence}
	}
	return answers, nil
}
