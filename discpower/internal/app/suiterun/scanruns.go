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
// tests: the run its reported outcome gives, or missing for a test the run did
// not report.
func AppendScanRun(runs map[ecosystem.TestID][]vacuity.Run, tests []ecosystem.TestID, reported map[ecosystem.TestID]ecosystem.TestOutcome, missing vacuity.Run) {
	for _, test := range tests {
		run := missing
		if outcome, found := reported[test]; found {
			run = ScanRunOf(outcome)
		}
		runs[test] = append(runs[test], run)
	}
}
