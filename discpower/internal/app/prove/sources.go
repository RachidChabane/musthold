package prove

import (
	"fmt"
	"io/fs"
	"path"
	"slices"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

const (
	// stateDirectory is where spec-proof keeps its project state; it holds no source.
	stateDirectory = ".spec-proof"
	// versionControlDirectory is never source.
	versionControlDirectory = ".git"
	rootDirectory           = "."
)

// discoverSources lists the source files of the project: the files an engine
// handles that are not test files and lie outside the excluded, dependency and
// state directories. The files come sorted by path.
func discoverSources(project fs.FS, layout ecosystem.Layout, engines *operatorgen.Registry) ([]mutation.SourceFile, error) {
	skipped := slices.Concat(layout.ExcludedDirs, layout.DependencyDirs, []string{stateDirectory, versionControlDirectory})
	var files []mutation.SourceFile
	err := fs.WalkDir(project, rootDirectory, func(file string, entry fs.DirEntry, walkErr error) error {
		switch {
		case walkErr != nil:
			return walkErr
		case entry.IsDir() && file != rootDirectory && slices.Contains(skipped, path.Base(file)):
			return fs.SkipDir
		case entry.IsDir() || layout.IsTestFile(file):
			return nil
		}
		if _, err := engines.For(file); err != nil {
			return nil
		}
		content, err := fs.ReadFile(project, file)
		if err != nil {
			return fmt.Errorf("read %q: %w", file, err)
		}
		files = append(files, mutation.SourceFile{Path: file, Content: content})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover the source files: %w", err)
	}
	return files, nil
}
