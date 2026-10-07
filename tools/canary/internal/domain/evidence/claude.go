package evidence

import (
	"encoding/json"
	"strings"
)

// Types and subtypes of the stream-json events of Claude Code the checks read.
const (
	ClaudeTypeSystem    = "system"
	ClaudeTypeAssistant = "assistant"
	ClaudeTypeUser      = "user"
	ClaudeTypeResult    = "result"
	ClaudeSubtypeInit   = "init"
	// ClaudeSubtypeHookResponse is the event a hook's completion produces under
	// --include-hook-events.
	ClaudeSubtypeHookResponse = "hook_response"
	ClaudeSubtypeHookStarted  = "hook_started"

	// BlockToolUse is the type of a tool call in the content of a message.
	BlockToolUse    = "tool_use"
	blockToolResult = "tool_result"
	blockText       = "text"
)

// Named is a plugin or a server as `system/init` lists it: a bare name or an
// object with a name.
type Named struct {
	Name   string
	Status string
}

// UnmarshalJSON accepts a string or an object with a `name` member.
func (n *Named) UnmarshalJSON(data []byte) error {
	var name string
	if json.Unmarshal(data, &name) == nil {
		n.Name = name
		return nil
	}
	var object struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal(data, &object); err != nil {
		return err
	}
	n.Name, n.Status = object.Name, object.Status
	return nil
}

// ClaudeInit is what `system/init` announces.
type ClaudeInit struct {
	Model        string
	Plugins      []Named
	PluginErrors []json.RawMessage
	Servers      []Named
	Tools        []string
}

// ClaudeResult is the closing `result` event.
type ClaudeResult struct {
	Subtype           string
	IsError           bool
	NumTurns          int
	TotalCostUSD      float64
	TerminalReason    string
	Models            []string
	PermissionDenials []json.RawMessage
}

// Block is one content block of an assistant or user message.
type Block struct {
	Type      string
	ID        string
	Name      string
	Input     json.RawMessage
	ToolUseID string
	IsError   bool
	// Text is the text of a text block, or the flattened content of a tool result.
	Text string
}

// ClaudeEvent is one line of the stream.
type ClaudeEvent struct {
	Line      int
	Type      string
	Subtype   string
	SessionID string
	// Parent is the tool_use id of the Agent call whose subagent produced the
	// event; it is empty for the main session.
	Parent string
	// Model is the model an assistant message was produced by.
	Model     string
	Init      ClaudeInit
	Result    ClaudeResult
	Blocks    []Block
	HookEvent string
	HookExit  *int
}

// ToolUse is a tool call of an assistant message.
type ToolUse struct {
	Line   int
	ID     string
	Name   string
	Input  json.RawMessage
	Parent string
}

// ToolResult is the outcome of a tool call.
type ToolResult struct {
	Line    int
	IsError bool
	Text    string
}

// ClaudeStream is a decoded stream-json event stream.
type ClaudeStream struct{ Events []ClaudeEvent }

type rawClaude struct {
	Type              string            `json:"type"`
	Subtype           string            `json:"subtype"`
	SessionID         string            `json:"session_id"`
	Parent            *string           `json:"parent_tool_use_id"`
	Model             string            `json:"model"`
	Plugins           []Named           `json:"plugins"`
	PluginErrors      []json.RawMessage `json:"plugin_errors"`
	Servers           []Named           `json:"mcp_servers"`
	Tools             []string          `json:"tools"`
	Message           *rawMessage       `json:"message"`
	IsError           bool              `json:"is_error"`
	NumTurns          int               `json:"num_turns"`
	TotalCostUSD      float64           `json:"total_cost_usd"`
	TerminalReason    string            `json:"terminal_reason"`
	ModelUsage        map[string]any    `json:"modelUsage"`
	PermissionDenials []json.RawMessage `json:"permission_denials"`
	HookEvent         string            `json:"hook_event"`
	HookEventName     string            `json:"hook_event_name"`
	ExitCode          *int              `json:"exit_code"`
}

type rawMessage struct {
	Model   string          `json:"model"`
	Content json.RawMessage `json:"content"`
}

type rawBlock struct {
	Type      string          `json:"type"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Input     json.RawMessage `json:"input"`
	ToolUseID string          `json:"tool_use_id"`
	IsError   bool            `json:"is_error"`
	Text      string          `json:"text"`
	Content   json.RawMessage `json:"content"`
}

// DecodeClaude decodes the stream. A line that does not have the shape of an
// event is kept as an event with no type.
func DecodeClaude(lines []Line) ClaudeStream {
	stream := ClaudeStream{Events: make([]ClaudeEvent, 0, len(lines))}
	for _, line := range lines {
		var raw rawClaude
		_ = json.Unmarshal(line.Raw, &raw)
		event := ClaudeEvent{Line: line.Number, Type: raw.Type, Subtype: raw.Subtype, SessionID: raw.SessionID}
		if raw.Parent != nil {
			event.Parent = *raw.Parent
		}
		event.HookEvent = raw.HookEvent
		if event.HookEvent == "" {
			event.HookEvent = raw.HookEventName
		}
		event.HookExit = raw.ExitCode
		switch {
		case raw.Type == ClaudeTypeSystem && raw.Subtype == ClaudeSubtypeInit:
			event.Init = ClaudeInit{Model: raw.Model, Plugins: raw.Plugins, PluginErrors: raw.PluginErrors, Servers: raw.Servers, Tools: raw.Tools}
		case raw.Type == ClaudeTypeResult:
			event.Result = ClaudeResult{
				Subtype: raw.Subtype, IsError: raw.IsError, NumTurns: raw.NumTurns, TotalCostUSD: raw.TotalCostUSD,
				TerminalReason: raw.TerminalReason, PermissionDenials: raw.PermissionDenials,
			}
			for model := range raw.ModelUsage {
				event.Result.Models = append(event.Result.Models, model)
			}
		case raw.Message != nil:
			event.Model = raw.Message.Model
			event.Blocks = decodeBlocks(raw.Message.Content)
		}
		stream.Events = append(stream.Events, event)
	}
	return stream
}

func decodeBlocks(content json.RawMessage) []Block {
	var raws []rawBlock
	if err := json.Unmarshal(content, &raws); err != nil {
		return nil
	}
	blocks := make([]Block, 0, len(raws))
	for _, raw := range raws {
		block := Block{Type: raw.Type, ID: raw.ID, Name: raw.Name, Input: raw.Input, ToolUseID: raw.ToolUseID, IsError: raw.IsError, Text: raw.Text}
		if raw.Type == blockToolResult {
			block.Text = flatten(raw.Content)
		}
		blocks = append(blocks, block)
	}
	return blocks
}

// flatten turns the content of a tool result, a string or a list of text blocks,
// into text.
func flatten(content json.RawMessage) string {
	var text string
	if json.Unmarshal(content, &text) == nil {
		return text
	}
	var parts []rawBlock
	if json.Unmarshal(content, &parts) != nil {
		return string(content)
	}
	var joined []string
	for _, part := range parts {
		if part.Type == blockText || part.Text != "" {
			joined = append(joined, part.Text)
		}
	}
	return strings.Join(joined, "\n")
}

// Init returns the `system/init` event.
func (s ClaudeStream) Init() (ClaudeEvent, bool) {
	for _, event := range s.Events {
		if event.Type == ClaudeTypeSystem && event.Subtype == ClaudeSubtypeInit {
			return event, true
		}
	}
	return ClaudeEvent{}, false
}

// Final returns the closing `result` event.
func (s ClaudeStream) Final() (ClaudeEvent, bool) {
	for _, event := range s.Events {
		if event.Type == ClaudeTypeResult {
			return event, true
		}
	}
	return ClaudeEvent{}, false
}

// ToolUses returns the calls of the tool in the stream, in order. The empty name
// returns every call.
func (s ClaudeStream) ToolUses(name string) []ToolUse {
	var calls []ToolUse
	for _, event := range s.Events {
		if event.Type != ClaudeTypeAssistant {
			continue
		}
		for _, block := range event.Blocks {
			if block.Type == BlockToolUse && (name == "" || block.Name == name) {
				calls = append(calls, ToolUse{Line: event.Line, ID: block.ID, Name: block.Name, Input: block.Input, Parent: event.Parent})
			}
		}
	}
	return calls
}

// ResultOf returns the outcome of the tool call with the id.
func (s ClaudeStream) ResultOf(toolUseID string) (ToolResult, bool) {
	for _, event := range s.Events {
		for _, block := range event.Blocks {
			if block.Type == blockToolResult && block.ToolUseID == toolUseID {
				return ToolResult{Line: event.Line, IsError: block.IsError, Text: block.Text}, true
			}
		}
	}
	return ToolResult{}, false
}

// Children returns the events a subagent produced, by the id of the Agent call
// that launched it.
func (s ClaudeStream) Children(agentCallID string) []ClaudeEvent {
	var children []ClaudeEvent
	for _, event := range s.Events {
		if event.Parent == agentCallID {
			children = append(children, event)
		}
	}
	return children
}

// HookEvents returns the hook completion events of a hook event name, such as
// `Stop`.
func (s ClaudeStream) HookEvents(hookEvent string) []ClaudeEvent {
	var found []ClaudeEvent
	for _, event := range s.Events {
		if event.Type == ClaudeTypeSystem && event.Subtype == ClaudeSubtypeHookResponse && event.HookEvent == hookEvent {
			found = append(found, event)
		}
	}
	return found
}
