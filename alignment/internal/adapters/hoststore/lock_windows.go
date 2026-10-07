//go:build windows

package hoststore

// acquireFileLock is in-process only on Windows: the store's mutex already
// serializes the writers of one process.
func acquireFileLock(string) (func(), error) { return func() {}, nil }
