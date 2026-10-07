package ports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Report is what Shallnot reports about the criteria of a project: the schema
// version of the report and the criteria with their examples and bound tests.
type Report struct {
	SchemaVersion string
	Criteria      []spec.Criterion
}

// CriteriaSource is the only way the module learns what the criteria are: it
// asks Shallnot, which alone parses specs and computes statement digests.
type CriteriaSource interface {
	Load(ctx context.Context) (Report, error)
}

// ProbeCall is a call of a bound operation, addressed by the target the
// ecosystem binds the operation to.
type ProbeCall struct {
	Target string
	Args   map[string]spec.Literal
}

// ProbeSignature is the shape of a bound operation as the ecosystem sees it.
type ProbeSignature struct {
	Target string
	Params []differential.Param
}

// Prober runs calls of bound operations in a workspace: it learns the signatures
// of the targets and observes what each call returns or raises. It backs the
// examples (E1), the differential search (F3) and the confirmation of witnesses
// (F4).
type Prober interface {
	Signatures(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, targets []string) ([]ProbeSignature, error)
	Invoke(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, calls []ProbeCall) ([]spec.Observation, error)
}

// Probing is implemented by an ecosystem strategy that can probe calls.
type Probing interface {
	Prober() Prober
}

// CoverageRecorder runs the tests once on the reference and learns which units
// each executed.
type CoverageRecorder interface {
	Record(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ecosystem.BoundTest) (coverage.Map, error)
}

// Covering is implemented by an ecosystem strategy that can record coverage.
type Covering interface {
	Coverage() CoverageRecorder
}

// ProofStore keeps the discriminating-power Statements of the project.
type ProofStore interface {
	// Has reports whether the Statement of the criterion with the cache key exists.
	Has(criterion spec.Ref, cacheKey string) (bool, error)
	// Write stores the canonical bytes of a Statement.
	Write(criterion spec.Ref, cacheKey string, canonical []byte) error
}

// MutantVault keeps mutant bodies outside the work tree.
type MutantVault interface {
	Put(id string, edits mutation.EditSet) error
	Get(id string) (mutation.EditSet, error)
}

// ExecutionMemo is the local memo of executed facts, keyed by content.
type ExecutionMemo interface {
	Get(key string) ([]byte, bool, error)
	Put(key string, value []byte) error
}

// Recording is an answer kept from an earlier run: the output and, when it was
// recorded, the executor that gave it.
type Recording struct {
	Output   []byte
	Executor *format.ExecutorProfile
}

// Recordings serves recorded answers by recording name; an absent one is an error.
type Recordings interface {
	Load(name string) (Recording, error)
}

// RecordingName names the recording of one vote of a request: the request id for
// the first vote, the request id and the vote number for the others.
func RecordingName(requestID string, vote int) string {
	if vote == 0 {
		return requestID
	}
	return fmt.Sprintf("%s.%d", requestID, vote)
}

// ErrSchemaViolation is returned by a ResponseValidator for an output that does
// not satisfy its schema; the message lists the violations.
var ErrSchemaViolation = errors.New("output violates the response schema")

// ResponseValidator checks an executor's output against a request's response
// schema.
type ResponseValidator interface {
	Validate(schema json.RawMessage, output []byte) error
}

// StatementValidator checks an encoded Statement against the embedded schemas.
type StatementValidator interface {
	Validate(statement []byte) error
}

// QueueItem is the queue-only metadata of a request: which criteria it serves,
// in which wave it was first emitted and what for. An executor never sees it.
type QueueItem struct {
	RequestID hostproto.RequestID `json:"requestId"`
	Criteria  []string            `json:"criteria"`
	Wave      int                 `json:"wave"`
	Purpose   string              `json:"purpose"`
}

// Queue is the module-owned queue state next to the host store.
type Queue interface {
	// Record adds the item, or adds its criteria to the item already stored for
	// the request.
	Record(item QueueItem) error
	// Items lists every item ordered by request id.
	Items() ([]QueueItem, error)
	// Wave returns the number of the current wave, zero before the first.
	Wave() (int, error)
	// AdvanceWave starts the next wave and returns its number.
	AdvanceWave() (int, error)
}

// HostStore is the host directory of the module: the shared protocol store, the
// discard of a response a reissued ticket replaces, and the queue.
type HostStore interface {
	hostproto.Store
	hostproto.ResponseDiscarder
	Queue
}
