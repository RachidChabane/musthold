package promise_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/promise"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

const (
	verifiesAdmission = " [verifies SP-2189~1]"
	verifiesStability = " [verifies SP-2190~1]"
)

const (
	pass  = witness.OutcomePassed
	fail  = witness.OutcomeFailed
	flaky = witness.OutcomeFlaky
	oops  = witness.OutcomeErrored
)

func runs(outcomes ...witness.Outcome) []witness.Outcome { return outcomes }

func TestAdmit(t *testing.T) {
	cases := []struct {
		name string
		runs []witness.Outcome
		want promise.Decision
	}{
		{"three passes admit the set", runs(pass, pass, pass), promise.Decision{Holds: true}},
		{"a set the reference fails every run is a question", runs(fail, fail, fail), promise.Decision{Question: promise.QuestionReferenceFails}},
		{"a set the reference fails one run is a question", runs(pass, fail, pass), promise.Decision{Question: promise.QuestionReferenceFails}},
		{"a set that turns flaky is a question", runs(pass, flaky, pass), promise.Decision{Question: promise.QuestionReferenceFails}},
		{"two passes are not enough", runs(pass, pass), promise.Decision{}},
		{"an errored run neither admits nor questions", runs(pass, oops, pass), promise.Decision{}},
		{"no run says nothing", nil, promise.Decision{}},
	}
	for _, c := range cases {
		t.Run(c.name+verifiesAdmission, func(t *testing.T) {
			if got := promise.Admit(c.runs); got != c.want {
				t.Fatalf("decided %+v, want %+v", got, c.want)
			}
		})
	}
	t.Run("a set is admitted exactly when every one of at least three runs passed"+verifiesAdmission, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			drawn := drawOutcomes(t, "runs")
			allPassed := !slices.ContainsFunc(drawn, func(o witness.Outcome) bool { return o != pass })
			if got := promise.Admit(drawn); got.Holds != (allPassed && len(drawn) >= promise.AdmissionRuns) {
				t.Fatalf("runs %v admitted %v", drawn, got.Holds)
			}
		})
	})
}

func TestConfirm(t *testing.T) {
	t.Run("a set backs a claim only when all six runs passed"+verifiesStability, func(t *testing.T) {
		if got := promise.Confirm(runs(pass, pass, pass), runs(pass, pass, pass)); !got.Holds {
			t.Fatalf("decided %+v", got)
		}
	})
	t.Run("a single failed run of the final stage makes the set unstable and withholds its routes"+verifiesStability, func(t *testing.T) {
		got := promise.Confirm(runs(pass, pass, pass), runs(pass, fail, pass))
		if got != (promise.Decision{Question: promise.QuestionUnstable}) {
			t.Fatalf("decided %+v", got)
		}
	})
	t.Run("a missing run of the final stage is not a pass"+verifiesStability, func(t *testing.T) {
		if got := promise.Confirm(runs(pass, pass, pass), runs(pass, pass)); got.Holds {
			t.Fatalf("decided %+v", got)
		}
	})
	t.Run("a set holds exactly when the three admission runs and the three final runs all passed"+verifiesStability, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			admission, final := drawOutcomes(t, "admission"), drawOutcomes(t, "final")
			all := slices.Concat(admission, final)
			six := len(admission) >= promise.AdmissionRuns && len(final) >= promise.FinalRuns &&
				!slices.ContainsFunc(all, func(o witness.Outcome) bool { return o != pass })
			if got := promise.Confirm(admission, final); got.Holds != six {
				t.Fatalf("admission %v final %v held %v", admission, final, got.Holds)
			}
		})
	})
}

func drawOutcomes(t *rapid.T, label string) []witness.Outcome {
	outcome := rapid.SampledFrom([]witness.Outcome{pass, pass, pass, fail, flaky, oops})
	return rapid.SliceOfN(outcome, 0, 5).Draw(t, label)
}
