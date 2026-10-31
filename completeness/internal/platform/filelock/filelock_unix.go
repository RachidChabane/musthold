//go:build unix

// Package filelock takes advisory locks on files, shared by the processes that
// open the same path: one that waits for the lock and one that does not.
package filelock

import (
	"errors"
	"fmt"
	"os"
	"syscall"
)

// ErrHeld is returned by Try while another holder keeps the lock.
var ErrHeld = errors.New("the lock is held")

// Acquire takes an exclusive lock on the file at path, creating it with the mode
// when needed, waits while another holder keeps it, and returns the function
// that releases it.
func Acquire(path string, mode os.FileMode) (func(), error) { return lock(path, mode, syscall.LOCK_EX) }

// Try takes an exclusive lock on the file at path without waiting: ErrHeld while
// another holder keeps it.
func Try(path string, mode os.FileMode) (func(), error) {
	release, err := lock(path, mode, syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) {
		return nil, ErrHeld
	}
	return release, err
}

func lock(path string, mode os.FileMode, how int) (func(), error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	for {
		err = syscall.Flock(int(file.Fd()), how)
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
