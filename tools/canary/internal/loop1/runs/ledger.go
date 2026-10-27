package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// CalendarCap is how long the run phase lasts after the start of its first
// attempt (PREREG §6.6, amendment a1).
const CalendarCap = 28 * 24 * time.Hour

// ReasonInterruptedAttempt is the harness error of an attempt directory that
// holds no run.json: its invocation ended before the attempt was recorded.
const ReasonInterruptedAttempt = "the attempt's directory holds no run.json: its invocation ended before it was recorded"

// Attempt is one attempt of a run, as its directory holds it.
type Attempt struct {
	Number int
	// Run is the attempt's run.json, or nil when the directory holds none.
	Run *study.Run
}

// Outcome is the attempt's outcome; an attempt without run.json is a harness
// error.
func (a Attempt) Outcome() study.Outcome {
	if a.Run == nil {
		return study.OutcomeHarnessError
	}
	return a.Run.Outcome
}

// Ledger is every attempt of one run of the order, by number.
type Ledger struct {
	Name     string
	Attempts []Attempt
}

// Counted returns the counted attempt of the run, if it has one.
func (l Ledger) Counted() (Attempt, bool) {
	for _, attempt := range l.Attempts {
		if attempt.Outcome().Counted() {
			return attempt, true
		}
	}
	return Attempt{}, false
}

// HarnessErrors counts the attempts that are harness errors: only they use up
// the attempts of a run (amendment a1).
func (l Ledger) HarnessErrors() int {
	count := 0
	for _, attempt := range l.Attempts {
		if attempt.Outcome() == study.OutcomeHarnessError {
			count++
		}
	}
	return count
}

// HarnessBlocked reports whether the run reached its limit of harness errors.
func (l Ledger) HarnessBlocked() bool {
	_, counted := l.Counted()
	return !counted && l.HarnessErrors() >= study.MaxAttempts
}

// Done reports whether the run needs no further attempt: it is counted or
// harness-blocked.
func (l Ledger) Done() bool {
	_, counted := l.Counted()
	return counted || l.HarnessBlocked()
}

// Last is the run's last attempt, if it has one.
func (l Ledger) Last() (Attempt, bool) {
	if len(l.Attempts) == 0 {
		return Attempt{}, false
	}
	return l.Attempts[len(l.Attempts)-1], true
}

// Next is the number of the run's next attempt.
func (l Ledger) Next() int {
	last, found := l.Last()
	if !found {
		return 1
	}
	return last.Number + 1
}

// NotBefore is the reset time a last attempt that was a quota stop recorded,
// before which the run starts no attempt.
func (l Ledger) NotBefore() *time.Time {
	last, found := l.Last()
	if !found || last.Run == nil || last.Run.Outcome != study.OutcomeQuotaStop {
		return nil
	}
	return last.Run.QuotaReset
}

// State is the run's state in the run phase: the outcome of its counted attempt,
// harness-blocked, or, once the calendar cap has ended the phase, quota-blocked,
// harness-blocked or not-run after its last attempt. Before the cap a run that
// still needs an attempt has the empty state.
func (l Ledger) State(capped bool) study.Outcome {
	if counted, found := l.Counted(); found {
		return counted.Outcome()
	}
	if l.HarnessBlocked() {
		return study.OutcomeHarnessBlocked
	}
	if !capped {
		return ""
	}
	last, found := l.Last()
	if !found {
		return study.OutcomeNotRun
	}
	return last.Outcome().Blocked()
}

// FirstStart is the start of the first attempt of the run phase, if any started.
func FirstStart(ledgers []Ledger) *time.Time {
	var first *time.Time
	for _, ledger := range ledgers {
		for _, attempt := range ledger.Attempts {
			if attempt.Run == nil || attempt.Run.StartedAt.IsZero() {
				continue
			}
			if started := attempt.Run.StartedAt; first == nil || started.Before(*first) {
				first = &started
			}
		}
	}
	return first
}

// Capped reports whether the calendar cap has ended the run phase at now.
func Capped(first *time.Time, now time.Time) bool {
	return first != nil && !now.Before(first.Add(CalendarCap))
}

// PhaseOver reports whether the run phase has ended: every run of the order is
// counted or blocked, or the calendar cap is reached (PREREG §6.6).
func PhaseOver(ledgers []Ledger, now time.Time) bool {
	if Capped(FirstStart(ledgers), now) {
		return true
	}
	for _, ledger := range ledgers {
		if !ledger.Done() {
			return false
		}
	}
	return true
}

// ErrOrder refuses an order that is not the configuration's seeded order or the
// inputs freeze's.
var ErrOrder = errors.New("the order of the runs differs")

// Order is the names of the configuration's runs, in the order its seed gives
// (PREREG §6.1).
func Order(c config.Config) []string {
	var names []string
	for _, run := range c.Runs() {
		names = append(names, run.Name())
	}
	return names
}

// CheckOrder refuses an inputs freeze whose order is not the configuration's.
func CheckOrder(c config.Config, inputs study.Inputs) error {
	if want := Order(c); !slices.Equal(want, inputs.Order) {
		return fmt.Errorf("%w: the configuration's seed gives %v, the inputs freeze holds %v", ErrOrder, want, inputs.Order)
	}
	return nil
}
