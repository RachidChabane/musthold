package format_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

const verifiesAdmission = " [verifies SP-149~1]"

func canonicalGolden(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDirectory, name))
	if err != nil {
		t.Fatal(err)
	}
	return bytes.TrimSuffix(raw, []byte("\n"))
}

func TestAdmit(t *testing.T) {
	valid := canonicalGolden(t, "alignment-valid.json")

	t.Run("a canonical valid statement is admitted with its location"+verifiesAdmission, func(t *testing.T) {
		admitted, err := format.Admit(valid)
		if err != nil {
			t.Fatal(err)
		}
		location := admitted.Location("alignment")
		if location.Module != "alignment" || location.SubjectKey != "PARK-2~1" || location.CacheKey != admitted.Proof.Common().CacheKey {
			t.Fatalf("location %+v", location)
		}
		if err := admitted.CheckLocation(location); err != nil {
			t.Fatal(err)
		}
		if other := (format.ProofLocation{Module: "alignment", SubjectKey: "PARK-3~1", CacheKey: location.CacheKey}); admitted.CheckLocation(other) == nil {
			t.Fatal("a statement at the wrong subject key is accepted")
		}
		if !bytes.Equal(admitted.Bytes, valid) {
			t.Fatal("admitted bytes differ")
		}
	})

	t.Run("a completeness subject key is the encoded spec path"+verifiesAdmission, func(t *testing.T) {
		admitted, err := format.Admit(canonicalGolden(t, "completeness-valid.json"))
		if err != nil {
			t.Fatal(err)
		}
		if key := admitted.Location("completeness").SubjectKey; key != "specs%2Fparkfee.md" {
			t.Fatalf("key %q", key)
		}
	})

	t.Run("a trailing newline makes the bytes non-canonical"+verifiesAdmission, func(t *testing.T) {
		if _, err := format.Admit(append(bytes.Clone(valid), '\n')); err == nil {
			t.Fatal("accepted")
		}
	})

	t.Run("an indented statement is not canonical"+verifiesAdmission, func(t *testing.T) {
		var indented bytes.Buffer
		if err := json.Indent(&indented, valid, "", "  "); err != nil {
			t.Fatal(err)
		}
		if _, err := format.Admit(indented.Bytes()); !errors.Is(err, format.ErrNotCanonical) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an invalid statement is refused with its violations"+verifiesAdmission, func(t *testing.T) {
		_, err := format.Admit(canonicalGolden(t, "alignment-not-blind.json"))
		var validation *format.ValidationError
		if !errors.As(err, &validation) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a predicate type outside the format is refused"+verifiesAdmission, func(t *testing.T) {
		if _, err := format.Admit(canonicalGolden(t, "envelope-unknown-predicate-type.json")); !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a document that is not JSON is refused"+verifiesAdmission, func(t *testing.T) {
		if _, err := format.Admit([]byte("{")); err == nil {
			t.Fatal("accepted")
		}
	})
}
