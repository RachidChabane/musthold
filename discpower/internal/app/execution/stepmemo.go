package execution

import (
	"context"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

// StepMemo keeps the executed facts of one step: it counts what the step stored
// and stores nothing once the step's context is done, so that a fact an
// interrupted execution produced, or a stage turned into a fact after it swallowed
// the interruption, is never memoized.
type StepMemo struct {
	ports.ExecutionMemo
	ctx    context.Context
	stored int
}

// NewStepMemo keeps the facts of the step whose context is given in the memo.
func NewStepMemo(ctx context.Context, memo ports.ExecutionMemo) *StepMemo {
	return &StepMemo{ExecutionMemo: memo, ctx: ctx}
}

// Put stores the fact while the step runs, and refuses it with the cause of the
// step's end once its context is done.
func (m *StepMemo) Put(key string, value []byte) error {
	if m.ctx.Err() != nil {
		return context.Cause(m.ctx)
	}
	if err := m.ExecutionMemo.Put(key, value); err != nil {
		return err
	}
	m.stored++
	return nil
}

// Stored counts the facts the step stored.
func (m *StepMemo) Stored() int { return m.stored }
