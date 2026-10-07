// Package localcache keeps what the module derives and may recompute, outside the
// work tree of the project: the mutant bodies and the memo of executed facts.
// Both live under the cache directory of the project, written atomically.
package localcache

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/platform/atomicfile"
)

// Layout of the cache directory and the modes of what it holds.
const (
	mutantsDirectory = "mutants"
	factsDirectory   = "facts"
	entryExt         = ".json"

	dirMode  os.FileMode = 0o700
	fileMode os.FileMode = 0o600
)

// ErrInvalidKey is returned for an identifier that is not lowercase hexadecimal,
// so that no key can address a file outside its directory.
var ErrInvalidKey = errors.New("invalid cache key")

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

// entryPath is the file of a key in a sub-directory of the cache.
func entryPath(cacheDir, directory, key string) (string, error) {
	if !isLowerHex(key) {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	return filepath.Join(cacheDir, directory, key+entryExt), nil
}

// store writes content at the path, creating its directory.
func store(path string, content []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return atomicfile.Write(path, content, fileMode)
}
