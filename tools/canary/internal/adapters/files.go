package adapters

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const (
	directoryMode  os.FileMode = 0o755
	fileMode       os.FileMode = 0o644
	executableMode os.FileMode = 0o755

	// firstLineBuffer is the read size of FirstLine: the first line of a rollout
	// holds the thread's identity and fits in it.
	firstLineBuffer = 1 << 20
)

// skippedDirectories are the caches a project copy leaves out.
var skippedDirectories = map[string]bool{"__pycache__": true, ".pytest_cache": true, "test-results": true}

// OSFiles is the operating system's file system.
type OSFiles struct{}

// MakeDir implements app.Files.
func (OSFiles) MakeDir(parent, pattern string) (string, error) {
	if err := os.MkdirAll(parent, directoryMode); err != nil {
		return "", err
	}
	dir, err := os.MkdirTemp(parent, pattern)
	if err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(dir)
}

// MkdirAll implements app.Files.
func (OSFiles) MkdirAll(dir string) error { return os.MkdirAll(dir, directoryMode) }

// CopyTree implements app.Files.
func (OSFiles) CopyTree(from, to string) error {
	return filepath.WalkDir(from, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(from, path)
		if err != nil {
			return err
		}
		target := filepath.Join(to, relative)
		if entry.IsDir() {
			if skippedDirectories[entry.Name()] {
				return fs.SkipDir
			}
			return os.MkdirAll(target, directoryMode)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, content, fileMode)
	})
}

// Write implements app.Files.
func (OSFiles) Write(path string, data []byte, executable bool) error {
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		return err
	}
	mode := fileMode
	if executable {
		mode = executableMode
	}
	return os.WriteFile(path, data, mode)
}

// Replace implements app.Files.
func (OSFiles) Replace(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chmod(path, info.Mode().Perm())
}

// FirstLine implements app.Files.
func (OSFiles) FirstLine(path string) ([]byte, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	reader := bufio.NewReaderSize(file, firstLineBuffer)
	line, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return bytes.TrimRight(line, "\r\n"), nil
}

// Read implements app.Files.
func (OSFiles) Read(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return data, err
}

// Exists implements app.Files.
func (OSFiles) Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Fingerprint implements app.Files.
func (OSFiles) Fingerprint(path string, depth int) (map[string]string, error) {
	found := map[string]string{}
	err := filepath.WalkDir(path, func(entry string, info fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, fs.ErrNotExist) || errors.Is(walkErr, fs.ErrPermission) {
				return nil
			}
			return walkErr
		}
		relative, err := filepath.Rel(path, entry)
		if err != nil {
			return err
		}
		levels := 0
		if relative != "." {
			levels = strings.Count(relative, string(filepath.Separator)) + 1
		}
		stat, err := info.Info()
		if err != nil {
			return nil
		}
		found[entry] = fmt.Sprintf("%d %d", stat.Size(), stat.ModTime().UnixNano())
		if info.IsDir() && levels >= depth {
			return fs.SkipDir
		}
		return nil
	})
	return found, err
}

// Glob implements app.Files.
func (OSFiles) Glob(pattern string) ([]string, error) { return filepath.Glob(pattern) }
