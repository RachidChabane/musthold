package cite_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/cite"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

const spec = `# Rates

- **RATE-1~1**: WHEN a parcel weighs up to 30 kg THE SYSTEM SHALL charge
  the band rate.
  - Example: quote(weight=2, zone=1) => 5
`

var oracle = map[string][]byte{
	"test_rate_2.py": []byte("def test_b():\n    assert quote(weight=40, zone=1) == 9\n"),
	"test_rate_1.py": []byte("import pytest\n\ndef test_a():\n    assert quote(weight=2, zone=1) == 5\n    with pytest.raises(ValueError):\n        quote(weight=-1, zone=1)\n    x = 1  # assert nothing\n"),
	"pytest.ini":     []byte("[pytest]\n"),
}

func names(assertions []cite.Assertion) []string {
	var found []string
	for _, assertion := range assertions {
		found = append(found, assertion.Name)
	}
	return found
}

func TestAssertions(t *testing.T) {
	t.Run("every assert statement and pytest.raises call of each test file is listed by file and line, in name order [verifies SP-2915~1]", func(t *testing.T) {
		got := names(cite.Assertions(oracle))
		want := []string{"test_rate_1.py:4", "test_rate_1.py:5", "test_rate_2.py:2"}
		if !slices.Equal(got, want) {
			t.Fatalf("got %v", got)
		}
	})
}

func TestUncited(t *testing.T) {
	assertions := cite.Assertions(oracle)
	t.Run("an assertion is cited by a sentence the spec holds once spaces are collapsed [verifies SP-2916~1]", func(t *testing.T) {
		answer := cite.Answer{Citations: []cite.Citation{
			{Assertion: "test_rate_1.py:4", Sentence: "WHEN a parcel weighs up to 30 kg THE SYSTEM SHALL charge the band rate."},
			{Assertion: "test_rate_1.py:5", Sentence: "a parcel weighs up to 30 kg"},
			{Assertion: "test_rate_2.py:2", Sentence: "WHEN a parcel weighs over 30 kg THE SYSTEM SHALL charge 9."},
		}}
		uncited := names(cite.Uncited(answer, assertions, []byte(spec)))
		if !slices.Equal(uncited, []string{"test_rate_2.py:2"}) {
			t.Fatalf("uncited %v", uncited)
		}
	})

	t.Run("an assertion without a citation, or with an empty sentence, is uncited [verifies SP-2916~1]", func(t *testing.T) {
		answer := cite.Answer{Citations: []cite.Citation{{Assertion: "test_rate_1.py:4", Sentence: " "}}}
		if got := cite.Uncited(answer, assertions, []byte(spec)); len(got) != 3 {
			t.Fatalf("uncited %v", names(got))
		}
	})
}

func TestReadAnswer(t *testing.T) {
	t.Run("the answer is the structured output of a session that reported no error, written as citations.json [verifies SP-2936~1] [verifies SP-2945~1]", func(t *testing.T) {
		output := []byte(`{"type":"result","is_error":false,"structured_output":{"citations":[{"assertion":"test_rate_1.py:4","sentence":"s"}]}}`)
		files, err := cite.AnswerFiles(output)
		if err != nil || len(files) != 1 || files[0].Path != task.CitationsFile {
			t.Fatalf("%+v %v", files, err)
		}
		answer, err := cite.ParseFile([]byte(files[0].Content))
		if err != nil || len(answer.Citations) != 1 || answer.Citations[0].Assertion != "test_rate_1.py:4" {
			t.Fatalf("%+v %v", answer, err)
		}
	})

	t.Run("an error result, a missing structured output or one of another shape is no answer [verifies SP-2945~1]", func(t *testing.T) {
		for output, want := range map[string]error{
			`{"is_error":true,"structured_output":{"citations":[]}}`: cite.ErrSession,
			`{"is_error":false}`: cite.ErrBadAnswer,
			`{"is_error":false,"structured_output":{"quotes":[]}}`: cite.ErrBadAnswer,
			`not json`: cite.ErrBadAnswer,
		} {
			if _, err := cite.ReadAnswer([]byte(output)); !errors.Is(err, want) {
				t.Errorf("%s: %v", strings.TrimSpace(output), err)
			}
		}
	})
}
