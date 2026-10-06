package workspace

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

// Cloner is a strategy for deriving a workspace from another.
type Cloner interface {
	Name() string
	// Clone copies the tree at source into the not yet existing destination.
	Clone(ctx context.Context, source, destination string, layout ecosystem.Layout) error
}

// Operating systems with a copy-on-write copy command.
const (
	osDarwin = "darwin"
	osLinux  = "linux"
	copyTool = "cp"
)

// DefaultCloners returns the platform's copy-on-write cloner when one exists,
// then the plain copy that links dependency directories.
func DefaultCloners() []Cloner {
	switch runtime.GOOS {
	case osDarwin:
		return []Cloner{CommandCloner{label: "apfs-clonefile", args: []string{"-c", "-R"}}, PlainCloner{}}
	case osLinux:
		return []Cloner{CommandCloner{label: "reflink", args: []string{"--reflink=always", "-R"}}, PlainCloner{}}
	default:
		return []Cloner{PlainCloner{}}
	}
}

// CommandCloner clones the whole tree with `cp` and a copy-on-write flag:
// APFS clonefile on macOS, reflink on Linux. It fails where the file system
// cannot clone.
type CommandCloner struct {
	label string
	args  []string
}

// Name identifies the strategy.
func (c CommandCloner) Name() string { return c.label }

// Clone runs `cp <flags> source destination`.
func (c CommandCloner) Clone(ctx context.Context, source, destination string, _ ecosystem.Layout) error {
	args := append(append([]string{}, c.args...), source, destination)
	if output, err := exec.CommandContext(ctx, copyTool, args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s", err, output)
	}
	return nil
}

// PlainCloner copies every file and symlinks the dependency directories to the
// reference's, which is always possible.
type PlainCloner struct{}

// Name identifies the strategy.
func (PlainCloner) Name() string { return "plain-copy" }

// Clone copies the tree except the dependency directories, which it links.
func (PlainCloner) Clone(_ context.Context, source, destination string, layout ecosystem.Layout) error {
	dependencies := dependencySet(layout)
	skip := func(rel, _ string) bool {
		_, isDependency := dependencies[rel]
		return isDependency
	}
	if err := copyTree(source, destination, skip); err != nil {
		return err
	}
	for dependency := range dependencies {
		target := filepath.Join(source, filepath.FromSlash(dependency))
		if _, err := os.Stat(target); err != nil {
			continue
		}
		link := filepath.Join(destination, filepath.FromSlash(dependency))
		if err := os.MkdirAll(filepath.Dir(link), directoryMode); err != nil {
			return err
		}
		if err := os.Symlink(target, link); err != nil {
			return err
		}
	}
	return nil
}

// copyTree copies the tree at source to destination, keeping modes and
// modification times. skip is asked about every directory, by its path relative
// to source (slash separated) and its name, and prunes it when true.
func copyTree(source, destination string, skip func(rel, name string) bool) error {
	return filepath.WalkDir(source, func(current string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, current)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, rel)
		switch {
		case entry.IsDir():
			if rel != "." && skip(filepath.ToSlash(rel), entry.Name()) {
				return filepath.SkipDir
			}
			info, err := entry.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		case entry.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(current)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case entry.Type().IsRegular():
			return copyFile(current, target)
		default:
			return nil
		}
	})
}

func copyFile(source, target string) error {
	info, err := os.Stat(source)
	if err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Chtimes(target, info.ModTime(), info.ModTime())
}
