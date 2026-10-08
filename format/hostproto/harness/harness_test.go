package harness_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

var sample = harness.Identity{
	Plugin: "spec-proof-sample",
	Agents: []hostproto.Agent{hostproto.AgentGenerator, hostproto.AgentJudge, hostproto.AgentDecider},
}

func TestHarnessStrategies(t *testing.T) {
	t.Run("each harness names the executor tools as it spells them [verifies SP-700~1]", func(t *testing.T) {
		want := map[string][]string{
			harness.FlagClaude:  {"mcp__plugin_spec-proof-sample_exec__fetch_request", "mcp__plugin_spec-proof-sample_exec__submit_response"},
			harness.FlagCodex:   {"mcp__spec_proof_sample_exec__fetch_request", "mcp__spec_proof_sample_exec__submit_response"},
			harness.FlagCopilot: {"spec-proof-sample-exec/fetch_request", "spec-proof-sample-exec/submit_response"},
		}
		for _, h := range harness.All(sample) {
			if got := h.ToolNames(); !slices.Equal(got, want[h.Flag]) {
				t.Errorf("%s: tools = %v, want %v", h.Flag, got, want[h.Flag])
			}
		}
	})

	t.Run("a hook payload's tool name is recognized as an executor tool of the harness only [verifies SP-700~1]", func(t *testing.T) {
		claude := harness.Claude(sample)
		for name, tool := range map[string]string{
			"mcp__plugin_spec-proof-sample_exec__fetch_request":   hostproto.ToolFetchRequest,
			"mcp__plugin_spec-proof-sample_exec__submit_response": hostproto.ToolSubmitResponse,
			"mcp__plugin_spec-proof-sample_exec__drain":           hostproto.ToolDrain,
		} {
			if got, ok := claude.ExecutorTool(name); !ok || got != tool {
				t.Errorf("%s: got %q, %v", name, got, ok)
			}
		}
		for _, name := range []string{
			"fetch_request", "Bash", "mcp__plugin_other_exec__fetch_request", "mcp__plugin_spec-proof-sample_other__fetch_request",
			"mcp__plugin_spec-proof-sample_exec__other", "mcp__spec-proof-sample-exec__fetch_request",
		} {
			if got, ok := claude.ExecutorTool(name); ok {
				t.Errorf("%s was recognized as %q", name, got)
			}
		}
		copilot := harness.Copilot(sample)
		for _, name := range []string{"spec-proof-sample-exec/fetch_request", "mcp__spec-proof-sample-exec__fetch_request"} {
			if _, ok := copilot.ExecutorTool(name); !ok {
				t.Errorf("copilot did not recognize %s", name)
			}
		}
	})

	t.Run("Codex recognizes the sanitized, the unsanitized and the slashed spelling of its tools [verifies SP-722~1]", func(t *testing.T) {
		codex := harness.Codex(sample)
		for _, name := range []string{
			"mcp__spec_proof_sample_exec__fetch_request", "mcp__spec-proof-sample-exec__fetch_request", "spec-proof-sample-exec/fetch_request",
		} {
			if got, ok := codex.ExecutorTool(name); !ok || got != hostproto.ToolFetchRequest {
				t.Errorf("%s: got %q, %v", name, got, ok)
			}
		}
		for _, name := range []string{"mcp__spec_proof_other_exec__fetch_request", "mcp__spec_proof_sample_exec__other", "spec-proof-sample-exec-fetch_request"} {
			if got, ok := codex.ExecutorTool(name); ok {
				t.Errorf("%s was recognized as %q", name, got)
			}
		}
		if got := codex.ToolMatcher(); got != "mcp__spec_proof_sample_exec__.*" {
			t.Errorf("matcher = %q", got)
		}
	})

	t.Run("Copilot recognizes the hyphen-joined spelling of its tools besides the slashed one [verifies SP-722~1]", func(t *testing.T) {
		copilot := harness.Copilot(sample)
		if got := copilot.ToolName(hostproto.ToolFetchRequest); got != "spec-proof-sample-exec/fetch_request" {
			t.Errorf("the allowlist spelling = %q", got)
		}
		for _, name := range []string{"spec-proof-sample-exec-fetch_request", "spec-proof-sample-exec-submit_response"} {
			if _, ok := copilot.ExecutorTool(name); !ok {
				t.Errorf("%s was not recognized", name)
			}
		}
		if _, ok := copilot.ExecutorTool("spec-proof-other-exec-fetch_request"); ok {
			t.Error("a tool of another server was recognized")
		}
	})

	t.Run("the isolation of a harness follows what its allowlist enforces [verifies SP-700~1]", func(t *testing.T) {
		want := map[string]format.Isolation{
			harness.FlagClaude:  format.IsolationRestricted,
			harness.FlagCodex:   format.IsolationUnrestricted,
			harness.FlagCopilot: format.IsolationRestrictedUnverified,
		}
		for _, h := range harness.All(sample) {
			if got := h.Isolation(); got != want[h.Flag] {
				t.Errorf("%s: isolation = %s, want %s", h.Flag, got, want[h.Flag])
			}
		}
	})

	t.Run("attestation is waived where the harness cannot issue a capability [verifies SP-700~1]", func(t *testing.T) {
		for _, h := range harness.All(sample) {
			wantRequired := h.Flag == harness.FlagClaude
			if got := h.AttestationRequired(true); got != wantRequired {
				t.Errorf("%s: required = %v, want %v", h.Flag, got, wantRequired)
			}
			if h.AttestationRequired(false) {
				t.Errorf("%s: required although the project waived it", h.Flag)
			}
		}
	})

	t.Run("a harness is found by its flag and an unknown one is refused [verifies SP-700~1]", func(t *testing.T) {
		for _, flag := range harness.Flags() {
			if h, err := harness.ByFlag(sample, flag); err != nil || h.Flag != flag {
				t.Errorf("%s: %+v, %v", flag, h, err)
			}
		}
		if _, err := harness.ByFlag(sample, "vim"); !errors.Is(err, harness.ErrUnknownHarness) {
			t.Errorf("error = %v", err)
		}
	})

	t.Run("an agent type is an executor of the plugin only in the form its harness reports, and maps back to its role [verifies SP-700~1]", func(t *testing.T) {
		forms := map[string]string{
			harness.FlagClaude: "spec-proof-sample:", harness.FlagCodex: "spec-proof-sample-", harness.FlagCopilot: "spec-proof-sample-",
		}
		for _, h := range harness.All(sample) {
			prefix := forms[h.Flag]
			for _, agent := range []string{"generator", "judge", "decider"} {
				role := "spec-proof-sample:" + agent
				if got := h.AgentType(role); got != prefix+agent {
					t.Errorf("%s: type of %s = %q", h.Flag, role, got)
				}
				if back, ok := h.RoleOf(prefix + agent); !ok || back != role {
					t.Errorf("%s: %s%s maps to %q, %v", h.Flag, prefix, agent, back, ok)
				}
			}
			foreign := "spec-proof-sample-judge"
			if h.Flag != harness.FlagClaude {
				foreign = "spec-proof-sample:judge"
			}
			for _, wrong := range []string{"", "judge", prefix, prefix + "boss", "x" + prefix + "judge", prefix + "judge:x", prefix + "judgey", foreign} {
				if role, ok := h.RoleOf(wrong); ok {
					t.Errorf("%s: %q was taken for %q", h.Flag, wrong, role)
				}
			}
		}
		if harness.Claude(sample).AgentType("judge") != "" {
			t.Error("a role without a plugin has an agent type")
		}
	})

	t.Run("only the agents a module ships are its executors [verifies SP-700~1]", func(t *testing.T) {
		judgesOnly := harness.Identity{Plugin: "spec-proof-sample", Agents: []hostproto.Agent{hostproto.AgentJudge}}
		for _, h := range harness.All(judgesOnly) {
			if _, ok := h.RoleOf(h.AgentTypeOf(hostproto.AgentJudge)); !ok {
				t.Errorf("%s: the judge of the module is not its executor", h.Flag)
			}
			if role, ok := h.RoleOf(h.AgentTypeOf(hostproto.AgentGenerator)); ok {
				t.Errorf("%s: the generator the module does not ship is taken for %q", h.Flag, role)
			}
		}
		if got := judgesOnly.Roles(); !slices.Equal(got, []string{"spec-proof-sample:judge"}) {
			t.Errorf("roles = %v", got)
		}
	})
}
