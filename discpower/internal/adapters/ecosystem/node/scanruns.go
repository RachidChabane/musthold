package node

import (
	"strings"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

// testIDSeparator joins the test file and the titles of a test id, and
// titleSeparator the titles: `test/rates.test.ts::shipping > is free from fifty`.
const (
	testIDSeparator = "::"
	titleSeparator  = " > "
)

// The statuses Jest's reporter gives a test.
const (
	jestStatusPassed = "passed"
	jestStatusFailed = "failed"
)

// runOutcomes is what one run of the runner gave the tests: their outcomes by
// id, the tests it collected, and the positive evidence that a test did not
// start.
type runOutcomes struct {
	cases    []ports.ScanCase
	outcomes map[ecosystem.TestID]ecosystem.TestOutcome
	// stoppedFiles are the test files that stopped before any of their tests
	// began: an import, a describe body or a setup file threw.
	stoppedFiles      map[string]bool
	globalSetupFailed bool
	timedOut          bool
}

// outcomesOf reads the outcomes of one run. A test the shipped environment or
// runner reported keeps its outcome, except that an id two tests share, or for
// Jest two tests whose ids are equal ignoring case, is errored: no selection
// runs one of them alone. A test only the reporter saw ran outside the scan's
// environment, which a docblock can choose: its status counts in a run of the
// whole suite, and in a run of a selection, which it did not keep alone, it is
// errored.
func outcomesOf(record runRecord, framework scanFramework, selected bool, timedOut bool) runOutcomes {
	run := runOutcomes{outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{}, stoppedFiles: map[string]bool{}, globalSetupFailed: record.globalSetupFailed, timedOut: timedOut}
	seen := map[ecosystem.TestID]int{}
	folded := map[string]map[ecosystem.TestID]bool{}
	started := map[string]bool{}
	for _, process := range record.processes {
		for _, test := range process.tests {
			if seen[test.id] == 0 {
				run.cases = append(run.cases, ports.ScanCase{ID: test.id, File: test.file})
			}
			seen[test.id]++
			run.outcomes[test.id] = outcomeOf(test.outcome)
			if test.outcome != outcomeSkipped {
				started[test.file] = true
			}
			key := string(test.id)
			if framework.caseInsensitiveTitles() {
				key = strings.ToLower(key)
			}
			if folded[key] == nil {
				folded[key] = map[ecosystem.TestID]bool{}
			}
			folded[key][test.id] = true
		}
		for _, file := range process.failedFiles {
			run.stoppedFiles[file] = true
		}
	}
	for test, count := range seen {
		if count > 1 {
			run.outcomes[test] = ecosystem.TestErrored
		}
	}
	for _, ids := range folded {
		if len(ids) < 2 {
			continue
		}
		for test := range ids {
			run.outcomes[test] = ecosystem.TestErrored
		}
	}
	for _, result := range record.reported {
		if result.FailedToRun {
			run.stoppedFiles[result.File] = true
		}
		for _, reported := range result.Tests {
			test := ecosystem.TestID(reported.ID)
			if seen[test] > 0 {
				continue
			}
			seen[test]++
			run.cases = append(run.cases, ports.ScanCase{ID: test, File: result.File})
			run.outcomes[test] = reportedOutcome(reported, selected)
			if run.outcomes[test] != ecosystem.TestSkipped {
				started[result.File] = true
			}
		}
	}
	for file := range run.stoppedFiles {
		if started[file] {
			delete(run.stoppedFiles, file)
		}
	}
	return run
}

// outcomeOf maps the outcome the shipped environment or runner gave.
func outcomeOf(outcome string) ecosystem.TestOutcome {
	switch outcome {
	case outcomePassed:
		return ecosystem.TestPassed
	case outcomeFailed:
		return ecosystem.TestFailed
	case outcomeSkipped:
		return ecosystem.TestSkipped
	case outcomeErrored:
		return ecosystem.TestErrored
	}
	return ecosystem.TestErrored
}

// reportedOutcome maps the status of a test only the reporter saw.
func reportedOutcome(reported reportedResult, selected bool) ecosystem.TestOutcome {
	switch {
	case selected || reported.Invocations > 1:
		return ecosystem.TestErrored
	case reported.Status == jestStatusPassed:
		return ecosystem.TestPassed
	case reported.Status == jestStatusFailed:
		return ecosystem.TestFailed
	}
	return ecosystem.TestSkipped
}

// missingRun is the run of a test the run did not report: not started when
// the global setup failed or the test's file stopped before any test began,
// timed out when the process timed out, errored otherwise.
func (r runOutcomes) missingRun(test ecosystem.TestID) vacuity.Run {
	switch {
	case r.globalSetupFailed, r.stoppedFiles[fileOfTest(test)]:
		return vacuity.RunNotStarted
	case r.timedOut:
		return vacuity.RunTimedOut
	}
	return vacuity.RunErrored
}

// fileOfTest is the test file of a test id.
func fileOfTest(test ecosystem.TestID) string {
	file, _, _ := strings.Cut(string(test), testIDSeparator)
	return file
}

// titlesOfTest are the titles of a test id, outermost first.
func titlesOfTest(test ecosystem.TestID) []string {
	_, titles, _ := strings.Cut(string(test), testIDSeparator)
	return strings.Split(titles, titleSeparator)
}
