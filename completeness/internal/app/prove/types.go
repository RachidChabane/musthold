// Package prove is the `prove` use case: a resumable wave state machine that
// reads the specs of a project, the change since a base, and the Statements other
// modules wrote, asks System One the closed questions that decide what is
// missing, unjustified or unclear, and writes one advisory Statement per spec
// file. Every pass recomputes the deterministic stages from the project and the
// host store, emits the host requests that can change an outcome, and either ends
// in Statements or leaves tickets for the host to answer. Because requests are
// content-addressed and the stages are deterministic, running the state machine
// again is the whole of resuming it.
package prove

import (
	"io"
	"io/fs"
	"time"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// Exit codes of the binary (ADR 0005). Completeness is advisory, so a question
// never changes the exit code; exit 1 means the host stopped answering.
const (
	ExitClean        = 0
	ExitBlocked      = 1
	ExitToolFailure  = 2
	ExitAwaitingHost = 3
)

// Selection tells which specs a run proves.
type Selection string

// Selections of `prove`.
const (
	// SelectChanged proves the specs whose inputs changed since their last Statement.
	SelectChanged Selection = "changed"
	// SelectAll proves every spec.
	SelectAll Selection = "all"
)

// Options are the choices of one invocation.
type Options struct {
	Selection Selection
	// Resume is the `--resume` flag: the run continues an earlier one.
	Resume bool
	// Base is the reference the change is measured from; empty stands for the
	// merge base with the default branch.
	Base string
}

// Settings are the project-level inputs of a run.
type Settings struct {
	// Policy holds the decision parameters.
	Policy policy.Policy
	// Module identifies the module in the Statements and the requests.
	Module format.Module
	// Plugin is the plugin the executor roles belong to.
	Plugin string
	// PendingModule names the module in its pending file, `<module>.json`.
	PendingModule string
	// ProducedBy tells where the Statements are produced; empty means locally.
	ProducedBy format.ProducedBy
}

// Deps are the ports a run works through.
type Deps struct {
	Criteria   ports.CriteriaSource
	Indexers   []ports.Indexer
	Changes    ports.ChangeSource
	Operations ports.Operations
	// Project is the tree of the project under proof, rooted at its root.
	Project fs.FS
	Store   hostproto.HostStore
	Proofs  ports.ProofStore
	// Evidence lists the Statements of the other modules.
	Evidence   ports.ProofSource
	Pending    ports.PendingStore
	Memo       ports.Memo
	Statements ports.StatementValidator
	Clock      func() time.Time
	Entropy    io.Reader
	// Replay and Record are optional.
	Replay requests.Replayer
	Record requests.Recorder
}

// Status tells what a run did with a spec.
type Status string

// Statuses of a spec.
const (
	// StatusProved: a Statement was written.
	StatusProved Status = "proved"
	// StatusFresh: the Statement of the current inputs already exists.
	StatusFresh Status = "fresh"
	// StatusAwaitingHost: tickets wait for a host's answers.
	StatusAwaitingHost Status = "awaiting_host"
)

// SpecResult is what a run found about one spec file.
type SpecResult struct {
	Spec      string
	Status    Status
	Questions int
	CacheKey  string
	// Unresponsive is true when the host stopped answering a request of the spec,
	// which leaves the questions that depend on it unasked.
	Unresponsive bool

	// criteria are the `ID~REV` names of the spec's criteria, which the pending
	// file lists.
	criteria []string
}

// Result is the result of a run.
type Result struct {
	Wave  int
	Specs []SpecResult
	// PendingTickets counts the tickets still waiting for an answer.
	PendingTickets int
	// Findings are module findings that do not change a Statement's meaning, such
	// as a source file that cannot be read.
	Findings []string

	pending []requests.PendingTicket
}

// ExitCode maps the result to the exit codes of ADR 0005: 3 while tickets wait,
// 1 when a host stopped answering, 0 otherwise.
func (r Result) ExitCode() int {
	if r.PendingTickets > 0 {
		return ExitAwaitingHost
	}
	for _, spec := range r.Specs {
		if spec.Unresponsive {
			return ExitBlocked
		}
	}
	return ExitClean
}
