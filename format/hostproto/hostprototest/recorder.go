package hostprototest

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	// closedConfidence is the confidence a first-option answer states.
	closedConfidence = "0.95"
	// answersOpen and answersClose frame the document of closed answers.
	answersOpen  = `{"answers": [`
	answersClose = `]}`
	// answerSeparator joins the answers of the document.
	answerSeparator = ", "
)

// ErrConflictingRecording is returned when one request is answered with two
// different outputs: a recording holds one answer per request.
var ErrConflictingRecording = errors.New("the request was answered twice with different outputs")

// ErrNoClosedQuestions is returned by the first-option source for a request that
// carries no closed question set.
var ErrNoClosedQuestions = errors.New("the request carries no closed questions")

// ErrNoOptions is returned for a closed question that offers no option.
var ErrNoOptions = errors.New("a closed question offers no option")

// FirstOption answers every closed question with its first option at a fixed
// confidence.
type FirstOption struct{}

// Answer implements AnswerSource.
func (FirstOption) Answer(_ string, view hostproto.ExecutorView) (string, error) {
	if len(view.Closed) == 0 {
		return "", ErrNoClosedQuestions
	}
	set, err := hostproto.ParseClosedSet(view.Closed)
	if err != nil {
		return "", err
	}
	answers := make([]string, 0, len(set.Questions))
	for _, question := range set.Questions {
		names := question.OptionNames()
		if len(names) == 0 {
			return "", fmt.Errorf("%w: %s", ErrNoOptions, question.ID)
		}
		answers = append(answers, closedAnswer(question.ID, names[0]))
	}
	return answersOpen + strings.Join(answers, answerSeparator) + answersClose, nil
}

// closedAnswer renders one answer with its members in alphabetical order.
func closedAnswer(id, choice string) string {
	return fmt.Sprintf(`{"choice": %s, "confidence": %s, "id": %s}`, quoted(choice), closedConfidence, quoted(id))
}

func quoted(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

// Recorder wraps an answer source and keeps every answer it gave, keyed by the
// request the ticket belongs to.
type Recorder struct {
	Source  AnswerSource
	Answers map[hostproto.RequestID]string
}

// NewRecorder records the answers of a source.
func NewRecorder(source AnswerSource) *Recorder {
	return &Recorder{Source: source, Answers: map[hostproto.RequestID]string{}}
}

// Answer implements AnswerSource.
func (r *Recorder) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	id, err := hostproto.ParseTicketID(view.Ticket)
	if err != nil {
		return "", err
	}
	answer, err := r.Source.Answer(persona, view)
	if err != nil {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if previous, found := r.Answers[id.RequestID]; found && previous != answer {
		return "", fmt.Errorf("%w: %s", ErrConflictingRecording, id.RequestID)
	}
	r.Answers[id.RequestID] = answer
	return answer, nil
}
