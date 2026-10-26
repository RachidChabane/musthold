package hostproto_test

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

func TestSchemas(t *testing.T) {
	const draft = "https://json-schema.org/draft/2020-12/schema"

	t.Run("each host-protocol schema is a 2020-12 document identified by its URI [verifies SP-145~1]", func(t *testing.T) {
		schemas := hostproto.Schemas()
		if len(schemas) != 3 {
			t.Fatalf("%d schemas", len(schemas))
		}
		for _, schema := range schemas {
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

	t.Run("a schema is found by its identifier and an unknown one is not [verifies SP-145~1]", func(t *testing.T) {
		for _, id := range []string{hostproto.RequestType, hostproto.TicketType, hostproto.ResponseType} {
			if schema, err := hostproto.SchemaOf(id); err != nil || schema.ID != id {
				t.Errorf("%s: %v", id, err)
			}
		}
		if _, err := hostproto.SchemaOf("https://example.com/other"); err == nil {
			t.Error("an unknown identifier resolved")
		}
	})

	t.Run("the request schema fixes the request type URI of its major version [verifies SP-145~1]", func(t *testing.T) {
		schema, err := hostproto.SchemaOf(hostproto.RequestType)
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Properties struct {
				Type struct {
					Const string `json:"const"`
				} `json:"type"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(schema.Bytes, &document); err != nil || document.Properties.Type.Const != hostproto.RequestType {
			t.Fatalf("type const = %q, %v", document.Properties.Type.Const, err)
		}
	})
}
