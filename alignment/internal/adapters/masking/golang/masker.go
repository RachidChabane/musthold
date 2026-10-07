// Package golang masks Go tests for a judge. It is the Go implementation of the
// ports.Masker strategy: it reads a test and the helpers it reaches with the
// standard go/parser, renames them to neutral names, strips every comment and
// citation, and renders the signatures of the code under test.
package golang

import (
	"errors"
	"fmt"
	"go/ast"
	"go/token"
	"io/fs"
	"path"
	"sort"
	"strings"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

// errInternal wraps a defect that surfaced as a panic while masking.
var errInternal = errors.New("go masker failed unexpectedly")

// Masker builds the masked input of Go tests. It has no state: every call reads
// the project tree it is given.
type Masker struct{}

var _ ports.Masker = (*Masker)(nil)

// New returns a Go masker.
func New() *Masker { return &Masker{} }

// Handles reports whether the file is a Go test file.
func (*Masker) Handles(file string) bool { return strings.HasSuffix(file, testFileSuffix) }

// Mask builds the masked test of a bound test from the project tree. It never
// panics: a defect is returned as an error.
func (m *Masker) Mask(project fs.FS, test criterion.BoundTest) (masked ports.MaskedTest, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			masked, err = ports.MaskedTest{}, fmt.Errorf("%w: %v", errInternal, recovered)
		}
	}()
	if !m.Handles(test.File) {
		return ports.MaskedTest{}, fmt.Errorf("%w: %s", ports.ErrUnsupportedFile, test.File)
	}
	return mask(project, test)
}

func mask(project fs.FS, test criterion.BoundTest) (ports.MaskedTest, error) {
	fset := token.NewFileSet()
	pkg, err := loadTestPackage(fset, project, test.File)
	if err != nil {
		return ports.MaskedTest{}, err
	}
	target := parseTestName(test.Name)
	functions := newCatalog(pkg.files)
	root, err := functions.findTest(pkg.target, target.function)
	if err != nil {
		return ports.MaskedTest{}, fmt.Errorf("%w: %s in %s", ports.ErrTestNotFound, target.function, test.File)
	}
	subtest := selectSubtest(root.node.(*ast.FuncDecl), target.subtestKey())
	included := functions.reachableFrom(root)

	signatures, codeFiles, err := newSignatureBuilder(fset, project, pkg, functions).build(included)
	if err != nil {
		return ports.MaskedTest{}, err
	}
	imports := collectImports(included)
	renameDeclarations(root, included)
	neutralizeLiterals(included, subtest, test.Name)
	unit, err := renderUnit(fset, imports, included)
	if err != nil {
		return ports.MaskedTest{}, err
	}
	return ports.MaskedTest{
		Unit:       unit,
		Signatures: signatures,
		TestFiles:  sourcePaths(included),
		CodeFiles:  codeFiles,
	}, nil
}

func sourcePaths(decls []*declaration) []string {
	seen := map[string]bool{}
	for _, decl := range decls {
		seen[decl.file.path] = true
	}
	return sortedKeys(seen)
}

func sortedKeys(set map[string]bool) []string {
	var keys []string
	for key := range set {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// directoryOf is the project directory of a project-relative file path.
func directoryOf(file string) string { return path.Dir(path.Clean(file)) }
