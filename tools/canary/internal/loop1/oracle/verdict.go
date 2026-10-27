package oracle

import (
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Pytest exit codes. Only these two mean that the tests ran: 0 all passed, 1 some
// failed. Any other code (interrupted by a collection or import error, internal
// error, usage error, nothing collected) means that no test result stands.
const (
	exitAllPassed  = 0
	exitSomeFailed = 1
)

// RunState is how one pytest run ended.
type RunState int

// The states of a pytest run.
const (
	// RunExecuted: the tests ran and the JUnit report lists them.
	RunExecuted RunState = iota
	// RunCollectionFailed: collection or import failed, so no result stands.
	RunCollectionFailed
	// RunTimedOut: the run reached its limit.
	RunTimedOut
	// RunOOMKilled: the container was killed for memory, which counts as a run
	// that reached its limit.
	RunOOMKilled
)

// RunReport is what one pytest run established: the failed flag of every test it
// listed. A run that is not executed counts as every test failing.
type RunReport struct {
	State  RunState
	Failed map[string]bool
}

// ClassifyRun turns the end of a pytest command into a report. The rule: a run
// that reached its limit is timed out, and a run killed for memory counts the
// same; a run whose exit code is neither 0 nor 1, or whose JUnit report is
// missing, unreadable or empty, failed at collection or import; any other run
// lists its cases, each failing when it has a failure or error child.
func ClassifyRun(result box.Result, junit []byte) RunReport {
	switch {
	case result.TimedOut:
		return RunReport{State: RunTimedOut}
	case result.OOMKilled:
		return RunReport{State: RunOOMKilled}
	}
	if exitCode := result.ExitCode; exitCode != exitAllPassed && exitCode != exitSomeFailed {
		return RunReport{State: RunCollectionFailed}
	}
	cases, err := ParseJUnit(junit)
	if err != nil {
		return RunReport{State: RunCollectionFailed}
	}
	failed := make(map[string]bool, len(cases))
	for _, c := range cases {
		failed[c.ID] = failed[c.ID] || c.Failed
	}
	return RunReport{State: RunExecuted, Failed: failed}
}

func (r RunReport) executed() bool { return r.State == RunExecuted }

// failedTest reports whether a test failed in this run: every test fails in a run
// that did not execute, and a test the run did not list did not pass.
func (r RunReport) failedTest(id string) bool {
	if !r.executed() {
		return true
	}
	failed, listed := r.Failed[id]
	return failed || !listed
}

// universe lists the tests the executed runs listed, sorted. When no run
// executed, the test files stand for the tests that could not be listed.
func universe(reports []RunReport, files []string) []string {
	seen := map[string]bool{}
	for _, report := range reports {
		for id := range report.Failed {
			seen[id] = true
		}
	}
	if len(seen) == 0 {
		return slices.Clone(files)
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// OracleVerdict is the oracle record of one final change (PREREG §4.1, §4.2):
// each test's failures in the three runs, the defect and the violated criteria.
// The files stand for the tests when no run could list them.
func OracleVerdict(reports [study.OracleRuns]RunReport, files []string) study.Oracle {
	ids := universe(reports[:], files)
	oracle := study.Oracle{Tests: make([]study.OracleTest, 0, len(ids)), Violated: []string{}, Flaky: []string{}}
	for run, report := range reports {
		oracle.TimedOut[run] = report.State == RunTimedOut
		oracle.OOMKilled[run] = report.State == RunOOMKilled
	}
	violated := map[string]bool{}
	for _, id := range ids {
		test := study.OracleTest{ID: id, Criterion: criterionOfTest(id)}
		for run, report := range reports {
			test.Failed[run] = report.failedTest(id)
		}
		oracle.Tests = append(oracle.Tests, test)
		if test.Defective() {
			oracle.Defective = true
			if test.Criterion != "" {
				violated[test.Criterion] = true
			}
		}
		if test.Flaky() {
			oracle.Flaky = append(oracle.Flaky, id)
		}
	}
	for criterion := range violated {
		oracle.Violated = append(oracle.Violated, criterion)
	}
	slices.Sort(oracle.Violated)
	return oracle
}

// KillVerdict is the kill matrix of one test directory (PREREG §4.3). reference
// holds the three runs on the reference tree; violations the three runs on each
// violation, by name. The valid tests pass on the reference in all three runs;
// every other test the reference runs listed contradicts the spec. When no
// reference run could list the tests, the files stand for the contradicting
// tests. A violation is killed when a valid test fails on it in at least two of
// three runs.
func KillVerdict(reference [study.HeldOutRuns]RunReport, violations map[string][study.HeldOutRuns]RunReport, files []string) study.KillMatrix {
	matrix := study.KillMatrix{Valid: []string{}, Contradicting: []string{}, Killed: map[string]bool{}}
	for _, id := range universe(reference[:], files) {
		if passesEveryRun(reference, id) {
			matrix.Valid = append(matrix.Valid, id)
		} else {
			matrix.Contradicting = append(matrix.Contradicting, id)
		}
	}
	for name, reports := range violations {
		matrix.Killed[name] = killedBy(matrix.Valid, reports)
	}
	return matrix
}

func passesEveryRun(reports [study.HeldOutRuns]RunReport, id string) bool {
	return !slices.ContainsFunc(reports[:], func(r RunReport) bool { return r.failedTest(id) })
}

func killedBy(valid []string, reports [study.HeldOutRuns]RunReport) bool {
	return slices.ContainsFunc(valid, func(id string) bool {
		failures := 0
		for _, report := range reports {
			if report.failedTest(id) {
				failures++
			}
		}
		return failures >= study.HeldOutQuorum
	})
}
