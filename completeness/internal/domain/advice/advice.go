// Package advice holds the rules that turn what System One said into the signals
// that raise a question (ADR 0003, "Completeness"). The rules are pure and use no
// verdict: the output of completeness is advice, and no rule here can block.
package advice

import (
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/blind"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/closedq"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/dimension"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/prompt"
	"github.com/RachidChabane/spec-proof/completeness/internal/domain/tournament"
)

// Gap is a dimension System One found missing from a spec.
type Gap struct {
	Dimension dimension.Dimension
	// LowConfidence is true when MISSING was the answer but its confidence is
	// below tau_adv.
	LowConfidence bool
}

// DimensionGaps lists the asked dimensions System One answered MISSING, in
// checklist order: with confidence at or above tauAdv, or flagged low confidence
// below it. A dimension that was not asked, such as one declared out of scope,
// never yields a gap, whatever the readings hold.
func DimensionGaps(asked []dimension.Dimension, readings map[string]closedq.Decided, tauAdv float64) []Gap {
	var gaps []Gap
	for _, d := range asked {
		decided, answered := readings[string(d)]
		if !answered || dimension.Standing(decided.Choice) != dimension.Missing {
			continue
		}
		gaps = append(gaps, Gap{Dimension: d, LowConfidence: decided.Confidence < tauAdv})
	}
	return gaps
}

// Unjustified reports whether the tournament over a change ended on NONE at a
// confidence at or above tauAdv: no requirement states the change.
func Unjustified(outcome tournament.Outcome, tauAdv float64) bool {
	return outcome.Done && outcome.Winner == blind.NoneAnswer && outcome.Confidence >= tauAdv
}

// Unclear lists the tokens System One found AMBIGUOUS or UNTESTABLE, with the
// standing, in the order of the tokens. The answer is the argmax; no threshold
// applies, because a requirement System One doubts is worth a second reading.
func Unclear(tokens []string, readings map[string]closedq.Decided) map[string]prompt.Testability {
	unclear := map[string]prompt.Testability{}
	for _, token := range tokens {
		decided, answered := readings[token]
		if !answered {
			continue
		}
		switch standing := prompt.Testability(decided.Choice); standing {
		case prompt.Ambiguous, prompt.Untestable:
			unclear[token] = standing
		}
	}
	return unclear
}
