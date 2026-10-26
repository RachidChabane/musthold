package prove

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
	"github.com/RachidChabane/musthold/alignment/internal/domain/prompt"
	"github.com/RachidChabane/musthold/alignment/internal/domain/verdict"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
)

// passState is what one pass shares across its tests.
type passState struct {
	deps         Deps
	settings     Settings
	project      criterion.Project
	policyDigest string
	options      Options
	bridge       *hostbridge.Bridge
	findings     []string
	proofs       map[criterion.Ref]*chosenProof
	files        map[string]fileDigest
	// proofPending holds the criteria the discriminating-power module waits on.
	proofPending map[string]bool
}

// fileDigest is the digest of a project file, or the fact that it cannot be read.
type fileDigest struct {
	digest string
	ok     bool
}

// loadProofPending reads which criteria the discriminating-power module still
// waits on, so that a verdict that rests on its proof is not written before it.
func (p *passState) loadProofPending() error {
	pending, err := p.deps.Pending.Read(discriminatingPowerModule)
	if err != nil {
		return fmt.Errorf("read the discriminating-power pending file: %w", err)
	}
	p.proofPending = map[string]bool{}
	for _, entry := range pending.Pending {
		p.proofPending[entry.Criterion] = true
	}
	return nil
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

func (p *passState) addFinding(text string) { p.findings = append(p.findings, text) }

// maskerFor finds the masker that serves a test file.
func (p *passState) maskerFor(file string) (ports.Masker, bool) {
	for _, masker := range p.deps.Maskers {
		if masker.Handles(file) {
			return masker, true
		}
	}
	return nil, false
}

// maskFailure is why no masked input could be built for a test.
type maskFailure struct {
	reason string
	detail string
}

// buildInput masks the test and shows it, with every criterion under a token, to
// the judges. It fails, with the reason, when the test cannot be shown blind.
func (p *passState) buildInput(test criterion.BoundTest) (ports.MaskedTest, prompt.TestInput, *maskFailure) {
	masker, found := p.maskerFor(test.File)
	if !found {
		return ports.MaskedTest{}, prompt.TestInput{}, &maskFailure{reason: verdict.ReasonUnreadableSource, detail: ports.ErrUnsupportedFile.Error() + ": " + test.File}
	}
	masked, err := masker.Mask(p.deps.Project, test)
	switch {
	case errors.Is(err, masking.ErrLeak):
		return ports.MaskedTest{}, prompt.TestInput{}, &maskFailure{reason: verdict.ReasonMaskedInputLeaks, detail: err.Error()}
	case err != nil:
		return ports.MaskedTest{}, prompt.TestInput{}, &maskFailure{reason: verdict.ReasonUnreadableSource, detail: err.Error()}
	}
	refs := p.project.Refs()
	for _, item := range []string{masked.Unit, masked.Signatures} {
		if err := masking.Ensure(item, test.Name, refs); err != nil {
			return ports.MaskedTest{}, prompt.TestInput{}, &maskFailure{reason: verdict.ReasonMaskedInputLeaks, detail: err.Error()}
		}
	}
	tokenized := masking.Tokenize(p.project.Criteria, test.Label())
	return masked, prompt.TestInput{Criteria: tokenized, Unit: masked.Unit, Signatures: masked.Signatures}, nil
}
