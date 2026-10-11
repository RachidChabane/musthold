package mutation

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
)

// editSets generates valid edit sets of up to four edits over a few files,
// qualnames and target kinds, the target sometimes left empty.
func editSets() *rapid.Generator[EditSet] {
	edit := rapid.Custom(func(t *rapid.T) Edit {
		return Edit{
			File:      rapid.SampledFrom([]string{"src/a.py", "src/b.py", "pkg/c.go"}).Draw(t, "file"),
			Qualname:  rapid.StringMatching(`[a-z_]{1,6}(\.[a-z_]{1,4})?`).Draw(t, "qualname"),
			Target:    rapid.SampledFrom(append([]TargetKind{""}, allTargets...)).Draw(t, "target"),
			NewSource: rapid.String().Draw(t, "source"),
		}
	})
	return rapid.Custom(func(t *rapid.T) EditSet {
		edits := rapid.SliceOfNDistinct(edit, 1, 4, unitKey).Draw(t, "edits")
		return EditSet{Edits: edits}
	})
}

func TestScanEdits(t *testing.T) {
	t.Run("the scan wire form of an edit set digests to its mutant id [verifies SP-2030~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			set := editSets().Draw(t, "set")
			digest, err := format.ScanEditsDigest(set.ScanEdits())
			if err != nil {
				t.Fatal(err)
			}
			if id := mustID(t, set); digest != id {
				t.Fatalf("scan digest %s, mutant id %s", digest, id)
			}
		})
	})

	t.Run("the scan wire form is in canonical order with the definition target omitted [verifies SP-2030~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			edits := editSets().Draw(t, "set").ScanEdits()
			if !slices.Equal(edits, format.CanonicalScanEdits(edits)) {
				t.Fatalf("edits %+v are not in canonical order", edits)
			}
			for _, edit := range edits {
				if edit.Target == string(TargetDefinition) {
					t.Fatalf("a definition target is spelled out: %+v", edit)
				}
			}
		})
	})

	t.Run("a mutant written into a scan statement passes its id check [verifies SP-2030~1]", func(t *testing.T) {
		set := EditSet{Edits: []Edit{
			{File: "src/parse.py", Qualname: "split", NewSource: "def split(line):\n    return line.split(';')"},
			{File: "src/parse.py", Qualname: "SEPARATOR", Target: TargetModuleAssignment, NewSource: "SEPARATOR = ';'"},
		}}
		mutant := format.ScanMutant{
			ID: mustID(t, set), Origin: format.ScanOriginModel, Edits: set.ScanEdits(),
			Lines: []format.ScanLine{{File: "src/parse.py", Line: 3}},
		}
		statement := format.NewScanStatement(format.SHA256Hex([]byte("commit"))[:40], format.SHA256Hex([]byte("tree")), format.Scan{
			Module: format.Module{Name: "spec-proof-dp", Version: "0.1.0"}, ProducedBy: format.ProducedByLocal,
			Mode: format.ScanModeRepository, Runner: "pytest", Seed: "0", Mutants: []format.ScanMutant{mutant},
		})
		if err := statement.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}
