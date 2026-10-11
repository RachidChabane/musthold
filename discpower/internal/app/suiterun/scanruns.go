package suiterun

import (
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

// ScanRunOf reads one test outcome as a run of a scan: only a pass is a pass and
// only a failure a failure; a skip and an error are neither.
func ScanRunOf(outcome ecosystem.TestOutcome) vacuity.Run {
	switch outcome {
	case ecosystem.TestPassed:
		return vacuity.RunPassed
	case ecosystem.TestFailed:
		return vacuity.RunFailed
	}
	return vacuity.RunErrored
}

// AppendScanRun appends one run of a selection to the runs of each of its
// tests: the run its reported outcome gives, or, for a test the run did not
// report, the run missing gives it.
func AppendScanRun(runs map[ecosystem.TestID][]vacuity.Run, tests []ecosystem.TestID, reported map[ecosystem.TestID]ecosystem.TestOutcome, missing func(ecosystem.TestID) vacuity.Run) {
	for _, test := range tests {
		var run vacuity.Run
		if outcome, found := reported[test]; found {
			run = ScanRunOf(outcome)
		} else {
			run = missing(test)
		}
		runs[test] = append(runs[test], run)
	}
}

// Always gives every test a run of the same outcome.
func Always(run vacuity.Run) func(ecosystem.TestID) vacuity.Run {
	return func(ecosystem.TestID) vacuity.Run { return run }
}
