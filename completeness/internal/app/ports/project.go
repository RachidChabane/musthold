// Package ports declares what the use cases of the module need from the outside
// world: the criteria source, the per-ecosystem source indexers, the change
// source, the host store, the proof stores and the validators.
package ports

import (
	"context"
	"errors"
	"time"

	"github.com/RachidChabane/musthold/completeness/internal/domain/code"
	"github.com/RachidChabane/musthold/completeness/internal/domain/criterion"
	"github.com/RachidChabane/musthold/format"
)

// Report is what Shallnot reports about a project: the schema version of the
// report and the specs with their active criteria and out-of-scope declarations.
type Report struct {
	SchemaVersion string
	Project       criterion.Project
}

// CriteriaSource is the only way the module learns what the specs say: it asks
// Shallnot, which alone parses specs and computes statement digests.
type CriteriaSource interface {
	Load(ctx context.Context) (Report, error)
}

// Indexer lists the units of the source files of one ecosystem. It is a strategy:
// a new ecosystem adds an indexer and changes nothing else.
type Indexer interface {
	// Handles reports whether the indexer serves the file, by its extension.
	Handles(file string) bool
	// IsTest reports whether the file holds tests.
	IsTest(file string) bool
	// Index lists the units of the source of a file.
	Index(file string, source []byte) (code.File, error)
}

// Change is one file a change touched.
type Change struct {
	// Path is the file after the change, relative to the project root with `/`
	// separators; OldPath is where it was before, which differs after a rename.
	Path    string
	OldPath string
	// Before is the content at the base, nil for a file the change added.
	Before []byte
	// After is the content in the work tree, nil for a file the change deleted.
	After []byte
}

// ChangeSet is what a change did to a project since its base.
type ChangeSet struct {
	// Base is the commit the change is measured from.
	Base    string
	Changes []Change
}

// ErrNoRepository is returned by a change source for a project that is not under
// version control or has no base to compare with.
var ErrNoRepository = errors.New("the project has no version-controlled base to compare with")

// ChangeSource reads what the work tree changed since a base.
type ChangeSource interface {
	// Changes returns the files the work tree changed since the base reference;
	// an empty reference stands for the merge base with the default branch.
	Changes(ctx context.Context, base string) (ChangeSet, error)
}

// Operations binds the operation names of spec examples to the functions that
// implement them, per language.
type Operations interface {
	// Bound lists the functions the operation is bound to, in any language.
	Bound(operation string) []string
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

// ProofStore keeps the completeness Statements of the project, one per spec file.
type ProofStore interface {
	// Has reports whether the Statement of the spec with the cache key exists.
	Has(specPath, cacheKey string) (bool, error)
	// Write stores the canonical bytes of a Statement.
	Write(specPath, cacheKey string, canonical []byte) error
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
// verified set first, then the local cache, the most recent first. Evidence
// crosses modules only as proofs.
type ProofSource interface {
	Candidates(kind format.Kind, ref criterion.Ref) ([]ProofFile, error)
}

// PendingStore writes the pending file of the module, which tells Shallnot what
// the module still waits for.
type PendingStore interface {
	// Write replaces the pending file of the module; an empty list removes it.
	Write(pending format.Pending) error
}
