// Package mutation holds the pure rules of a mutant: the edit format and the
// mutant id, the path policy, and the splice with its post-splice invariant.
// An EditSet replaces whole definitions or module-level statements of source
// files, and its canonical digest is the mutant id.
package mutation

import "errors"

// Errors a splice or a path check can return.
var (
	// ErrPathRefused is returned for a path outside the source roots, a test file
	// or a path that is not relative POSIX.
	ErrPathRefused = errors.New("path refused")
	// ErrUnknownDefinition is returned when no unit of the edit's target kind has
	// the qualname.
	ErrUnknownDefinition = errors.New("unknown definition")
	// ErrAmbiguousDefinition is returned when several units of the edit's target
	// kind share the qualname.
	ErrAmbiguousDefinition = errors.New("ambiguous definition")
	// ErrUnsupportedTarget is returned for an edit whose target kind the splicer
	// has no locator for.
	ErrUnsupportedTarget = errors.New("unsupported target kind")
	// ErrNameTaken is returned for a module addition whose name is already
	// declared in the file.
	ErrNameTaken = errors.New("name already declared")
	// ErrReparse is returned when the spliced source can no longer be located.
	ErrReparse = errors.New("spliced source does not parse")
	// ErrInvariantViolated is returned when the post-splice invariant does not hold.
	ErrInvariantViolated = errors.New("post-splice invariant violated")
)

// IsRefusal reports whether err is a path refusal, which is a caller error
// rather than a property of the mutant.
func IsRefusal(err error) bool { return errors.Is(err, ErrPathRefused) }
