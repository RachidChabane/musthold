// Package pendingfile keeps the pending file of a module, `.musthold/pending/<module>.json`,
// which tells Shallnot what the module still waits for and how many host answers
// it accepted. The file is written in the form the format fixes, atomically, and
// read back through the format's own decoder.
package pendingfile

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/completeness/internal/app/ports"
	"github.com/RachidChabane/musthold/completeness/internal/platform/atomicfile"
	"github.com/RachidChabane/musthold/completeness/internal/platform/filelock"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

const (
	stateDirectory = ".musthold"
	dirMode        = 0o755
	fileMode       = 0o644
	lockMode       = 0o600
)

var _ ports.PendingStore = Directory{}

// Directory is the pending directory of a project.
type Directory struct {
	Root string
	// Lock is the file that serializes the writers of the pending files across
	// processes, kept outside the work tree; empty serializes nothing.
	Lock string
}

func (d Directory) path(module string) string {
	return filepath.Join(d.Root, stateDirectory, format.PendingDirectory, module+format.PendingFileExtension)
}

// Write replaces the entries of the module's pending file with those of
// pending, which must be valid, and keeps the larger count of answers of the two.
// A module that waits for nothing and accepted no answer has no file.
func (d Directory) Write(pending format.Pending) error {
	if err := pending.Validate(); err != nil {
		return fmt.Errorf("the pending file is invalid: %w", err)
	}
	return d.update(pending.Module, func(stored format.Pending) format.Pending { return pending.Over(stored) })
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

// Tally is the answer tally of one module, kept in its pending file.
type Tally struct {
	Directory Directory
	Module    string
}

var _ requests.AnswerTally = Tally{}

// Accepted implements requests.AnswerTally: the file keeps its entries and takes
// the larger count of answers, and exists from the first answer on.
func (t Tally) Accepted(total int) error {
	return t.Directory.update(t.Module, func(stored format.Pending) format.Pending {
		return format.Pending{Module: t.Module, Pending: stored.Pending, Answered: total}.Over(stored)
	})
}

// update rewrites the module's pending file with what next makes of the stored
// one, under the lock. A stored file that does not decode counts as empty.
func (d Directory) update(module string, next func(stored format.Pending) format.Pending) error {
	if d.Lock != "" {
		release, err := filelock.Acquire(d.Lock, lockMode)
		if err != nil {
			return fmt.Errorf("lock the pending files: %w", err)
		}
		defer release()
	}
	stored, err := d.Read(module)
	if err != nil {
		stored = format.Pending{Module: module}
	}
	return d.store(next(stored))
}

// store writes the file atomically, or removes it when it need not be kept.
func (d Directory) store(pending format.Pending) error {
	path := d.path(pending.Module)
	if !pending.Kept() {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("remove %s: %w", path, err)
		}
		return nil
	}
	if pending.Pending == nil {
		pending.Pending = []format.PendingEntry{}
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
