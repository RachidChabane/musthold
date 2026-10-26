package format_test

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/RachidChabane/musthold/format"
)

func TestEmbeddedSchemas(t *testing.T) {
	const draft = "https://json-schema.org/draft/2020-12/schema"

	t.Run("every embedded schema is a 2020-12 document identified by its URI [verifies SP-22~1]", func(t *testing.T) {
		for _, schema := range format.Schemas() {
			var document struct {
				ID     string `json:"$id"`
				Schema string `json:"$schema"`
			}
			if err := json.Unmarshal(schema.Bytes, &document); err != nil {
				t.Fatalf("%s: %v", schema.ID, err)
			}
			if document.ID != schema.ID || document.Schema != draft {
				t.Errorf("%s: $id %q, $schema %q", schema.ID, document.ID, document.Schema)
			}
		}
	})

	t.Run("each predicate type URI resolves to the schema it identifies [verifies SP-22~1]", func(t *testing.T) {
		for _, kind := range format.Kinds() {
			schema, err := format.PredicateSchema(kind.PredicateType())
			if err != nil || schema.ID != kind.PredicateType() {
				t.Errorf("%s: %v", kind, err)
			}
		}
		for _, notPredicate := range []string{format.CommonSchemaID, "https://example.com/other"} {
			if _, err := format.PredicateSchema(notPredicate); !errors.Is(err, format.ErrUnknownPredicateType) {
				t.Errorf("%s: error = %v", notPredicate, err)
			}
		}
	})
}
