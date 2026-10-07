package hostharness_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestHarnessStrategies(t *testing.T) {
	t.Run("each harness names the executor tools as it spells them [verifies SP-228~1]", func(t *testing.T) {
		want := map[string][]string{
			hostharness.FlagClaude:  {"mcp__plugin_spec-proof-dp_exec__fetch_request", "mcp__plugin_spec-proof-dp_exec__submit_response"},
			hostharness.FlagCodex:   {"mcp__spec-proof-dp-exec__fetch_request", "mcp__spec-proof-dp-exec__submit_response"},
			hostharness.FlagCopilot: {"spec-proof-dp-exec/fetch_request", "spec-proof-dp-exec/submit_response"},
		}
		for _, harness := range hostharness.All() {
			if got := harness.ToolNames(); !slices.Equal(got, want[harness.Flag]) {
				t.Errorf("%s: tools = %v, want %v", harness.Flag, got, want[harness.Flag])
			}
		}
	})

	t.Run("a hook payload's tool name is recognized as an executor tool of the harness only [verifies SP-228~1]", func(t *testing.T) {
		claude := hostharness.Claude()
		for name, tool := range map[string]string{
			"mcp__plugin_spec-proof-dp_exec__fetch_request":   hostproto.ToolFetchRequest,
			"mcp__plugin_spec-proof-dp_exec__submit_response": hostproto.ToolSubmitResponse,
			"mcp__plugin_spec-proof-dp_exec__drain":           hostproto.ToolDrain,
		} {
			if got, ok := claude.ExecutorTool(name); !ok || got != tool {
				t.Errorf("%s: got %q, %v", name, got, ok)
			}
		}
		for _, name := range []string{
			"fetch_request", "Bash", "mcp__plugin_other_exec__fetch_request", "mcp__plugin_spec-proof-dp_other__fetch_request",
			"mcp__plugin_spec-proof-dp_exec__other", "mcp__spec-proof-dp-exec__fetch_request",
		} {
			if got, ok := claude.ExecutorTool(name); ok {
				t.Errorf("%s was recognized as %q", name, got)
			}
		}
		copilot := hostharness.Copilot()
		for _, name := range []string{"spec-proof-dp-exec/fetch_request", "mcp__spec-proof-dp-exec__fetch_request"} {
			if _, ok := copilot.ExecutorTool(name); !ok {
				t.Errorf("copilot did not recognize %s", name)
			}
		}
	})

	t.Run("the isolation of a harness follows what its allowlist enforces [verifies SP-228~1]", func(t *testing.T) {
		want := map[string]format.Isolation{
			hostharness.FlagClaude:  format.IsolationRestricted,
			hostharness.FlagCodex:   format.IsolationUnrestricted,
			hostharness.FlagCopilot: format.IsolationRestrictedUnverified,
		}
		for _, harness := range hostharness.All() {
			if got := harness.Isolation(); got != want[harness.Flag] {
				t.Errorf("%s: isolation = %s, want %s", harness.Flag, got, want[harness.Flag])
			}
		}
	})

	t.Run("attestation is waived where the harness cannot issue a capability [verifies SP-228~1]", func(t *testing.T) {
		optOut := false
		required, waived := projectconfig.HostSettings{}, projectconfig.HostSettings{RequireAttestation: &optOut}
		for _, harness := range hostharness.All() {
			wantRequired := harness.Flag == hostharness.FlagClaude
			if got := harness.AttestationRequired(required); got != wantRequired {
				t.Errorf("%s: required = %v, want %v", harness.Flag, got, wantRequired)
			}
			if harness.AttestationRequired(waived) {
				t.Errorf("%s: required although the project waived it", harness.Flag)
			}
		}
	})

	t.Run("a harness is found by its flag and an unknown one is refused [verifies SP-228~1]", func(t *testing.T) {
		for _, flag := range hostharness.Flags() {
			if harness, err := hostharness.ByFlag(flag); err != nil || harness.Flag != flag {
				t.Errorf("%s: %+v, %v", flag, harness, err)
			}
		}
		if _, err := hostharness.ByFlag("vim"); !errors.Is(err, hostharness.ErrUnknownHarness) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("an agent type is an executor of the plugin only in the form its harness reports, and maps back to its role [verifies SP-228~1]", func(t *testing.T) {
		forms := map[string]string{
			hostharness.FlagClaude: "spec-proof-dp:", hostharness.FlagCodex: "spec-proof-dp-", hostharness.FlagCopilot: "spec-proof-dp-",
		}
		for _, harness := range hostharness.All() {
			prefix := forms[harness.Flag]
			for _, agent := range []string{"generator", "judge", "decider"} {
				role := "spec-proof-dp:" + agent
				if got := harness.AgentType(role); got != prefix+agent {
					t.Errorf("%s: type of %s = %q", harness.Flag, role, got)
				}
				if back, ok := harness.RoleOf(prefix + agent); !ok || back != role {
					t.Errorf("%s: %s%s maps to %q, %v", harness.Flag, prefix, agent, back, ok)
				}
			}
			foreign := "spec-proof-dp-judge"
			if harness.Flag != hostharness.FlagClaude {
				foreign = "spec-proof-dp:judge"
			}
			for _, wrong := range []string{"", "judge", prefix, prefix + "boss", "x" + prefix + "judge", prefix + "judge:x", prefix + "judgey", foreign} {
				if role, ok := harness.RoleOf(wrong); ok {
					t.Errorf("%s: %q was taken for %q", harness.Flag, wrong, role)
				}
			}
		}
		if hostharness.Claude().AgentType("judge") != "" {
			t.Error("a role without a plugin has an agent type")
		}
	})
}
