// Package prove is the `prove` use case: a resumable wave state machine that
// measures the discriminating power of the tests bound to each criterion. Every
// pass recomputes the deterministic stages from the project and the host store,
// emits the host requests whose prerequisites are consumable, and either ends in
// a Statement per criterion or leaves tickets for the host to answer. Because
// requests are content-addressed and the stages are deterministic, running the
// state machine again is the whole of resuming it.
package prove

import (
	"io"
	"io/fs"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/requests"
)

// Exit codes of the binary (ADR 0005).
const (
	ExitClean        = 0
	ExitBlocked      = 1
	ExitToolFailure  = 2
	ExitAwaitingHost = 3
)

// Selection tells which criteria a run proves.
type Selection string

// Selections of `prove`.
const (
	// SelectChanged proves the criteria whose inputs changed since their last
	// Statement.
	SelectChanged Selection = "changed"
	// SelectAll proves every criterion.
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
	// Root is the absolute path of the project root.
	Root       string
	Ecosystem  ecosystem.Name
	Policy     policy.Policy
	Operations execution.Operations
	// Seed draws the operator mutants, the sentinels and the differential inputs.
	Seed uint64
	// Module identifies the module in the Statements and the requests.
	Module format.Module
	// Plugin is the plugin the executor roles belong to.
	Plugin string
	// ProducedBy tells where the Statements are produced; empty means locally.
	ProducedBy format.ProducedBy
	// ClassifyFirst leaves the first vote of a closed request to the classification
	// of the host mod before the host votes are asked: set when the switch is
	// `auto` and the admission table holds an entry for this module.
	ClassifyFirst bool
	// Refactors are the planted refactors every witness must pass to be
	// admitted; with none, every witness the reference passes is admitted.
	Refactors []Refactor
}

// Refactor is one planted refactor: a change that preserves every criterion.
type Refactor struct {
	Name  string
	Edits mutation.EditSet
}

// Admission tells how a witness is admitted under the settings: with the planted
// refactors when some are named, on the reference alone otherwise.
func (s Settings) Admission() witness.Admission {
	if len(s.Refactors) > 0 {
		return witness.AdmissionWithRefactors
	}
	return witness.AdmissionReferenceOnly
}

// Deps are the ports a run works through.
type Deps struct {
	Criteria   ports.CriteriaSource
	Strategy   ports.Ecosystem
	Workspaces ports.Workspaces
	Engines    *operatorgen.Registry
	Project    fs.FS
	Store      hostproto.HostStore
	Proofs     ports.ProofStore
	Vault      ports.MutantVault
	Memo       ports.ExecutionMemo
	Statements ports.StatementValidator
	// Facts keeps the facts record of every Statement written.
	Facts   ports.FactsStore
	Clock   func() time.Time
	Entropy io.Reader
	// Replay and Record are optional.
	Replay requests.Replayer
	Record requests.Recorder
}

// CriterionStatus tells what a run did with a criterion.
type CriterionStatus string

// Statuses of a criterion.
const (
	// StatusProved: a Statement was written.
	StatusProved CriterionStatus = "proved"
	// StatusFresh: the Statement of the current inputs already exists.
	StatusFresh CriterionStatus = "fresh"
	// StatusAwaitingHost: tickets wait for a host's answers.
	StatusAwaitingHost CriterionStatus = "awaiting_host"
)

// CriterionResult is what a run found about one criterion.
type CriterionResult struct {
	Ref      spec.Ref
	Status   CriterionStatus
	Outcome  power.Outcome
	CacheKey string
	// Survivors are the mutants that block the criterion, with what their local
	// findings show.
	Survivors []Survivor
}

// Result is the result of a run.
type Result struct {
	Wave     int
	Criteria []CriterionResult
	// PendingTickets counts the tickets still waiting for an answer.
	PendingTickets int
	// Findings are module findings that do not change an outcome, such as an
	// example whose operation is unbound.
	Findings []string

	pending []requests.PendingTicket
}

// ExitCode maps the result to the exit codes of ADR 0005: 3 while tickets wait,
// 1 when a criterion blocks or escalates, 0 otherwise.
func (r Result) ExitCode() int {
	if r.PendingTickets > 0 {
		return ExitAwaitingHost
	}
	for _, criterion := range r.Criteria {
		if criterion.Outcome.Result.Severity() > power.Pass.Severity() {
			return ExitBlocked
		}
	}
	return ExitClean
}
