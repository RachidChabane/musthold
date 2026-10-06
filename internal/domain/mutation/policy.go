package mutation

import (
	"fmt"
	"path"
	"strings"

	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
)

const (
	parentSegment = ".."
	rootSegment   = "."
	posixSep      = "/"
	windowsSep    = `\`
)

// Policy decides which files an edit may touch.
type Policy struct {
	// SourceRoots are the relative POSIX directories that hold source code; "."
	// stands for the whole project.
	SourceRoots []string
	// Layout tells test files from source files.
	Layout ecosystem.Layout
}

// Check refuses a path that is not relative POSIX, escapes the project, lies
// outside every source root or is a test file.
func (p Policy) Check(file string) error {
	if file == "" || path.IsAbs(file) || strings.Contains(file, windowsSep) || path.Clean(file) != file {
		return fmt.Errorf("%w: %q is not a clean relative POSIX path", ErrPathRefused, file)
	}
	if file == parentSegment || strings.HasPrefix(file, parentSegment+posixSep) {
		return fmt.Errorf("%w: %q escapes the project", ErrPathRefused, file)
	}
	if p.Layout.IsTestFile(file) {
		return fmt.Errorf("%w: %q is a test file", ErrPathRefused, file)
	}
	for _, root := range p.SourceRoots {
		if inside(root, file) {
			return nil
		}
	}
	return fmt.Errorf("%w: %q is outside the source roots", ErrPathRefused, file)
}

func inside(root, file string) bool {
	root = path.Clean(root)
	return root == rootSegment || file == root || strings.HasPrefix(file, root+posixSep)
}
