package responsecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/alignment/internal/domain/prompt"
	"github.com/RachidChabane/musthold/format/hostproto"
)

func testInput(count int) prompt.TestInput {
	criteria := make([]criterion.Criterion, count)
	for index := range criteria {
		criteria[index] = criterion.Criterion{Ref: criterion.Ref{ID: fmt.Sprintf("PAR-%d", index+1), Revision: 1}, Statement: "WHEN x THE SYSTEM SHALL y."}
	}
	return prompt.TestInput{Criteria: masking.Tokenize(criteria, "seed"), Unit: "def test_case(): pass", Signatures: "def f(): ..."}
}

func judgeSchema(t testing.TB) (json.RawMessage, []string) {
	t.Helper()
	input := testInput(4)
	built, err := prompt.BuildJudge(input)
	if err != nil {
		t.Fatal(err)
	}
	return built.Schema, input.Criteria.Tokens
}

func closedSchema(t testing.TB, final bool) (json.RawMessage, []string) {
	t.Helper()
	input := testInput(4)
	built, err := prompt.BuildClosed(prompt.ClosedInput{Test: input, Group: input.Criteria.Tokens, Final: final})
	if err != nil {
		t.Fatal(err)
	}
	return built.Schema, input.Criteria.Tokens
}

func answer(requirement, coverage string, confidence int) string {
	return fmt.Sprintf(`{"requirement":%q,"coverage":%q,"confidence":%d}`, requirement, coverage, confidence)
}

func TestValidate(t *testing.T) {
	schema, tokens := judgeSchema(t)
	validator := New()

	t.Run("outputs that satisfy the judge schema are accepted [verifies SP-347~1]", func(t *testing.T) {
		accepted := []string{
			answer(tokens[0], "FULL", 90),
			answer(tokens[1], "PARTIAL", 0),
			answer(tokens[2], "WEAK", 100),
			answer("NONE", "NONE", 50),
		}
		for _, output := range accepted {
			if err := validator.Validate(schema, []byte(output)); err != nil {
				t.Errorf("%s: %v", output, err)
			}
		}
	})

	t.Run("each kind of violation is refused with ErrSchemaViolation [verifies SP-347~1]", func(t *testing.T) {
		refused := map[string]string{
			"missing required member":    `{"requirement":"NONE","coverage":"NONE"}`,
			"unknown token":              answer("K-zzzz", "FULL", 90),
			"wrong type":                 `{"requirement":"NONE","coverage":"NONE","confidence":"high"}`,
			"extra member":               `{"requirement":"NONE","coverage":"NONE","confidence":1,"rationale":"x"}`,
			"out of range":               answer("NONE", "NONE", 101),
			"not an object":              `["NONE"]`,
			"not JSON":                   `{"requirement":`,
			"NONE with a coverage":       answer("NONE", "FULL", 90),
			"a token with coverage NONE": answer(tokens[0], "NONE", 90),
		}
		for name, output := range refused {
			if err := validator.Validate(schema, []byte(output)); !errors.Is(err, hostproto.ErrSchemaViolation) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("every violation is listed [verifies SP-347~1]", func(t *testing.T) {
		err := validator.Validate(schema, []byte(`{"requirement":"K-zzzz","coverage":"MAYBE","confidence":"high","extra":1}`))
		if !errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		for _, want := range []string{"/requirement", "/coverage", "/confidence", "extra"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the message does not mention %q:\n%v", want, err)
			}
		}
	})

	t.Run("the closed schema requires each question to be answered once, with a known choice [verifies SP-347~1]", func(t *testing.T) {
		closed, tokens := closedSchema(t, true)
		accepted := fmt.Sprintf(`{"answers":[{"id":"criterion","choice":%q,"confidence":0.9},{"id":"coverage","choice":"FULL"}]}`, tokens[0])
		if err := validator.Validate(closed, []byte(accepted)); err != nil {
			t.Fatalf("accepted: %v", err)
		}
		for name, output := range map[string]string{
			"a question missing":   fmt.Sprintf(`{"answers":[{"id":"criterion","choice":%q}]}`, tokens[0]),
			"a question twice":     fmt.Sprintf(`{"answers":[{"id":"criterion","choice":%q},{"id":"criterion","choice":"NONE"}]}`, tokens[0]),
			"an unknown question":  `{"answers":[{"id":"criterion","choice":"NONE"},{"id":"other","choice":"FULL"}]}`,
			"a choice of another":  `{"answers":[{"id":"criterion","choice":"FULL"},{"id":"coverage","choice":"FULL"}]}`,
			"a confidence above 1": `{"answers":[{"id":"criterion","choice":"NONE","confidence":2},{"id":"coverage","choice":"NONE"}]}`,
		} {
			if err := validator.Validate(closed, []byte(output)); !errors.Is(err, hostproto.ErrSchemaViolation) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})
}

func TestCompilation(t *testing.T) {
	t.Run("a schema that does not compile is reported apart from a violation [verifies SP-347~1]", func(t *testing.T) {
		err := New().Validate(json.RawMessage(`{"type":12}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		if err := New().Validate(json.RawMessage(`not json`), []byte(`{}`)); !errors.Is(err, ErrBadSchema) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a reference to a remote resource is refused and never fetched [verifies SP-347~1]", func(t *testing.T) {
		const remote = "https://example.com/remote.json"
		err := New().Validate(json.RawMessage(`{"type":"object","properties":{"a":{"$ref":"`+remote+`"}}}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || !strings.Contains(err.Error(), remote) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a compiled schema is cached by the digest of its bytes [verifies SP-347~1]", func(t *testing.T) {
		validator := New()
		schema, tokens := judgeSchema(t)
		for range 3 {
			_ = validator.Validate(schema, []byte(answer(tokens[0], "FULL", 1)))
		}
		if len(validator.compiled) != 1 {
			t.Fatalf("%d compiled schemas", len(validator.compiled))
		}
		_ = validator.Validate(json.RawMessage(`{"type":"object"}`), []byte(`{}`))
		if len(validator.compiled) != 2 {
			t.Fatalf("%d compiled schemas", len(validator.compiled))
		}
	})
}

func TestProperties(t *testing.T) {
	schema, tokens := judgeSchema(t)
	validator := New()
	coverages := []string{"FULL", "PARTIAL", "WEAK", "NONE"}

	t.Run("an output is accepted exactly when NONE and the coverage NONE go together [verifies SP-347~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			requirement := rapid.SampledFrom(append([]string{"NONE"}, tokens...)).Draw(t, "requirement")
			coverage := rapid.SampledFrom(coverages).Draw(t, "coverage")
			confidence := rapid.IntRange(0, 100).Draw(t, "confidence")
			err := validator.Validate(schema, []byte(answer(requirement, coverage, confidence)))
			consistent := (requirement == "NONE") == (coverage == "NONE")
			if consistent != (err == nil) {
				t.Fatalf("%s/%s: error = %v", requirement, coverage, err)
			}
			if err != nil && !errors.Is(err, hostproto.ErrSchemaViolation) {
				t.Fatalf("error = %v", err)
			}
		})
	})
}
