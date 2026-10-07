// Package atomicfile writes files so that a reader never observes a partial
// content: the bytes go to a temporary file of the same directory, are flushed
// to disk and then renamed over the target.
package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TempPrefix starts the name of every temporary file; readers that list a
// directory skip names with this prefix.
const TempPrefix = ".tmp-"

const tempPattern = TempPrefix + "*"

// ErrExists is returned by WriteNew when the target already exists.
var ErrExists = os.ErrExist

// IsTemp reports whether a directory entry name is a temporary file.
func IsTemp(name string) bool { return strings.HasPrefix(name, TempPrefix) }

// Write atomically replaces the file at path with data, giving it the mode.
func Write(path string, data []byte, mode os.FileMode) error {
	temp, err := stage(path, data, mode)
	if err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		_ = os.Remove(temp)
		return fmt.Errorf("rename into %s: %w", path, err)
	}
	return nil
}

// WriteNew atomically creates the file at path, or returns ErrExists when it is
// already there. The content appears whole or not at all.
func WriteNew(path string, data []byte, mode os.FileMode) error {
	temp, err := stage(path, data, mode)
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(temp) }()
	if err := os.Link(temp, path); err != nil {
		if errors.Is(err, os.ErrExist) {
			return fmt.Errorf("%s: %w", path, ErrExists)
		}
		return fmt.Errorf("link into %s: %w", path, err)
	}
	return nil
}

// stage writes data to a flushed temporary file next to path and returns its name.
func stage(path string, data []byte, mode os.FileMode) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), tempPattern)
	if err != nil {
		return "", fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	name := file.Name()
	if err := fill(file, data, mode); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("write %s: %w", path, err)
	}
	return name, nil
}

func fill(file *os.File, data []byte, mode os.FileMode) error {
	_, writeErr := file.Write(data)
	var chmodErr, syncErr error
	if writeErr == nil {
		chmodErr = file.Chmod(mode)
		syncErr = file.Sync()
	}
	return errors.Join(writeErr, chmodErr, syncErr, file.Close())
}
