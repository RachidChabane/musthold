package hostproto

import (
	"context"
	"errors"
	"time"
)

// Step bounds. A command that waits on a host, `prove` and a scan that asks the
// host, works in steps: each invocation stops once its work budget is spent,
// keeps what it established, and returns within StepLimit, so that a harness
// whose tool calls time out never sees one outlive it. Running the command again
// continues from what is stored.
const (
	// StepLimit is the most time one step takes, persisting and reporting
	// included.
	StepLimit = 90 * time.Second
	// DefaultStepWork is the work budget of a step when the configuration names
	// none.
	DefaultStepWork = 60 * time.Second
	// MaxStepWork is the largest work budget a configuration may name: it leaves
	// a step the time to persist and report within StepLimit.
	MaxStepWork = 75 * time.Second
	// MinStepWork is the smallest work budget a configuration may name.
	MinStepWork = time.Second
)

// ErrStepSpent is the cause of a step context whose work budget ran out.
var ErrStepSpent = errors.New("the step spent its work budget")

// WithStep bounds the work of one step: the returned context is done once the
// budget is spent, with ErrStepSpent as its cause.
func WithStep(parent context.Context, work time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeoutCause(parent, work, ErrStepSpent)
}

// StepSpent reports whether the step's work budget ran out. Whatever error a
// stage returned once it did is a consequence of the cut, not a finding.
func StepSpent(ctx context.Context) bool { return errors.Is(context.Cause(ctx), ErrStepSpent) }

// StepEnd says why a step returned before its run finished.
type StepEnd string

// Ends of a step.
const (
	// StepFinished: the step did all the work it could before the host answers.
	StepFinished StepEnd = ""
	// StepBudgetSpent: the work budget ran out; running the same command again
	// continues.
	StepBudgetSpent StepEnd = "budget_spent"
	// StepSessionBusy: another run holds the session; running the same command
	// again once it returned continues.
	StepSessionBusy StepEnd = "session_busy"
)

// UnfinishedMarker starts the line a command prints when its step ended before
// its run finished: `unfinished <end>`.
const UnfinishedMarker = "unfinished"

// Unfinished reports whether the step ended before its run finished.
func (e StepEnd) Unfinished() bool { return e != StepFinished }

// Line is the line a command prints for an unfinished step.
func (e StepEnd) Line() string { return UnfinishedMarker + " " + string(e) }
