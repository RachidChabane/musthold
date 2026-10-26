package prove_test

import (
	"fmt"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/policy"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/format"
)

const (
	ownTestCount   = 2
	otherTestCount = 2
	maxVotes       = 4
)

var criterion = spec.Ref{ID: "PAR-3", Revision: 1}

func testsNamed(prefix string, count int) []ecosystem.TestID {
	tests := make([]ecosystem.TestID, count)
	for index := range tests {
		tests[index] = ecosystem.TestID(fmt.Sprintf("%s%d", prefix, index))
	}
	return tests
}

var otherCriterion = spec.Ref{ID: "PAR-4", Revision: 1}

func subject() power.Subject {
	bindings := map[ecosystem.TestID][]spec.Ref{}
	for _, test := range testsNamed("own", ownTestCount) {
		bindings[test] = []spec.Ref{criterion}
	}
	for _, test := range testsNamed("other", otherTestCount) {
		bindings[test] = []spec.Ref{otherCriterion}
	}
	return power.NewSubject(criterion, bindings, map[ecosystem.TestID]bool{})
}

func outcomes() *rapid.Generator[format.Outcome] {
	return rapid.SampledFrom([]format.Outcome{format.OutcomeKilled, format.OutcomeSurvived, format.OutcomeError})
}

// facts draws the executed facts of a judged mutant: no example failed.
func facts(t *rapid.T) power.Facts {
	results := map[ecosystem.TestID]format.Outcome{}
	for _, test := range append(subject().Tests, subject().OtherTests...) {
		results[test] = outcomes().Draw(t, string(test))
	}
	status := rapid.SampledFrom([]power.DifferentialStatus{power.DiffersAt, power.NoDiffFound, power.Skipped}).Draw(t, "differential")
	return power.Facts{ID: "mutant", Origin: power.OriginTargeted, Sites: []string{"parcel/rates.py::shipping_fee"}, Outcomes: results, Differential: power.Differential{Status: status}}
}

func rules(t *rapid.T) policy.Policy {
	rules := policy.Default()
	rules.SignedPassBasis = rapid.SampledFrom(policy.Bases()).Draw(t, "basis")
	rules.MinIsolation = rapid.SampledFrom(format.Isolations()).Draw(t, "floor")
	rules.TauJudge = format.UniformThreshold(rapid.Float64Range(0, 1).Draw(t, "tau"))
	return rules
}

// vote draws a judge's vote; the witness checks allowed are the given ones.
func vote(t *rapid.T, label string, witnesses []power.WitnessCheck) power.Vote {
	return power.Vote{
		Answer:        rapid.SampledFrom([]power.Answer{power.AnswerValid, power.AnswerEquivalent, power.AnswerNotViolating, power.AnswerUndetermined}).Draw(t, label+"answer"),
		Confidence:    rapid.Float64Range(0, 1).Draw(t, label+"confidence"),
		Isolation:     rapid.SampledFrom(format.Isolations()).Draw(t, label+"isolation"),
		Marked:        rapid.Bool().Draw(t, label+"marked"),
		Witness:       rapid.SampledFrom(witnesses).Draw(t, label+"witness"),
		WitnessDigest: format.SHA256Hex([]byte(label)),
	}
}
