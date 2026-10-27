package runs

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
)

// Tree lists the files under a directory of the host. A run reads only below its
// run directory (PREREG §7.3): the project's files to hash and digest, and the
// records to scan for the token.
type Tree interface {
	// Files lists, sorted, the paths relative to root of every file below it,
	// leaving out the directories named in skip wherever they are.
	Files(root string, skip []string) ([]string, error)
	// Read reads a file, or the target of a symbolic link, which is never
	// followed out of the run directory.
	Read(path string) ([]byte, error)
}

// OSTree is the host's file system.
type OSTree struct{}

// Files implements Tree. A symbolic link is listed as a file and not followed.
func (OSTree) Files(root string, skip []string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != root && slices.Contains(skip, entry.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		files = append(files, filepath.ToSlash(relative))
		return nil
	})
	slices.Sort(files)
	return files, err
}

// Read implements Tree.
func (OSTree) Read(path string) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, err := os.Readlink(path)
		return []byte(target), err
	}
	return os.ReadFile(path)
}
