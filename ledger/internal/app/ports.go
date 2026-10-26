// Package app holds the use cases of the ledger module: sign statements, append
// signed proofs to the ledger, export them and audit the ledger. Each use case
// depends on narrow ports that the adapters implement.
package app

import (
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/ledger/internal/domain/ledger"
)

// Statement is a bare statement as a module cached it.
type Statement struct {
	Location format.ProofLocation
	Bytes    []byte
}

// Bundle is a signed statement, ready for the ledger.
type Bundle struct {
	Location format.ProofLocation
	Bytes    []byte
}

// StatementSource lists the statements to sign.
type StatementSource interface {
	Statements() ([]Statement, error)
}

// BundleSink stores signed bundles.
type BundleSink interface {
	WriteBundles(bundles []Bundle) error
}

// Signer is the strategy that turns exact statement bytes into a signed bundle: an
// Ed25519 identity or a Sigstore keyless identity.
type Signer interface {
	Sign(statement []byte) ([]byte, error)
}

// SchemaChecker validates a statement against the JSON Schemas of the format.
type SchemaChecker interface {
	Validate(statement []byte) error
}

// IntegrityCheck compares the statements with a digest manifest written when they
// were generated.
type IntegrityCheck interface {
	Check(statements []Statement) error
}

// AdditionSource lists files to add to the ledger.
type AdditionSource interface {
	Additions() ([]ledger.Addition, error)
}

// Clock tells the time of a commit.
type Clock interface {
	Now() time.Time
}

// LedgerRef reads and moves the ledger branch.
type LedgerRef interface {
	// Tip returns the head of the ledger branch, and false when it does not exist.
	Tip() (ledger.ObjectID, bool, error)
	// TreeOf returns the root tree of a commit.
	TreeOf(commit ledger.ObjectID) (ledger.ObjectID, error)
	// Advance moves the branch from previous (nil when it does not exist) to next.
	// It fails when the branch is no longer at previous, and when next is not a
	// descendant of it, so the branch only ever fast-forwards.
	Advance(previous *ledger.ObjectID, next ledger.ObjectID) error
}

// Committer writes the objects of a ledger commit.
type Committer interface {
	ledger.TreeStore
	Commit(tree ledger.ObjectID, parent *ledger.ObjectID, message string, at time.Time) (ledger.ObjectID, error)
}

// LedgerRemote synchronises the ledger branch with a remote. It only ever
// fast-forwards.
type LedgerRemote interface {
	// Fetch brings the remote ledger branch into the local one; a remote without
	// the branch is not an error.
	Fetch() error
	// Push publishes the local ledger branch without forcing.
	Push() error
}

// LedgerFiles reads the files of a ledger commit.
type LedgerFiles interface {
	// Files lists every file of the commit, recursively.
	Files(commit ledger.ObjectID) ([]ledger.FileEntry, error)
	ReadBlob(id ledger.ObjectID) ([]byte, error)
}

// LedgerHistory reads the history of the ledger branch.
type LedgerHistory interface {
	// History lists the commits reachable from the tip with their parents.
	History(tip ledger.ObjectID) ([]ledger.CommitRecord, error)
	// Rewrites lists the paths some commit of the history deleted or modified.
	Rewrites(tip ledger.ObjectID) ([]string, error)
}

// FileSink materializes files below a directory it owns.
type FileSink interface {
	// Prepare makes the directory ready: it refuses one that holds files.
	Prepare() error
	WriteFile(path string, content []byte) error
}
