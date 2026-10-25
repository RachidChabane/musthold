package scan

import (
	"context"
	"errors"
)

// ErrNotAtHead is returned by a HeadFileReader for a path that is not a file of
// the head commit's tree.
var ErrNotAtHead = errors.New("file is not in the head commit")

// Source is a named quotable document: a name the statement cites and the exact
// bytes a quote is checked against.
type Source struct {
	Name    string
	Content []byte
}

// SavedSource is a source kept under the scan's folder.
type SavedSource struct {
	Name string
	// Digest is the SHA-256 of the saved bytes, lowercase hex.
	Digest string
}

// SourceStore keeps the sources of a scan so a reader can re-run the quote check
// on the very bytes the scan used.
type SourceStore interface {
	// Save writes the sources of the scan of a commit and gives each one's digest,
	// in the order given.
	Save(commit string, sources []Source) ([]SavedSource, error)
	// Load reads a saved source back.
	Load(commit, name string) ([]byte, error)
}

// HeadFileReader reads a file from the tree of the head commit, not from the work
// tree, so the bytes are those of the commit the scan names. A path that is not
// a file of that tree gives ErrNotAtHead.
type HeadFileReader interface {
	ReadAtHead(ctx context.Context, path string) ([]byte, error)
}
