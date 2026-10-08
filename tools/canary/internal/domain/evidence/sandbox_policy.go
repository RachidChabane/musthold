package evidence

import "encoding/json"

// SandboxPolicyReadOnly is the policy type a read-only sandbox records.
const SandboxPolicyReadOnly = "read-only"

const rolloutTurnContext = "turn_context"

// SandboxPolicy is the sandbox a spawned thread ran under, as its rollout records
// it in the `turn_context` of its first turn.
type SandboxPolicy struct {
	// Type is the policy type: "read-only", "workspace-write", and so on. It is
	// empty when the rollout records none.
	Type string
	// Line is the 1-based line of the turn context in the rollout.
	Line int
}

// Recorded says whether the rollout names a sandbox policy.
func (p SandboxPolicy) Recorded() bool { return p.Type != "" }

// ReadOnly says whether the policy is the read-only sandbox.
func (p SandboxPolicy) ReadOnly() bool { return p.Type == SandboxPolicyReadOnly }

type rawTurnContext struct {
	SandboxPolicy struct {
		Type string `json:"type"`
	} `json:"sandbox_policy"`
}

// decodeSandboxPolicy reads the sandbox policy of the first turn context of a
// rollout that names one.
func decodeSandboxPolicy(lines []Line) SandboxPolicy {
	for _, line := range lines {
		var item rawRolloutLine
		if json.Unmarshal(line.Raw, &item) != nil || item.Type != rolloutTurnContext {
			continue
		}
		var context rawTurnContext
		if json.Unmarshal(item.Payload, &context) != nil || context.SandboxPolicy.Type == "" {
			continue
		}
		return SandboxPolicy{Type: context.SandboxPolicy.Type, Line: line.Number}
	}
	return SandboxPolicy{}
}
