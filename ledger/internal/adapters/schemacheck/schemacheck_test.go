package schemacheck_test

import (
	"bytes"
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/ledger/internal/adapters/schemacheck"
	"github.com/RachidChabane/spec-proof/ledger/internal/testkit"
)

const verifiesSchemas = " [verifies SP-200~1]"

func checker(t *testing.T) *schemacheck.Checker {
	t.Helper()
	checker, err := schemacheck.New()
	if err != nil {
		t.Fatal(err)
	}
	return checker
}

func TestValidate(t *testing.T) {
	validator := checker(t)

	t.Run("the valid statements of the golden vectors satisfy their schemas"+verifiesSchemas, func(t *testing.T) {
		for _, statement := range testkit.ValidStatements(t) {
			if err := validator.Validate(statement.Bytes); err != nil {
				t.Errorf("%s: %v", statement.Location.StatementPath(), err)
			}
		}
	})

	t.Run("a statement that breaks its schema is refused"+verifiesSchemas, func(t *testing.T) {
		valid := testkit.ValidStatements(t)[0].Bytes
		broken := bytes.Replace(valid, []byte(`"producedBy":"ci"`), []byte(`"producedBy":"nowhere"`), 1)
		if err := validator.Validate(broken); !errors.Is(err, schemacheck.ErrInvalid) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a predicate type outside the format is refused"+verifiesSchemas, func(t *testing.T) {
		valid := testkit.ValidStatements(t)[0].Bytes
		unknown := bytes.Replace(valid, []byte("/alignment/v1"), []byte("/unheard-of/v1"), 1)
		if err := validator.Validate(unknown); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a document that is not a statement is refused"+verifiesSchemas, func(t *testing.T) {
		for _, document := range []string{`{`, `[]`, `"text"`, `null`} {
			if err := validator.Validate([]byte(document)); !errors.Is(err, schemacheck.ErrMalformed) && !errors.Is(err, format.ErrUnknownPredicateType) {
				t.Errorf("%s: error %v", document, err)
			}
		}
	})
}

func FuzzValidate(f *testing.F) {
	for _, statement := range testkit.ValidStatements(f) {
		f.Add(statement.Bytes)
	}
	f.Add([]byte(`{"predicateType":"https://rachid-chabane.com/attestations/alignment/v1"}`))
	validator, err := schemacheck.New()
	if err != nil {
		f.Fatal(err)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		_ = validator.Validate(data)
	})
}
