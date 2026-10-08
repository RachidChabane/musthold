package hostharness_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestHarnessesOfTheModule(t *testing.T) {
	t.Run("each harness names the executor tools of the module's plugin as it spells them [verifies SP-742~1]", func(t *testing.T) {
		want := map[string][]string{
			hostharness.FlagClaude:  {"mcp__plugin_spec-proof-alignment_exec__fetch_request", "mcp__plugin_spec-proof-alignment_exec__submit_response"},
			hostharness.FlagCodex:   {"mcp__spec_proof_alignment_exec__fetch_request", "mcp__spec_proof_alignment_exec__submit_response"},
			hostharness.FlagCopilot: {"spec-proof-alignment-exec/fetch_request", "spec-proof-alignment-exec/submit_response"},
		}
		for _, harness := range hostharness.All() {
			if got := harness.ToolNames(); !slices.Equal(got, want[harness.Flag]) {
				t.Errorf("%s: tools = %v, want %v", harness.Flag, got, want[harness.Flag])
			}
		}
	})

	t.Run("the module's two executor agents map to the roles of its requests on every harness [verifies SP-742~1]", func(t *testing.T) {
		if want := []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}; !slices.Equal(hostharness.Identity.Agents, want) {
			t.Fatalf("agents = %v, want %v", hostharness.Identity.Agents, want)
		}
		for _, harness := range hostharness.All() {
			for _, agent := range hostharness.Identity.Agents {
				role := hostproto.RoleFor(hostharness.PluginName, agent)
				if back, ok := harness.RoleOf(harness.AgentType(role)); !ok || back != role {
					t.Errorf("%s: %s maps to %q, %v", harness.Flag, role, back, ok)
				}
			}
		}
	})

	t.Run("an agent the module does not ship is not one of its executors on any harness [verifies SP-742~1]", func(t *testing.T) {
		generator := hostproto.RoleFor(hostharness.PluginName, hostproto.AgentGenerator)
		for _, harness := range hostharness.All() {
			if back, ok := harness.RoleOf(harness.AgentType(generator)); ok {
				t.Errorf("%s: the generator maps to %q", harness.Flag, back)
			}
		}
	})

	t.Run("the isolation and the attestation requirement follow what each harness enforces [verifies SP-742~1]", func(t *testing.T) {
		want := map[string]format.Isolation{
			hostharness.FlagClaude:  format.IsolationRestricted,
			hostharness.FlagCodex:   format.IsolationUnrestricted,
			hostharness.FlagCopilot: format.IsolationRestrictedUnverified,
		}
		for _, harness := range hostharness.All() {
			if got := harness.Isolation(); got != want[harness.Flag] {
				t.Errorf("%s: isolation = %s, want %s", harness.Flag, got, want[harness.Flag])
			}
			if got := harness.AttestationRequired(true); got != (harness.Flag == hostharness.FlagClaude) {
				t.Errorf("%s: attestation required = %v", harness.Flag, got)
			}
		}
	})

	t.Run("a harness is found by its flag and an unknown one is refused [verifies SP-742~1]", func(t *testing.T) {
		for _, flag := range hostharness.Flags() {
			if harness, err := hostharness.ByFlag(flag); err != nil || harness.Flag != flag {
				t.Errorf("%s: %+v, %v", flag, harness, err)
			}
		}
		if _, err := hostharness.ByFlag("vim"); err == nil {
			t.Error("an unknown harness was found")
		}
	})
}
