// Package ports declares what the use cases of the module need from the outside
// world: the criteria source, the per-ecosystem masked input builders, the host
// store, the proof stores and the validators.
package ports

import (
	"errors"
	"io/fs"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
)

// MaskedTest is a test built blind for a judge.
type MaskedTest struct {
	// Unit is the test and the helpers it reaches, with the imports they use, under
	// neutral names, without comments, docstrings, citation markers or any other
	// mention of a criterion.
	Unit string
	// Signatures are the public signatures, without bodies, of the code under test
	// that the unit calls.
	Signatures string
	// TestFiles lists the project files the unit was built from, relative to the
	// project root with `/` separators, sorted.
	TestFiles []string
	// CodeFiles lists the project files the signatures were read from, sorted.
	CodeFiles []string
}

// Errors of a masker.
var (
	// ErrTestNotFound is returned when a bound test is not declared where the
	// report says it is.
	ErrTestNotFound = errors.New("the bound test is not declared in its file")
	// ErrUnsupportedFile is returned for a test file no masker serves.
	ErrUnsupportedFile = errors.New("no masker serves the test file")
)

// Masker builds the masked input of the tests of one ecosystem. It is a strategy:
// a new ecosystem adds a masker and changes nothing else.
type Masker interface {
	// Handles reports whether the masker serves the file, by its extension.
	Handles(file string) bool
	// Mask builds the masked test of a bound test from the project tree. The tree
	// is rooted at the project root.
	Mask(project fs.FS, test criterion.BoundTest) (MaskedTest, error)
}
