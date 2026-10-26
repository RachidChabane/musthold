// Package python builds the masked input of a Python test: the test and the
// helpers it reaches under neutral names, without comments, docstrings or
// citation markers, with the imports they use and the signatures of the project
// code they import.
package python

import (
	"fmt"
	"io/fs"
	"path"
	"strings"

	"github.com/odvcencio/gotreesitter"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/masking"
)

const (
	// testFileExtension is the extension of the files the masker serves.
	testFileExtension = ".py"
)

var _ ports.Masker = (*Masker)(nil)

// Masker implements ports.Masker for pytest tests.
type Masker struct{}

// New returns a Masker.
func New() *Masker { return &Masker{} }

// Handles reports whether the file is a Python file.
func (*Masker) Handles(file string) bool { return path.Ext(file) == testFileExtension }

// Mask builds the masked unit and the signatures of a bound test. It returns
// ports.ErrTestNotFound when the file does not declare the test, and
// masking.ErrLeak when the test cannot be masked because its code still spells
// out its name.
func (*Masker) Mask(project fs.FS, test criterion.BoundTest) (ports.MaskedTest, error) {
	source, err := fs.ReadFile(project, test.File)
	if err != nil {
		return ports.MaskedTest{}, fmt.Errorf("read test file %q: %w", test.File, err)
	}
	index, err := indexFile(source)
	if err != nil {
		return ports.MaskedTest{}, fmt.Errorf("mask %q: %w", test.Label(), err)
	}
	defer index.release()

	bound, err := index.locate(test)
	if err != nil {
		return ports.MaskedTest{}, err
	}
	masked := index.build(project, test.File, index.planUnit(bound))
	if err := ensureNoLeak(masked, test.Name); err != nil {
		return ports.MaskedTest{}, fmt.Errorf("mask %q: %w", test.Label(), err)
	}
	masked.TestFiles = []string{test.File}
	return masked, nil
}

// locate finds the declaration of the bound test: a method of the class its
// classname ends with, a module-level function, or a method of any class, in
// that order.
func (x *fileIndex) locate(test criterion.BoundTest) (target, error) {
	name := masking.NameWithoutParameters(test.Name)
	if class, found, ok := x.findClassOf(test.Classname, name); ok {
		return x.methodTarget(class, found), nil
	}
	if found, ok := x.findFunction(name); ok {
		return target{piece: x.testPiece(modulePiece, name, found.order, found.statement)}, nil
	}
	if class, found, ok := x.findAnyMethod(name); ok {
		return x.methodTarget(class, found), nil
	}
	return target{}, fmt.Errorf("%w: %s in %s", ports.ErrTestNotFound, test.Label(), test.File)
}

func (x *fileIndex) methodTarget(class classScope, found method) target {
	return target{class: &class, piece: x.testPiece(methodPiece, found.name, found.order, found.statement)}
}

// build writes the unit and its signatures.
func (x *fileIndex) build(project fs.FS, testFile string, p plan) ports.MaskedTest {
	pieces := p.pieces()
	names := newRenamer(p)
	blocks := make([]string, 0, len(pieces)+1)
	kept := x.usedImports(usedNames(pieces))
	blocks = append(blocks, importBlock(kept))
	for _, chosen := range pieces {
		blocks = append(blocks, x.renderPiece(chosen, names))
	}
	signatures, codeFiles := buildSignatures(project, testFile, kept, attributeReads(pieces))
	return ports.MaskedTest{Unit: join(blocks), Signatures: signatures, CodeFiles: codeFiles}
}

// usedImports keeps the imports of the file that bind a name the unit uses,
// reduced to the aliases it uses.
func (x *fileIndex) usedImports(used map[string]bool) []importStatement {
	var kept []importStatement
	for _, statement := range x.importStatements() {
		if reduced, ok := statement.keep(used); ok {
			kept = append(kept, reduced)
		}
	}
	return kept
}

func importBlock(imports []importStatement) string {
	lines := make([]string, len(imports))
	for index, statement := range imports {
		lines[index] = statement.String()
	}
	return strings.Join(lines, lineSeparator)
}

func (x *fileIndex) testPiece(kind pieceKind, name string, order int, statement *gotreesitter.Node) piece {
	return piece{kind: kind, name: name, order: order, statement: statement, fragment: x.scan(statement)}
}

// ensureNoLeak refuses a masked test that still reveals its name or carries the
// marker word.
func ensureNoLeak(masked ports.MaskedTest, testName string) error {
	if err := masking.Ensure(masked.Unit, testName, nil); err != nil {
		return err
	}
	return masking.Ensure(masked.Signatures, testName, nil)
}
