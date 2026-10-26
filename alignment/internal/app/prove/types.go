// Package prove is the `prove` use case: a resumable wave state machine that
// judges, for every test bound to a criterion, whether the test verifies what the
// criterion states. Every pass recomputes the deterministic stages from the
// project and the host store, emits the host requests that can change an outcome,
// and either ends in a Statement per (test, criterion) or leaves tickets for the
// host to answer. Because requests are content-addressed and the stages are
// deterministic, running the state machine again is the whole of resuming it.
package prove

import (
	"io"
	"io/fs"
	"time"

	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// Exit codes of the binary (ADR 0005).
const (
	ExitClean        = 0
	ExitBlocked      = 1
	ExitToolFailure  = 2
	ExitAwaitingHost = 3
)

// Selection tells which tests a run proves.
type Selection string

// Selections of `prove`.
const (
	// SelectChanged proves the tests whose inputs changed since their last Statement.
	SelectChanged Selection = "changed"
	// SelectAll proves every test.
	SelectAll Selection = "all"
)

// Options are the choices of one invocation.
type Options struct {
	Selection Selection
	// Resume is the `--resume` flag: the run continues an earlier one.
	Resume bool
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
	Criteria ports.CriteriaSource
	Maskers  []ports.Masker
	// Project is the tree of the project under proof, rooted at its root.
	Project    fs.FS
	Store      hostproto.HostStore
	Proofs     ports.ProofStore
	Power      ports.ProofSource
	Pending    ports.PendingStore
	Memo       ports.Memo
	Statements ports.StatementValidator
	Clock      func() time.Time
	Entropy    io.Reader
	// Replay and Record are optional.
	Replay requests.Replayer
	Record requests.Recorder
}

// Status tells what a run did with a test and a declared criterion.
type Status string

// Statuses of a test.
const (
	// StatusProved: a Statement was written.
	StatusProved Status = "proved"
	// StatusFresh: the Statement of the current inputs already exists.
	StatusFresh Status = "fresh"
	// StatusAwaitingHost: tickets wait for a host's answers.
	StatusAwaitingHost Status = "awaiting_host"
	// StatusAwaitingProof: the verdict is settled but the discriminating-power
	// proof it should rest on is still being produced.
	StatusAwaitingProof Status = "awaiting_proof"
)

// TestResult is what a run found about one test and one declared criterion.
type TestResult struct {
	Test      string
	Criterion criterion.Ref
	Status    Status
	Verdict   format.Verdict
	// Reason is the escalation reason of the decision, when it has one.
	Reason   string
	CacheKey string
}

// Result is the result of a run.
type Result struct {
	Wave  int
	Tests []TestResult
	// PendingTickets counts the tickets still waiting for an answer.
	PendingTickets int
	// Findings are module findings that do not change a verdict, such as a
	// discriminating-power Statement that could not be read.
	Findings []string

	pending []requests.PendingTicket
}

// ExitCode maps the result to the exit codes of ADR 0005: 3 while tickets wait,
// 1 while a test is not ALIGNED or a proof is awaited, 0 otherwise.
func (r Result) ExitCode() int {
	if r.PendingTickets > 0 {
		return ExitAwaitingHost
	}
	for _, test := range r.Tests {
		if test.Status == StatusAwaitingProof || test.Status == StatusAwaitingHost {
			return ExitBlocked
		}
		if test.Verdict != format.VerdictAligned {
			return ExitBlocked
		}
	}
	return ExitClean
}
