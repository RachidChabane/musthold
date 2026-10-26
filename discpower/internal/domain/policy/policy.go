// Package policy holds the decision parameters of ADR 0003 and ADR 0005 and the
// digest that stands for them as the `decision_policy` input of a proof.
package policy

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/musthold/format"
)

// Defaults of the routing, thresholds and limits (ADR 0003, "Routing,
// thresholds, calibration"; ADR 0005, "Claim power").
const (
	DefaultTauJudge       = 0.70
	DefaultJudgeK         = 2
	DefaultTauScreen      = 0.20
	DefaultDecideK        = 3
	DefaultMinValid       = 3
	DefaultMinSites       = 2
	DefaultMaxRounds      = 3
	DefaultKOp            = 400
	DefaultMaxHostTickets = 40
	// SlotsPerRound is the number of mutant slots one generation request asks
	// for: direct (S1), boundary (S2) and partial (S3).
	SlotsPerRound = 3
)

// SignedPassBasis selects which evidence may carry a pass (ADR 0005, trust model).
type SignedPassBasis string

// Pass bases.
const (
	// ExecutedOnly lets only executed or owner-attributed evidence decide: judged
	// claims never clear a mutant and the screen is bypassed.
	ExecutedOnly SignedPassBasis = "executed-only"
	// Corroborated is the default: a judged claim carries power only with the
	// executed corroboration the claim-power table names.
	Corroborated SignedPassBasis = "corroborated"
	// Judged additionally accepts, with `judge_k` agreeing judges, a mutant
	// judged NOT_VIOLATING without executed corroboration, and counts it.
	Judged SignedPassBasis = "judged"
)

var bases = []SignedPassBasis{ExecutedOnly, Corroborated, Judged}

// Bases lists the pass bases, the most conservative first.
func Bases() []SignedPassBasis { return slices.Clone(bases) }

// Policy is the decision policy of a run.
type Policy struct {
	TauJudge        format.Threshold `json:"tauJudge"`
	JudgeK          int              `json:"judgeK"`
	TauScreen       format.Threshold `json:"tauScreen"`
	DecideK         int              `json:"decideK"`
	MinValid        int              `json:"minValid"`
	MinSites        int              `json:"minSites"`
	MaxRounds       int              `json:"maxRounds"`
	KOp             int              `json:"kOp"`
	MaxHostTickets  int              `json:"maxHostTickets"`
	SignedPassBasis SignedPassBasis  `json:"signedPassBasis"`
	MinIsolation    format.Isolation `json:"minIsolation"`
}

// Default returns the policy of ADR 0003 and ADR 0005.
func Default() Policy {
	return Policy{
		TauJudge:        format.UniformThreshold(DefaultTauJudge),
		JudgeK:          DefaultJudgeK,
		TauScreen:       format.UniformThreshold(DefaultTauScreen),
		DecideK:         DefaultDecideK,
		MinValid:        DefaultMinValid,
		MinSites:        DefaultMinSites,
		MaxRounds:       DefaultMaxRounds,
		KOp:             DefaultKOp,
		MaxHostTickets:  DefaultMaxHostTickets,
		SignedPassBasis: Corroborated,
		MinIsolation:    format.IsolationRestricted,
	}
}

// ErrInvalid is returned for a policy outside its domain.
var ErrInvalid = errors.New("invalid policy")

// Validate rejects a threshold outside [0,1] or keyed by an unknown confidence
// source, a count below one and an unknown basis or isolation level.
func (p Policy) Validate() error {
	switch {
	case p.TauJudge.Validate() != nil, p.TauScreen.Validate() != nil:
		return fmt.Errorf("%w: a threshold is outside [0,1] or keys an unknown confidence source", ErrInvalid)
	case p.JudgeK < 1, p.DecideK < 1, p.MinValid < 1, p.MinSites < 1, p.MaxRounds < 1, p.KOp < 1, p.MaxHostTickets < 1:
		return fmt.Errorf("%w: a count is below 1", ErrInvalid)
	case !slices.Contains(bases, p.SignedPassBasis):
		return fmt.Errorf("%w: unknown pass basis %q", ErrInvalid, p.SignedPassBasis)
	case !slices.Contains(format.Isolations(), p.MinIsolation):
		return fmt.Errorf("%w: unknown isolation level %q", ErrInvalid, p.MinIsolation)
	}
	return nil
}

// Digest is the SHA-256 of the canonical form of the policy: the `decision_policy`
// input, so that any parameter change changes the cache key of every proof.
func (p Policy) Digest() (string, error) {
	canonical, err := format.Canonicalize(p)
	if err != nil {
		return "", err
	}
	return format.SHA256Hex(canonical), nil
}

// BypassesScreen reports whether every differing operator survivor enters the
// pool, which `executed-only` requires because a screen-out is a judged exclusion.
func (p Policy) BypassesScreen() bool { return p.SignedPassBasis == ExecutedOnly }
