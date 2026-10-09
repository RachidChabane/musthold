package hostproto

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// A Codex agent's rollout is a JSONL file: a `session_meta` line that names the
// thread and, for a spawned thread, the thread that spawned it, then one line
// per item. The `response_item` lines are what the model was given and did, in
// order; every other line is bookkeeping of the harness.

// ChildHistory is what a spawned thread's rollout shows of the history it started
// with.
type ChildHistory string

// Histories of a spawned thread.
const (
	// ChildBlind: the thread started with the harness's own context and the task
	// message alone.
	ChildBlind ChildHistory = "blind"
	// ChildInherited: the thread started with items of the agent that spawned it.
	ChildInherited ChildHistory = "inherited"
	// ChildUnproven: the rollout cannot show which of the two the thread started with.
	ChildUnproven ChildHistory = "unproven"
)

// Line and item types of a rollout.
const (
	rolloutSessionMeta  = "session_meta"
	rolloutResponseItem = "response_item"

	itemMessage              = "message"
	itemAgentMessage         = "agent_message"
	itemFunctionCall         = "function_call"
	itemFunctionCallOutput   = "function_call_output"
	itemCustomToolCall       = "custom_tool_call"
	itemCustomToolCallOutput = "custom_tool_call_output"
	itemReasoning            = "reasoning"
	itemLocalShellCall       = "local_shell_call"
	itemWebSearchCall        = "web_search_call"

	roleDeveloper = "developer"
	roleSystem    = "system"
	roleUser      = "user"
	roleAssistant = "assistant"

	blockInputText  = "input_text"
	blockOutputText = "output_text"

	// mcpNamespacePrefix starts the namespace Codex gives the tools of an MCP server.
	mcpNamespacePrefix = mcpPrefix
	// contextTagOpen and contextTagClose delimit the tag that opens a block of
	// harness context.
	contextTagOpen  = "<"
	contextTagClose = ">"
)

// codexContextTags are the tags that open the blocks of a user message the
// harness writes into every thread: the plugins it recommends and the environment.
var codexContextTags = []string{"recommended_plugins", "environment_context"}

// codexParentActivity are the item types that record what an agent did; before a
// thread's task message they can only be the spawning agent's.
var codexParentActivity = []string{
	itemFunctionCall, itemFunctionCallOutput, itemCustomToolCall, itemCustomToolCallOutput,
	itemReasoning, itemLocalShellCall, itemWebSearchCall,
}

// CodexChildRun is what the rollout of a spawned executor thread shows.
type CodexChildRun struct {
	// ThreadID is the thread the rollout belongs to.
	ThreadID string
	// ParentThreadID is the thread that spawned it.
	ParentThreadID string
	// AgentRole is the role file the thread ran as.
	AgentRole string
	// History is the judgment of the history the thread started with, and Reason
	// says why it is not blind.
	History ChildHistory
	Reason  string
	// Ticket is the ticket of the task message, or of the first fetch when the
	// task message does not show it; TicketFound is false when neither does.
	Ticket      TicketID
	TicketFound bool
	// Calls are the tool calls the thread made after its task message, in order.
	Calls []ToolCall
	// Reply is the text of the thread's last assistant message.
	Reply string
}

type rolloutLine struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

type rolloutSession struct {
	ID     string `json:"id"`
	Source struct {
		Subagent struct {
			ThreadSpawn struct {
				ParentThreadID string `json:"parent_thread_id"`
				AgentRole      string `json:"agent_role"`
			} `json:"thread_spawn"`
		} `json:"subagent"`
	} `json:"source"`
}

type rolloutItem struct {
	Type      string `json:"type"`
	Role      string `json:"role"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Arguments string `json:"arguments"`
	Content   []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
}

// itemRole is what an item that precedes the task message is to the judgment.
type itemRole int

const (
	itemHarnessContext itemRole = iota
	itemTaskMessage
	itemOfTheParent
	itemUnknown
)

// ReadCodexChildRollout judges the history a spawned thread started with from
// its rollout. The thread is blind when the first line is a `session_meta` that
// names the thread that spawned it, and every `response_item` before the task
// message is harness context: a developer or system message, or a user message
// whose every block is a text opened by a known context tag. The task message is
// the first `agent_message` item, or the first user message whose text starts
// with `ticket ` and holds exactly one ticket. The thread inherited history when
// an assistant message, a tool call or output, reasoning or any other user text
// precedes the task message, or when a second task message follows the first.
// The blindness is unproven when the rollout has no task message, when an item of
// another kind precedes it, when a user message there opens with an unknown tag,
// when a line other than an unterminated last one is not JSON, or when the first
// line does not name a spawning thread.
func ReadCodexChildRollout(rollout []byte) CodexChildRun {
	reader := childReader{run: CodexChildRun{History: ChildBlind}}
	reader.read(rollout)
	return reader.run
}

type childReader struct {
	run       CodexChildRun
	started   bool
	anchored  bool
	judgement bool
}

// judge records the first finding that the thread is not blind.
func (r *childReader) judge(history ChildHistory, reason string) {
	if r.judgement {
		return
	}
	r.judgement = true
	r.run.History, r.run.Reason = history, reason
}

func (r *childReader) read(rollout []byte) {
	lines := bytes.Split(withoutUnterminatedTail(rollout), []byte("\n"))
	for index, raw := range lines {
		number := index + 1
		text := bytes.TrimSpace(raw)
		if len(text) == 0 {
			continue
		}
		var line rolloutLine
		if err := json.Unmarshal(text, &line); err != nil {
			r.judge(ChildUnproven, fmt.Sprintf("line %d is not JSON", number))
			continue
		}
		if !r.started {
			r.started = true
			if !r.readSession(line) {
				r.judge(ChildUnproven, "the rollout does not start with the session of a spawned thread")
			}
			continue
		}
		if line.Type == rolloutResponseItem {
			r.readItem(number, line.Payload)
		}
	}
	switch {
	case !r.started:
		r.judge(ChildUnproven, "the rollout is empty")
	case !r.anchored:
		r.judge(ChildUnproven, "the rollout holds no task message")
	}
}

func (r *childReader) readSession(line rolloutLine) bool {
	if line.Type != rolloutSessionMeta {
		return false
	}
	var session rolloutSession
	if json.Unmarshal(line.Payload, &session) != nil {
		return false
	}
	spawn := session.Source.Subagent.ThreadSpawn
	r.run.ThreadID, r.run.ParentThreadID, r.run.AgentRole = session.ID, spawn.ParentThreadID, spawn.AgentRole
	return spawn.ParentThreadID != ""
}

func (r *childReader) readItem(number int, payload json.RawMessage) {
	var item rolloutItem
	if json.Unmarshal(payload, &item) != nil {
		r.judge(ChildUnproven, fmt.Sprintf("line %d holds an item that cannot be read", number))
		return
	}
	role := classifyItem(item)
	if !r.anchored {
		switch role {
		case itemHarnessContext:
		case itemTaskMessage:
			r.anchored = true
			r.takeTicket(item)
		case itemOfTheParent:
			r.judge(ChildInherited, fmt.Sprintf("line %d: %s precedes the task message", number, describeItem(item)))
		default:
			r.judge(ChildUnproven, fmt.Sprintf("line %d: %s precedes the task message", number, describeItem(item)))
		}
		return
	}
	switch {
	case role == itemTaskMessage:
		r.judge(ChildInherited, fmt.Sprintf("line %d: a second task message follows the first", number))
	case item.Type == itemFunctionCall:
		r.run.Calls = append(r.run.Calls, ToolCall{Name: qualifiedToolName(item), Input: argumentsOf(item.Arguments)})
		r.takeFetchedTicket()
	case item.Type == itemMessage && item.Role == roleAssistant:
		r.run.Reply = itemText(item)
	}
}

// classifyItem tells what an item is when it precedes the task message.
func classifyItem(item rolloutItem) itemRole {
	switch {
	case item.Type == itemAgentMessage:
		return itemTaskMessage
	case slices.Contains(codexParentActivity, item.Type):
		return itemOfTheParent
	case item.Type != itemMessage:
		return itemUnknown
	}
	switch item.Role {
	case roleDeveloper, roleSystem:
		return itemHarnessContext
	case roleAssistant:
		return itemOfTheParent
	case roleUser:
		return classifyUserMessage(item)
	default:
		return itemUnknown
	}
}

func classifyUserMessage(item rolloutItem) itemRole {
	if isTaskText(itemText(item)) {
		return itemTaskMessage
	}
	if len(item.Content) == 0 {
		return itemUnknown
	}
	tagged := true
	for _, block := range item.Content {
		if block.Type != blockInputText {
			return itemUnknown
		}
		tag, opened := openingTag(block.Text)
		if !opened {
			tagged = false
			continue
		}
		if !slices.Contains(codexContextTags, tag) {
			return itemUnknown
		}
	}
	if !tagged {
		return itemOfTheParent
	}
	return itemHarnessContext
}

// isTaskText reports whether a user message is the task an executor is spawned
// with: it starts with the executor prompt and holds exactly one ticket.
func isTaskText(text string) bool {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, executorPromptPrefix) {
		return false
	}
	_, err := FindTicketID(trimmed)
	return err == nil
}

// openingTag returns the tag that opens a text, as in `<environment_context>`.
func openingTag(text string) (string, bool) {
	rest, found := strings.CutPrefix(strings.TrimSpace(text), contextTagOpen)
	if !found {
		return "", false
	}
	tag, _, closed := strings.Cut(rest, contextTagClose)
	if !closed || tag == "" {
		return "", false
	}
	return tag, true
}

func itemText(item rolloutItem) string {
	var parts []string
	for _, block := range item.Content {
		if block.Type == blockInputText || block.Type == blockOutputText {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

func describeItem(item rolloutItem) string {
	if item.Type == itemMessage {
		return fmt.Sprintf("a %s message", item.Role)
	}
	return fmt.Sprintf("an item of type %q", item.Type)
}

func (r *childReader) takeTicket(item rolloutItem) {
	if item.Type != itemMessage {
		return
	}
	if ticket, err := FindTicketID(itemText(item)); err == nil {
		r.run.Ticket, r.run.TicketFound = ticket, true
	}
}

func (r *childReader) takeFetchedTicket() {
	if r.run.TicketFound {
		return
	}
	call := r.run.Calls[len(r.run.Calls)-1]
	if localToolName(call.Name) != ToolFetchRequest {
		return
	}
	if ticket, err := ParseTicketID(callsTicket(call)); err == nil {
		r.run.Ticket, r.run.TicketFound = ticket, true
	}
}

// qualifiedToolName is the name of a called tool: the namespace of an MCP server
// joined to the tool by `__`, as the hooks spell it.
func qualifiedToolName(item rolloutItem) string {
	if strings.HasPrefix(item.Namespace, mcpNamespacePrefix) {
		return item.Namespace + mcpSeparator + item.Name
	}
	return item.Namespace + item.Name
}

// argumentsOf turns the arguments of a call, a string holding a JSON document,
// into the document; anything else is kept as a JSON string.
func argumentsOf(arguments string) json.RawMessage {
	if json.Valid([]byte(arguments)) {
		return json.RawMessage(arguments)
	}
	encoded, err := json.Marshal(arguments)
	if err != nil {
		return nil
	}
	return encoded
}

// withoutUnterminatedTail drops a last line that no newline ends and that is not
// JSON: the harness may still be writing it when a hook reads the rollout.
func withoutUnterminatedTail(data []byte) []byte {
	if len(data) == 0 || data[len(data)-1] == '\n' {
		return data
	}
	complete := bytes.LastIndexByte(data, '\n') + 1
	if json.Valid(bytes.TrimSpace(data[complete:])) {
		return data
	}
	return data[:complete]
}
