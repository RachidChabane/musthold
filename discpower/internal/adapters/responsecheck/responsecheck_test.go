package responsecheck

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

const schemaDirectory = "../../domain/prompt/schemas"

func loadSchema(t testing.TB, name string) json.RawMessage {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(schemaDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestValidate(t *testing.T) {
	validity := loadSchema(t, "judge-validity.json")
	validator := New()

	t.Run("outputs that satisfy the schema are accepted [verifies SP-156~1]", func(t *testing.T) {
		accepted := []string{
			`{"validity":"VALID"}`,
			`{"validity":"EQUIVALENT","confidence":0.5,"rationale":"same behaviour"}`,
			`{"validity":"NOT_VIOLATING","witness":{"operation":"line_total","args":{"quantity":{"type":"integer","value":"3"}},"expected":{"kind":"returns","type":"integer","value":"12"}}}`,
			`{"validity":"UNDETERMINED","witness":{"operation":"line_total","args":{},"expected":{"kind":"raises","value":"ValueError"}}}`,
		}
		for _, output := range accepted {
			if err := validator.Validate(validity, []byte(output)); err != nil {
				t.Errorf("%s: %v", output, err)
			}
		}
	})

	t.Run("each kind of violation is refused with ErrSchemaViolation [verifies SP-156~1]", func(t *testing.T) {
		refused := map[string]string{
			"missing required member": `{"confidence":0.5}`,
			"wrong enum":              `{"validity":"MAYBE"}`,
			"wrong type":              `{"validity":"VALID","confidence":"high"}`,
			"extra member":            `{"validity":"VALID","comment":"x"}`,
			"out of range":            `{"validity":"VALID","confidence":2}`,
			"not an object":           `["VALID"]`,
			"not JSON":                `{"validity":`,
			"nested missing member":   `{"validity":"VALID","witness":{"operation":"f","args":{}}}`,
			"nested wrong shape":      `{"validity":"VALID","witness":{"operation":"f","args":{"a":{"type":"integer"}},"expected":{"kind":"returns","value":"1"}}}`,
			"nested wrong enum":       `{"validity":"VALID","witness":{"operation":"f","args":{"a":{"type":"float","value":"1"}},"expected":{"kind":"returns","value":"1"}}}`,
			"nested extra member":     `{"validity":"VALID","witness":{"operation":"f","args":{},"expected":{"kind":"returns","value":"1","extra":1}}}`,
		}
		for name, output := range refused {
			if err := validator.Validate(validity, []byte(output)); !errors.Is(err, ports.ErrSchemaViolation) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("every violation is listed [verifies SP-156~1]", func(t *testing.T) {
		err := validator.Validate(validity, []byte(`{"validity":"MAYBE","confidence":"high","extra":1}`))
		if !errors.Is(err, ports.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		for _, want := range []string{"/validity", "/confidence", "extra"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("the message does not mention %q:\n%v", want, err)
			}
		}
	})

	t.Run("the other embedded schemas accept and refuse outputs [verifies SP-156~1]", func(t *testing.T) {
		cases := []struct {
			schema, accepted, refused string
		}{
			{"generate.json",
				`{"mutants":[{"slot":"S1","mechanism":"off by one","edits":[{"file":"a.py","qualname":"f","new_source":"def f(): pass"}]}]}`,
				`{"mutants":[{"slot":"S9","mechanism":"x","edits":[]}]}`},
			{"judge-equivalent.json", `{"equivalent":"DIFFERENT","confidence":1}`, `{"equivalent":"SAME"}`},
			{"closed.json", `{"answers":[{"id":"q1","choice":"VIOLATES"}]}`, `{"answers":[{"id":"","choice":"CONSISTENT"}]}`},
		}
		for _, c := range cases {
			schema := loadSchema(t, c.schema)
			if err := validator.Validate(schema, []byte(c.accepted)); err != nil {
				t.Errorf("%s accepted: %v", c.schema, err)
			}
			if err := validator.Validate(schema, []byte(c.refused)); !errors.Is(err, ports.ErrSchemaViolation) {
				t.Errorf("%s refused: %v", c.schema, err)
			}
		}
	})
}

func TestCompilation(t *testing.T) {
	t.Run("a schema that does not compile is reported apart from a violation [verifies SP-156~1]", func(t *testing.T) {
		err := New().Validate(json.RawMessage(`{"type":12}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || errors.Is(err, ports.ErrSchemaViolation) {
			t.Fatalf("error = %v", err)
		}
		if err := New().Validate(json.RawMessage(`not json`), []byte(`{}`)); !errors.Is(err, ErrBadSchema) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a reference to a remote resource is refused and never fetched [verifies SP-156~1]", func(t *testing.T) {
		const remote = "https://example.com/remote.json"
		err := New().Validate(json.RawMessage(`{"type":"object","properties":{"a":{"$ref":"`+remote+`"}}}`), []byte(`{}`))
		if !errors.Is(err, ErrBadSchema) || !strings.Contains(err.Error(), remote) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a compiled schema is cached by the digest of its bytes [verifies SP-156~1]", func(t *testing.T) {
		validator := New()
		schema := loadSchema(t, "judge-validity.json")
		for range 3 {
			_ = validator.Validate(schema, []byte(`{"validity":"VALID"}`))
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
	validity := loadSchema(t, "judge-validity.json")
	validator := New()
	verdicts := []string{"VALID", "EQUIVALENT", "NOT_VIOLATING", "UNDETERMINED"}

	t.Run("an output is accepted exactly when its verdict is in the enum [verifies SP-156~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			verdict := rapid.OneOf(rapid.SampledFrom(verdicts), rapid.StringMatching(`[A-Z_]{1,12}`)).Draw(t, "verdict")
			output, _ := json.Marshal(map[string]string{"validity": verdict})
			err := validator.Validate(validity, output)
			known := false
			for _, candidate := range verdicts {
				known = known || candidate == verdict
			}
			if known != (err == nil) {
				t.Fatalf("verdict %q: error = %v", verdict, err)
			}
			if err != nil && !errors.Is(err, ports.ErrSchemaViolation) {
				t.Fatalf("error = %v", err)
			}
		})
	})
}
