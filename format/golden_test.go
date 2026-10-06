package format_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/internal/testkit/golden"
)

const (
	goldenDirectory = "testdata/golden"
	manifestName    = "manifest.json"
	goldenFileMode  = 0o644
	goldenDirMode   = 0o755
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden vectors from internal/testkit/golden")

func manifestBytes(t *testing.T) []byte {
	t.Helper()
	encoded, err := json.MarshalIndent(golden.Manifest(golden.Cases()), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	return append(encoded, '\n')
}

func TestMain(m *testing.M) {
	flag.Parse()
	if *updateGolden {
		if err := writeGolden(); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

func writeGolden() error {
	if err := os.MkdirAll(goldenDirectory, goldenDirMode); err != nil {
		return err
	}
	entries, err := os.ReadDir(goldenDirectory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(goldenDirectory, entry.Name())); err != nil {
			return err
		}
	}
	for _, c := range golden.Cases() {
		if err := os.WriteFile(filepath.Join(goldenDirectory, c.FileName()), append(c.Document, '\n'), goldenFileMode); err != nil {
			return err
		}
	}
	encoded, err := json.MarshalIndent(golden.Manifest(golden.Cases()), "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(goldenDirectory, manifestName), append(encoded, '\n'), goldenFileMode)
}

func readManifest(t *testing.T) []golden.ManifestEntry {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(goldenDirectory, manifestName))
	if err != nil {
		t.Fatal(err)
	}
	var entries []golden.ManifestEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	return entries
}

func TestGoldenVectors(t *testing.T) {
	entries := readManifest(t)

	t.Run("the manifest covers valid and every kind of negative vector [verifies SP-24~1]", func(t *testing.T) {
		required := []format.ViolationCode{
			format.CodeJudgedWithoutConfidence, format.CodeExecutedWithConfidence, format.CodeWrongClass,
			format.CodeSubject, format.CodeUnsorted, format.CodeCacheKeyMismatch,
		}
		seen := map[format.ViolationCode]bool{}
		valid := 0
		for _, entry := range entries {
			if len(entry.Violations) == 0 && entry.Decode == golden.DecodeOK {
				valid++
			}
			for _, code := range entry.Violations {
				seen[code] = true
			}
		}
		for _, code := range required {
			if !seen[code] {
				t.Errorf("no vector raises %s", code)
			}
		}
		if valid < len(format.Kinds()) {
			t.Errorf("only %d valid vectors", valid)
		}
	})

	for _, entry := range entries {
		t.Run(entry.File+" decodes and validates as its manifest states [verifies SP-24~1]", func(t *testing.T) {
			document, err := os.ReadFile(filepath.Join(goldenDirectory, entry.File))
			if err != nil {
				t.Fatal(err)
			}
			proof, err := format.Decode(document)
			if entry.Decode == golden.DecodeUnknownPredicateType {
				if !errors.Is(err, format.ErrUnknownPredicateType) {
					t.Fatalf("decode error = %v, want ErrUnknownPredicateType", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			got := violationCodes(t, proof.Validate())
			want := slices.Clone(entry.Violations)
			slices.Sort(want)
			if !slices.Equal(got, want) && (len(got) != 0 || len(want) != 0) {
				t.Fatalf("violations = %v, want %v", got, want)
			}
		})
	}

	t.Run("the committed vectors are the ones the generator produces [verifies SP-24~1]", func(t *testing.T) {
		for _, c := range golden.Cases() {
			committed, err := os.ReadFile(filepath.Join(goldenDirectory, c.FileName()))
			if err != nil {
				t.Fatalf("%s: %v (run go test ./format -update-golden)", c.FileName(), err)
			}
			if !bytes.Equal(bytes.TrimRight(committed, "\n"), c.Document) {
				t.Errorf("%s is stale (run go test ./format -update-golden)", c.FileName())
			}
		}
		committedManifest, err := os.ReadFile(filepath.Join(goldenDirectory, manifestName))
		if err != nil || !bytes.Equal(committedManifest, manifestBytes(t)) {
			t.Error("the manifest is stale (run go test ./format -update-golden)")
		}
	})
}
