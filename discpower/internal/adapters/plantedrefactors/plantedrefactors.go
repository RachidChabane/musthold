// Package plantedrefactors reads the planted refactors an admission input names:
// a directory of edit sets, one JSON file `<name>.json` per refactor, each a
// change that preserves every criterion.
package plantedrefactors

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/prove"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
)

const extension = ".json"

// Load reads the refactors of a directory, relative to the project root unless
// absolute, sorted by name. An empty directory name names no refactor.
func Load(root, dir string) ([]prove.Refactor, error) {
	if dir == "" {
		return nil, nil
	}
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(root, filepath.FromSlash(dir))
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read the planted refactors: %w", err)
	}
	var refactors []prove.Refactor
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != extension {
			continue
		}
		content, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("read the planted refactor %s: %w", entry.Name(), err)
		}
		var edits mutation.EditSet
		if err := json.Unmarshal(content, &edits); err != nil {
			return nil, fmt.Errorf("read the planted refactor %s: %w", entry.Name(), err)
		}
		if err := edits.Validate(); err != nil {
			return nil, fmt.Errorf("the planted refactor %s: %w", entry.Name(), err)
		}
		refactors = append(refactors, prove.Refactor{Name: strings.TrimSuffix(entry.Name(), extension), Edits: edits})
	}
	slices.SortFunc(refactors, func(a, b prove.Refactor) int { return strings.Compare(a.Name, b.Name) })
	return refactors, nil
}
