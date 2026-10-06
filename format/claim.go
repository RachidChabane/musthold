package format

import (
	"fmt"
	"math"
)

// Claim is a verdict-bearing value with the class of its evidence. A judged claim
// carries a confidence in [0,1]; an executed claim never does. Build claims with
// Executed and Judged so the invariant holds by construction; Validate checks
// claims that were decoded from the wire.
type Claim[V comparable] struct {
	Value      V        `json:"value"`
	Class      Class    `json:"class"`
	Confidence *float64 `json:"confidence,omitempty"`
}

// Executed builds a claim backed by an execution result.
func Executed[V comparable](value V) Claim[V] {
	return Claim[V]{Value: value, Class: ClassExecuted}
}

// Judged builds a claim backed by a judgment. The confidence must lie in [0,1].
func Judged[V comparable](value V, confidence float64) (Claim[V], error) {
	if !confidenceInRange(confidence) {
		return Claim[V]{}, fmt.Errorf("confidence %v is outside [%v,%v]", confidence, MinConfidence, MaxConfidence)
	}
	return Claim[V]{Value: value, Class: ClassJudged, Confidence: &confidence}, nil
}

func confidenceInRange(confidence float64) bool {
	return !math.IsNaN(confidence) && confidence >= MinConfidence && confidence <= MaxConfidence
}

// check reports every violation of the claim rules at path. allowed is the class
// the predicate permits for this claim.
func (c Claim[V]) check(sink *collector, path string, allowed Class) {
	if c.Class != ClassExecuted && c.Class != ClassJudged {
		sink.add(CodeUnknownValue, path+".class", "class %q is neither %q nor %q", c.Class, ClassExecuted, ClassJudged)
		return
	}
	if c.Class != allowed {
		sink.add(CodeWrongClass, path+".class", "class %q where the predicate requires %q", c.Class, allowed)
	}
	switch {
	case c.Class == ClassJudged && c.Confidence == nil:
		sink.add(CodeJudgedWithoutConfidence, path, "a judged claim requires a confidence")
	case c.Class == ClassJudged && !confidenceInRange(*c.Confidence):
		sink.add(CodeConfidenceRange, path+".confidence", "confidence %v is outside [%v,%v]", *c.Confidence, MinConfidence, MaxConfidence)
	case c.Class == ClassExecuted && c.Confidence != nil:
		sink.add(CodeExecutedWithConfidence, path, "an executed claim forbids a confidence")
	}
}
