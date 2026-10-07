//go:build unix

package hoststore

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// acquireFileLock takes an exclusive advisory lock on the file at path, creating
// it when needed, and returns the function that releases it.
func acquireFileLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, FileMode)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if !errors.Is(err, syscall.EINTR) {
			break
		}
	}
	if err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("flock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		_ = file.Close()
	}, nil
}
