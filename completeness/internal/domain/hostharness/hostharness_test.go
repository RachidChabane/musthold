package hostharness_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

func TestHarnessesOfTheModule(t *testing.T) {
	t.Run("each harness names the executor tools of the module's plugin as it spells them [verifies SP-782~1]", func(t *testing.T) {
		want := map[string][]string{
			hostharness.FlagClaude:  {"mcp__plugin_musthold-completeness_exec__fetch_request", "mcp__plugin_musthold-completeness_exec__submit_response"},
			hostharness.FlagCodex:   {"mcp__musthold_completeness_exec__fetch_request", "mcp__musthold_completeness_exec__submit_response"},
			hostharness.FlagCopilot: {"musthold-completeness-exec/fetch_request", "musthold-completeness-exec/submit_response"},
		}
		for _, harness := range hostharness.All() {
			if got := harness.ToolNames(); !slices.Equal(got, want[harness.Flag]) {
				t.Errorf("%s: tools = %v, want %v", harness.Flag, got, want[harness.Flag])
			}
		}
	})

	t.Run("the module's only executor agent, the decider, maps to the role of its requests on every harness [verifies SP-782~1]", func(t *testing.T) {
		if want := []hostproto.Agent{hostproto.AgentDecider}; !slices.Equal(hostharness.Identity.Agents, want) {
			t.Fatalf("agents = %v, want %v", hostharness.Identity.Agents, want)
		}
		for _, harness := range hostharness.All() {
			role := hostproto.RoleFor(hostharness.PluginName, hostproto.AgentDecider)
			if back, ok := harness.RoleOf(harness.AgentType(role)); !ok || back != role {
				t.Errorf("%s: %s maps to %q, %v", harness.Flag, role, back, ok)
			}
		}
	})

	t.Run("the judge and the generator are not executors of the module on any harness [verifies SP-782~1]", func(t *testing.T) {
		for _, agent := range []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentGenerator} {
			role := hostproto.RoleFor(hostharness.PluginName, agent)
			for _, harness := range hostharness.All() {
				if back, ok := harness.RoleOf(harness.AgentType(role)); ok {
					t.Errorf("%s: the %s maps to %q", harness.Flag, agent, back)
				}
			}
		}
	})

	t.Run("the isolation and the attestation requirement follow what each harness enforces [verifies SP-782~1]", func(t *testing.T) {
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

	t.Run("a harness is found by its flag and an unknown one is refused [verifies SP-782~1]", func(t *testing.T) {
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
