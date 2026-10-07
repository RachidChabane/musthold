// Package ports declares what the drain use case needs from the outside world:
// the proof module, the System One endpoint, the machine's memory and the log.
package ports

import (
	"context"

	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/question"
)

// Exported is one closed request a module waits to have answered. When the module
// emitted a question set the relay cannot read, Problem says why and Set is empty.
type Exported struct {
	Ticket  string
	Nonce   string
	Set     question.Set
	Problem error
}

// ModuleGateway is the closed-request side of a proof module.
type ModuleGateway interface {
	// Export lists the closed requests that wait for their first vote.
	Export(ctx context.Context) ([]Exported, error)
	// Import hands the reply of the endpoint to one request to the module under a
	// source name; the module translates it with the question set of the ticket.
	Import(ctx context.Context, ticket, nonce, source string, reply []byte) error
}

// Decision is what an endpoint answered to one question set.
type Decision struct {
	// Model is the model that answered: the configured one, else the one the
	// endpoint named, else empty.
	Model   string
	Answers question.Answers
	// Reply is the body the endpoint answered, as the module imports it.
	Reply []byte
}

// SystemOne asks a System One endpoint one question set at a time.
type SystemOne interface {
	// Decide returns the answers to the set, or an error carrying a failure kind.
	Decide(ctx context.Context, set question.Set) (Decision, error)
}

// UnloadResult says what an unload attempt found.
type UnloadResult struct {
	// Applicable is false when the endpoint is not one that holds a model in memory.
	Applicable bool
	// Detail explains a skipped unload.
	Detail string
}

// Unloader frees the memory a local model server holds after a batch.
type Unloader interface {
	Unload(ctx context.Context, model string) (UnloadResult, error)
}

// MemoryVerdict is the answer of the memory rule.
type MemoryVerdict struct {
	// Allowed is true when the machine may run a batch.
	Allowed bool
	// Skipped is true when the rule does not apply on this system.
	Skipped bool
	// Reason explains a refusal or a skip.
	Reason string
}

// MemoryGuard is the repository's memory rule for heavy local work.
type MemoryGuard interface {
	Check(ctx context.Context) (MemoryVerdict, error)
}

// Logger receives the progress lines of a drain.
type Logger interface {
	Logf(format string, args ...any)
}
