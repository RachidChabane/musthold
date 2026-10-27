package runs

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ReasonInterruptedAttempt is the harness error of an attempt directory that
// holds no run.json: its invocation ended before the attempt was recorded.
const ReasonInterruptedAttempt = "the attempt's directory holds no run.json: its invocation ended before it was recorded"

// Interrupted reports whether an attempt's directory holds no run.json.
func Interrupted(attempt phase.Attempt) bool { return attempt.Run.Outcome == "" }

// NextAttempt is the number of a run's next attempt: one past its last.
func NextAttempt(attempts []phase.Attempt) int {
	if len(attempts) == 0 {
		return 1
	}
	return attempts[len(attempts)-1].Number + 1
}

// NotBefore is the reset time a run's last attempt recorded when it was a quota
// stop: the run starts no attempt before it (PREREG §6.5).
func NotBefore(attempts []phase.Attempt) *time.Time {
	if len(attempts) == 0 {
		return nil
	}
	last := attempts[len(attempts)-1].Run
	if last.Outcome != study.OutcomeQuotaStop {
		return nil
	}
	return last.QuotaReset
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
