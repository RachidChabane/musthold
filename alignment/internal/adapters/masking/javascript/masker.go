// Package javascript builds the masked input of a JavaScript or TypeScript test
// written for Jest or Vitest: the test and what it reaches, with every title
// neutral and every declaration under a neutral name, without comments, and the
// signatures of the project modules it imports.
package javascript

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/masking"
)

// errInternal wraps a defect that surfaced as a panic while masking.
var errInternal = errors.New("javascript masker failed unexpectedly")

var _ ports.Masker = (*Masker)(nil)

// Masker implements ports.Masker for the tests of the Jest and Vitest runners.
type Masker struct{}

// New returns a Masker.
func New() *Masker { return &Masker{} }

// Handles reports whether the file is a JavaScript or TypeScript file.
func (*Masker) Handles(file string) bool {
	_, served := grammarOf(file)
	return served
}

// Mask builds the masked unit and the signatures of a bound test. It returns
// ports.ErrTestNotFound when the file declares no test the reported name
// designates, and masking.ErrLeak when the test cannot be masked because its code
// still spells out the words of its titles. It never panics: a defect is returned
// as an error.
func (m *Masker) Mask(project fs.FS, test criterion.BoundTest) (masked ports.MaskedTest, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			masked, err = ports.MaskedTest{}, fmt.Errorf("%w: %v", errInternal, recovered)
		}
	}()
	if !m.Handles(test.File) {
		return ports.MaskedTest{}, fmt.Errorf("%w: %s", ports.ErrUnsupportedFile, path.Ext(test.File))
	}
	return mask(project, test)
}

func mask(project fs.FS, test criterion.BoundTest) (ports.MaskedTest, error) {
	source, err := fs.ReadFile(project, test.File)
	if err != nil {
		return ports.MaskedTest{}, fmt.Errorf("read test file %q: %w", test.File, err)
	}
	index, err := indexFile(test.File, source)
	if err != nil {
		return ports.MaskedTest{}, fmt.Errorf("mask %q: %w", test.Label(), err)
	}
	defer index.Release()

	located, err := index.locate(test)
	if err != nil {
		return ports.MaskedTest{}, err
	}
	planned := index.planUnit(located)
	imports := index.usedImports(planned)
	masked := ports.MaskedTest{Unit: index.renderUnit(planned, imports), TestFiles: []string{test.File}}
	masked.Signatures, masked.CodeFiles = buildSignatures(project, test.File, append(imports, index.requiresOf(planned)...), planned.namespaceReads())
	if err := ensureNoLeak(masked, located.words); err != nil {
		return ports.MaskedTest{}, fmt.Errorf("mask %q: %w", test.Label(), err)
	}
	return masked, nil
}

// usedImports keeps the imports of the file that bind a name the unit uses,
// reduced to the names it uses.
func (x *fileIndex) usedImports(p plan) []importStatement {
	used := p.usedNames()
	var kept []importStatement
	for _, statement := range x.importStatements() {
		if reduced, ok := statement.keep(used); ok {
			kept = append(kept, reduced)
		}
	}
	return kept
}

// requiresOf lists the modules the declarations of the unit load with `require`.
func (x *fileIndex) requiresOf(p plan) []importStatement {
	var found []importStatement
	for _, pieces := range p.blocks {
		for _, chosen := range pieces {
			if chosen.member.kind == memberDeclaration {
				found = append(found, x.requireOf(chosen.member.statement)...)
			}
		}
	}
	return found
}

// ensureNoLeak refuses a masked test that still reveals the words of its titles or
// carries the marker word.
func ensureNoLeak(masked ports.MaskedTest, titleWords string) error {
	if err := masking.Ensure(masked.Unit, titleWords, nil); err != nil {
		return err
	}
	return masking.Ensure(masked.Signatures, titleWords, nil)
}
