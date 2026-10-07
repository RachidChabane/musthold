package hostproto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
)

// ClosedType is the type of one closed question: the three types of the
// `/v1/systemone` endpoint.
type ClosedType string

// Closed question types.
const (
	// ClosedChoice picks one named option.
	ClosedChoice ClosedType = "choice"
	// ClosedNoul answers yes or no: the options are `true` and `false`.
	ClosedNoul ClosedType = "noul"
	// ClosedScore picks one level of an ordered scale.
	ClosedScore ClosedType = "score"
)

// Limits of a closed question set, those of the `/v1/systemone` endpoint.
const (
	// MaxClosedQuestions is the most questions one request carries.
	MaxClosedQuestions = 64
	// MinClosedOptions is the fewest options of a choice question or levels of a score.
	MinClosedOptions = 2
	// MaxChoiceOptions is the most options of a choice question.
	MaxChoiceOptions = 26
	// NoulTrue and NoulFalse name the two options of a noul question.
	NoulTrue  = "true"
	NoulFalse = "false"
	// halfProbability is where a noul answer turns from false to true.
	halfProbability = 0.5
)

var closedTypes = []ClosedType{ClosedChoice, ClosedNoul, ClosedScore}

// ClosedTypes lists the closed question types.
func ClosedTypes() []ClosedType { return slices.Clone(closedTypes) }

// ConfidenceMapping is the id of the mapping that turns what an answerer gave
// into the confidence a Statement records.
func (t ClosedType) ConfidenceMapping() string {
	switch t {
	case ClosedNoul:
		return format.MappingSystemOneNoul
	case ClosedScore:
		return format.MappingSystemOneScore
	}
	return format.MappingSystemOneChoice
}

// ClosedOption is one answer of a closed question with the criteria text that
// tells a model what it means.
type ClosedOption struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ClosedQuestion is one named question of a closed request.
type ClosedQuestion struct {
	ID           string         `json:"id"`
	Type         ClosedType     `json:"type"`
	Instructions string         `json:"instructions"`
	Options      []ClosedOption `json:"options,omitempty"`
}

// ClosedSet is the question set of a `decide.closed` request: the state the
// questions are about and the ordered questions.
type ClosedSet struct {
	State     string           `json:"state"`
	Questions []ClosedQuestion `json:"questions"`
}

// ChoiceQuestion builds a question that picks one of the options.
func ChoiceQuestion(id, instructions string, options []ClosedOption) ClosedQuestion {
	return ClosedQuestion{ID: id, Type: ClosedChoice, Instructions: instructions, Options: options}
}

// ScoreQuestion builds a question that picks one level of an ordered scale,
// lowest first.
func ScoreQuestion(id, instructions string, levels []ClosedOption) ClosedQuestion {
	return ClosedQuestion{ID: id, Type: ClosedScore, Instructions: instructions, Options: levels}
}

// NoulQuestion builds a yes or no question; the meanings are the criteria text of
// the two answers.
func NoulQuestion(id, instructions, whenTrue, whenFalse string) ClosedQuestion {
	return ClosedQuestion{ID: id, Type: ClosedNoul, Instructions: instructions, Options: []ClosedOption{
		{Name: NoulTrue, Description: whenTrue}, {Name: NoulFalse, Description: whenFalse},
	}}
}

// OptionNames lists the names of the options in order.
func (q ClosedQuestion) OptionNames() []string {
	names := make([]string, len(q.Options))
	for index, option := range q.Options {
		names[index] = option.Name
	}
	return names
}

// Names of the closed members of a document.
const (
	memberAnswers = "answers"
	memberID      = "id"
	memberChoice  = "choice"
	memberConf    = "confidence"
)

// Marshal encodes the set as the `closed` member of a request.
func (s ClosedSet) Marshal() (json.RawMessage, error) { return json.Marshal(s) }

// ParseClosedSet reads the `closed` member of a request and checks it.
func ParseClosedSet(raw json.RawMessage) (ClosedSet, error) {
	var set ClosedSet
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&set); err != nil {
		return ClosedSet{}, fmt.Errorf("read the closed question set: %w", err)
	}
	if problems := set.Problems(); len(problems) > 0 {
		return ClosedSet{}, &InvalidError{Problems: problems}
	}
	return set, nil
}

// closedProblems lists the rules the `closed` member of a request breaks.
func closedProblems(raw json.RawMessage) []Problem {
	_, err := ParseClosedSet(raw)
	var invalid *InvalidError
	switch {
	case err == nil:
		return nil
	case errors.As(err, &invalid):
		return invalid.Problems
	}
	return []Problem{{Path: "closed", Message: err.Error()}}
}

// Problems lists every rule the set breaks.
func (s ClosedSet) Problems() []Problem {
	var sink problemSink
	if s.State == "" {
		sink.add("closed.state", "must not be empty")
	}
	if len(s.Questions) < 1 || len(s.Questions) > MaxClosedQuestions {
		sink.add("closed.questions", "%d questions, want 1 to %d", len(s.Questions), MaxClosedQuestions)
	}
	seen := map[string]bool{}
	for index, question := range s.Questions {
		path := fmt.Sprintf("closed.questions[%d]", index)
		if question.ID == "" || seen[question.ID] {
			sink.add(path+".id", "%q is empty or used twice", question.ID)
		}
		seen[question.ID] = true
		question.check(&sink, path)
	}
	return sink.problems
}

func (q ClosedQuestion) check(sink *problemSink, path string) {
	if q.Instructions == "" {
		sink.add(path+".instructions", "must not be empty")
	}
	switch q.Type {
	case ClosedChoice:
		q.checkOptions(sink, path, MinClosedOptions, MaxChoiceOptions)
	case ClosedScore:
		q.checkOptions(sink, path, MinClosedOptions, math.MaxInt)
	case ClosedNoul:
		q.checkNoul(sink, path)
	default:
		sink.add(path+".type", "%q is not one of %v", q.Type, closedTypes)
	}
}

func (q ClosedQuestion) checkOptions(sink *problemSink, path string, least, most int) {
	if len(q.Options) < least || len(q.Options) > most {
		sink.add(path+".options", "%d options of a %s question, want at least %d", len(q.Options), q.Type, least)
	}
	names := map[string]bool{}
	for index, option := range q.Options {
		if option.Name == "" || names[option.Name] {
			sink.add(fmt.Sprintf("%s.options[%d].name", path, index), "%q is empty or used twice", option.Name)
		}
		names[option.Name] = true
		if option.Description == "" {
			sink.add(fmt.Sprintf("%s.options[%d].description", path, index), "must not be empty")
		}
	}
}

func (q ClosedQuestion) checkNoul(sink *problemSink, path string) {
	if len(q.Options) == 0 {
		return
	}
	if !slices.Equal(q.OptionNames(), []string{NoulTrue, NoulFalse}) {
		sink.add(path+".options", "a noul question has the options %q and %q, in that order", NoulTrue, NoulFalse)
	}
	for index, option := range q.Options {
		if option.Description == "" {
			sink.add(fmt.Sprintf("%s.options[%d].description", path, index), "must not be empty")
		}
	}
}

// choiceNames are the answers a question accepts in a response.
func (q ClosedQuestion) choiceNames() []string {
	if q.Type == ClosedNoul {
		return []string{NoulTrue, NoulFalse}
	}
	return q.OptionNames()
}

// ResponseSchema builds the schema the answers of the set must satisfy: one
// answer per question, by id, whose choice is one of the question's options.
func (s ClosedSet) ResponseSchema() (json.RawMessage, error) {
	forms := make([]map[string]any, len(s.Questions))
	required := make([]map[string]any, len(s.Questions))
	for index, question := range s.Questions {
		forms[index] = map[string]any{
			"type": "object", "additionalProperties": false, "required": []string{memberID, memberChoice},
			"properties": map[string]any{
				memberID:     map[string]any{"const": question.ID},
				memberChoice: map[string]any{"type": "string", "enum": question.choiceNames()},
				memberConf:   map[string]any{"type": "number", "minimum": format.MinConfidence, "maximum": format.MaxConfidence},
			},
		}
		required[index] = map[string]any{
			"contains":    map[string]any{"type": "object", "required": []string{memberID}, "properties": map[string]any{memberID: map[string]any{"const": question.ID}}},
			"minContains": 1, "maxContains": 1,
		}
	}
	return json.Marshal(map[string]any{
		"$schema": "https://json-schema.org/draft/2020-12/schema",
		"type":    "object", "additionalProperties": false, "required": []string{memberAnswers},
		"properties": map[string]any{memberAnswers: map[string]any{
			"type": "array", "minItems": len(s.Questions), "maxItems": len(s.Questions),
			"items": map[string]any{"oneOf": forms},
			"allOf": required,
		}},
	})
}

// ClosedAnswer is the answer to one closed question.
type ClosedAnswer struct {
	Choice        string
	Confidence    float64
	HasConfidence bool
}

// ParseClosedAnswers reads a response that satisfies the response schema of a set
// into the answers by question id.
func ParseClosedAnswers(output []byte) (map[string]ClosedAnswer, error) {
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
	answers := make(map[string]ClosedAnswer, len(document.Answers))
	for _, answer := range document.Answers {
		closed := ClosedAnswer{Choice: answer.Choice}
		if answer.Confidence != nil {
			closed.Confidence, closed.HasConfidence = *answer.Confidence, true
		}
		answers[answer.ID] = closed
	}
	return answers, nil
}

// AnswerDocument encodes answers, in the order of the set's questions, as the
// output document of a response.
func (s ClosedSet) AnswerDocument(answers map[string]ClosedAnswer) ([]byte, error) {
	list := make([]map[string]any, 0, len(s.Questions))
	for _, question := range s.Questions {
		answer, answered := answers[question.ID]
		if !answered {
			return nil, fmt.Errorf("question %q has no answer", question.ID)
		}
		entry := map[string]any{memberID: question.ID, memberChoice: answer.Choice}
		if answer.HasConfidence {
			entry[memberConf] = answer.Confidence
		}
		list = append(list, entry)
	}
	return json.Marshal(map[string]any{memberAnswers: list})
}

// Members of a `/v1/systemone` request and response.
const (
	systemOneModel        = "model"
	systemOneState        = "state"
	systemOneQuestions    = "questions"
	systemOneType         = "type"
	systemOneInstructions = "instructions"
	systemOneCriteria     = "criteria"
	systemOneChoice       = "choice"
	systemOneNoul         = "noul"
	systemOneScore        = "score"
	systemOneConfidence   = "confidence"
)

// SystemOneRequest renders the set as the body of a `/v1/systemone` call to the
// model: the questions are a named map, in the order of the set, whose choice
// criteria map each option to its description, whose score criteria list the
// levels from the lowest and whose noul criteria give the two meanings.
func (s ClosedSet) SystemOneRequest(model string) ([]byte, error) {
	var body bytes.Buffer
	body.WriteByte('{')
	if model != "" {
		if err := writeMember(&body, systemOneModel, model); err != nil {
			return nil, err
		}
		body.WriteByte(',')
	}
	if err := writeMember(&body, systemOneState, s.State); err != nil {
		return nil, err
	}
	body.WriteByte(',')
	body.WriteString(`"` + systemOneQuestions + `":{`)
	for index, question := range s.Questions {
		if index > 0 {
			body.WriteByte(',')
		}
		if err := writeSystemOneQuestion(&body, question); err != nil {
			return nil, err
		}
	}
	body.WriteString("}}")
	return body.Bytes(), nil
}

func writeSystemOneQuestion(body *bytes.Buffer, question ClosedQuestion) error {
	name, err := json.Marshal(question.ID)
	if err != nil {
		return err
	}
	body.Write(name)
	body.WriteString(":{")
	if err := writeMember(body, systemOneType, question.Type); err != nil {
		return err
	}
	body.WriteByte(',')
	if err := writeMember(body, systemOneInstructions, question.Instructions); err != nil {
		return err
	}
	criteria, err := systemOneCriteriaOf(question)
	if err != nil {
		return err
	}
	if criteria != nil {
		body.WriteString(`,"` + systemOneCriteria + `":`)
		body.Write(criteria)
	}
	body.WriteByte('}')
	return nil
}

func writeMember(body *bytes.Buffer, name string, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return err
	}
	body.WriteString(`"` + name + `":`)
	body.Write(encoded)
	return nil
}

func systemOneCriteriaOf(question ClosedQuestion) ([]byte, error) {
	switch {
	case question.Type == ClosedScore:
		levels := make([]string, len(question.Options))
		for index, option := range question.Options {
			levels[index] = option.Description
		}
		return json.Marshal(levels)
	case len(question.Options) == 0:
		return nil, nil
	}
	var criteria bytes.Buffer
	criteria.WriteByte('{')
	for index, option := range question.Options {
		if index > 0 {
			criteria.WriteByte(',')
		}
		if err := writeMember(&criteria, option.Name, option.Description); err != nil {
			return nil, err
		}
	}
	criteria.WriteByte('}')
	return criteria.Bytes(), nil
}

// ErrSystemOneResponse is returned for a `/v1/systemone` response that does not
// answer every question of the set.
var ErrSystemOneResponse = errors.New("the system one response does not answer the question set")

// AnswersFromSystemOne turns the response of a `/v1/systemone` call into the
// output document of a response to the set. A choice answer keeps its option and
// its `confidence`; a noul answer is true from probability one half and carries
// `max(p, 1-p)`; a score answer is the level nearest to the score and carries
// the `confidence` of the endpoint.
func (s ClosedSet) AnswersFromSystemOne(response []byte) ([]byte, error) {
	var document struct {
		Answers map[string]struct {
			Choice     *string  `json:"choice"`
			Noul       *float64 `json:"noul"`
			Score      *float64 `json:"score"`
			Confidence *float64 `json:"confidence"`
		} `json:"answers"`
	}
	if err := json.Unmarshal(response, &document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSystemOneResponse, err)
	}
	answers := make(map[string]ClosedAnswer, len(s.Questions))
	for _, question := range s.Questions {
		given, answered := document.Answers[question.ID]
		if !answered {
			return nil, fmt.Errorf("%w: no answer to %q", ErrSystemOneResponse, question.ID)
		}
		var answer ClosedAnswer
		switch {
		case question.Type == ClosedChoice && given.Choice != nil:
			answer = ClosedAnswer{Choice: *given.Choice}
			answer.Confidence, answer.HasConfidence = optional(given.Confidence)
		case question.Type == ClosedNoul && given.Noul != nil:
			answer = noulAnswer(*given.Noul)
		case question.Type == ClosedScore && given.Score != nil:
			answer = ClosedAnswer{Choice: levelNear(question, *given.Score)}
			answer.Confidence, answer.HasConfidence = optional(given.Confidence)
		default:
			return nil, fmt.Errorf("%w: the answer to %q is not a %s answer", ErrSystemOneResponse, question.ID, question.Type)
		}
		if !slices.Contains(question.choiceNames(), answer.Choice) {
			return nil, fmt.Errorf("%w: %q is not an option of %q", ErrSystemOneResponse, answer.Choice, question.ID)
		}
		answers[question.ID] = answer
	}
	return s.AnswerDocument(answers)
}

func optional(value *float64) (float64, bool) {
	if value == nil {
		return 0, false
	}
	return *value, true
}

func noulAnswer(probability float64) ClosedAnswer {
	if probability >= halfProbability {
		return ClosedAnswer{Choice: NoulTrue, Confidence: probability, HasConfidence: true}
	}
	return ClosedAnswer{Choice: NoulFalse, Confidence: 1 - probability, HasConfidence: true}
}

// levelNear is the name of the level of a score question nearest to the score.
func levelNear(question ClosedQuestion, score float64) string {
	index := int(math.Round(score))
	index = max(0, min(len(question.Options)-1, index))
	return question.Options[index].Name
}

// ErrNoClosedQuestions is returned for a ticket whose view carries no question set.
var ErrNoClosedQuestions = errors.New("the ticket asks no closed questions")

// AnswersFromSystemOneFor turns the response of a `/v1/systemone` call into the
// output of the response to the ticket the view belongs to: the endpoint answered
// the question set the view shows, in its own terms.
func AnswersFromSystemOneFor(view ExecutorView, endpointResponse []byte) ([]byte, error) {
	set, err := ParseClosedSet(view.Closed)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrNoClosedQuestions, err)
	}
	return set.AnswersFromSystemOne(endpointResponse)
}

// ClosedExport is one closed request as `systemone export` hands it to a relay: the
// ticket and nonce to import the answers with, and the set the relay renders
// with SystemOneRequest.
type ClosedExport struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Closed json.RawMessage `json:"closed"`
}

// ExportClosed lists the closed requests of a listing that no vote answered yet
// and that a System One endpoint may answer: the first vote of each.
func ExportClosed(listing Listing) []ClosedExport {
	exported := []ClosedExport{}
	for _, item := range listing.Items {
		if len(item.View.Closed) > 0 && item.Ticket.Vote == 0 {
			exported = append(exported, ClosedExport{Ticket: item.Ticket.TicketID.String(), Nonce: item.Ticket.Nonce, Closed: item.View.Closed})
		}
	}
	return exported
}
