// Package vacuity holds the pure rules of `musthold scan` (ADR 0014): which
// mutants a scan has proven to change behaviour, and, from each test's own line
// coverage and its settled outcomes on those mutants, whether the test verifies
// nothing, caught a change, or is unproven. It reads no file and runs nothing.
package vacuity

import "github.com/RachidChabane/musthold/format"

// Run is what one run of a test did.
type Run string

// Outcomes of one run. A timeout, an error and a run that did not start are
// neither a pass nor a failure.
const (
	RunPassed   Run = "passed"
	RunFailed   Run = "failed"
	RunTimedOut Run = "timed_out"
	RunErrored  Run = "errored"
	// RunNotStarted: the process of the run stopped before any test began, on
	// positive evidence the runner gives, never for a run that is merely
	// missing.
	RunNotStarted Run = "not_started"
)

// RunsPerOutcome is how many runs settle an outcome: a test is green, kills or
// survives only when every one of this many runs agrees.
const RunsPerOutcome = format.RunsPerScanOutcome

// Settled is the settled outcome of a test on a mutant: one of the outcomes a
// scan statement records as a change, or not_started, which a scan statement
// lists apart and which counts neither for nor against the test.
type Settled string

// Settled outcomes of a test on a mutant.
const (
	SettledKilled       = Settled(format.ScanKilled)
	SettledSurvived     = Settled(format.ScanSurvived)
	SettledInconclusive = Settled(format.ScanInconclusive)
	// SettledNotStarted: every run stopped before any test began.
	SettledNotStarted Settled = "not_started"
)

// Recorded is the outcome a scan statement records for a change settled so;
// not_started has none.
func (s Settled) Recorded() (format.ScanOutcome, bool) {
	if s == SettledNotStarted {
		return "", false
	}
	return format.ScanOutcome(s), true
}

// Settle reads the runs of a test on a mutant: not_started when every run of
// the required number did not start, killed when every one failed, survived
// when every one passed, and inconclusive otherwise, which covers a timeout, an
// error, a missing run, runs that disagree and any mix with a run that did not
// start.
func Settle(runs []Run) Settled {
	switch {
	case all(runs, RunNotStarted):
		return SettledNotStarted
	case all(runs, RunFailed):
		return SettledKilled
	case all(runs, RunPassed):
		return SettledSurvived
	}
	return SettledInconclusive
}

// Green reports whether the runs of a test on the reference all passed, as many
// as an outcome needs; a run that did not start is no pass.
func Green(runs []Run) bool { return all(runs, RunPassed) }

// all reports whether there are exactly RunsPerOutcome runs and every one is the
// given outcome.
func all(runs []Run, outcome Run) bool {
	if len(runs) != RunsPerOutcome {
		return false
	}
	for _, run := range runs {
		if run != outcome {
			return false
		}
	}
	return true
}

// failedOnce reports whether any run failed; a run that did not start never
// failed.
func failedOnce(runs []Run) bool {
	for _, run := range runs {
		if run == RunFailed {
			return true
		}
	}
	return false
}
