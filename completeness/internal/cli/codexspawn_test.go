package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/cli"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// codexHost is a project served on Codex, whose executors keep the shell.
func codexHost(t *testing.T) *commandHost {
	t.Helper()
	observed := hostprototest.ClaudeCodeObserved()
	observed.Allowlist = hostproto.AllowlistAbsent
	return newCommandHost(t, observed, hostproto.Policy{})
}

// spawn is the PreToolUse payload of the main agent spawning an agent.
func spawn(toolName, agentType string, history map[string]any) map[string]any {
	input := map[string]any{"agent_type": agentType, "message": "ticket 864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"}
	for member, value := range history {
		input[member] = value
	}
	return map[string]any{"hook_event_name": "PreToolUse", "session_id": "01a12286-ded0-79f1-a8d0-73748a01598a", "tool_name": toolName, "tool_input": input}
}

func TestCodexSpawnGuard(t *testing.T) {
	t.Run("a spawn of an executor that would inherit the main agent's history is denied with the way to spawn it blind, and nothing is journaled [verifies SP-1940~1]", func(t *testing.T) {
		h := codexHost(t)
		decider := h.harness.AgentTypeOf(hostproto.AgentDecider)
		for name, history := range map[string]map[string]any{"no history member": nil, "all of the turns": {"fork_turns": "all"}, "the context": {"fork_context": true}} {
			for _, tool := range hostproto.CodexSpawnToolNames() {
				stdout, code := h.runHook(hostproto.HookPreTool, spawn(tool, decider, history))
				var decision struct {
					Output struct {
						Permission string `json:"permissionDecision"`
						Reason     string `json:"permissionDecisionReason"`
					} `json:"hookSpecificOutput"`
				}
				if code != cli.ExitClean || json.Unmarshal([]byte(stdout), &decision) != nil {
					t.Fatalf("%s via %s: exit %d, stdout %q", name, tool, code, stdout)
				}
				if decision.Output.Permission != "deny" || !strings.Contains(decision.Output.Reason, `fork_turns set to "none"`) {
					t.Errorf("%s via %s: %+v", name, tool, decision.Output)
				}
			}
		}
		if events := h.Journal(); len(events) != 0 {
			t.Fatalf("events = %v", events)
		}
	})

	t.Run("a blind spawn of an executor and a spawn of another agent get no decision [verifies SP-1940~1]", func(t *testing.T) {
		h := codexHost(t)
		decider := h.harness.AgentTypeOf(hostproto.AgentDecider)
		for name, payload := range map[string]map[string]any{
			"fork_turns none":    spawn("spawn_agent", decider, map[string]any{"fork_turns": "none"}),
			"fork_context false": spawn("multi_agent_v1spawn_agent", decider, map[string]any{"fork_context": false}),
			"another agent":      spawn("spawn_agent", "explorer", nil),
		} {
			if stdout, code := h.runHook(hostproto.HookPreTool, payload); code != cli.ExitClean || stdout != "" {
				t.Errorf("%s: exit %d, stdout %q", name, code, stdout)
			}
		}
	})
}
