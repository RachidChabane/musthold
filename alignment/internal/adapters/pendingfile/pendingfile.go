// Package pendingfile keeps the pending file of a module, `.spec-proof/pending/<module>.json`,
// which tells Shallnot what the module still waits for. The file is written in
// the form the format fixes and read back through the format's own decoder.
package pendingfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
	"github.com/RachidChabane/spec-proof/alignment/internal/platform/atomicfile"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	stateDirectory = ".spec-proof"
	dirMode        = 0o755
	fileMode       = 0o644
)

var _ ports.PendingStore = Directory{}

// Directory is the pending directory of a project.
type Directory struct {
	Root string
}

func (d Directory) path(module string) string {
	return filepath.Join(d.Root, stateDirectory, format.PendingDirectory, module+format.PendingFileExtension)
}

// Write replaces the pending file of the module with the entries of pending,
// which must be valid. A module that waits for nothing has no file.
func (d Directory) Write(pending format.Pending) error {
	if err := pending.Validate(); err != nil {
		return fmt.Errorf("the pending file is invalid: %w", err)
	}
	path := d.path(pending.Module)
	if len(pending.Pending) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	encoded, err := format.Canonicalize(pending)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), dirMode); err != nil {
		return fmt.Errorf("create %s: %w", filepath.Dir(path), err)
	}
	return atomicfile.Write(path, encoded, fileMode)
}

// Read returns the pending file of a module; a module with none waits for nothing.
func (d Directory) Read(module string) (format.Pending, error) {
	content, err := os.ReadFile(d.path(module))
	if errors.Is(err, fs.ErrNotExist) {
		return format.Pending{Module: module}, nil
	}
	if err != nil {
		return format.Pending{}, fmt.Errorf("read the pending file of %s: %w", module, err)
	}
	return format.DecodePending(content)
}
