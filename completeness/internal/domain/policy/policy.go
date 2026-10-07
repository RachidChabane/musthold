// Package policy holds the decision parameters of completeness (ADR 0003,
// "Routing, thresholds, calibration") and the digest that stands for them as the
// `decision_policy` input of a proof.
package policy

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/format"
)

// Defaults of the parameters.
const (
	DefaultTauAdv       = 0.50
	DefaultDecideK      = 3
	DefaultMaxHunkBytes = 6000
	DefaultMaxHunks     = 50
)

// Policy is the decision policy of a run.
type Policy struct {
	// TauAdv is the confidence at or above which a System One answer produces a
	// question.
	TauAdv format.Threshold `json:"tauAdv"`
	// DecideK is the number of host votes of a closed question when no System One
	// endpoint answers.
	DecideK int `json:"decideK"`
	// MaxHunkBytes bounds the text of a changed function shown to System One.
	MaxHunkBytes int `json:"maxHunkBytes"`
	// MaxHunks bounds the changed units asked about per spec, in file and line
	// order; the rest are reported, not asked.
	MaxHunks int `json:"maxHunks"`
}

// Default returns the policy of ADR 0003.
func Default() Policy {
	return Policy{TauAdv: format.UniformThreshold(DefaultTauAdv), DecideK: DefaultDecideK, MaxHunkBytes: DefaultMaxHunkBytes, MaxHunks: DefaultMaxHunks}
}

// ErrInvalid is returned for a policy outside its domain.
var ErrInvalid = errors.New("invalid policy")

// Validate rejects a threshold outside [0,1] and a count below one.
func (p Policy) Validate() error {
	switch {
	case p.TauAdv.Validate() != nil:
		return fmt.Errorf("%w: tau_adv is outside [0,1] or keys an unknown confidence source", ErrInvalid)
	case p.DecideK < 1, p.MaxHunkBytes < 1, p.MaxHunks < 1:
		return fmt.Errorf("%w: a count is below 1", ErrInvalid)
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
