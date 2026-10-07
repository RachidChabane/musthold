// Package policy holds the decision parameters of alignment (ADR 0003, "Routing,
// thresholds, calibration"; ADR 0005, "Claim power") and the digest that stands
// for them as the `decision_policy` input of a proof.
package policy

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format"
)

// Defaults of the thresholds and limits.
const (
	DefaultTauRoute          = 0.90
	DefaultAlignK            = 5
	DefaultModalMin          = 0.80
	DefaultThetaOwn          = 0.50
	DefaultThetaOther        = 0.20
	DefaultThetaNone         = 0.20
	DefaultShadowRate        = 0.10
	DefaultMaxReasoningCalls = 40
	DefaultDecideK           = 3
)

// Policy is the decision policy of a run.
type Policy struct {
	// TauRoute is the raw confidence a System One answer needs for the two-signal
	// ALIGNED verdict.
	TauRoute float64 `json:"tauRoute"`
	// AlignK is the number of blind judge tickets of one alignment request.
	AlignK int `json:"alignK"`
	// ModalMin is the modal share a judge verdict needs.
	ModalMin float64 `json:"modalMin"`
	// ThetaOwn, ThetaOther and ThetaNone are the matrix thresholds.
	ThetaOwn   float64 `json:"thetaOwn"`
	ThetaOther float64 `json:"thetaOther"`
	ThetaNone  float64 `json:"thetaNone"`
	// ShadowRate is the share of two-signal ALIGNED verdicts audited by the judge.
	ShadowRate float64 `json:"shadowRate"`
	// MaxReasoningCalls bounds the judge and decider tickets of one test.
	MaxReasoningCalls int `json:"maxReasoningCalls"`
	// DecideK is the number of host votes of a closed question when no System One
	// endpoint answers.
	DecideK int `json:"decideK"`
	// TwoSignal enables the System One plus matrix ALIGNED verdict; disabled, only
	// the judge issues ALIGNED.
	TwoSignal bool `json:"twoSignal"`
	// JudgeAlone lets a judge verdict issue ALIGNED without a SPECIFIC or OVERBROAD
	// matrix; disabled, such a verdict is UNDETERMINED.
	JudgeAlone bool `json:"judgeAlone"`
	// MinIsolation is the isolation floor below which a response loses its pass
	// credit.
	MinIsolation format.Isolation `json:"minIsolation"`
}

// Default returns the policy of ADR 0003 and ADR 0005.
func Default() Policy {
	return Policy{
		TauRoute: DefaultTauRoute, AlignK: DefaultAlignK, ModalMin: DefaultModalMin,
		ThetaOwn: DefaultThetaOwn, ThetaOther: DefaultThetaOther, ThetaNone: DefaultThetaNone,
		ShadowRate: DefaultShadowRate, MaxReasoningCalls: DefaultMaxReasoningCalls, DecideK: DefaultDecideK,
		TwoSignal: true, JudgeAlone: true, MinIsolation: format.IsolationRestricted,
	}
}

// ErrInvalid is returned for a policy outside its domain.
var ErrInvalid = errors.New("invalid policy")

// Validate rejects a fraction outside [0,1], a count below one, a judge budget
// that cannot hold the tickets of one verdict and an unknown isolation level.
func (p Policy) Validate() error {
	fractions := []float64{p.TauRoute, p.ModalMin, p.ThetaOwn, p.ThetaOther, p.ThetaNone, p.ShadowRate}
	switch {
	case slices.ContainsFunc(fractions, func(value float64) bool { return value < 0 || value > 1 }):
		return fmt.Errorf("%w: a threshold is outside [0,1]", ErrInvalid)
	case p.AlignK < 1, p.DecideK < 1, p.MaxReasoningCalls < 1:
		return fmt.Errorf("%w: a count is below 1", ErrInvalid)
	case !slices.Contains(format.Isolations(), p.MinIsolation):
		return fmt.Errorf("%w: unknown isolation level %q", ErrInvalid, p.MinIsolation)
	}
	return nil
}

// Digest is the SHA-256 of the canonical form of the policy: the
// `decision_policy` input, so that any parameter change changes the cache key of
// every proof.
func (p Policy) Digest() (string, error) {
	canonical, err := format.Canonicalize(p)
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}
