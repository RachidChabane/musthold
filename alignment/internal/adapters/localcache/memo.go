// Package localcache keeps what the module derives and may recompute, outside the
// work tree of the project: the memo of what a run decided, under the cache
// directory of the project, written atomically.
package localcache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/platform/atomicfile"
)

// Layout of the cache directory and the modes of what it holds.
const (
	memoDirectory = "alignment"
	entryExt      = ".json"

	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// ErrInvalidKey is returned for an identifier that is not lowercase hexadecimal,
// so that no key can address a file outside its directory.
var ErrInvalidKey = errors.New("invalid cache key")

var _ ports.Memo = (*Memo)(nil)

// Memo implements ports.Memo: an entry is kept at
// `<cacheDir>/alignment/<key>.json`.
type Memo struct{ cacheDir string }

// NewMemo returns the memo under the cache directory of a project.
func NewMemo(cacheDir string) *Memo { return &Memo{cacheDir: cacheDir} }

func isLowerHex(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

func (m *Memo) path(key string) (string, error) {
	if !isLowerHex(key) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(m.cacheDir, memoDirectory, key+entryExt), nil
}

// Get implements ports.Memo. An absent or unreadable entry is a miss, never an
// error; only a key that cannot name an entry is refused.
func (m *Memo) Get(key string) ([]byte, bool, error) {
	path, err := m.path(key)
	if err != nil {
		return nil, false, err
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, false, nil
	}
	return content, true, nil
}

// Put implements ports.Memo.
func (m *Memo) Put(key string, value []byte) error {
	path, err := m.path(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return atomicfile.Write(path, value, fileMode)
}
