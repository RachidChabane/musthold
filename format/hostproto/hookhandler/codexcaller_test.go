package hookhandler_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// realCodexToolName is the name Codex gives the fetch tool of the server
// `spec-proof-dp-exec`.
const realCodexToolName = "mcp__spec_proof_dp_exec__fetch_request"

func codexExecutorPayload(t *testing.T, ticket string, identityMembers map[string]any) []byte {
	t.Helper()
	members := map[string]any{
		"hook_event_name": "PreToolUse", "session_id": "019e-session", "tool_name": realCodexToolName, "tool_use_id": "call_1",
		"tool_input": map[string]string{"ticket": ticket, "cap": ""},
	}
	for key, value := range identityMembers {
		members[key] = value
	}
	return []byte(marshal(t, members))
}

func TestCodexCallerIdentification(t *testing.T) {
	codex := harness.Codex(identity)

	t.Run("the main agent's call carries neither agent_id nor agent_type and is denied and journaled [verifies SP-705~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(codex).Handle(hostproto.HookPreTool, codexExecutorPayload(t, f.ticketText(), nil))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
		if events := f.events(); len(events) != 1 || events[0].Kind != hostproto.EventExposureAttempt {
			t.Fatalf("events = %v", events)
		}
	})

	t.Run("a call of the module's executor role is allowed with the capability of its agent [verifies SP-705~1]", func(t *testing.T) {
		f := newFixture(t)
		members := map[string]any{"agent_id": agentID, "agent_type": codex.AgentType(judgeRole)}
		output, err := f.handler(codex).Handle(hostproto.HookPreTool, codexExecutorPayload(t, f.ticketText(), members))
		decided := claudeDecision(t, output)
		if err != nil || decided.Permission != "allow" || decided.Updated["cap"] == nil {
			t.Fatalf("output %q, error %v", output, err)
		}
	})

	t.Run("a call of an executor of another role or of an agent that is not an executor is denied [verifies SP-705~1]", func(t *testing.T) {
		for name, members := range map[string]map[string]any{
			"another role":       {"agent_id": agentID, "agent_type": codex.AgentType("spec-proof-dp:decider")},
			"a foreign agent":    {"agent_id": agentID, "agent_type": "explorer"},
			"an id without type": {"agent_id": otherAgentID},
		} {
			f := newFixture(t)
			output, err := f.handler(codex).Handle(hostproto.HookPreTool, codexExecutorPayload(t, f.ticketText(), members))
			if err != nil || claudeDecision(t, output).Permission != "deny" {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
	})
}
