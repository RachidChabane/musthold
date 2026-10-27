// Package runs is the `run` verb of the loop-1 study (PREREG §6, §8): the runs of
// the frozen order, one attempt at a time, each in containers of the study's
// image through the stages of the agent-loop runner, with the study's records,
// outcomes and deviations. Its rules (the outcome of an attempt, the quota stops,
// the state of a run from its attempts, the order and the digest of an equipped
// tree) are pure; the attempt and the batch reach processes, files and the clock
// through ports.
package runs

import (
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/startup"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Facts are what the outcome of an attempt is judged on.
type Facts struct {
	// Harness is set when the harness failed the attempt before or around its
	// session: a preparation or collection step failed, the engine refused a
	// container, the session could not start, or the equipped project differs
	// from the inputs freeze. It says which.
	Harness string
	// Killed is why the runner ended the session, or empty.
	Killed string
	// Started is set once the stream held the `system/init` event.
	Started bool
	// OOMKilled is set when the session's container was killed for memory.
	OOMKilled bool
	// BudgetCapped is set when the result event reports the spend cap.
	BudgetCapped bool
	// Quota is set when the stream holds a quota stop (PREREG §6.5).
	Quota bool
	// ModelMismatch names the session model when it is not the configured one.
	ModelMismatch string
}

// Reasons of a harness error that the classification names itself.
const (
	ReasonInterrupted   = "the runner was interrupted"
	ReasonMemoryEarly   = "the container was killed for memory before the session started"
	ReasonNoInit        = "the session's stream holds no system/init event"
	ReasonModelMismatch = "system/init names another session model: "
)

// Classify judges how an attempt ended (PREREG §6.4, amendment a1) and, for a
// harness error, why.
func Classify(f Facts) (study.Outcome, string) {
	switch {
	case f.Harness != "":
		return study.OutcomeHarnessError, f.Harness
	case f.Quota:
		return study.OutcomeQuotaStop, ""
	case f.Killed == evidence.KilledInterrupted:
		return study.OutcomeHarnessError, ReasonInterrupted
	case startup.Failed(f.Killed):
		return study.OutcomeHarnessError, "the session did not start: " + f.Killed
	case f.OOMKilled && !f.Started:
		return study.OutcomeHarnessError, ReasonMemoryEarly
	case !f.Started:
		return study.OutcomeHarnessError, ReasonNoInit
	case f.ModelMismatch != "":
		return study.OutcomeHarnessError, ReasonModelMismatch + f.ModelMismatch
	case f.OOMKilled:
		return study.OutcomeCappedMemory, ""
	case f.Killed == evidence.KilledTurnCap:
		return study.OutcomeCappedMessages, ""
	case f.Killed == evidence.KilledTimeout:
		return study.OutcomeCappedTime, ""
	case f.BudgetCapped:
		return study.OutcomeCappedBudget, ""
	}
	return study.OutcomeCompleted, ""
}
