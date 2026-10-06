// Package mutation holds the pure rules for applying a mutant to a source file:
// the edit format, the path policy and the splice with its post-splice
// invariant.
package mutation

import (
	"errors"
	"slices"
)

// Errors a splice or a path check can return.
var (
	// ErrPathRefused is returned for a path outside the source roots, a test file
	// or a path that is not relative POSIX.
	ErrPathRefused = errors.New("path refused")
	// ErrUnknownDefinition is returned when no definition has the qualname.
	ErrUnknownDefinition = errors.New("unknown definition")
	// ErrAmbiguousDefinition is returned when several definitions share the qualname.
	ErrAmbiguousDefinition = errors.New("ambiguous definition")
	// ErrReparse is returned when the spliced source can no longer be located.
	ErrReparse = errors.New("spliced source does not parse")
	// ErrInvariantViolated is returned when the post-splice invariant does not hold.
	ErrInvariantViolated = errors.New("post-splice invariant violated")
)

// Edit replaces the definition named Qualname in File with NewSource.
type Edit struct {
	File      string
	Qualname  string
	NewSource string
}

// EditSet is the unit a mutant compiles to.
type EditSet struct {
	Edits []Edit
}

// Files lists the distinct files the set edits, in first-seen order.
func (s EditSet) Files() []string {
	var files []string
	for _, edit := range s.Edits {
		if !slices.Contains(files, edit.File) {
			files = append(files, edit.File)
		}
	}
	return files
}

// EditsOf returns the edits of one file, in order.
func (s EditSet) EditsOf(file string) []Edit {
	var edits []Edit
	for _, edit := range s.Edits {
		if edit.File == file {
			edits = append(edits, edit)
		}
	}
	return edits
}

// IsRefusal reports whether err is a path refusal, which is a caller error
// rather than a property of the mutant.
func IsRefusal(err error) bool { return errors.Is(err, ErrPathRefused) }
