// Package factsrecord keeps the facts record of each Statement under the state
// directory of the project: `<dir>/<ID~REV>/<cacheKey>.json`, and the record of
// the last Statement written for the criterion at `<dir>/<ID~REV>/latest.json`.
package factsrecord

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/platform/atomicfile"
)

var _ ports.FactsStore = (*Store)(nil)

const (
	directoryMode = 0o700
	fileMode      = 0o600
	latestName    = "latest"
	extension     = ".json"
)

// Errors of the store.
var (
	ErrInvalidKey = errors.New("invalid facts record key")
	ErrNoRecord   = errors.New("no facts record")
)

var cacheKeyRegexp = regexp.MustCompile(`^[0-9a-f]{64}$`)

// Store implements ports.FactsStore.
type Store struct{ dir string }

// New returns the store under a directory.
func New(dir string) *Store { return &Store{dir: dir} }

// Write keeps the record under its cache key and as the latest of the criterion.
func (s *Store) Write(criterion spec.Ref, cacheKey string, record []byte) error {
	if !cacheKeyRegexp.MatchString(cacheKey) {
		return fmt.Errorf("%w: %q", ErrInvalidKey, cacheKey)
	}
	dir := filepath.Join(s.dir, criterion.String())
	if err := os.MkdirAll(dir, directoryMode); err != nil {
		return fmt.Errorf("create the facts directory: %w", err)
	}
	for _, name := range []string{cacheKey, latestName} {
		if err := atomicfile.Write(filepath.Join(dir, name+extension), record, fileMode); err != nil {
			return fmt.Errorf("write the facts record: %w", err)
		}
	}
	return nil
}

// Latest returns the record of the last Statement written for the criterion.
func (s *Store) Latest(criterion spec.Ref) ([]byte, error) {
	content, err := os.ReadFile(filepath.Join(s.dir, criterion.String(), latestName+extension))
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("%w for %s", ErrNoRecord, criterion)
	}
	return content, err
}
