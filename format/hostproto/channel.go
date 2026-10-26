package hostproto

import "github.com/RachidChabane/musthold/format"

// ToolAllowlist is what was observed about the harness's per-agent tool allowlist.
type ToolAllowlist string

// Observed allowlist states.
const (
	// AllowlistEnforced: the harness restricts the executor to the named tools.
	AllowlistEnforced ToolAllowlist = "enforced"
	// AllowlistUnverified: the harness documents an allowlist nobody verified.
	AllowlistUnverified ToolAllowlist = "unverified"
	// AllowlistAbsent: the harness has no per-agent allowlist of built-in tools.
	AllowlistAbsent ToolAllowlist = "absent"
)

// Availability tells whether the selected channel can serve requests now.
type Availability string

// Availability values.
const (
	AvailabilityReady   Availability = "ready"
	AvailabilityPending Availability = "pending"
)

// Reasons a selection is pending.
const ReasonHooksMissing = "hooks_missing"

// Observed is the state of the host as seen by `system/init`, `doctor` and the
// `initialize` handshake, never the state its files declare.
type Observed struct {
	PluginPresent    bool
	ServerPresent    bool
	HooksPresent     bool
	SamplingDeclared bool
	Allowlist        ToolAllowlist
}

// Policy is the user's configuration of the host.
type Policy struct {
	// SamplingEnabled is `host.sampling: on`.
	SamplingEnabled bool
	// RequireAttestation is `host.require_attestation`.
	RequireAttestation bool
}

// Selection is the channel requests travel through and the isolation it reaches.
type Selection struct {
	Channel      format.Channel
	Isolation    format.Isolation
	Availability Availability
	Reason       string
}

// SelectChannel picks the channel from what was observed: the plugin subagent when
// its plugin and server are present, then sampling when the client declared it and
// the policy allows it, then the main agent. The isolation follows from what the
// harness enforces. When attestation is required and the hooks that issue
// capabilities are missing, the subagent channel is pending, never degraded.
func SelectChannel(observed Observed, policy Policy) Selection {
	switch {
	case observed.PluginPresent && observed.ServerPresent:
		return selectSubagent(observed, policy)
	case observed.SamplingDeclared && policy.SamplingEnabled:
		return Selection{Channel: format.ChannelHostSampled, Isolation: format.IsolationRestricted, Availability: AvailabilityReady}
	default:
		return Selection{Channel: format.ChannelHostInline, Isolation: format.IsolationInline, Availability: AvailabilityReady}
	}
}

func selectSubagent(observed Observed, policy Policy) Selection {
	selection := Selection{Channel: format.ChannelHostSubagent, Availability: AvailabilityReady}
	if policy.RequireAttestation && !observed.HooksPresent {
		selection.Availability = AvailabilityPending
		selection.Reason = ReasonHooksMissing
	}
	switch observed.Allowlist {
	case AllowlistEnforced:
		selection.Isolation = format.IsolationRestricted
		if observed.HooksPresent {
			selection.Isolation = format.IsolationRestrictedAttested
		}
	case AllowlistUnverified:
		selection.Isolation = format.IsolationRestrictedUnverified
	default:
		selection.Isolation = format.IsolationUnrestricted
	}
	return selection
}
