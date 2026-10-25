package format_test

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

const verifiesSP2161 = " [verifies SP-2161~1]"

func withSourceName(name string) func(*scanStatement) {
	return func(s *scanStatement) { s.Predicate.Promises[0].Source.Name = name }
}

func TestScanSourceNames(t *testing.T) {
	accepted := []string{
		format.ScanSourceReadme,
		format.ScanSourceDescription,
		format.ScanSourceIssue(1),
		format.ScanSourceIssue(4096),
		"issue #10",
	}
	refused := []string{
		"", "README", "readme.md", "README.md ", "description ", "Description",
		"issue", "issue #", "issue #0", "issue #01", "issue #-3", "issue 12", "issue #12 ", "issue #1a",
		"Issue #12", "diff", "CHANGELOG.md", "docs/README.md", "pull request",
	}
	for _, name := range accepted {
		t.Run("the source name "+name+" is accepted"+verifiesSP2161, func(t *testing.T) {
			if !format.IsScanSourceName(name) {
				t.Error("refused by IsScanSourceName")
			}
			if err := proofs.Scan(withSourceName(name)).Validate(); err != nil {
				t.Errorf("refused by the statement: %v", err)
			}
		})
	}
	for _, name := range refused {
		t.Run("the source name "+name+" is refused"+verifiesSP2161, func(t *testing.T) {
			if format.IsScanSourceName(name) {
				t.Error("accepted by IsScanSourceName")
			}
			want := format.CodeUnknownValue
			if name == "" {
				want = format.CodeMissingField
			}
			codes := violationCodes(t, proofs.Scan(withSourceName(name)).Validate())
			if !slices.Contains(codes, want) {
				t.Errorf("codes = %v, want %s among them", codes, want)
			}
		})
	}
	t.Run("the constructor writes the number in decimal after the issue marker"+verifiesSP2161, func(t *testing.T) {
		if got := format.ScanSourceIssue(7); got != "issue #7" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("the validator and the scan schema agree on every source name"+verifiesSP2161, func(t *testing.T) {
		schema := scanStatementSchema(t)
		rapid.Check(t, func(t *rapid.T) {
			name := rapid.OneOf(
				rapid.Just(format.ScanSourceReadme),
				rapid.Just(format.ScanSourceDescription),
				rapid.Custom(func(t *rapid.T) string { return format.ScanSourceIssue(rapid.IntRange(-2, 1000000).Draw(t, "number")) }),
				rapid.StringMatching(`[A-Za-z.# 0-9]{1,12}`),
			).Draw(t, "name")
			statement := proofs.Scan(withSourceName(name))
			encoded, err := json.Marshal(statement)
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(encoded))
			if err != nil {
				t.Fatal(err)
			}
			want := format.IsScanSourceName(name)
			if got := schema.Validate(instance) == nil; got != want {
				t.Fatalf("%q: schema accepts = %v, want %v", name, got, want)
			}
			if got := statement.Validate() == nil; got != want {
				t.Fatalf("%q: validator accepts = %v, want %v", name, got, want)
			}
		})
	})
}
