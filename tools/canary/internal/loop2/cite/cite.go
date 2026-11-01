// Package cite is check A11 of the loop-2 study (ADR 0017): the assertions of
// an oracle, the spec sentence a model cites behind each, the answer of the
// citation session as the construction records keep it, and the rule that
// finds the assertions no sentence of the spec supports. It is pure; the
// session itself is the adapter in cite/session.
package cite

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

// The lines that hold an assertion, once trimmed.
var assertionStarts = []string{"assert ", "assert(", "pytest.raises(", "with pytest.raises("}

// Assertion is one assertion of an oracle: its name `<file>:<line>` and its
// text.
type Assertion struct {
	Name string
	Text string
}

// Assertions lists, in each test file of an oracle in name order, every line
// that starts an assert statement or a pytest.raises call.
func Assertions(oracle map[string][]byte) []Assertion {
	names := make([]string, 0, len(oracle))
	for name := range oracle {
		if strings.HasPrefix(name, l1accept.OracleFilePrefix) && strings.HasSuffix(name, l1accept.PythonSuffix) {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var found []Assertion
	for _, name := range names {
		for number, line := range strings.Split(string(oracle[name]), "\n") {
			trimmed := strings.TrimSpace(line)
			for _, start := range assertionStarts {
				if strings.HasPrefix(trimmed, start) {
					found = append(found, Assertion{Name: fmt.Sprintf("%s:%d", name, number+1), Text: trimmed})
					break
				}
			}
		}
	}
	return found
}

// Citation is the spec sentence a model cites behind one assertion.
type Citation struct {
	Assertion string `json:"assertion"`
	Sentence  string `json:"sentence"`
}

// Answer is the citation session's structured answer, and citations.json.
type Answer struct {
	Citations []Citation `json:"citations"`
}

// Uncited lists the assertions that no citation supports: one without a
// citation, or whose sentence the spec does not hold once spaces are collapsed.
func Uncited(answer Answer, assertions []Assertion, spec []byte) []Assertion {
	text := collapse(string(spec))
	cited := map[string]string{}
	for _, citation := range answer.Citations {
		cited[citation.Assertion] = collapse(citation.Sentence)
	}
	var uncited []Assertion
	for _, assertion := range assertions {
		sentence := cited[assertion.Name]
		if sentence == "" || !strings.Contains(text, sentence) {
			uncited = append(uncited, assertion)
		}
	}
	return uncited
}

func collapse(text string) string { return strings.Join(strings.Fields(text), " ") }

// Errors of the citation session's output.
var (
	ErrBadAnswer = errors.New("the session gave no structured answer of citations")
	ErrSession   = errors.New("the session reported an error")
)

// sessionResult is the part of `claude -p --output-format json` the answer is
// read from.
type sessionResult struct {
	IsError          bool            `json:"is_error"`
	StructuredOutput json.RawMessage `json:"structured_output"`
}

// ReadAnswer reads the citation session's standard output: its structured
// output, when the session did not report an error.
func ReadAnswer(output []byte) (Answer, error) {
	var result sessionResult
	if err := json.Unmarshal(output, &result); err != nil {
		return Answer{}, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	if result.IsError {
		return Answer{}, ErrSession
	}
	if len(result.StructuredOutput) == 0 || string(result.StructuredOutput) == "null" {
		return Answer{}, ErrBadAnswer
	}
	decoder := json.NewDecoder(bytes.NewReader(result.StructuredOutput))
	decoder.DisallowUnknownFields()
	var answer Answer
	if err := decoder.Decode(&answer); err != nil {
		return Answer{}, fmt.Errorf("%w: %v", ErrBadAnswer, err)
	}
	return answer, nil
}

// AnswerFiles reads the session's output as the file construction writes:
// citations.json holding the answer.
func AnswerFiles(output []byte) ([]construct.File, error) {
	answer, err := ReadAnswer(output)
	if err != nil {
		return nil, err
	}
	encoded, err := json.MarshalIndent(answer, "", "  ")
	if err != nil {
		return nil, err
	}
	return []construct.File{{Path: task.CitationsFile, Content: string(encoded) + "\n"}}, nil
}

// ParseFile reads citations.json.
func ParseFile(data []byte) (Answer, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var answer Answer
	if err := decoder.Decode(&answer); err != nil {
		return Answer{}, fmt.Errorf("%s: %w", task.CitationsFile, err)
	}
	return answer, nil
}
