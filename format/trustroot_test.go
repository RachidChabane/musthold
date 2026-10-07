package format_test

import (
	"encoding/json"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const verifiesTUFRoot = " [verifies SP-225~1]"

func TestPublicGoodTUFRoot(t *testing.T) {
	t.Run("the embedded root is a TUF root document"+verifiesTUFRoot, func(t *testing.T) {
		var document struct {
			Signed struct {
				Type    string                     `json:"_type"`
				Version int                        `json:"version"`
				Keys    map[string]json.RawMessage `json:"keys"`
				Roles   map[string]json.RawMessage `json:"roles"`
			} `json:"signed"`
			Signatures []json.RawMessage `json:"signatures"`
		}
		if err := json.Unmarshal(format.PublicGoodTUFRoot(), &document); err != nil {
			t.Fatal(err)
		}
		if document.Signed.Type != "root" || document.Signed.Version < 1 || len(document.Signed.Keys) == 0 || len(document.Signed.Roles) == 0 || len(document.Signatures) == 0 {
			t.Fatalf("root %+v", document)
		}
	})

	t.Run("each call hands out a copy"+verifiesTUFRoot, func(t *testing.T) {
		first := format.PublicGoodTUFRoot()
		first[0] = 'X'
		if second := format.PublicGoodTUFRoot(); second[0] == 'X' {
			t.Fatal("the embedded root was modified through a returned slice")
		}
	})
}
