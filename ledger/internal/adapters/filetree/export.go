package filetree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
)

// ExportDir is the directory the ledger is exported into.
type ExportDir struct {
	Root string
}

// Prepare creates the directory, or accepts an existing empty one. A directory
// that holds anything is refused: exported files mixed with older ones could let
// a stale proof pass for a fresh one.
func (d ExportDir) Prepare() error {
	entries, err := os.ReadDir(d.Root)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return os.MkdirAll(d.Root, directoryMode)
	case err != nil:
		return err
	case len(entries) > 0:
		return refusal.Newf("%s is not empty", d.Root)
	default:
		return nil
	}
}

// WriteFile writes a file at a slash-separated path below the directory. A path
// that would leave the directory is refused.
func (d ExportDir) WriteFile(path string, content []byte) error {
	local := filepath.FromSlash(path)
	if !filepath.IsLocal(local) {
		return refusal.Newf("%s leaves the export directory", path)
	}
	target := filepath.Join(d.Root, local)
	if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(target), err)
	}
	return os.WriteFile(target, content, fileMode)
}
