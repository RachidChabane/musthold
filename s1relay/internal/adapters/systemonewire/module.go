// Package systemonewire is the one place that knows the wire forms of the relay:
// the closed request a proof module exports and the answer it imports, and the
// request and reply of a System One endpoint. Everything else of the relay works
// on the question types of the domain.
package systemonewire

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
)

// ModuleRequest is one entry of the output of `systemone export`.
type ModuleRequest struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Closed json.RawMessage `json:"closed"`
}

// DecodeExport reads the output of `systemone export`.
func DecodeExport(output []byte) ([]ModuleRequest, error) {
	var requests []ModuleRequest
	if err := json.Unmarshal(output, &requests); err != nil {
		return nil, fmt.Errorf("read the export: %w", err)
	}
	return requests, nil
}

type moduleSet struct {
	State     string           `json:"state"`
	Questions []moduleQuestion `json:"questions"`
}

type moduleQuestion struct {
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Question string            `json:"question"`
	Options  []json.RawMessage `json:"options"`
}

type moduleOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// DecodeClosed reads the question set of a closed request as the modules emit it.
// An option is a bare name or an object holding a name and a description; a
// question without a type is a choice.
func DecodeClosed(closed json.RawMessage) (question.Set, error) {
	var decoded moduleSet
	if err := json.Unmarshal(closed, &decoded); err != nil {
		return question.Set{}, fmt.Errorf("read the closed request: %w", err)
	}
	set := question.Set{State: decoded.State}
	for _, raw := range decoded.Questions {
		options, err := decodeOptions(raw.Options)
		if err != nil {
			return question.Set{}, fmt.Errorf("question %q: %w", raw.ID, err)
		}
		set.Questions = append(set.Questions, question.Question{
			ID: raw.ID, Type: typeOf(raw.Type), Text: raw.Question, Options: options,
		})
	}
	if err := set.Validate(); err != nil {
		return question.Set{}, err
	}
	return set, nil
}

func typeOf(declared string) question.Type {
	if declared == "" {
		return question.TypeChoice
	}
	return question.Type(declared)
}

func decodeOptions(raw []json.RawMessage) ([]question.Option, error) {
	options := make([]question.Option, len(raw))
	for index, entry := range raw {
		var name string
		if err := json.Unmarshal(entry, &name); err == nil {
			options[index] = question.Option{Name: name}
			continue
		}
		var object moduleOption
		if err := json.Unmarshal(entry, &object); err != nil {
			return nil, fmt.Errorf("option %d is neither a name nor an object: %w", index, err)
		}
		options[index] = question.Option{Name: object.Name, Description: object.Description}
	}
	return options, nil
}

type moduleAnswers struct {
	Answers []moduleAnswer `json:"answers"`
}

type moduleAnswer struct {
	ID         string   `json:"id"`
	Choice     string   `json:"choice"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// EncodeAnswers renders the answers as `systemone import` reads them from stdin.
func EncodeAnswers(answers question.Answers) ([]byte, error) {
	document := moduleAnswers{Answers: make([]moduleAnswer, len(answers))}
	for index, answer := range answers {
		document.Answers[index] = moduleAnswer{ID: answer.QuestionID, Choice: answer.Choice, Confidence: answer.Confidence}
	}
	var buffer bytes.Buffer
	if err := json.NewEncoder(&buffer).Encode(document); err != nil {
		return nil, fmt.Errorf("encode the answers: %w", err)
	}
	return buffer.Bytes(), nil
}
