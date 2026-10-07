// Package proofcache keeps the completeness Statements of a project in its work
// tree, at `.spec-proof/cache/completeness/<spec path>/<cacheKey>.json`, where the
// spec path has its separators percent-encoded so that it is one directory name.
// A Statement is validated against the embedded schemas before it is written.
package proofcache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/platform/atomicfile"
	"github.com/RachidChabane/spec-proof/format"
)

// Location of the cache inside a project and the modes of what it holds.
const (
	stateDirectory = ".spec-proof"
	cacheDirectory = "cache"
	moduleFolder   = "completeness"

	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
)

// Errors returned by the cache.
var (
	// ErrInvalidStatement wraps the validator's refusal of a Statement.
	ErrInvalidStatement = errors.New("statement refused by the validator")
	// ErrInvalidKey is returned for a spec path or a cache key that cannot name a file.
	ErrInvalidKey = errors.New("invalid proof cache key")
)

var _ ports.ProofStore = (*Cache)(nil)

// Cache implements ports.ProofStore over a project root.
type Cache struct {
	root      string
	validator ports.StatementValidator
}

// New returns the cache of the project at the absolute root.
func New(root string, validator ports.StatementValidator) *Cache {
	return &Cache{root: root, validator: validator}
}

// Has implements ports.ProofStore.
func (c *Cache) Has(specPath, cacheKey string) (bool, error) {
	path, err := c.statementPath(specPath, cacheKey)
	if err != nil {
		return false, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return info.Mode().IsRegular(), nil
}

// Write implements ports.ProofStore: it refuses a Statement the validator
// refuses, and otherwise replaces the stored one atomically.
func (c *Cache) Write(specPath, cacheKey string, canonical []byte) error {
	path, err := c.statementPath(specPath, cacheKey)
	if err != nil {
		return err
	}
	if err := c.validator.Validate(canonical); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidStatement, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return atomicfile.Write(path, canonical, fileMode)
}

func (c *Cache) statementPath(specPath, cacheKey string) (string, error) {
	if !format.IsRelativePOSIXPath(specPath) {
		return "", fmt.Errorf("%w: spec path %q", ErrInvalidKey, specPath)
	}
	location := format.ProofLocation{Module: moduleFolder, SubjectKey: format.SubjectKey(specPath), CacheKey: cacheKey}
	if !location.IsValid() {
		return "", fmt.Errorf("%w: cache key %q", ErrInvalidKey, cacheKey)
	}
	return filepath.Join(c.root, stateDirectory, cacheDirectory, filepath.FromSlash(location.StatementPath())), nil
}
