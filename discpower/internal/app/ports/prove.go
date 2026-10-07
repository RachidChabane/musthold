package ports

import (
	"context"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
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

// StatementValidator checks an encoded Statement against the embedded schemas.
type StatementValidator interface {
	Validate(statement []byte) error
}
