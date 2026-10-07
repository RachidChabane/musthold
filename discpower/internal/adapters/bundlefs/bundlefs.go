// Package bundlefs reads and writes the files of a host bundle in a directory.
package bundlefs

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostbundle"
)

const (
	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
	// MaxFileBytes bounds one file of a bundle.
	MaxFileBytes = 4 << 20
)

// Read returns the regular files under dir keyed by their slash-separated path
// relative to it. A file larger than MaxFileBytes is an error.
func Read(dir string) (map[string][]byte, error) {
	files := map[string][]byte{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if info.Size() > MaxFileBytes {
			return fmt.Errorf("%s is larger than %d bytes", path, MaxFileBytes)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(relative)] = content
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the bundle in %s: %w", dir, err)
	}
	return files, nil
}

// Write replaces the content of dir with the files.
func Write(dir string, files []hostbundle.File) error {
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for _, file := range files {
		target := filepath.Join(dir, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return err
		}
		if err := os.WriteFile(target, file.Content, fileMode); err != nil {
			return err
		}
	}
	return nil
}
