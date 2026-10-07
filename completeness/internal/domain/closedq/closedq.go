// Package closedq is the one place that knows the shape of a closed request, the
// `decide.closed` kind of the host protocol: the question set a System One
// endpoint or a host decider is shown, the schema its answer must satisfy, how
// the answer is parsed and how the votes of a request are read into one answer
// per question with the source of its confidence.
//
// The shape mirrors the closed-question protocol of `/v1/systemone`. It lives in
// this package alone, so that moving it to the shared format library touches no
// other file.
package closedq

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// TypeChoice is the question type of a choice among named options.
const TypeChoice = "choice"

// MaxQuestions is the largest number of questions one request holds.
const MaxQuestions = 64

// Option is one answer of a closed question with the text that tells a model
// what it means.
type Option struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Question is one closed question.
type Question struct {
	ID       string   `json:"id"`
	Type     string   `json:"type"`
	Question string   `json:"question"`
	Options  []Option `json:"options"`
}

// Choice builds a choice question.
func Choice(id, text string, options []Option) Question {
	return Question{ID: id, Type: TypeChoice, Question: text, Options: options}
}

// OptionNames lists the names of the options of a question, in order.
func (q Question) OptionNames() []string {
	names := make([]string, len(q.Options))
	for index, option := range q.Options {
		names[index] = option.Name
	}
	return names
}

// Set is the question set of a closed request: the state the questions are
// about and the questions.
type Set struct {
	State     string     `json:"state"`
	Questions []Question `json:"questions"`
}

// Marshal encodes the question set as the request carries it.
func (s Set) Marshal() (json.RawMessage, error) { return json.Marshal(s) }

type member map[string]any

// Schema is the JSON Schema 2020-12 of the answer: an array holding exactly one
// answer for each question, each with one of the question's options and an
// optional confidence in [0,1].
func (s Set) Schema() (json.RawMessage, error) {
	forms := make([]member, len(s.Questions))
	required := make([]member, len(s.Questions))
	for index, question := range s.Questions {
		forms[index] = member{
			"type": "object", "additionalProperties": false, "required": []string{"id", "choice"},
			"properties": member{
				"id":         member{"const": question.ID},
				"choice":     member{"type": "string", "enum": question.OptionNames()},
				"confidence": member{"type": "number", "minimum": 0, "maximum": 1},
			},
		}
		required[index] = member{
			"contains":    member{"type": "object", "required": []string{"id"}, "properties": member{"id": member{"const": question.ID}}},
			"minContains": 1, "maxContains": 1,
		}
	}
	return json.Marshal(member{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object", "additionalProperties": false, "required": []string{"answers"},
		"properties": member{"answers": member{
			"type": "array", "minItems": len(s.Questions), "maxItems": len(s.Questions),
			"items": member{"oneOf": forms},
			"allOf": required,
		}},
	})
}

// Answer is the answer to one closed question.
type Answer struct {
	Choice        string
	Confidence    float64
	HasConfidence bool
}

// Parse reads an output that satisfies the schema into the answers by question id.
func Parse(output []byte) (map[string]Answer, error) {
	var document struct {
		Answers []struct {
			ID         string   `json:"id"`
			Choice     string   `json:"choice"`
			Confidence *float64 `json:"confidence"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(output, &document); err != nil {
		return nil, fmt.Errorf("read the closed response: %w", err)
	}
	answers := make(map[string]Answer, len(document.Answers))
	for _, answer := range document.Answers {
		closed := Answer{Choice: answer.Choice}
		if answer.Confidence != nil {
			closed.Confidence, closed.HasConfidence = *answer.Confidence, true
		}
		answers[answer.ID] = closed
	}
	return answers, nil
}

// Vote is one answer to a closed request as the module reads it.
type Vote struct {
	Output []byte
	// Relayed is true for the answer of a System One endpoint; it stands for the
	// votes of the host.
	Relayed bool
	// Source names the model and endpoint of a relayed answer.
	Source string
}

// Decided is what the votes of a request settled for one question.
type Decided struct {
	Choice string
	// Confidence is the confidence the endpoint stated, none counting as zero, or
	// the share of the votes that gave the choice.
	Confidence float64
}

// Reading is what the votes of one request say.
type Reading struct {
	// Questions maps a question id to the decided answer; a question nobody
	// answered is absent.
	Questions map[string]Decided
	// Source says where the confidence came from: the model of the relay, or
	// `host-votes(k)` for k host votes.
	Source string
}

// HostVotesSource names the confidence source of k host votes.
func HostVotesSource(votes int) string { return fmt.Sprintf("host-votes(%d)", votes) }

// Read turns the votes of a closed request into one answer per question. A
// relayed first vote replaces the host votes and carries the confidence the
// endpoint gave. Host votes give the modal choice of each question, ties to the
// smaller choice, with its share of all votes as confidence; a vote that does not
// parse counts for no choice.
func Read(votes []Vote) Reading {
	if len(votes) == 0 {
		return Reading{Questions: map[string]Decided{}}
	}
	if votes[0].Relayed {
		return readRelayed(votes[0])
	}
	counts := map[string]map[string]int{}
	for _, vote := range votes {
		parsed, err := Parse(vote.Output)
		if err != nil {
			continue
		}
		for id, answer := range parsed {
			if counts[id] == nil {
				counts[id] = map[string]int{}
			}
			counts[id][answer.Choice]++
		}
	}
	reading := Reading{Questions: map[string]Decided{}, Source: HostVotesSource(len(votes))}
	for id, choices := range counts {
		choice, count := modalOf(choices)
		reading.Questions[id] = Decided{Choice: choice, Confidence: float64(count) / float64(len(votes))}
	}
	return reading
}

func readRelayed(vote Vote) Reading {
	reading := Reading{Questions: map[string]Decided{}, Source: vote.Source}
	parsed, err := Parse(vote.Output)
	if err != nil {
		return reading
	}
	for id, answer := range parsed {
		reading.Questions[id] = Decided{Choice: answer.Choice, Confidence: statedConfidence(answer)}
	}
	return reading
}

func statedConfidence(answer Answer) float64 {
	if answer.HasConfidence {
		return answer.Confidence
	}
	return 0
}

// modalOf returns the commonest choice and its count, ties going to the
// lexicographically smaller choice.
func modalOf(counts map[string]int) (string, int) {
	choices := make([]string, 0, len(counts))
	for choice := range counts {
		choices = append(choices, choice)
	}
	slices.SortFunc(choices, func(a, b string) int { return cmp.Or(cmp.Compare(counts[b], counts[a]), strings.Compare(a, b)) })
	return choices[0], counts[choices[0]]
}
