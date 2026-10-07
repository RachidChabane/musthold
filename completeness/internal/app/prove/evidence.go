package prove

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/justify"
	"github.com/RachidChabane/spec-proof/format"
)

// Names of the `proof` inputs: the consumed kind and the criterion, and for an
// alignment Statement the test too.
const (
	proofNameSeparator = ":"
	findingUnreadable  = "unreadable_proof"
)

// powerEvidence is what a usable discriminating-power Statement says about a
// criterion: the examples it ran, the files of its code tree and the mutants no
// criterion owns.
type powerEvidence struct {
	ref          criterion.Ref
	examples     []int
	files        []string
	unattributed []string
}

// loadEvidence reads, for every criterion, the discriminating-power and alignment
// Statements that are valid, state the criterion's current text and whose test and
// code inputs still match the project. A Statement that fails any of these is
// stale and is not read. What remains justifies changed units and raises
// questions about unconstrained behaviour.
func (p *passState) loadEvidence() error {
	p.powers = map[criterion.Ref]powerEvidence{}
	for _, c := range p.project.Criteria() {
		if err := p.loadPower(c); err != nil {
			return err
		}
		if err := p.loadAligned(c); err != nil {
			return err
		}
	}
	slices.SortFunc(p.proofInputs, compareInputs)
	return nil
}

func (p *passState) loadPower(c criterion.Criterion) error {
	candidates, err := p.deps.Evidence.Candidates(format.KindDiscriminatingPower, c.Ref)
	if err != nil {
		return fmt.Errorf("read the discriminating-power Statements of %s: %w", c.Ref, err)
	}
	for _, candidate := range candidates {
		statement, usable := p.readPower(c, candidate)
		if !usable {
			continue
		}
		evidence := powerEvidence{ref: c.Ref}
		for _, example := range statement.Examples {
			evidence.examples = append(evidence.examples, example.Index)
		}
		for _, input := range statement.Inputs {
			if input.Role == format.RoleCodeTree {
				evidence.files = append(evidence.files, input.Name)
			}
		}
		for _, mutant := range statement.Mutants {
			if mutant.Extensions != nil && mutant.Extensions.Unattributed {
				evidence.unattributed = append(evidence.unattributed, mutant.Digest)
			}
		}
		p.powers[c.Ref] = evidence
		p.proofInputs = append(p.proofInputs, proofInput(string(format.KindDiscriminatingPower)+proofNameSeparator+c.Ref.String(), candidate))
		if entries := p.probeEntries(c, evidence.examples); len(entries) > 0 {
			p.evidence = append(p.evidence, justify.Evidence{Ref: c.Ref, Entries: entries})
		}
		return nil
	}
	return nil
}

// probeEntries lists the function names the example probes of a criterion start
// from: the operations of the examples its discriminating-power Statement ran, and
// what the project configuration binds them to.
func (p *passState) probeEntries(c criterion.Criterion, ran []int) []string {
	var entries []string
	for _, example := range c.Examples {
		if !slices.Contains(ran, example.Index) {
			continue
		}
		entries = append(entries, example.Operation)
		for _, bound := range p.deps.Operations.Bound(example.Operation) {
			entries = append(entries, justify.OperationEntry(bound))
		}
	}
	slices.Sort(entries)
	return slices.Compact(entries)
}

func (p *passState) loadAligned(c criterion.Criterion) error {
	candidates, err := p.deps.Evidence.Candidates(format.KindAlignment, c.Ref)
	if err != nil {
		return fmt.Errorf("read the alignment Statements of %s: %w", c.Ref, err)
	}
	seen := map[string]bool{}
	for _, candidate := range candidates {
		statement, usable := p.readAlignment(c, candidate)
		if !usable || seen[statement.Test] {
			continue
		}
		seen[statement.Test] = true
		if statement.Verdict.Value != format.VerdictAligned {
			continue
		}
		name := string(format.KindAlignment) + proofNameSeparator + c.Ref.String() + proofNameSeparator + statement.Test
		p.proofInputs = append(p.proofInputs, proofInput(name, candidate))
		if entries := justify.TestEntries(statement.Test); len(entries) > 0 {
			p.evidence = append(p.evidence, justify.Evidence{Ref: c.Ref, Entries: entries})
		}
	}
	return nil
}

func proofInput(name string, file ports.ProofFile) format.Input {
	return format.Input{Role: format.RoleProof, Name: name, Digest: format.Digest{SHA256: format.SHA256Hex(file.Bytes)}}
}

func (p *passState) readPower(c criterion.Criterion, file ports.ProofFile) (format.DiscriminatingPower, bool) {
	decoded, err := format.DecodeAs[format.DiscriminatingPower](file.Bytes)
	if err != nil || decoded.Validate() != nil {
		p.addFinding(fmt.Sprintf("%s %s: a discriminating-power Statement of the criterion is not valid", findingUnreadable, c.Ref))
		return format.DiscriminatingPower{}, false
	}
	if !p.current(decoded.Subject, c, decoded.Predicate.Inputs) {
		return format.DiscriminatingPower{}, false
	}
	return decoded.Predicate, true
}

func (p *passState) readAlignment(c criterion.Criterion, file ports.ProofFile) (format.Alignment, bool) {
	decoded, err := format.DecodeAs[format.Alignment](file.Bytes)
	if err != nil || decoded.Validate() != nil {
		p.addFinding(fmt.Sprintf("%s %s: an alignment Statement of the criterion is not valid", findingUnreadable, c.Ref))
		return format.Alignment{}, false
	}
	if !p.current(decoded.Subject, c, decoded.Predicate.Inputs) {
		return format.Alignment{}, false
	}
	return decoded.Predicate, true
}

// current reports whether a Statement states the criterion's current text and
// every test and code file it rests on is still in the project with the content
// the Statement saw.
func (p *passState) current(subject []format.Subject, c criterion.Criterion, inputs []format.Input) bool {
	if len(subject) != 1 || subject[0].Digest.SHA256 != c.StatementDigest {
		return false
	}
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

// digestOfFile digests a file of the project, memoized for the pass.
func (p *passState) digestOfFile(name string) (string, bool) {
	if known, found := p.files[name]; found {
		return known.digest, known.ok
	}
	content, err := fs.ReadFile(p.deps.Project, path.Clean(name))
	entry := fileDigest{}
	if err == nil {
		entry = fileDigest{digest: format.SHA256Hex(content), ok: true}
	}
	p.files[name] = entry
	return entry.digest, entry.ok
}

// fileDigest is the digest of a project file, or the fact that it cannot be read.
type fileDigest struct {
	digest string
	ok     bool
}
