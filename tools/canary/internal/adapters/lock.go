package adapters

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
)

// ErrSessionRunning is returned when another canary session holds the lock.
var ErrSessionRunning = errors.New("another canary session is running")

// FileLocker takes advisory locks on files.
type FileLocker struct{}

// fileLock is the advisory lock that lets at most one canary session run at a time.
type fileLock struct{ file *os.File }

// Acquire takes the lock held by the file at path without waiting. The lock is
// released with Release, or when the process ends.
func (FileLocker) Acquire(path string) (app.Lock, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = file.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return nil, fmt.Errorf("%w (lock %s)", ErrSessionRunning, path)
		}
		return nil, err
	}
	_ = file.Truncate(0)
	_, _ = file.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	return &fileLock{file: file}, nil
}

// Release gives the lock up.
func (l *fileLock) Release() {
	_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	_ = l.file.Close()
}
