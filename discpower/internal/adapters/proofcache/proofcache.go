// Package proofcache keeps the discriminating-power Statements of a project in
// its work tree, at `.musthold/cache/discriminating-power/<ID>~<REV>/<cacheKey>.json`.
// A Statement is validated against the embedded schemas before it is written.
package proofcache

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/platform/atomicfile"
	"github.com/RachidChabane/musthold/format"
)

// Location of the cache inside a project and the modes of what it holds.
const (
	cacheDirectory  = ".musthold"
	cacheSubdir     = "cache"
	predicateFolder = "discriminating-power"
	statementExt    = ".json"

	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
)

// Errors returned by the cache.
var (
	// ErrInvalidStatement wraps the validator's refusal of a Statement.
	ErrInvalidStatement = errors.New("statement refused by the validator")
	// ErrInvalidKey is returned for a criterion or a cache key that cannot name a file.
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
func (c *Cache) Has(criterion spec.Ref, cacheKey string) (bool, error) {
	path, err := c.statementPath(criterion, cacheKey)
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
func (c *Cache) Write(criterion spec.Ref, cacheKey string, canonical []byte) error {
	path, err := c.statementPath(criterion, cacheKey)
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

func (c *Cache) statementPath(criterion spec.Ref, cacheKey string) (string, error) {
	if criterion.ID == "" || strings.ContainsAny(criterion.ID, `/\`) || criterion.ID == "." || criterion.ID == ".." {
		return "", fmt.Errorf("%w: criterion %q", ErrInvalidKey, criterion.ID)
	}
	if !format.IsSHA256Hex(cacheKey) {
		return "", fmt.Errorf("%w: cache key %q", ErrInvalidKey, cacheKey)
	}
	return filepath.Join(c.root, cacheDirectory, cacheSubdir, predicateFolder, criterion.String(), cacheKey+statementExt), nil
}
