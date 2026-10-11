package format_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/golden"
)

const scanGoldenDirectory = "testdata/scan"

func scanManifestBytes() ([]byte, error) {
	encoded, err := json.MarshalIndent(golden.ScanManifest(golden.ScanCases()), "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

// writeScanGolden rewrites the scan vectors and their manifest from the
// generator.
func writeScanGolden() error {
	if err := os.RemoveAll(scanGoldenDirectory); err != nil {
		return err
	}
	if err := os.MkdirAll(scanGoldenDirectory, goldenDirMode); err != nil {
		return err
	}
	for _, c := range golden.ScanCases() {
		if err := os.WriteFile(filepath.Join(scanGoldenDirectory, c.FileName()), append(c.Document, '\n'), goldenFileMode); err != nil {
			return err
		}
	}
	manifest, err := scanManifestBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(scanGoldenDirectory, manifestName), manifest, goldenFileMode)
}

func readScanManifest(t *testing.T) []golden.ScanManifestEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(scanGoldenDirectory, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	var entries []golden.ScanManifestEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

// scanStatementSchema compiles the scan schema with the common definitions it
// refers to, refusing every other resource, and returns the schema of a whole
// scan statement.
func scanStatementSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	compiler.UseLoader(refusingLoader{t: t})
	common := format.Schemas()[slices.IndexFunc(format.Schemas(), func(s format.Schema) bool { return s.ID == format.CommonSchemaID })]
	for _, schema := range []format.Schema{common, format.ScanSchema()} {
		document, err := jsonschema.UnmarshalJSON(bytes.NewReader(schema.Bytes))
		if err != nil {
			t.Fatalf("parse %s: %v", schema.ID, err)
		}
		if err := compiler.AddResource(schema.ID, document); err != nil {
			t.Fatalf("add %s: %v", schema.ID, err)
		}
	}
	compiled, err := compiler.Compile(format.PredicateTypeScan + format.ScanStatementSchemaPointer)
	if err != nil {
		t.Fatalf("compile the scan statement schema: %v", err)
	}
	return compiled
}

type refusingLoader struct{ t *testing.T }

func (l refusingLoader) Load(url string) (any, error) {
	l.t.Fatalf("the scan schema loads %s, which is not embedded", url)
	return nil, nil
}

func TestScanSchema(t *testing.T) {
	t.Run("the scan schema is a 2020-12 document identified by the scan predicate type [verifies SP-2011~1]", func(t *testing.T) {
		var document struct {
			ID     string `json:"$id"`
			Schema string `json:"$schema"`
		}
		if err := json.Unmarshal(format.ScanSchema().Bytes, &document); err != nil {
			t.Fatal(err)
		}
		if document.ID != format.PredicateTypeScan || document.Schema != "https://json-schema.org/draft/2020-12/schema" {
			t.Fatalf("$id %q, $schema %q", document.ID, document.Schema)
		}
	})

	schema := scanStatementSchema(t)
	for _, entry := range readScanManifest(t) {
		t.Run(entry.File+" has the schema result its manifest states [verifies SP-2011~1]", func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(scanGoldenDirectory, entry.File))
			if err != nil {
				t.Fatal(err)
			}
			instance, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
			if err != nil {
				t.Fatal(err)
			}
			err = schema.Validate(instance)
			switch entry.Schema {
			case golden.SchemaValid:
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
			case golden.SchemaInvalid:
				if err == nil {
					t.Fatal("expected a schema violation")
				}
			default:
				t.Fatalf("unknown schema expectation %q", entry.Schema)
			}
		})
	}
}

func TestScanGoldenVectors(t *testing.T) {
	entries := readScanManifest(t)

	t.Run("the scan manifest covers a valid vector and every scan violation [verifies SP-2011~1]", func(t *testing.T) {
		required := []format.ViolationCode{
			format.CodeRequirement, format.CodeSubject, format.CodeMutantID, format.CodeScanReference,
			format.CodeScanClass, format.CodeScanCounts, format.CodeScanRoute, format.CodeSentinelNotEquivalent,
		}
		seen := map[format.ViolationCode]bool{}
		valid := 0
		for _, entry := range entries {
			if len(entry.Violations) == 0 {
				valid++
			}
			for _, code := range entry.Violations {
				seen[code] = true
			}
		}
		for _, code := range required {
			if !seen[code] {
				t.Errorf("no scan vector raises %s", code)
			}
		}
		if valid == 0 {
			t.Error("no valid scan vector")
		}
	})

	for _, entry := range entries {
		t.Run(entry.File+" decodes and validates as its manifest states [verifies SP-2011~1]", func(t *testing.T) {
			document, err := os.ReadFile(filepath.Join(scanGoldenDirectory, entry.File))
			if err != nil {
				t.Fatal(err)
			}
			statement, err := format.DecodeScan(document)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := violationCodes(t, statement.Validate())
			want := slices.Clone(entry.Violations)
			slices.Sort(want)
			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("violations = %v, want %v", got, want)
			}
		})
	}

	t.Run("the committed scan vectors are the ones the generator produces [verifies SP-2011~1]", func(t *testing.T) {
		for _, c := range golden.ScanCases() {
			committed, err := os.ReadFile(filepath.Join(scanGoldenDirectory, c.FileName()))
			if err != nil {
				t.Fatalf("%s: %v (run go test ./format -update-golden)", c.FileName(), err)
			}
			if !bytes.Equal(bytes.TrimRight(committed, "\n"), c.Document) {
				t.Errorf("%s is stale (run go test ./format -update-golden)", c.FileName())
			}
		}
		manifest, err := scanManifestBytes()
		if err != nil {
			t.Fatal(err)
		}
		committed, err := os.ReadFile(filepath.Join(scanGoldenDirectory, manifestName))
		if err != nil || !bytes.Equal(committed, manifest) {
			t.Error("the scan manifest is stale (run go test ./format -update-golden)")
		}
	})
}
