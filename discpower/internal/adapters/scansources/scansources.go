// Package scansources keeps the quotable sources of a scan under
// `.spec-proof/scan/<commit>/sources/` of the repository, so that a reader can
// re-run the quote check on the bytes the scan used.
package scansources

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/format"
)

// Layout of the folder.
const (
	outputFolder  = ".spec-proof"
	scanFolder    = "scan"
	sourcesFolder = "sources"
	dirMode       = 0o755
	fileMode      = 0o644
	tempPattern   = ".source-*"
	// escapeMark starts the escape of a byte a file name cannot carry.
	escapeMark = '%'
	// maxFileNameLength bounds the escaped name, below the 255 bytes file systems allow.
	maxFileNameLength = 200
	// Characters that make a name a path.
	slash     = "/"
	backslash = `\`
	nul       = "\x00"
	// Path elements that climb or stay.
	currentElement = "."
	parentElement  = ".."
	// Commit ids are lowercase hexadecimal, of SHA-1 or SHA-256 length.
	sha1Length   = 40
	sha256Length = 64
)

// Errors of the store.
var (
	// ErrNoRoot is returned when the store has no repository root.
	ErrNoRoot = errors.New("scansources has no repository root")
	// ErrUnsafeName is returned for a source name that is empty, holds a NUL, or
	// has a "." or ".." element, which could leave the sources folder.
	ErrUnsafeName = errors.New("source name is not safe to store")
	// ErrNameTooLong is returned when the file name of a source would be too long.
	ErrNameTooLong = errors.New("source name is too long to store")
	// ErrBadCommit is returned when the commit is not a lowercase hexadecimal id.
	ErrBadCommit = errors.New("commit is not a full hexadecimal id")
	// ErrDuplicateName is returned when two sources of one save share a name.
	ErrDuplicateName = errors.New("two sources share a name")
)

var _ scan.SourceStore = Store{}

// Store writes sources under a repository.
type Store struct {
	// Root is the repository's work tree.
	Root string
}

// Save writes every source and gives each one's digest, in the order given.
// Nothing is written when a name or the commit is refused.
func (s Store) Save(commit string, sources []scan.Source) ([]scan.SavedSource, error) {
	dir, err := s.dir(commit)
	if err != nil {
		return nil, err
	}
	files := make([]string, len(sources))
	seen := map[string]string{}
	for index, source := range sources {
		file, err := FileName(source.Name)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[file]; dup {
			return nil, fmt.Errorf("%w: %q", ErrDuplicateName, source.Name)
		}
		seen[file], files[index] = source.Name, file
	}
	if err := os.MkdirAll(dir, dirMode); err != nil {
		return nil, fmt.Errorf("create %s: %w", dir, err)
	}
	saved := make([]scan.SavedSource, len(sources))
	for index, source := range sources {
		if err := writeAtomically(dir, files[index], source.Content); err != nil {
			return nil, err
		}
		saved[index] = scan.SavedSource{Name: source.Name, Digest: format.SHA256Hex(source.Content)}
	}
	return saved, nil
}

// Load reads a saved source back.
func (s Store) Load(commit, name string) ([]byte, error) {
	dir, err := s.dir(commit)
	if err != nil {
		return nil, err
	}
	file, err := FileName(name)
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(filepath.Join(dir, file))
	if err != nil {
		return nil, fmt.Errorf("read source %q: %w", name, err)
	}
	return content, nil
}

func (s Store) dir(commit string) (string, error) {
	if s.Root == "" {
		return "", ErrNoRoot
	}
	if !isCommitID(commit) {
		return "", fmt.Errorf("%w: %q", ErrBadCommit, commit)
	}
	return filepath.Join(s.Root, outputFolder, scanFolder, commit, sourcesFolder), nil
}

func isCommitID(commit string) bool {
	if len(commit) != sha1Length && len(commit) != sha256Length {
		return false
	}
	return strings.Trim(commit, "0123456789abcdef") == ""
}

// FileName maps a source name to the name of its file. The map is injective: a
// name keeps its letters, digits, '.', '-' and '_', and every other byte, the
// path separators included, becomes %XX. A name that is empty, holds a NUL or has
// a "." or ".." element is refused.
func FileName(name string) (string, error) {
	if name == "" || strings.Contains(name, nul) {
		return "", fmt.Errorf("%w: %q", ErrUnsafeName, name)
	}
	for _, element := range strings.FieldsFunc(name, isSeparator) {
		if element == currentElement || element == parentElement {
			return "", fmt.Errorf("%w: %q", ErrUnsafeName, name)
		}
	}
	var escaped strings.Builder
	for _, b := range []byte(name) {
		if isPlain(b) {
			escaped.WriteByte(b)
		} else {
			fmt.Fprintf(&escaped, "%c%02X", escapeMark, b)
		}
	}
	file := escaped.String()
	if strings.HasPrefix(file, currentElement) {
		file = string(escapeMark) + "2E" + file[len(currentElement):]
	}
	if len(file) > maxFileNameLength {
		return "", fmt.Errorf("%w: %q", ErrNameTooLong, name)
	}
	return file, nil
}

func isSeparator(r rune) bool { return strings.ContainsRune(slash+backslash, r) }

func isPlain(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z', b >= 'A' && b <= 'Z', b >= '0' && b <= '9':
		return true
	}
	return b == '.' || b == '-' || b == '_'
}

func writeAtomically(dir, file string, content []byte) error {
	temp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("create source file: %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := temp.Write(content); err != nil {
		temp.Close()
		return fmt.Errorf("write source %s: %w", file, err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("write source %s: %w", file, err)
	}
	if err := os.Chmod(temp.Name(), fileMode); err != nil {
		return fmt.Errorf("write source %s: %w", file, err)
	}
	if err := os.Rename(temp.Name(), filepath.Join(dir, file)); err != nil {
		return fmt.Errorf("write source %s: %w", file, err)
	}
	return nil
}
