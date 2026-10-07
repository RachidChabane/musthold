package prove

import (
	"fmt"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/matrix"
	"github.com/RachidChabane/spec-proof/format"
)

// inputNameMatrixEvidence names the input that stands for the discriminating-power
// Statements of the other criteria a test is bound to. A `proof` input is named by
// its kind alone, so only the Statement of the declared criterion is one; the
// others still reach the cache key through this descriptor.
const inputNameMatrixEvidence = "matrix-evidence"

// chosenProof is the discriminating-power Statement the module reads for one
// criterion: the Statement, its exact bytes, where it came from, and whether a
// usable one exists at all.
type chosenProof struct {
	statement format.DiscriminatingPower
	bytes     []byte
	origin    ports.ProofOrigin
	usable    bool
}

func (c chosenProof) digest() string { return format.SHA256Hex(c.bytes) }

// proofFor chooses the discriminating-power Statement of a criterion: the first,
// verified set before local cache, that is a valid Statement of the criterion's
// current text whose test and code inputs still match the project. A Statement
// that fails any of these is stale and is not read.
func (p *passState) proofFor(c criterion.Criterion) (*chosenProof, error) {
	if chosen, known := p.proofs[c.Ref]; known {
		return chosen, nil
	}
	chosen := &chosenProof{}
	p.proofs[c.Ref] = chosen
	candidates, err := p.deps.Power.Candidates(format.KindDiscriminatingPower, c.Ref)
	if err != nil {
		return nil, fmt.Errorf("read the discriminating-power Statements of %s: %w", c.Ref, err)
	}
	for _, candidate := range candidates {
		statement, usable := p.readProof(c, candidate)
		if usable {
			*chosen = chosenProof{statement: statement, bytes: candidate.Bytes, origin: candidate.Origin, usable: true}
			return chosen, nil
		}
	}
	return chosen, nil
}

func (p *passState) readProof(c criterion.Criterion, file ports.ProofFile) (format.DiscriminatingPower, bool) {
	decoded, err := format.DecodeAs[format.DiscriminatingPower](file.Bytes)
	if err != nil || decoded.Validate() != nil {
		p.addFinding(fmt.Sprintf("unreadable_proof %s: a discriminating-power Statement of the criterion is not valid", c.Ref))
		return format.DiscriminatingPower{}, false
	}
	subject := decoded.Subject
	if len(subject) != 1 || subject[0].Digest.SHA256 != c.StatementDigest {
		return format.DiscriminatingPower{}, false
	}
	if !p.inputsCurrent(decoded.Predicate.Inputs) {
		return format.DiscriminatingPower{}, false
	}
	return decoded.Predicate, true
}

// inputsCurrent reports whether every test and code file a Statement rests on is
// still in the project with the content the Statement saw.
func (p *passState) inputsCurrent(inputs []format.Input) bool {
	for _, input := range inputs {
		if input.Role != format.RoleTestSource && input.Role != format.RoleCodeTree {
			continue
		}
		digest, readable := p.digestOfFile(input.Name)
		if !readable || digest != input.Digest.SHA256 {
			return false
		}
	}
	return true
}

// testEvidence is what the discriminating-power Statements say about one test and
// one declared criterion: the matrix, and the `proof` inputs the verdict rests on.
type testEvidence struct {
	matrix matrix.Result
	// proofInputs are the inputs for the Statements the matrix was read from: the
	// `proof` input of the declared criterion's, and the evidence descriptor of the
	// other criteria's.
	proofInputs []format.Input
	// corroboration is the digest of the declared criterion's Statement, the
	// executed evidence a SPECIFIC or OVERBROAD matrix stands on.
	corroboration string
	// usable is true when the declared criterion has a usable Statement that
	// covers the test.
	usable bool
}

// evidenceFor reads the matrix of a test against a declared criterion from the
// Statements of the criteria the test is bound to. Without a usable Statement of
// the declared criterion that lists the test, the matrix is absent.
func (p *passState) evidenceFor(subject criterion.Subject, declared criterion.Criterion, policyThresholds matrix.Thresholds) (testEvidence, error) {
	result := testEvidence{matrix: matrix.Absence()}
	statements := map[criterion.Ref]format.DiscriminatingPower{}
	var inputs []format.Input
	var others []string
	test := subject.Test.ResultID()
	for _, ref := range subject.Declared {
		c, found := p.project.Find(ref)
		if !found {
			continue
		}
		chosen, err := p.proofFor(c)
		if err != nil {
			return testEvidence{}, err
		}
		if !chosen.usable || !evidence.Covers(chosen.statement, test) {
			continue
		}
		statements[ref] = chosen.statement
		if ref == declared.Ref {
			inputs = append(inputs, format.ProofInput(format.KindDiscriminatingPower, ref.String(), chosen.bytes))
			result.usable, result.corroboration = true, chosen.digest()
		} else {
			others = append(others, ref.String()+"="+chosen.digest())
		}
	}
	if !result.usable {
		return result, nil
	}
	if len(others) > 0 {
		slices.Sort(others)
		inputs = append(inputs, descriptor(format.RoleMutantSet, inputNameMatrixEvidence, format.SHA256Hex([]byte(strings.Join(others, responseSeparator)))))
	}
	result.matrix = matrix.Classify(evidence.FactsFor(test, statements), test, declared.Ref, policyThresholds)
	result.proofInputs = inputs
	return result, nil
}
