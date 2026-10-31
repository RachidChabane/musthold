//go:build windows

// Package filelock takes advisory locks on files. On Windows a lock keeps out
// the other holders of the same process only.
package filelock

import (
	"errors"
	"os"
	"sync"
)

// ErrHeld is returned by Try while another holder keeps the lock.
var ErrHeld = errors.New("the lock is held")

var held = struct {
	sync.Mutex
	paths map[string]bool
}{paths: map[string]bool{}}

// Acquire is in-process only on Windows: the callers' own mutexes serialize the
// writers of one process.
func Acquire(string, os.FileMode) (func(), error) { return func() {}, nil }

// Try takes the path for this process without waiting.
func Try(path string, _ os.FileMode) (func(), error) {
	held.Lock()
	defer held.Unlock()
	if held.paths[path] {
		return nil, ErrHeld
	}
	held.paths[path] = true
	return func() {
		held.Lock()
		defer held.Unlock()
		delete(held.paths, path)
	}, nil
}
