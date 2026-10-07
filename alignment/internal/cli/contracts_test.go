package cli_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	powerCache    = ".spec-proof/cache/discriminating-power"
	pythonTestDir = "tests/test_pricing.py"
)

// TestStatementContracts checks the shape of what an honest run writes against the
// contracts of the format and of the gate that reads it.
func TestStatementContracts(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.driveAttested(testkit.Oracle{})
	statements := h.statements()
	if len(statements) == 0 {
		t.Fatal("no Statement was written")
	}

	t.Run("the spec input is named ID~REV and repeats the statement digest of the subject [verifies SP-410~2]", func(t *testing.T) {
		for key, statement := range statements {
			var specs []format.Input
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleSpec {
					specs = append(specs, input)
				}
			}
			want := format.SpecInput(statement.Predicate.Requirement.ID, statement.Predicate.Requirement.Revision, statement.Subject[0].Digest.SHA256)
			if len(specs) != 1 || specs[0] != want {
				t.Errorf("%s: spec inputs %+v, want %+v", key, specs, want)
			}
		}
	})

	t.Run("the file of the test is a test_source input with the digest of its exact bytes [verifies SP-410~2]", func(t *testing.T) {
		for key, statement := range statements {
			want := format.FileInput(format.RoleTestSource, pythonTestDir, testkit.FileDigest(t, h.root, pythonTestDir))
			if !slices.Contains(statement.Predicate.Inputs, want) {
				t.Errorf("%s: no descriptor %+v among %+v", key, want, statement.Predicate.Inputs)
			}
		}
	})

	t.Run("every file descriptor names a path relative to the directory holding shallnot.yaml with / separators [verifies SP-410~2]", func(t *testing.T) {
		for key, statement := range statements {
			for _, input := range statement.Predicate.Inputs {
				if input.Role.IsFileRole() && (!format.IsRelativePOSIXPath(input.Name) || input.Digest.SHA256 != testkit.FileDigest(t, h.root, input.Name)) {
					t.Errorf("%s: descriptor %+v", key, input)
				}
			}
		}
	})

	t.Run("the proof input is named by the consumed kind, names its criterion and digests the Statement file [verifies SP-410~2]", func(t *testing.T) {
		for key, statement := range statements {
			ref := statement.Predicate.Requirement
			criterion := format.SubjectName(ref.ID, ref.Revision)
			cached := cachedProofBytes(t, h.root, criterion)
			var proofs []format.Input
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleProof {
					proofs = append(proofs, input)
				}
			}
			if len(proofs) != 1 || !reflect.DeepEqual(proofs[0], format.ProofInput(format.KindDiscriminatingPower, criterion, cached)) {
				t.Errorf("%s: proof inputs %+v for the file of %s", key, proofs, criterion)
			}
		}
	})

	t.Run("a Statement of a test bound to one criterion lists no input of another criterion's proof [verifies SP-410~2]", func(t *testing.T) {
		for key, statement := range statements {
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleProof && input.Annotations.Criterion != statement.Subject[0].Name {
					t.Errorf("%s: proof input about %s", key, input.Annotations.Criterion)
				}
			}
		}
	})

	t.Run("the source of the confidence is a decision member and never a decider [verifies SP-409~1]", func(t *testing.T) {
		for key, statement := range statements {
			decision := statement.Predicate.Extensions.Decision
			if decision == nil || !decision.ConfidenceSource.IsKnown() {
				t.Errorf("%s: decision %+v", key, decision)
				continue
			}
			for _, decider := range decision.Deciders {
				if strings.ContainsAny(decider, ":(") {
					t.Errorf("%s: decider %q carries a source", key, decider)
				}
			}
			switch decision.Basis {
			case format.BasisSystemOne:
				if decision.ConfidenceSource != format.HostVotesSource(3) || decision.ConfidenceMapping != format.MappingModalShare {
					t.Errorf("%s: system one decision %+v", key, decision)
				}
			case format.BasisJudges:
				if decision.ConfidenceSource != format.HostVotesSource(5) || decision.ConfidenceMapping != format.MappingModalShare {
					t.Errorf("%s: judged decision %+v", key, decision)
				}
			}
		}
	})
}

// cachedProofBytes reads the discriminating-power Statement file of a criterion
// from the project's local cache.
func cachedProofBytes(t *testing.T, root, criterion string) []byte {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, filepath.FromSlash(powerCache), criterion))
	if err != nil || len(entries) != 1 {
		t.Fatalf("the cached proof of %s: %v (%d files)", criterion, err, len(entries))
	}
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(powerCache), criterion, entries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	return content
}
