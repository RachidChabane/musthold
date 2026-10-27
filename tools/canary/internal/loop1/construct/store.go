package construct

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ErrBadOutput is returned for a set of files that cannot be written as asked.
var ErrBadOutput = errors.New("the files cannot be used")

const (
	storeDirMode  = 0o755
	storeFileMode = 0o644
	indent        = "  "
)

// Directories of a task that the author writes.
var authorEntries = []string{study.TaskFile, accept.FixtureSubdir, accept.ReferenceSubdir, accept.ViolationsSubdir, accept.RefactorsSubdir}

// Directories that hold variants of the reference.
var variantEntries = []string{accept.ViolationsSubdir, accept.RefactorsSubdir}

var ignoredNames = map[string]bool{"__pycache__": true, ".pytest_cache": true, "test-results": true}

// Store reads and writes the files of tasks under a repository root.
type Store struct{ Repo string }

func (s Store) abs(relative string) string {
	return filepath.Join(s.Repo, filepath.FromSlash(relative))
}

func (s Store) taskPath(name string, parts ...string) string {
	return s.abs(path.Join(append([]string{accept.TaskDir(name)}, parts...)...))
}

// Exists reports whether a task has a directory.
func (s Store) Exists(name string) bool {
	info, err := os.Stat(s.taskPath(name))
	return err == nil && info.IsDir()
}

// Complete reports whether a task holds its description and its tests.
func (s Store) Complete(name string) bool {
	for _, file := range []string{study.TaskFile, path.Join(accept.OracleSubdir, accept.OracleConfigFile)} {
		if _, err := os.Stat(s.taskPath(name, file)); err != nil {
			return false
		}
	}
	return true
}

// Remove deletes a task's directory.
func (s Store) Remove(name string) error {
	if err := os.RemoveAll(s.taskPath(name)); err != nil {
		return fmt.Errorf("remove the task %s: %w", name, err)
	}
	return nil
}

// ReadFiles lists the files under a directory of the repository, their paths
// relative to it and given a prefix. Caches a run leaves behind are skipped.
func (s Store) ReadFiles(dir, prefix string) ([]File, error) {
	root := s.abs(dir)
	var files []File
	err := filepath.WalkDir(root, func(location string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if ignoredNames[entry.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		content, err := os.ReadFile(location)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, location)
		if err != nil {
			return err
		}
		files = append(files, File{Path: path.Join(prefix, filepath.ToSlash(relative)), Content: string(content)})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read the files under %s: %w", dir, err)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Read returns one file of a task.
func (s Store) Read(name, relative string) ([]byte, error) {
	return os.ReadFile(s.taskPath(name, relative))
}

// WriteAuthor replaces the author's files of a task with a new set and writes
// the task description, whose standard members the harness fills.
func (s Store) WriteAuthor(name string, files []File, task study.Task) error {
	if err := s.clear(name, authorEntries); err != nil {
		return err
	}
	for _, file := range files {
		if file.Path != study.TaskFile {
			if err := s.write(name, file.Path, file.Content); err != nil {
				return err
			}
		}
	}
	return s.WriteTask(task)
}

// WriteTask writes a task's description.
func (s Store) WriteTask(task study.Task) error {
	encoded, err := json.MarshalIndent(task, "", indent)
	if err != nil {
		return fmt.Errorf("encode the task %s: %w", task.Name, err)
	}
	return s.write(task.Name, study.TaskFile, string(encoded)+"\n")
}

// WriteVariants replaces the violations and refactors of a task.
func (s Store) WriteVariants(name string, files []File) error {
	if err := s.clear(name, variantEntries); err != nil {
		return err
	}
	for _, file := range files {
		if err := s.write(name, file.Path, file.Content); err != nil {
			return err
		}
	}
	return nil
}

// WriteTests replaces the oracle of a task.
func (s Store) WriteTests(name string, files []File) error {
	if err := s.clear(name, []string{accept.OracleSubdir}); err != nil {
		return err
	}
	for _, file := range files {
		if err := s.write(name, path.Join(accept.OracleSubdir, file.Path), file.Content); err != nil {
			return err
		}
	}
	return nil
}

func (s Store) clear(name string, entries []string) error {
	for _, entry := range entries {
		if err := os.RemoveAll(s.taskPath(name, entry)); err != nil {
			return fmt.Errorf("clear %s of %s: %w", entry, name, err)
		}
	}
	return nil
}

func (s Store) write(name, relative, content string) error {
	location := s.taskPath(name, relative)
	if err := os.MkdirAll(filepath.Dir(location), storeDirMode); err != nil {
		return fmt.Errorf("create the directory of %s: %w", relative, err)
	}
	if err := os.WriteFile(location, []byte(content), storeFileMode); err != nil {
		return fmt.Errorf("write %s: %w", relative, err)
	}
	return nil
}

// checkPaths refuses paths that are empty, absolute, unclean or repeated.
func checkPaths(files []File) error {
	seen := map[string]bool{}
	for _, file := range files {
		clean := path.Clean(file.Path)
		switch {
		case file.Path == "" || clean != file.Path || path.IsAbs(file.Path) || strings.HasPrefix(clean, "../") || clean == "..":
			return fmt.Errorf("%w: %q is not a clean relative path", ErrBadOutput, file.Path)
		case seen[file.Path]:
			return fmt.Errorf("%w: %s is written twice", ErrBadOutput, file.Path)
		}
		seen[file.Path] = true
	}
	return nil
}

// topLevel returns the first element of a slash path.
func topLevel(file string) string {
	first, _, _ := strings.Cut(file, "/")
	return first
}

func checkEntries(files []File, allowed []string) error {
	if err := checkPaths(files); err != nil {
		return err
	}
	for _, file := range files {
		if !slices.Contains(allowed, topLevel(file.Path)) || (file.Path == topLevel(file.Path) && file.Path != study.TaskFile) {
			return fmt.Errorf("%w: %s lies outside %s", ErrBadOutput, file.Path, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// CheckAuthorFiles accepts the files of a whole task: task.json and files under
// fixture/, reference/, violations/ and refactors/.
func CheckAuthorFiles(files []File) error {
	if err := checkEntries(files, authorEntries); err != nil {
		return err
	}
	if _, found := findFile(files, study.TaskFile); !found {
		return fmt.Errorf("%w: %s is missing", ErrBadOutput, study.TaskFile)
	}
	return nil
}

// CheckVariantFiles accepts files under violations/ and refactors/ alone.
func CheckVariantFiles(files []File) error {
	return checkEntries(files, variantEntries)
}

// CheckTestFiles accepts pytest.ini and test_*.py files directly in the
// directory of the tests, and nothing else.
func CheckTestFiles(files []File) error {
	if err := checkPaths(files); err != nil {
		return err
	}
	for _, file := range files {
		isTest := strings.HasPrefix(file.Path, accept.OracleFilePrefix) && strings.HasSuffix(file.Path, ".py")
		if strings.Contains(file.Path, "/") || (file.Path != accept.OracleConfigFile && !isTest) {
			return fmt.Errorf("%w: %s is neither %s nor a %s*.py file", ErrBadOutput, file.Path, accept.OracleConfigFile, accept.OracleFilePrefix)
		}
	}
	return nil
}

func findFile(files []File, name string) (File, bool) {
	for _, file := range files {
		if file.Path == name {
			return file, true
		}
	}
	return File{}, false
}

// NormaliseTask reads the task description the author wrote and sets the
// members the harness decides: the name, the fixture and the truth paths.
func NormaliseTask(content, name string) (study.Task, error) {
	var task study.Task
	if err := json.Unmarshal([]byte(content), &task); err != nil {
		return study.Task{}, fmt.Errorf("%w: %s is not a JSON object: %v", ErrBadOutput, study.TaskFile, err)
	}
	task.Name = name
	return accept.WithStandardPaths(task), nil
}
