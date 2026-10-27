package study

// Outcome is how one attempt of a run ended (PREREG §6.4), or why a run of the
// order has no counted attempt.
type Outcome string

// The outcomes of an attempt.
const (
	// OutcomeCompleted: the CLI exited by itself.
	OutcomeCompleted Outcome = "completed"
	// OutcomeCappedMessages and OutcomeCappedTime: the runner ended the session at
	// its assistant-message or wall-time cap.
	OutcomeCappedMessages Outcome = "capped-messages"
	OutcomeCappedTime     Outcome = "capped-time"
	// OutcomeCappedBudget: the CLI's result event reports the spend cap reached.
	OutcomeCappedBudget Outcome = "capped-budget"
	// OutcomeCappedMemory: the container was killed for memory after the session
	// started.
	OutcomeCappedMemory Outcome = "capped-memory"
	// OutcomeQuotaStop: the session hit a usage or rate limit.
	OutcomeQuotaStop Outcome = "quota-stop"
	// OutcomeHarnessError: the harness, not the session, ended the attempt.
	OutcomeHarnessError Outcome = "harness-error"
)

// The states of a run of the order that holds no counted attempt.
const (
	// OutcomeQuotaBlocked and OutcomeHarnessBlocked: the run's third attempt was
	// void, for the cause the name gives.
	OutcomeQuotaBlocked   Outcome = "quota-blocked"
	OutcomeHarnessBlocked Outcome = "harness-blocked"
	// OutcomeNotRun: the calendar cap ended the run phase before the run started.
	OutcomeNotRun Outcome = "not-run"
)

// MaxAttempts is how many times one run of the order is attempted (PREREG §6.4).
const MaxAttempts = 3

// Counted reports whether an attempt with this outcome enters the analysis.
func (o Outcome) Counted() bool {
	switch o {
	case OutcomeCompleted, OutcomeCappedMessages, OutcomeCappedTime, OutcomeCappedBudget, OutcomeCappedMemory:
		return true
	}
	return false
}

// Void reports whether an attempt with this outcome is rerun.
func (o Outcome) Void() bool { return o == OutcomeQuotaStop || o == OutcomeHarnessError }

// Capped reports whether the session was ended at a cap.
func (o Outcome) Capped() bool { return o.Counted() && o != OutcomeCompleted }

// Blocked is the state a run takes when its last allowed attempt was void with
// this outcome.
func (o Outcome) Blocked() Outcome {
	if o == OutcomeQuotaStop {
		return OutcomeQuotaBlocked
	}
	return OutcomeHarnessBlocked
}
