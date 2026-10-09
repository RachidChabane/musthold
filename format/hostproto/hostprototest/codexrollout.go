package hostprototest

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// Thread and harness details the rollouts written here carry.
const (
	// CodexParentThread is the main thread that spawns the executors.
	CodexParentThread = "01a12286-ded0-79f1-a8d0-73748a01598a"
	// CodexCLIVersion is the version the rollouts name.
	CodexCLIVersion = harness.CodexVersion

	codexContextText = "<recommended_plugins>\nHere is a list of plugins that are available but not installed.\n</recommended_plugins>"
	codexEnvironment = "<environment_context>\n  <cwd>/work/project</cwd>\n  <shell>zsh</shell>\n</environment_context>"
	codexSkillsText  = "<skills_instructions>\n## Skills\nA skill is a set of local instructions to follow that is stored in a `SKILL.md` file.\n</skills_instructions>"
	codexEncrypted   = "gAAAAABqyAgfliC50kt4LzZPKkOuZdjiOh8fy9z5YvFwVvc1lXE0CmjqINRMvtea6BwcMfp2ELB4zgWGDXNxknjJgR09bdPPLoZs3tAW47Vn"
	codexTaskHeader  = "Message Type: NEW_TASK\nTask name: /root/ticket\nSender: /root\nPayload:\n"
)

// CodexRollout writes the rollout of a Codex thread line by line, in the layout
// the harness gives it: a `session_meta` line, then `response_item` lines among
// the bookkeeping lines of the harness.
type CodexRollout struct {
	lines [][]byte
	err   error
}

// NewCodexRollout starts the rollout of a thread spawned by CodexParentThread to
// run as role.
func NewCodexRollout(threadID, role string) *CodexRollout {
	rollout := &CodexRollout{}
	return rollout.line("session_meta", map[string]any{
		"id": threadID, "cli_version": CodexCLIVersion, "originator": "codex_exec",
		"source": map[string]any{"subagent": map[string]any{"thread_spawn": map[string]any{
			"parent_thread_id": CodexParentThread, "depth": 1, "agent_path": nil, "agent_role": role,
		}}},
	})
}

// NewCodexMainRollout starts the rollout of a main thread, which no thread spawned.
func NewCodexMainRollout(threadID string) *CodexRollout {
	rollout := &CodexRollout{}
	return rollout.line("session_meta", map[string]any{"id": threadID, "cli_version": CodexCLIVersion, "originator": "codex_exec", "source": "exec"})
}

func (r *CodexRollout) line(kind string, payload any) *CodexRollout {
	encoded, err := json.Marshal(map[string]any{"timestamp": "2026-10-09T21:17:45.535Z", "type": kind, "payload": payload})
	if err != nil && r.err == nil {
		r.err = err
	}
	r.lines = append(r.lines, encoded)
	return r
}

func (r *CodexRollout) item(payload map[string]any) *CodexRollout {
	return r.line("response_item", payload)
}

func (r *CodexRollout) message(role string, texts ...string) *CodexRollout {
	blockType := "input_text"
	if role == "assistant" {
		blockType = "output_text"
	}
	content := make([]map[string]string, len(texts))
	for index, text := range texts {
		content[index] = map[string]string{"type": blockType, "text": text}
	}
	return r.item(map[string]any{"type": "message", "role": role, "content": content})
}

// Developer adds a developer message.
func (r *CodexRollout) Developer(text string) *CodexRollout {
	return r.message("developer", text, codexSkillsText)
}

// Context adds the user message of harness context every thread receives.
func (r *CodexRollout) Context() *CodexRollout {
	return r.message("user", codexContextText, codexEnvironment)
}

// User adds a user message.
func (r *CodexRollout) User(text string) *CodexRollout { return r.message("user", text) }

// Assistant adds an assistant message.
func (r *CodexRollout) Assistant(text string) *CodexRollout { return r.message("assistant", text) }

// Task adds the task message of an executor: `ticket <ticket>`.
func (r *CodexRollout) Task(ticket hostproto.TicketID) *CodexRollout {
	return r.User(hostproto.ExecutorPrompt(ticket))
}

// AgentMessage adds a task delivered as an agent message, whose payload is
// encrypted.
func (r *CodexRollout) AgentMessage() *CodexRollout {
	return r.item(map[string]any{
		"type": "agent_message", "author": "/root", "recipient": "/root/ticket",
		"content": []map[string]string{{"type": "input_text", "text": codexTaskHeader}, {"type": "encrypted_content", "encrypted_content": codexEncrypted}},
	})
}

// Call adds a function call, under the namespace of its MCP server when it is a
// tool of one, and its output.
func (r *CodexRollout) Call(call hostproto.ToolCall) *CodexRollout {
	name, namespace := call.Name, ""
	if tool, ok := hostproto.ParseMCPToolName(call.Name); ok {
		name, namespace = tool.Tool, "mcp__"+tool.Server
	}
	callID := fmt.Sprintf("call_%d", len(r.lines))
	payload := map[string]any{"type": "function_call", "name": name, "arguments": string(call.Input), "call_id": callID}
	if namespace != "" {
		payload["namespace"] = namespace
	}
	r.item(payload)
	return r.item(map[string]any{"type": "function_call_output", "call_id": callID, "output": "Wall time: 0.0317 seconds\nOutput:"})
}

// Item adds a response item of a type and nothing else.
func (r *CodexRollout) Item(itemType string) *CodexRollout {
	return r.item(map[string]any{"type": itemType})
}

// Bookkeeping adds the lines the harness writes around the items.
func (r *CodexRollout) Bookkeeping() *CodexRollout {
	r.line("event_msg", map[string]any{"type": "task_started", "model_context_window": 258400})
	return r.line("turn_context", map[string]any{"approval_policy": "never", "sandbox_policy": map[string]any{"type": "workspace-write"}})
}

// Bytes returns the rollout, one line per entry, each ended by a newline.
func (r *CodexRollout) Bytes() ([]byte, error) {
	if r.err != nil {
		return nil, r.err
	}
	return append(bytes.Join(r.lines, []byte("\n")), '\n'), nil
}

// CodexBlindRollout is the rollout of an executor thread that started blind: the
// role's instructions, the harness context and the task message, then its tool
// calls and its reply.
func CodexBlindRollout(threadID, role string, ticket hostproto.TicketID, calls []hostproto.ToolCall, reply string) ([]byte, error) {
	return codexExecutorRollout(NewCodexRollout(threadID, role).Bookkeeping().Developer(codexRoleText).Context(), ticket, calls, reply)
}

// CodexInheritedRollout is the rollout of an executor thread that started with
// the history of the main agent: its prompt and its turn come before the task
// message.
func CodexInheritedRollout(threadID, role string, ticket hostproto.TicketID, calls []hostproto.ToolCall, reply string) ([]byte, error) {
	start := NewCodexRollout(threadID, role).Bookkeeping().Developer(codexRoleText).Context().User(codexMainPrompt).Assistant(codexMainTurn)
	return codexExecutorRollout(start, ticket, calls, reply)
}

func codexExecutorRollout(start *CodexRollout, ticket hostproto.TicketID, calls []hostproto.ToolCall, reply string) ([]byte, error) {
	rollout := start.Task(ticket)
	for _, call := range calls {
		rollout.Call(call)
	}
	return rollout.Assistant(reply).Bytes()
}

// Texts of the rollouts of executor threads.
const (
	// codexRoleText is the developer message of an executor role.
	codexRoleText = "Call `fetch_request` with the ticket from your task and an empty `cap`. Follow the instructions it returns."
	// codexMainPrompt and codexMainTurn are the prompt and the first turn of the main agent.
	codexMainPrompt = "Run the proofs of the project and fix what they report."
	codexMainTurn   = "The proofs wait for a decider: I will spawn one per ticket."
)

// RunTranscript is the transcript a harness leaves for a stopped executor run of
// a role, in the layout of that harness, with the tool calls spelled as given. A
// Codex rollout shows the history the run started with.
func RunTranscript(h harness.Harness, role string, run Run, calls []hostproto.ToolCall) ([]byte, error) {
	if h.Transcript != hostproto.TranscriptCodexRollout {
		return AgentTranscript(calls)
	}
	if run.InheritedHistory {
		return CodexInheritedRollout(run.AgentID, h.AgentType(role), run.Ticket, calls, run.Reply)
	}
	return CodexBlindRollout(run.AgentID, h.AgentType(role), run.Ticket, calls, run.Reply)
}

// AgentTranscript is a transcript of one JSON line per assistant message, each
// holding one `tool_use` block of a call.
func AgentTranscript(calls []hostproto.ToolCall) ([]byte, error) {
	var out bytes.Buffer
	for _, call := range calls {
		line, err := json.Marshal(map[string]any{
			"type":    "assistant",
			"message": map[string]any{"content": []map[string]any{{"type": "tool_use", "name": call.Name, "input": call.Input}}},
		})
		if err != nil {
			return nil, err
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}
