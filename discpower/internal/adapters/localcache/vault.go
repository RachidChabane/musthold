package localcache

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
)

// ErrMutantNotFound is returned by Vault.Get for a mutant that was never kept.
var ErrMutantNotFound = errors.New("mutant not found")

var _ ports.MutantVault = (*Vault)(nil)

// Vault implements ports.MutantVault: the edit set of a mutant is kept as JSON at
// `<cacheDir>/mutants/<id>.json`.
type Vault struct{ cacheDir string }

// NewVault returns the vault under the cache directory of a project. The
// directory must not be inside the project's work tree.
func NewVault(cacheDir string) *Vault { return &Vault{cacheDir: cacheDir} }

// Put implements ports.MutantVault.
func (v *Vault) Put(id string, edits mutation.EditSet) error {
	path, err := entryPath(v.cacheDir, mutantsDirectory, id)
	if err != nil {
		return err
	}
	content, err := json.Marshal(edits)
	if err != nil {
		return fmt.Errorf("encode mutant %s: %w", id, err)
	}
	return store(path, content)
}

// Get implements ports.MutantVault.
func (v *Vault) Get(id string) (mutation.EditSet, error) {
	var edits mutation.EditSet
	path, err := entryPath(v.cacheDir, mutantsDirectory, id)
	if err != nil {
		return edits, err
	}
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return edits, fmt.Errorf("%w: %s", ErrMutantNotFound, id)
	}
	if err != nil {
		return edits, fmt.Errorf("read mutant %s: %w", id, err)
	}
	if err := json.Unmarshal(content, &edits); err != nil {
		return mutation.EditSet{}, fmt.Errorf("decode mutant %s: %w", id, err)
	}
	return edits, nil
}
