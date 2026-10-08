// Package app holds the use cases of the release pipeline. Each use case depends
// on the narrow ports below and never on a command, a file or a repository.
package app

import (
	"time"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

// RepositoryReader reads the version-control facts a release depends on.
type RepositoryReader interface {
	Head() (string, error)
	// TagCommit returns the commit a tag names, and false when the tag does not exist.
	TagCommit(tag string) (string, bool, error)
	DirtyPaths() ([]string, error)
	Tags() ([]string, error)
	CommitTime(commit string) (time.Time, error)
}

// HistoryReader reads the changes a release note lists.
type HistoryReader interface {
	// PreviousTag returns the latest tag of the module below the given tag, empty
	// when there is none.
	PreviousTag(module string, before domain.Tag) (string, error)
	// CommitsSince lists the commits from a tag (or the beginning) to HEAD that
	// touched any of the paths.
	CommitsSince(previous string, paths []string) ([]domain.Commit, error)
}

// FilePresence tells whether a file exists.
type FilePresence interface {
	Exists(path string) (bool, error)
}

// ModuleFiles reads the committed files the pre-flight rules inspect.
type ModuleFiles interface {
	FilePresence
	GoMod(module string) (domain.GoModFacts, error)
	Manifests(roots []string) ([]domain.ManifestFact, error)
	Marketplace() ([]domain.MarketplaceEntry, error)
	// Notice reads the NOTICE file of the repository root; a missing file is not an error.
	Notice() (domain.NoticeFacts, error)
}

// Checks runs the commands the pre-flight rules depend on and reports how they
// ended.
type Checks interface {
	Build(module string) domain.Outcome
	Test(module string) domain.Outcome
	// PluginGenerated runs the plugin generators and reports the files they change.
	PluginGenerated(module string) domain.Outcome
}

// Compiler cross-builds the binary of a module.
type Compiler interface {
	Compile(spec domain.BuildSpec) error
}

// Archiver packs the entries of an archive into the bytes of its container.
type Archiver interface {
	Pack(format domain.ArchiveFormat, entries []domain.ArchiveEntry, modTime time.Time) ([]byte, error)
}

// BinaryInspector reads the build information of a built binary.
type BinaryInspector interface {
	Inspect(binary []byte) (domain.BuildFacts, error)
}

// Files reads and writes the files of the release: sources, the staging area and
// the dist directory.
type Files interface {
	FilePresence
	ReadFile(path string) ([]byte, error)
	WriteFile(path string, data []byte, mode int64) error
	RemoveAll(path string) error
}

// DistReader reads a dist directory into the facts a check compares to the release.
type DistReader interface {
	ReadDist(dir string, tag domain.Tag, targets []domain.Target) (domain.DistFacts, error)
}
