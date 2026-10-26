package prove

import (
	"os"
	"reflect"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/matrix"
	"github.com/RachidChabane/musthold/alignment/internal/testkit"
	"github.com/RachidChabane/musthold/format"
)

// powerSource serves the Statements it was given, by criterion, as the verified set.
type powerSource map[criterion.Ref][]byte

func (s powerSource) Candidates(_ format.Kind, ref criterion.Ref) ([]ports.ProofFile, error) {
	if bytes, found := s[ref]; found {
		return []ports.ProofFile{{Origin: ports.OriginVerified, Bytes: bytes}}, nil
	}
	return nil, nil
}

// twoCriteriaFixture is a test bound to two criteria, each with a
// discriminating-power Statement that lists the test.
func twoCriteriaFixture(t *testing.T) (*passState, criterion.Subject, map[criterion.Ref][]byte) {
	t.Helper()
	root := testkit.FixturesDir() + "/" + testkit.PythonParcel
	statements := map[criterion.Ref][]byte{}
	project := criterion.Project{}
	test := criterion.BoundTest{Classname: "tests.test_pricing", Name: "test_line_total_multiplies", File: "tests/test_pricing.py", Line: 5}
	for _, ref := range []criterion.Ref{testkit.Multiply, testkit.Surcharge} {
		digest := testkit.MultiplyDigest
		if ref == testkit.Surcharge {
			digest = testkit.SurchargeDigest
		}
		power := testkit.Power{
			Ref: ref, StatementDigest: digest, Tests: []string{testkit.PyMultiplies},
			Mutants:   []testkit.Mutant{{Digest: "m1", Kind: testkit.ValidJudged, Killers: []string{testkit.PyMultiplies}}, {Digest: "s1", Kind: testkit.Sentinel}},
			TestFiles: []string{"tests/test_pricing.py"}, CodeFiles: []string{"src/parcel/pricing.py"},
		}
		statements[ref] = power.Statement(t, root)
		project.Criteria = append(project.Criteria, criterion.Criterion{Ref: ref, StatementDigest: digest, Tests: []criterion.BoundTest{test}})
	}
	pass := &passState{
		deps:    Deps{Power: powerSource(statements), Project: os.DirFS(root)},
		project: project,
		proofs:  map[criterion.Ref]*chosenProof{},
		files:   map[string]fileDigest{},
	}
	return pass, criterion.Subject{Test: test, Declared: []criterion.Ref{testkit.Multiply, testkit.Surcharge}}, statements
}

func TestEvidenceInputs(t *testing.T) {
	pass, subject, statements := twoCriteriaFixture(t)
	declared, _ := pass.project.Find(testkit.Multiply)
	got, err := pass.evidenceFor(subject, declared, matrix.Thresholds{Own: 0.5, Other: 0.2, None: 0.2})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the Statement of the declared criterion is the one proof input, by the kind, the criterion and the file bytes [verifies SP-410~2]", func(t *testing.T) {
		var proofs []format.Input
		for _, input := range got.proofInputs {
			if input.Role == format.RoleProof {
				proofs = append(proofs, input)
			}
		}
		want := format.ProofInput(format.KindDiscriminatingPower, testkit.Multiply.String(), statements[testkit.Multiply])
		if len(proofs) != 1 || !reflect.DeepEqual(proofs[0], want) {
			t.Fatalf("proof inputs %+v, want %+v", proofs, want)
		}
	})

	t.Run("the Statement of another criterion the test is bound to reaches the cache key through the matrix evidence [verifies SP-410~2]", func(t *testing.T) {
		var evidence []format.Input
		for _, input := range got.proofInputs {
			if input.Name == inputNameMatrixEvidence {
				evidence = append(evidence, input)
			}
		}
		if len(evidence) != 1 || evidence[0].Role != format.RoleMutantSet || !format.IsSHA256Hex(evidence[0].Digest.SHA256) {
			t.Fatalf("evidence inputs %+v", evidence)
		}
		before := evidence[0].Digest.SHA256

		changed, _, changedStatements := twoCriteriaFixture(t)
		changedStatements[testkit.Surcharge] = append(changedStatements[testkit.Surcharge], '\n')
		changed.deps.Power = powerSource(changedStatements)
		again, err := changed.evidenceFor(subject, declared, matrix.Thresholds{Own: 0.5, Other: 0.2, None: 0.2})
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range again.proofInputs {
			if input.Name == inputNameMatrixEvidence && input.Digest.SHA256 == before {
				t.Fatal("another criterion's Statement changed and the matrix evidence did not")
			}
		}
	})

	t.Run("the inputs of a test bound to one criterion hold no matrix evidence [verifies SP-410~2]", func(t *testing.T) {
		alone, _, _ := twoCriteriaFixture(t)
		single := criterion.Subject{Test: subject.Test, Declared: []criterion.Ref{testkit.Multiply}}
		result, err := alone.evidenceFor(single, declared, matrix.Thresholds{Own: 0.5, Other: 0.2, None: 0.2})
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range result.proofInputs {
			if input.Name == inputNameMatrixEvidence {
				t.Fatalf("unexpected evidence input %+v", input)
			}
		}
	})
}
