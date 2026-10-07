// Package question holds the closed questions the relay carries between a proof
// module and a System One endpoint, and the rule that tells a usable answer
// from an unusable one. It knows no wire format.
package question

import (
	"errors"
	"fmt"
)

// Type is the kind of a closed question.
type Type string

// The question types the proof modules ask.
const (
	// TypeChoice asks for one option among named options.
	TypeChoice Type = "choice"
	// TypeScore asks for one level among ordered levels, weakest first.
	TypeScore Type = "score"
)

// Confidence bounds an endpoint may state.
const (
	MinConfidence = 0.0
	MaxConfidence = 1.0
)

// Option is one answer of a closed question with the text that tells a model what
// it means.
type Option struct {
	Name        string
	Description string
}

// Meaning is the text an endpoint is shown for the option: its description, or
// its name when it has none.
func (o Option) Meaning() string {
	if o.Description == "" {
		return o.Name
	}
	return o.Description
}

// Question is one closed question.
type Question struct {
	ID      string
	Type    Type
	Text    string
	Options []Option
}

// Set is the state the questions are about and the questions, in order.
type Set struct {
	State     string
	Questions []Question
}

// Answer is the answer to one question. A confidence the endpoint did not state
// stays nil.
type Answer struct {
	QuestionID string
	Choice     string
	Confidence *float64
}

// Answers holds one Answer per question of a Set, in the order of the Set.
type Answers []Answer

// Errors of a set or an answer that cannot be used.
var (
	ErrNoQuestion       = errors.New("the question set holds no question")
	ErrUnsupportedType  = errors.New("unsupported question type")
	ErrDuplicateID      = errors.New("two questions share an id")
	ErrNoOption         = errors.New("a question holds no option")
	ErrDuplicateOption  = errors.New("a question lists an option twice")
	ErrMissingAnswer    = errors.New("a question is not answered")
	ErrUnknownChoice    = errors.New("the answer is not one of the options")
	ErrConfidenceRange  = errors.New("the confidence is outside [0,1]")
	ErrAnswerCountOrder = errors.New("the answers do not follow the questions")
	ErrEmptyQuestionID  = errors.New("a question has no id")
)

// Validate reports why the set cannot be asked: no question, a question without an
// id or options, a repeated id or option, or a type no endpoint answers.
func (s Set) Validate() error {
	if len(s.Questions) == 0 {
		return ErrNoQuestion
	}
	seen := make(map[string]bool, len(s.Questions))
	for _, q := range s.Questions {
		if q.ID == "" {
			return ErrEmptyQuestionID
		}
		if seen[q.ID] {
			return fmt.Errorf("%w: %q", ErrDuplicateID, q.ID)
		}
		seen[q.ID] = true
		if err := q.validate(); err != nil {
			return fmt.Errorf("question %q: %w", q.ID, err)
		}
	}
	return nil
}

func (q Question) validate() error {
	if q.Type != TypeChoice && q.Type != TypeScore {
		return fmt.Errorf("%w: %q", ErrUnsupportedType, q.Type)
	}
	if len(q.Options) == 0 {
		return ErrNoOption
	}
	names := make(map[string]bool, len(q.Options))
	for _, option := range q.Options {
		if names[option.Name] {
			return fmt.Errorf("%w: %q", ErrDuplicateOption, option.Name)
		}
		names[option.Name] = true
	}
	return nil
}

// OptionIndex is the position of the named option, or -1.
func (q Question) OptionIndex(name string) int {
	for index, option := range q.Options {
		if option.Name == name {
			return index
		}
	}
	return -1
}

// Check reports why the answers cannot be handed to the module: not one answer per
// question in order, a choice outside the options of its question, or a confidence
// outside [0,1]. A partial answer is never usable.
func (s Set) Check(answers Answers) error {
	if len(answers) != len(s.Questions) {
		return fmt.Errorf("%w: %d answers for %d questions", ErrAnswerCountOrder, len(answers), len(s.Questions))
	}
	for index, q := range s.Questions {
		answer := answers[index]
		if answer.QuestionID != q.ID {
			return fmt.Errorf("%w: answer %d is for %q, the question is %q", ErrAnswerCountOrder, index, answer.QuestionID, q.ID)
		}
		if answer.Choice == "" {
			return fmt.Errorf("%w: %q", ErrMissingAnswer, q.ID)
		}
		if q.OptionIndex(answer.Choice) < 0 {
			return fmt.Errorf("%w: %q for %q", ErrUnknownChoice, answer.Choice, q.ID)
		}
		if c := answer.Confidence; c != nil && (*c < MinConfidence || *c > MaxConfidence || *c != *c) {
			return fmt.Errorf("%w: %v for %q", ErrConfidenceRange, *c, q.ID)
		}
	}
	return nil
}
