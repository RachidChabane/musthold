package question_test

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
)

func choice(id string, names ...string) question.Question {
	options := make([]question.Option, len(names))
	for index, name := range names {
		options[index] = question.Option{Name: name}
	}
	return question.Question{ID: id, Type: question.TypeChoice, Options: options}
}

func confidence(value float64) *float64 { return &value }

func TestSetValidate(t *testing.T) {
	cases := []struct {
		name string
		set  question.Set
		want error
	}{
		{"a set with one choice is valid", question.Set{Questions: []question.Question{choice("a", "X", "Y")}}, nil},
		{"a set without question", question.Set{}, question.ErrNoQuestion},
		{"a question without id", question.Set{Questions: []question.Question{choice("", "X")}}, question.ErrEmptyQuestionID},
		{"two questions with one id", question.Set{Questions: []question.Question{choice("a", "X"), choice("a", "Y")}}, question.ErrDuplicateID},
		{"a question without option", question.Set{Questions: []question.Question{choice("a")}}, question.ErrNoOption},
		{"an option listed twice", question.Set{Questions: []question.Question{choice("a", "X", "X")}}, question.ErrDuplicateOption},
		{"a question of an unknown type", question.Set{Questions: []question.Question{{ID: "a", Type: "noul", Options: []question.Option{{Name: "X"}}}}}, question.ErrUnsupportedType},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-616~1]", func(t *testing.T) {
			if err := c.set.Validate(); !errors.Is(err, c.want) {
				t.Fatalf("Validate() = %v, want %v", err, c.want)
			}
		})
	}
}

func TestOptionMeaning(t *testing.T) {
	t.Run("an option without description is described by its name [verifies SP-604~1]", func(t *testing.T) {
		if got := (question.Option{Name: "blue"}).Meaning(); got != "blue" {
			t.Fatalf("Meaning() = %q", got)
		}
		if got := (question.Option{Name: "blue", Description: "the sky"}).Meaning(); got != "the sky" {
			t.Fatalf("Meaning() = %q", got)
		}
	})
}

func TestSetCheck(t *testing.T) {
	set := question.Set{Questions: []question.Question{choice("first", "A", "B"), choice("second", "X", "Y")}}
	cases := []struct {
		name    string
		answers question.Answers
		want    error
	}{
		{"one answer per question in order", question.Answers{{QuestionID: "first", Choice: "A"}, {QuestionID: "second", Choice: "Y", Confidence: confidence(0.5)}}, nil},
		{"a missing answer", question.Answers{{QuestionID: "first", Choice: "A"}}, question.ErrAnswerCountOrder},
		{"answers out of order", question.Answers{{QuestionID: "second", Choice: "X"}, {QuestionID: "first", Choice: "A"}}, question.ErrAnswerCountOrder},
		{"an empty choice", question.Answers{{QuestionID: "first"}, {QuestionID: "second", Choice: "X"}}, question.ErrMissingAnswer},
		{"a choice that is not an option", question.Answers{{QuestionID: "first", Choice: "C"}, {QuestionID: "second", Choice: "X"}}, question.ErrUnknownChoice},
		{"a confidence above one", question.Answers{{QuestionID: "first", Choice: "A", Confidence: confidence(1.5)}, {QuestionID: "second", Choice: "X"}}, question.ErrConfidenceRange},
		{"a negative confidence", question.Answers{{QuestionID: "first", Choice: "A", Confidence: confidence(-0.1)}, {QuestionID: "second", Choice: "X"}}, question.ErrConfidenceRange},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-606~1]", func(t *testing.T) {
			if err := set.Check(c.answers); !errors.Is(err, c.want) {
				t.Fatalf("Check() = %v, want %v", err, c.want)
			}
		})
	}
}
