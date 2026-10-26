package systemonewire

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/question"
)

// MaxQuestions is the most questions one `/v1/systemone` request holds.
const MaxQuestions = hostproto.MaxClosedQuestions

// Question types of the `/v1/systemone` route.
const (
	wireChoice = "choice"
	wireScore  = "score"
)

// ErrTooManyQuestions is returned for a set the route cannot take.
var ErrTooManyQuestions = errors.New("the question set is larger than the endpoint accepts")

// EncodeRequest renders a question set as the body of a `/v1/systemone` request,
// through the host protocol's definition of it. The model is left out when empty.
func EncodeRequest(model string, set question.Set) ([]byte, error) {
	if err := set.Validate(); err != nil {
		return nil, err
	}
	if len(set.Questions) > MaxQuestions {
		return nil, fmt.Errorf("%w: %d questions, at most %d", ErrTooManyQuestions, len(set.Questions), MaxQuestions)
	}
	return ClosedSetOf(set).SystemOneRequest(model)
}

type endpointReply struct {
	Model   string                     `json:"model"`
	Answers map[string]json.RawMessage `json:"answers"`
}

type endpointAnswer struct {
	Type       string   `json:"type"`
	Choice     *string  `json:"choice"`
	Score      *float64 `json:"score"`
	Confidence *float64 `json:"confidence"`
}

// DecodeReply reads the body of a `/v1/systemone` reply to a question set. It
// returns the answers in the order of the questions and the model the reply names.
// A choice names an option; a score is the probability-weighted level, which maps
// to the nearest level. A confidence the endpoint did not state stays absent, and
// fields the relay does not know are ignored.
func DecodeReply(body []byte, set question.Set) (question.Answers, string, error) {
	var reply endpointReply
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, "", fmt.Errorf("read the reply: %w", err)
	}
	answers := make(question.Answers, len(set.Questions))
	for index, q := range set.Questions {
		raw, found := reply.Answers[q.ID]
		if !found {
			return nil, "", fmt.Errorf("the reply does not answer %q", q.ID)
		}
		answer, err := decodeAnswer(raw, q)
		if err != nil {
			return nil, "", fmt.Errorf("answer to %q: %w", q.ID, err)
		}
		answers[index] = answer
	}
	return answers, reply.Model, nil
}

func decodeAnswer(raw json.RawMessage, q question.Question) (question.Answer, error) {
	var wire endpointAnswer
	if err := json.Unmarshal(raw, &wire); err != nil {
		return question.Answer{}, err
	}
	answer := question.Answer{QuestionID: q.ID, Confidence: wire.Confidence}
	switch q.Type {
	case question.TypeChoice:
		if wire.Type != wireChoice || wire.Choice == nil {
			return question.Answer{}, fmt.Errorf("a choice was asked and the reply is of type %q", wire.Type)
		}
		answer.Choice = *wire.Choice
	case question.TypeScore:
		if wire.Type != wireScore || wire.Score == nil {
			return question.Answer{}, fmt.Errorf("a score was asked and the reply is of type %q", wire.Type)
		}
		level := math.Round(*wire.Score)
		if math.IsNaN(level) || level < 0 || level >= float64(len(q.Options)) {
			return question.Answer{}, fmt.Errorf("the score %v is outside the %d levels", *wire.Score, len(q.Options))
		}
		answer.Choice = q.Options[int(level)].Name
	default:
		return question.Answer{}, fmt.Errorf("%w: %q", question.ErrUnsupportedType, q.Type)
	}
	return answer, nil
}
