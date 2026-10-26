// Package workspace manages the working copies mutants run in: a reference
// workspace copied once from the project, mutant workspaces cloned from the
// built reference, all under $TMPDIR/musthold-work/<run>/, outside the
// repository.
package workspace

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const (
	// RootDirName is the directory under the temporary directory that holds runs.
	RootDirName = "musthold-work"
	runPrefix   = "run-"

	referenceDirName = "reference"
	mutantsDirName   = "mutants"
	resultsDirName   = "results"

	directoryMode = 0o755
	// modifiedStep is how much later than the newest build output a written
	// file's modification time is set.
	modifiedStep = time.Second
)

// alwaysExcluded are directory names never copied: version control data.
var alwaysExcluded = []string{".git"}

// ErrOutsideWorkspace is returned for a file path that leaves the workspace.
var ErrOutsideWorkspace = errors.New("path outside the workspace")

var _ ports.Workspaces = (*Manager)(nil)

// Manager implements ports.Workspaces on the local file system.
type Manager struct {
	runDir  string
	cloners []Cloner
	counter atomic.Uint64
}

// Option configures a Manager.
type Option func(*Manager)

// WithCloners replaces the clone strategies, tried in order until one succeeds.
func WithCloners(cloners ...Cloner) Option {
	return func(m *Manager) { m.cloners = cloners }
}

// New creates the run directory under base, or under the temporary directory
// when base is empty, and returns a Manager that clones with the best strategy
// the platform offers.
func New(base string, opts ...Option) (*Manager, error) {
	if base == "" {
		base = os.TempDir()
	}
	root := filepath.Join(base, RootDirName)
	if err := os.MkdirAll(root, directoryMode); err != nil {
		return nil, fmt.Errorf("create %q: %w", root, err)
	}
	runDir, err := os.MkdirTemp(root, runPrefix)
	if err != nil {
		return nil, fmt.Errorf("create the run directory: %w", err)
	}
	m := &Manager{runDir: runDir, cloners: DefaultCloners()}
	for _, opt := range opts {
		opt(m)
	}
	return m, nil
}

// RunDir is the directory that holds every workspace of this run.
func (m *Manager) RunDir() string { return m.runDir }

// Close removes the whole run directory.
func (m *Manager) Close() error { return os.RemoveAll(m.runDir) }

// Prepare copies the project at source into the reference workspace.
func (m *Manager) Prepare(_ context.Context, source string, layout ecosystem.Layout) (ecosystem.Workspace, error) {
	workspace := ecosystem.Workspace{
		Dir:        filepath.Join(m.runDir, referenceDirName),
		ResultsDir: filepath.Join(m.runDir, resultsDirName, referenceDirName),
	}
	skip := excluder(layout)
	if err := copyTree(source, workspace.Dir, func(rel, name string) bool { return skip(name) }); err != nil {
		return ecosystem.Workspace{}, fmt.Errorf("copy %q: %w", source, err)
	}
	if err := os.MkdirAll(workspace.ResultsDir, directoryMode); err != nil {
		return ecosystem.Workspace{}, err
	}
	return workspace, nil
}

// Clone derives a mutant workspace from the reference with the first clone
// strategy that works, then removes the excluded directories from the clone.
func (m *Manager) Clone(ctx context.Context, reference ecosystem.Workspace, layout ecosystem.Layout) (ecosystem.Workspace, error) {
	name := strconv.FormatUint(m.counter.Add(1), 10)
	workspace := ecosystem.Workspace{
		Dir:        filepath.Join(m.runDir, mutantsDirName, name),
		ResultsDir: filepath.Join(m.runDir, resultsDirName, mutantsDirName, name),
	}
	if err := os.MkdirAll(filepath.Dir(workspace.Dir), directoryMode); err != nil {
		return ecosystem.Workspace{}, err
	}
	if err := m.cloneWith(ctx, reference.Dir, workspace.Dir, layout); err != nil {
		return ecosystem.Workspace{}, err
	}
	if err := pruneExcluded(workspace.Dir, layout); err != nil {
		return ecosystem.Workspace{}, err
	}
	if err := os.MkdirAll(workspace.ResultsDir, directoryMode); err != nil {
		return ecosystem.Workspace{}, err
	}
	return workspace, nil
}

func (m *Manager) cloneWith(ctx context.Context, source, destination string, layout ecosystem.Layout) error {
	var failures []error
	for _, cloner := range m.cloners {
		err := cloner.Clone(ctx, source, destination, layout)
		if err == nil {
			return nil
		}
		failures = append(failures, fmt.Errorf("%s: %w", cloner.Name(), err))
		if removeErr := os.RemoveAll(destination); removeErr != nil {
			failures = append(failures, removeErr)
		}
	}
	return fmt.Errorf("clone %q: %w", source, errors.Join(failures...))
}

// ReadFile reads a file of the workspace.
func (m *Manager) ReadFile(workspace ecosystem.Workspace, file string) ([]byte, error) {
	full, err := resolve(workspace, file)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(full)
}

// WriteFile replaces a file of the workspace. The file's modification time is
// set one step later than the newest file of the workspace outside the
// dependency directories, so that an incremental build or a bytecode cache
// keyed on modification times always sees the splice as newer.
func (m *Manager) WriteFile(workspace ecosystem.Workspace, layout ecosystem.Layout, file string, content []byte) error {
	full, err := resolve(workspace, file)
	if err != nil {
		return err
	}
	mode := fs.FileMode(0o644)
	if info, statErr := os.Stat(full); statErr == nil {
		mode = info.Mode().Perm()
	}
	newest, err := newestModification(workspace.Dir, layout)
	if err != nil {
		return err
	}
	if err := os.WriteFile(full, content, mode); err != nil {
		return err
	}
	stamp := time.Now()
	if newest.After(stamp) {
		stamp = newest
	}
	stamp = stamp.Add(modifiedStep)
	return os.Chtimes(full, stamp, stamp)
}

// Release removes the workspace and its results.
func (m *Manager) Release(workspace ecosystem.Workspace) error {
	return errors.Join(os.RemoveAll(workspace.Dir), os.RemoveAll(workspace.ResultsDir))
}

func resolve(workspace ecosystem.Workspace, file string) (string, error) {
	if file == "" || path.IsAbs(file) || path.Clean(file) != file || file == ".." || strings.HasPrefix(file, "../") {
		return "", fmt.Errorf("%w: %q", ErrOutsideWorkspace, file)
	}
	return filepath.Join(workspace.Dir, filepath.FromSlash(file)), nil
}

func excluder(layout ecosystem.Layout) func(name string) bool {
	names := append(append([]string{}, alwaysExcluded...), layout.ExcludedDirs...)
	return func(name string) bool {
		for _, excluded := range names {
			if name == excluded {
				return true
			}
		}
		return false
	}
}

func pruneExcluded(dir string, layout ecosystem.Layout) error {
	skip := excluder(layout)
	dependencies := dependencySet(layout)
	var found []string
	err := filepath.WalkDir(dir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() || current == dir {
			return nil
		}
		rel, _ := filepath.Rel(dir, current)
		if _, isDependency := dependencies[filepath.ToSlash(rel)]; isDependency {
			return filepath.SkipDir
		}
		if skip(entry.Name()) {
			found = append(found, current)
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, current := range found {
		if err := os.RemoveAll(current); err != nil {
			return err
		}
	}
	return nil
}

func newestModification(dir string, layout ecosystem.Layout) (time.Time, error) {
	dependencies := dependencySet(layout)
	var newest time.Time
	err := filepath.WalkDir(dir, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, current)
		if _, isDependency := dependencies[filepath.ToSlash(rel)]; isDependency && entry.IsDir() {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.ModTime().After(newest) {
			newest = info.ModTime()
		}
		return nil
	})
	return newest, err
}

func dependencySet(layout ecosystem.Layout) map[string]struct{} {
	set := make(map[string]struct{}, len(layout.DependencyDirs))
	for _, dependency := range layout.DependencyDirs {
		set[path.Clean(dependency)] = struct{}{}
	}
	return set
}
