package schemacheck_test

import (
	"errors"
	"os"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/format"
)

const goldenDirectory = "../../../../format/testdata/golden/"

func read(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(goldenDirectory + name)
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestValidate(t *testing.T) {
	checker, err := schemacheck.New()
	if err != nil {
		t.Fatal(err)
	}

	t.Run("a valid alignment Statement is accepted [verifies SP-354~1]", func(t *testing.T) {
		if err := checker.Validate(read(t, "alignment-valid.json")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("an alignment Statement that breaks its schema is refused [verifies SP-354~1]", func(t *testing.T) {
		for _, name := range []string{"alignment-executed-verdict.json", "alignment-judged-without-confidence.json", "alignment-not-blind.json"} {
			if err := checker.Validate(read(t, name)); !errors.Is(err, schemacheck.ErrInvalid) {
				t.Errorf("%s: error %v", name, err)
			}
		}
	})

	t.Run("a document that is not a Statement of the format is refused before the schema [verifies SP-354~1]", func(t *testing.T) {
		if err := checker.Validate([]byte("not json")); !errors.Is(err, schemacheck.ErrMalformed) {
			t.Errorf("not json: %v", err)
		}
		if err := checker.Validate([]byte(`{"predicateType":"https://example.com/other/v1"}`)); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Errorf("unknown predicate: %v", err)
		}
	})

	t.Run("a schema that references a resource that is not embedded fails to compile [verifies SP-354~1]", func(t *testing.T) {
		if _, err := schemacheck.NewWith([]format.Schema{{ID: format.CommonSchemaID, Bytes: []byte(`{"$ref":"https://example.com/remote.json"}`)}}); err == nil {
			t.Fatal("a remote reference compiled")
		}
	})
}
