// Package fileset expands glob patterns over a file tree into the file set that
// the format package digests.
package fileset

import (
	"fmt"
	"io/fs"
	"slices"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/RachidChabane/spec-proof/format"
)

// Collect expands the doublestar patterns over fsys and returns the regular
// files they match, de-duplicated across patterns and sorted by path. Matched
// directories are skipped; their files are matched by patterns such as `dir/**`.
func Collect(fsys fs.FS, globs []string) ([]format.File, error) {
	paths := map[string]struct{}{}
	for _, pattern := range globs {
		matches, err := doublestar.Glob(fsys, pattern)
		if err != nil {
			return nil, fmt.Errorf("glob %q: %w", pattern, err)
		}
		for _, match := range matches {
			paths[match] = struct{}{}
		}
	}

	files := make([]format.File, 0, len(paths))
	for path := range paths {
		info, err := fs.Stat(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("stat %q: %w", path, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		content, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read %q: %w", path, err)
		}
		files = append(files, format.File{Path: path, Content: content})
	}
	slices.SortFunc(files, func(a, b format.File) int { return strings.Compare(a.Path, b.Path) })
	return files, nil
}
