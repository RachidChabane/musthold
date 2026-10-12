package scan

import (
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

func TestStoppedSentinels(t *testing.T) {
	t.Run("a sentinel on which a run of a test did not start is counted once; an operator mutant or a sentinel that did not run is not [verifies SP-2129~3]", func(t *testing.T) {
		const first, second = ecosystem.TestID("tests/test_a.py::test_a"), ecosystem.TestID("tests/test_b.py::test_b")
		stopped := map[ecosystem.TestID][]vacuity.Run{
			first:  {vacuity.RunNotStarted, vacuity.RunPassed, vacuity.RunPassed},
			second: {vacuity.RunNotStarted, vacuity.RunNotStarted, vacuity.RunNotStarted},
		}
		passed := map[ecosystem.TestID][]vacuity.Run{first: {vacuity.RunPassed, vacuity.RunPassed, vacuity.RunPassed}}
		e := emission{
			drawn: drawn{candidates: []Candidate{
				{ID: "stopped sentinel", Sentinel: true}, {ID: "passed sentinel", Sentinel: true},
				{ID: "unrun sentinel", Sentinel: true}, {ID: "stopping operator mutant"},
			}},
			executed: []outcome{
				{state: stateRan, runs: stopped}, {state: stateRan, runs: passed},
				{state: stateNotRun}, {state: stateRan, runs: stopped},
			},
		}
		if got := e.stoppedSentinels(); got != 1 {
			t.Fatalf("stopped sentinels = %d", got)
		}
	})
}
