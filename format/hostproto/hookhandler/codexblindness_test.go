package hookhandler_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/executorrun"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/hookhandler"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// spawnPayload is the PreToolUse payload of a spawn by the main agent, which
// carries neither agent_id nor agent_type.
func spawnPayload(t *testing.T, toolName string, input map[string]any) []byte {
	t.Helper()
	return []byte(marshal(t, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": "01a12286-ded0-79f1-a8d0-73748a01598a", "turn_id": "01a12286-e03d-7153-bacc-3d575d25ac3a",
		"tool_name": toolName, "tool_use_id": "call_dcb31215077b4b80b1c0b8ffff2dcc82", "tool_input": input,
	}))
}

func spawnInput(agentType string, history map[string]any) map[string]any {
	input := map[string]any{"agent_type": agentType, "message": "ticket 864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"}
	for member, value := range history {
		input[member] = value
	}
	return input
}

// stateless serves the hooks with a store that cannot be opened.
func stateless(h harness.Harness) hookhandler.Handler {
	unusable := func() (hookhandler.Store, error) { return nil, errors.New("the state directory cannot be created") }
	return hookhandler.Handler{Harness: h, Open: unusable, Transcripts: memoryTranscripts{}}
}

func TestCodexSpawnGuard(t *testing.T) {
	codex := harness.Codex(identity)
	decider := codex.AgentType(deciderRole)

	t.Run("a spawn of an executor that does not ask for a blind child is denied with the way to spawn it again, without opening state [verifies SP-1937~1]", func(t *testing.T) {
		for name, history := range map[string]map[string]any{
			"no history member":  nil,
			"fork_turns all":     {"fork_turns": "all"},
			"fork_turns a count": {"fork_turns": 2},
			"fork_context true":  {"fork_context": true},
			"a forked context":   {"fork_turns": "none", "fork_context": true},
		} {
			for _, tool := range hostproto.CodexSpawnToolNames() {
				output, err := stateless(codex).Handle(hostproto.HookPreTool, spawnPayload(t, tool, spawnInput(decider, history)))
				if err != nil {
					t.Fatalf("%s via %s: %v", name, tool, err)
				}
				decided := claudeDecision(t, output)
				if decided.Permission != "deny" || !strings.Contains(decided.Reason, `fork_turns set to "none"`) || !strings.Contains(decided.Reason, decider) {
					t.Errorf("%s via %s: %+v", name, tool, decided)
				}
			}
		}
	})

	t.Run("a spawn of an executor in a blind form, a spawn of another agent and an unreadable spawn get no decision [verifies SP-1937~1]", func(t *testing.T) {
		for name, payload := range map[string][]byte{
			"fork_turns none":           spawnPayload(t, "spawn_agent", spawnInput(decider, map[string]any{"fork_turns": "none"})),
			"fork_context false":        spawnPayload(t, "spawn_agent", spawnInput(decider, map[string]any{"fork_context": false})),
			"both blind forms":          spawnPayload(t, "collaborationspawn_agent", spawnInput(decider, map[string]any{"fork_turns": "none", "fork_context": false})),
			"another agent type":        spawnPayload(t, "spawn_agent", spawnInput("explorer", nil)),
			"an executor of no module":  spawnPayload(t, "spawn_agent", spawnInput("musthold-other-decider", nil)),
			"an input that is a string": []byte(`{"hook_event_name":"PreToolUse","tool_name":"spawn_agent","tool_input":"spawn"}`),
		} {
			output, err := stateless(codex).Handle(hostproto.HookPreTool, payload)
			if err != nil || len(output) != 0 {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
	})

	t.Run("a harness that does not guard spawns leaves a tool named like the spawn tool alone [verifies SP-1937~1]", func(t *testing.T) {
		for _, h := range []harness.Harness{harness.Claude(identity), harness.Copilot(identity)} {
			payload := spawnPayload(t, "spawn_agent", spawnInput(h.AgentType(deciderRole), nil))
			if h.Flag == harness.FlagCopilot {
				payload = []byte(marshal(t, map[string]any{"sessionId": "s", "toolName": "spawn_agent", "toolArgs": marshal(t, spawnInput(h.AgentType(deciderRole), nil))}))
			}
			output, err := stateless(h).Handle(hostproto.HookPreTool, payload)
			if err != nil || len(output) != 0 {
				t.Errorf("%s: output %q, error %v", h.Flag, output, err)
			}
		}
	})
}

// codexStop is the SubagentStop payload of a Codex executor of the role.
func codexStop(t *testing.T, agent, agentType, reply string) []byte {
	t.Helper()
	return []byte(marshal(t, map[string]any{
		"hook_event_name": "SubagentStop", "session_id": "01a12286-ded0-79f1-a8d0-73748a01598a", "turn_id": "01a12287-526c-7771-867e-2bc6ce240f62",
		"agent_id": agent, "agent_type": agentType, "transcript_path": "/runs/main.jsonl",
		"agent_transcript_path": transcriptPath, "last_assistant_message": reply,
	}))
}

func recordedRollout(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("..", "testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func TestCodexStopBlindness(t *testing.T) {
	codex := harness.Codex(identity)
	judge := codex.AgentType(judgeRole)
	const recordedTicket = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"

	blindRollout := func(t *testing.T, f *fixture) []byte {
		t.Helper()
		content, err := hostprototest.CodexBlindRollout(agentID, judge, f.ticket.TicketID,
			[]hostproto.ToolCall{call(t, codex, hostproto.ToolFetchRequest, f.ticketText())}, executorrun.Receipt(f.ticket.TicketID))
		if err != nil {
			t.Fatal(err)
		}
		return content
	}
	inheritedRollout := func(t *testing.T, f *fixture) []byte {
		t.Helper()
		content, err := hostprototest.NewCodexRollout(agentID, judge).Developer("Fetch the ticket.").Context().
			User("Run the proofs of the project.").Assistant("I will spawn a judge.").Task(f.ticket.TicketID).Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return content
	}

	t.Run("a blind run with the bare receipt journals nothing [verifies SP-1935~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = blindRollout(t, f)
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		if len(f.events()) != 0 {
			t.Fatalf("events = %v", f.events())
		}
	})

	t.Run("a run that inherited history is journaled against the ticket of its task with the reason [verifies SP-1935~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = inheritedRollout(t, f)
		output, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, executorrun.Receipt(f.ticket.TicketID)))
		if err != nil || len(output) != 0 {
			t.Fatalf("output %q, error %v", output, err)
		}
		event := f.lastEvent(hostproto.EventHistoryInherited)
		if event.Ticket != f.ticketText() || event.AgentID != agentID || event.Role != judgeRole || !strings.Contains(event.Detail, "precedes the task message") {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("an unreadable rollout leaves the blindness unproven against the ticket of the receipt, or against the agent alone [verifies SP-1935~1]", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		event := f.lastEvent(hostproto.EventBlindnessUnproven)
		if event.Ticket != f.ticketText() || !strings.HasPrefix(event.Detail, "transcript unreadable") {
			t.Fatalf("event = %+v", event)
		}
		f = newFixture(t)
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, "I could not do it")); err != nil {
			t.Fatal(err)
		}
		if event := f.lastEvent(hostproto.EventBlindnessUnproven); event.Ticket != "" || event.AgentID != agentID {
			t.Fatalf("event = %+v", event)
		}
		if slices.Contains(f.kinds(), hostproto.EventLeaked) {
			t.Fatalf("a reply that names no ticket was journaled as a leak: %v", f.kinds())
		}
	})

	t.Run("the blind rollout of another thread proves nothing for the stopped agent [verifies SP-1935~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = blindRollout(t, f)
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, otherAgentID, judge, executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		if event := f.lastEvent(hostproto.EventBlindnessUnproven); event.Ticket != f.ticketText() || !strings.Contains(event.Detail, agentID) {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a reply other than the bare receipt is journaled as a leak [verifies SP-1935~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = blindRollout(t, f)
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, "the answer is VALID")); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(f.kinds(), []hostproto.EventKind{hostproto.EventLeaked}) || f.lastEvent(hostproto.EventLeaked).Ticket != f.ticketText() {
			t.Fatalf("events = %v", f.events())
		}
	})

	t.Run("recorded rollouts of a plain and of an encrypted task are blind [verifies SP-1935~1]", func(t *testing.T) {
		for name, test := range map[string]struct{ file, agent string }{
			"plain task":     {"codex-child-rollout.jsonl", "01a12287-50ec-7773-98c2-aeb643ce6d1e"},
			"encrypted task": {"codex-child-rollout-agent-message.jsonl", "01a11d5f-bce6-7f81-9c67-d5cca4d0661f"},
		} {
			f := newFixture(t)
			f.transcripts[transcriptPath] = recordedRollout(t, test.file)
			decider := codex.AgentType(deciderRole)
			if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, codexStop(t, test.agent, decider, "receipt "+recordedTicket)); err != nil {
				t.Fatal(err)
			}
			if len(f.events()) != 0 {
				t.Errorf("%s: events = %v", name, f.events())
			}
		}
	})

	t.Run("a stop of an agent that is not an executor of the module opens no state [verifies SP-1935~1]", func(t *testing.T) {
		if _, err := stateless(codex).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, "explorer", "done")); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("a harness whose transcript no check reads is refused [verifies SP-1935~1]", func(t *testing.T) {
		unknown := codex
		unknown.Transcript = ""
		f := newFixture(t)
		_, err := f.handler(unknown).Handle(hostproto.HookSubagentStop, codexStop(t, agentID, judge, "done"))
		if !errors.Is(err, hookhandler.ErrUnsupportedTranscript) {
			t.Fatalf("error %v", err)
		}
	})
}

// The decoded spawn input of a recorded PreToolUse payload asks for a blind child.
func TestRecordedSpawnPayload(t *testing.T) {
	t.Run("the recorded spawn of a decider with fork_context false gets no decision [verifies SP-1937~1]", func(t *testing.T) {
		const recorded = `{"session_id":"01a12286-ded0-79f1-a8d0-73748a01598a","turn_id":"01a12286-e03d-7153-bacc-3d575d25ac3a","transcript_path":"/Users/u/.codex/sessions/2026/10/09/rollout-2026-10-09T23-17-06-01a12286-ded0-79f1-a8d0-73748a01598a.jsonl","cwd":"/work/project","hook_event_name":"PreToolUse","model":"gpt-6-luna","permission_mode":"bypassPermissions","tool_name":"spawn_agent","tool_input":{"agent_type":"musthold-dp-decider","fork_context":false,"message":"ticket 864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"},"tool_use_id":"call_dcb31215077b4b80b1c0b8ffff2dcc82"}`
		if !json.Valid([]byte(recorded)) {
			t.Fatal("the payload is not JSON")
		}
		output, err := stateless(harness.Codex(identity)).Handle(hostproto.HookPreTool, []byte(recorded))
		if err != nil || len(output) != 0 {
			t.Fatalf("output %q, error %v", output, err)
		}
		denied, err := stateless(harness.Codex(identity)).Handle(hostproto.HookPreTool, []byte(strings.Replace(recorded, `"fork_context":false,`, "", 1)))
		if err != nil || claudeDecision(t, denied).Permission != "deny" {
			t.Fatalf("output %q, error %v", denied, err)
		}
	})
}
