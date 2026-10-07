package localcache

import (
	"os"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
)

var _ ports.ExecutionMemo = (*Memo)(nil)

// Memo implements ports.ExecutionMemo: a fact is kept at
// `<cacheDir>/facts/<key>.json`.
type Memo struct{ cacheDir string }

// NewMemo returns the memo under the cache directory of a project.
func NewMemo(cacheDir string) *Memo { return &Memo{cacheDir: cacheDir} }

// Get implements ports.ExecutionMemo. An absent or unreadable entry is a miss,
// never an error; only a key that cannot name an entry is refused.
func (m *Memo) Get(key string) ([]byte, bool, error) {
	path, err := entryPath(m.cacheDir, factsDirectory, key)
	if err != nil {
		return nil, false, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false, nil
	}
	return content, true, nil
}

// Put implements ports.ExecutionMemo.
func (m *Memo) Put(key string, value []byte) error {
	path, err := entryPath(m.cacheDir, factsDirectory, key)
	if err != nil {
		return err
	}
	return store(path, value)
}
