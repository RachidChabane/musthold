package app

// Files is the file system the canary prepares a run in and records it to.
type Files interface {
	// MakeDir creates a fresh directory under parent and returns its path with
	// symbolic links resolved: the journal of a project is addressed by the digest
	// of its resolved root.
	MakeDir(parent, pattern string) (string, error)
	// MkdirAll creates a directory and its parents.
	MkdirAll(dir string) error
	// CopyTree copies a directory tree, leaving out interpreter and test caches.
	CopyTree(from, to string) error
	// Write creates or replaces a file, creating its directory.
	Write(path string, data []byte, executable bool) error
	// Read returns the file's content, and nil when the file does not exist.
	Read(path string) ([]byte, error)
	// Exists reports whether the path exists.
	Exists(path string) bool
	// Fingerprint maps each entry of the path, down to depth levels below it, to its
	// size and modification time. A path that does not exist has no entries.
	Fingerprint(path string, depth int) (map[string]string, error)
	// Glob returns the paths that match the pattern.
	Glob(pattern string) ([]string, error)
}
