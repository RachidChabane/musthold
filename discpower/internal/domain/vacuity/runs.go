// Package vacuity holds the pure rules of `spec-proof scan` (ADR 0014): which
// mutants a scan has proven to change behaviour, and, from each test's own line
// coverage and its settled outcomes on those mutants, whether the test verifies
// nothing, caught a change, or is unproven. It reads no file and runs nothing.
package vacuity

import "github.com/RachidChabane/spec-proof/format"

// Run is what one run of a test did.
type Run string

// Outcomes of one run. A timeout and an error are neither a pass nor a failure.
const (
	RunPassed   Run = "passed"
	RunFailed   Run = "failed"
	RunTimedOut Run = "timed_out"
	RunErrored  Run = "errored"
)

// RunsPerOutcome is how many runs settle an outcome: a test is green, kills or
// survives only when every one of this many runs agrees.
const RunsPerOutcome = format.RunsPerScanOutcome

// Settle reads the runs of a test on a mutant: killed when every run of the
// required number failed, survived when every one passed, inconclusive
// otherwise, which covers a timeout, an error, a missing run and runs that
// disagree.
func Settle(runs []Run) format.ScanOutcome {
	switch {
	case all(runs, RunFailed):
		return format.ScanKilled
	case all(runs, RunPassed):
		return format.ScanSurvived
	}
	return format.ScanInconclusive
}

// Green reports whether the runs of a test on the reference all passed, as many
// as an outcome needs.
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

// failedOnce reports whether any run failed.
func failedOnce(runs []Run) bool {
	for _, run := range runs {
		if run == RunFailed {
			return true
		}
	}
	return false
}
