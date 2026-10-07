package ports

import (
	"context"
	"io/fs"
	"time"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

// Report is what Shallnot reports about the criteria of a project: the schema
// version of the report and the active criteria with their bound tests.
type Report struct {
	SchemaVersion string
	Project       criterion.Project
}

// CriteriaSource is the only way the module learns what the criteria are: it
// asks Shallnot, which alone parses specs and computes statement digests.
type CriteriaSource interface {
	Load(ctx context.Context) (Report, error)
}

// Command is one process to run.
type Command struct {
	Argv []string
	Dir  string
	// Env lists KEY=VALUE entries added to the inherited environment.
	Env []string
	// Timeout is the deadline after which the process group is killed; zero means
	// no deadline beyond the context's.
	Timeout time.Duration
}

// CommandResult is what a finished process left behind.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	// TimedOut is true when the deadline killed the process group.
	TimedOut bool
}

// ProcessRunner runs a command with a process group, a deadline and captured
// output. A non-zero exit status is a result, not an error.
type ProcessRunner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

// StatementValidator checks an encoded Statement against the embedded schemas.
type StatementValidator interface {
	Validate(statement []byte) error
}

// ProofStore keeps the alignment Statements of the project.
type ProofStore interface {
	// Has reports whether the Statement of the criterion with the cache key exists.
	Has(criterion criterion.Ref, cacheKey string) (bool, error)
	// Write stores the canonical bytes of a Statement.
	Write(criterion criterion.Ref, cacheKey string, canonical []byte) error
}

// Memo is the local memo of what a run decided, keyed by content.
type Memo interface {
	Get(key string) ([]byte, bool, error)
	Put(key string, value []byte) error
}

// ProofOrigin tells where a consumed Statement was read from.
type ProofOrigin string

// Origins of a consumed Statement.
const (
	// OriginVerified: the verified set the verifier wrote.
	OriginVerified ProofOrigin = "verified"
	// OriginLocalCache: the module's local cache of unsigned Statements.
	OriginLocalCache ProofOrigin = "cache"
)

// ProofFile is a Statement file of another module: its exact bytes, which the
// `proof` input digests, and where it was read from.
type ProofFile struct {
	Origin ProofOrigin
	Bytes  []byte
}

// ProofSource lists the Statements another module wrote for a criterion, the
// verified set first, then the local cache. Evidence crosses modules only as
// proofs.
type ProofSource interface {
	Candidates(kind format.Kind, ref criterion.Ref) ([]ProofFile, error)
}

// PendingStore reads and writes the pending files of the modules, one per module,
// that tell Shallnot what a module still waits for.
type PendingStore interface {
	// Write replaces the pending file of the module; an empty list removes it.
	Write(pending format.Pending) error
	// Read returns the pending file of a module, empty when there is none.
	Read(module string) (format.Pending, error)
}

// Project is the tree of the project under proof.
type Project struct {
	// Root is the absolute path of the project root.
	Root string
	// Tree is the project's files, rooted at Root.
	Tree fs.FS
}
