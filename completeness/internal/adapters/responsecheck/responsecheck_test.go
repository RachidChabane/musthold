package responsecheck

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/blind"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func tokenized(count int) blind.Tokenized {
	criteria := make([]criterion.Criterion, count)
	for index := range criteria {
		criteria[index] = criterion.Criterion{Ref: criterion.Ref{ID: fmt.Sprintf("PAR-%d", index+1), Revision: 1}, Statement: "WHEN x THE SYSTEM SHALL y."}
	}
	return blind.Tokenize(criteria, "seed", nil)
}

func hunkSchema(t testing.TB) (json.RawMessage, []string) {
	t.Helper()
	criteria := tokenized(4)
	built, err := prompt.BuildHunk(prompt.HunkInput{Criteria: criteria, Group: criteria.Tokens, Change: "changed total"})
	if err != nil {
		t.Fatal(err)
	}
	return built.Schema, criteria.Tokens
}

func dimensionSchema(t testing.TB) json.RawMessage {
	t.Helper()
	built, err := prompt.BuildDimensions(prompt.DimensionsInput{Criteria: tokenized(2), Dimensions: dimension.Checklist()})
	if err != nil {
		t.Fatal(err)
	}
	return built.Schema
}

func hunkAnswer(choice string) string {
	return fmt.Sprintf(`{"answers":[{"id":"criterion","choice":%q,"confidence":0.9}]}`, choice)
}

func TestValidate(t *testing.T) {
	schema, tokens := hunkSchema(t)
	validator := New()

	t.Run("outputs that satisfy the closed schema are accepted [verifies SP-536~1]", func(t *testing.T) {
		for _, output := range []string{hunkAnswer(tokens[0]), hunkAnswer("NONE"), `{"answers":[{"id":"criterion","choice":"NONE"}]}`} {
			if err := validator.Validate(schema, []byte(output)); err != nil {
				t.Errorf("%s: %v", output, err)
			}
		}
	})

	t.Run("each kind of violation is refused with ErrSchemaViolation [verifies SP-536~1]", func(t *testing.T) {
		refused := map[string]string{
			"a question missing":    `{"answers":[]}`,
			"a question twice":      fmt.Sprintf(`{"answers":[{"id":"criterion","choice":%q},{"id":"criterion","choice":"NONE"}]}`, tokens[0]),
			"an unknown question":   `{"answers":[{"id":"other","choice":"NONE"}]}`,
			"an unknown choice":     hunkAnswer("K-zzzz"),
			"a confidence above 1":  `{"answers":[{"id":"criterion","choice":"NONE","confidence":2}]}`,
			"an extra member":       `{"answers":[{"id":"criterion","choice":"NONE","why":"x"}]}`,
			"a wrong type":          `{"answers":[{"id":"criterion","choice":3}]}`,
			"not an object":         `["NONE"]`,
			"not JSON":              `{"answers":`,
			"an extra document key": `{"answers":[{"id":"criterion","choice":"NONE"}],"note":"x"}`,
		}
		for name, output := range refused {
			if err := validator.Validate(schema, []byte(output)); !errors.Is(err, hostproto.ErrSchemaViolation) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("every violation is listed [verifies SP-536~1]", func(t *testing.T) {
		err := validator.Validate(schema, []byte(`{"answers":[{"id":"criterion","choice":"K-zzzz","confidence":2}],"extra":1}`))
		if !errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		if !strings.Contains(err.Error(), "extra") {
			t.Errorf("the message does not mention the extra member:\n%v", err)
		}
	})
}

func TestCompilation(t *testing.T) {
	t.Run("a schema that does not compile is reported apart from a violation [verifies SP-536~1]", func(t *testing.T) {
		err := New().Validate(json.RawMessage(`{"type":12}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		if err := New().Validate(json.RawMessage(`not json`), []byte(`{}`)); !errors.Is(err, ErrBadSchema) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a reference to a remote resource is refused and never fetched [verifies SP-536~1]", func(t *testing.T) {
		const remote = "https://example.com/remote.json"
		err := New().Validate(json.RawMessage(`{"type":"object","properties":{"a":{"$ref":"`+remote+`"}}}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || !strings.Contains(err.Error(), remote) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a compiled schema is cached by the digest of its bytes [verifies SP-536~1]", func(t *testing.T) {
		validator := New()
		schema, tokens := hunkSchema(t)
		for range 3 {
			_ = validator.Validate(schema, []byte(hunkAnswer(tokens[0])))
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
	schema := dimensionSchema(t)
	validator := New()
	standings := []string{"COVERED", "NOT_APPLICABLE", "MISSING", "PARTIAL"}

	t.Run("a dimension answer is accepted exactly when every dimension has a known standing [verifies SP-536~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			checklist := dimension.Checklist()
			answers := make([]string, len(checklist))
			consistent := true
			for index, d := range checklist {
				standing := rapid.SampledFrom(standings).Draw(t, string(d))
				consistent = consistent && standing != "PARTIAL"
				answers[index] = fmt.Sprintf(`{"id":%q,"choice":%q}`, d, standing)
			}
			err := validator.Validate(schema, []byte(`{"answers":[`+strings.Join(answers, ",")+`]}`))
			if consistent != (err == nil) {
				t.Fatalf("answers %v: error = %v", answers, err)
			}
			if err != nil && !errors.Is(err, hostproto.ErrSchemaViolation) {
				t.Fatalf("error = %v", err)
			}
		})
	})
}
