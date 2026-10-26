// Package kotlin builds the masked input of a JUnit 5 test written in Kotlin: the
// test and what it reaches, in one class under neutral names, without comments,
// display names or citation tags, with the imports they use, and the signatures of
// the project declarations it names.
package kotlin

import (
	"errors"
	"fmt"
	"io/fs"
	"path"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

// testFileExtension is the extension of the files the masker serves.
const testFileExtension = ".kt"

// errInternal wraps a defect that surfaced as a panic while masking.
var errInternal = errors.New("kotlin masker failed unexpectedly")

var _ ports.Masker = (*Masker)(nil)

// Masker implements ports.Masker for JUnit 5 tests.
type Masker struct{}

// New returns a Masker.
func New() *Masker { return &Masker{} }

// Handles reports whether the file is a Kotlin file.
func (*Masker) Handles(file string) bool { return path.Ext(file) == testFileExtension }

// Mask builds the masked unit and the signatures of a bound test. It returns
// ports.ErrTestNotFound when the file declares no test the reported name
// designates, and masking.ErrLeak when the test cannot be masked because its code
// still spells out the words that name it. It never panics: a defect is returned as
// an error.
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
	used := planned.usedNames()
	var imports []importDeclaration
	for _, declaration := range index.importDeclarations() {
		if declaration.keep(used) {
			imports = append(imports, declaration)
		}
	}
	declared := map[string]bool{}
	for name := range planned.renames {
		declared[name] = true
	}
	masked := ports.MaskedTest{Unit: index.renderUnit(planned, imports), TestFiles: []string{test.File}}
	masked.Signatures, masked.CodeFiles = buildSignatures(project, signatureQuery{
		testFile: test.File, packageName: index.packageName(), imports: imports, used: used, declared: declared,
	})
	if err := ensureNoLeak(masked, located.info.Words()); err != nil {
		return ports.MaskedTest{}, fmt.Errorf("mask %q: %w", test.Label(), err)
	}
	return masked, nil
}

// ensureNoLeak refuses a masked test that still reveals one of the names its
// author gave it or carries the marker word.
func ensureNoLeak(masked ports.MaskedTest, names []string) error {
	for _, name := range names {
		if err := masking.Ensure(masked.Unit, name, nil); err != nil {
			return err
		}
		if err := masking.Ensure(masked.Signatures, name, nil); err != nil {
			return err
		}
	}
	return masking.Ensure(masked.Unit+masked.Signatures, "", nil)
}
