package hostprototest

import (
	"fmt"

	"pgregory.net/rapid"
)

// Severity orders the outcomes of a gate decision from the most pass-favourable
// to the most blocking.
type Severity int

// Severities.
const (
	SeverityPass Severity = iota
	SeverityEscalate
	SeverityBlock
)

// maxExtraResponses bounds how many responses the property adds.
const maxExtraResponses = 4

// FindMonotonicityViolation adds the responses one at a time to the base set and
// returns an error as soon as the decision's severity drops. Every response added
// must point toward block or escalate, which the caller guarantees.
func FindMonotonicityViolation[R any](decide func([]R) Severity, base, additions []R) error {
	previous := decide(base)
	current := append([]R(nil), base...)
	for index, addition := range additions {
		current = append(current, addition)
		after := decide(current)
		if after < previous {
			return fmt.Errorf("adding hardening response %d lowered the severity from %d to %d", index, previous, after)
		}
		previous = after
	}
	return nil
}

// CheckMonotone checks the monotonicity property of ADR 0005 on a decision
// function: adding any response that points toward a block or an escalation never
// lowers the severity of the decision, so BLOCK never turns into PASS.
//
// decide maps a set of responses to a severity, hardens says whether a response
// points toward block or escalate, and responses generates responses of any
// direction. Modules that compose verdicts from host responses call it inside
// rapid.Check.
func CheckMonotone[R any](t *rapid.T, decide func([]R) Severity, hardens func(R) bool, responses *rapid.Generator[R]) {
	base := rapid.SliceOfN(responses, 0, maxExtraResponses).Draw(t, "base")
	additions := rapid.SliceOfN(responses.Filter(hardens), 1, maxExtraResponses).Draw(t, "additions")
	if err := FindMonotonicityViolation(decide, base, additions); err != nil {
		t.Fatal(err)
	}
}
