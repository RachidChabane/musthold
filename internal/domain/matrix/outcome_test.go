package matrix_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/matrix"
)

const (
	testA ecosystem.TestID = "test_a"
	testB ecosystem.TestID = "test_b"
)

func run(outcomes map[ecosystem.TestID]ecosystem.TestOutcome) ecosystem.RunResult {
	return ecosystem.RunResult{Outcomes: outcomes}
}

func TestClassify(t *testing.T) {
	bound := []ecosystem.TestID{testA, testB}
	passed, failed := ecosystem.TestPassed, ecosystem.TestFailed
	cases := []struct {
		name     string
		evidence matrix.Evidence
		want     format.Outcome
	}{
		{"every bound test passed", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed, testB: passed})}, format.OutcomeSurvived},
		{"one bound test failed", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed, testB: failed})}, format.OutcomeKilled},
		{"every bound test failed", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: failed, testB: failed})}, format.OutcomeKilled},
		{"the build check failed", matrix.Evidence{BuildOK: false, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: failed, testB: failed})}, format.OutcomeError},
		{"collection failed", matrix.Evidence{BuildOK: true, Bound: bound, Run: ecosystem.RunResult{CollectionFailed: true}}, format.OutcomeError},
		{"the run timed out", matrix.Evidence{BuildOK: true, Bound: bound, Run: ecosystem.RunResult{TimedOut: true, Outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{testA: failed}}}, format.OutcomeError},
		{"a bound test is absent", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed})}, format.OutcomeError},
		{"a bound test is skipped", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed, testB: ecosystem.TestSkipped})}, format.OutcomeError},
		{"a bound test errored without running", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed, testB: ecosystem.TestErrored})}, format.OutcomeError},
		{"another bound test failed although one is absent", matrix.Evidence{BuildOK: true, Bound: bound, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: failed})}, format.OutcomeKilled},
		{"no test is bound", matrix.Evidence{BuildOK: true, Run: run(nil)}, format.OutcomeError},
		{"a failure of an unbound test does not count", matrix.Evidence{BuildOK: true, Bound: []ecosystem.TestID{testA}, Run: run(map[ecosystem.TestID]ecosystem.TestOutcome{testA: passed, testB: failed})}, format.OutcomeSurvived},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-30~1]", func(t *testing.T) {
			if got := matrix.Classify(c.evidence); got != c.want {
				t.Fatalf("outcome = %s, want %s", got, c.want)
			}
		})
	}

	t.Run("the outcome is always one of the three outcomes of the format [verifies SP-30~1]", func(t *testing.T) {
		outcomes := []ecosystem.TestOutcome{ecosystem.TestPassed, ecosystem.TestFailed, ecosystem.TestErrored, ecosystem.TestSkipped}
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 4).Draw(t, "count")
			evidence := matrix.Evidence{
				BuildOK: rapid.Bool().Draw(t, "buildOK"),
				Run:     ecosystem.RunResult{Outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{}, TimedOut: rapid.Bool().Draw(t, "timedOut")},
			}
			for index := 0; index < count; index++ {
				id := ecosystem.TestID(rapid.StringMatching(`t[0-9]`).Draw(t, "id"))
				evidence.Bound = append(evidence.Bound, id)
				if rapid.Bool().Draw(t, "reported") {
					evidence.Run.Outcomes[id] = rapid.SampledFrom(outcomes).Draw(t, "outcome")
				}
			}
			got := matrix.Classify(evidence)
			for _, known := range format.Outcomes() {
				if got == known {
					return
				}
			}
			t.Fatalf("unknown outcome %q", got)
		})
	})
}
