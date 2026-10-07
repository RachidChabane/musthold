package app

import "time"

// NoIntegrity is the IntegrityCheck of a run without digest manifest.
type NoIntegrity struct{}

// Check accepts every statement.
func (NoIntegrity) Check([]Statement) error { return nil }

// NoRemote is the LedgerRemote of a run on the local repository only.
type NoRemote struct{}

// Fetch does nothing.
func (NoRemote) Fetch() error { return nil }

// Push does nothing.
func (NoRemote) Push() error { return nil }

// SystemClock is the Clock of the machine.
type SystemClock struct{}

// Now returns the current time.
func (SystemClock) Now() time.Time { return time.Now() }
