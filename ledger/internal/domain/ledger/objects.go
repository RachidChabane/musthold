// Package ledger is the domain of the proof ledger: the paths it holds, the
// rule that no path is ever rewritten, the seed of a head commit and the
// properties an audit checks. It knows no git command and no file system; the
// stores it needs are ports.
package ledger

// ObjectID names a git object: a blob, a tree or a commit.
type ObjectID string

// EntryKind is the kind of object a tree entry points to.
type EntryKind string

// Kinds of tree entry.
const (
	EntryBlob EntryKind = "blob"
	EntryTree EntryKind = "tree"
)

// Modes of tree entry: a regular file and a directory.
const (
	ModeBlob = "100644"
	ModeTree = "040000"
)

// TreeEntry is one entry of a git tree.
type TreeEntry struct {
	Mode string
	Kind EntryKind
	ID   ObjectID
	Name string
}

// FileEntry is one file of a recursively listed tree.
type FileEntry struct {
	Path string
	ID   ObjectID
}

// CommitRecord is one commit of the ledger history with its parents.
type CommitRecord struct {
	ID      ObjectID
	Parents []ObjectID
}
