package evidence

import (
	"encoding/json"
	"strings"
)

// The event stream of `copilot -p --output-format json` is JSON lines. The
// canary relies on the session event envelope the Copilot SDK documents
// (https://docs.github.com/copilot/how-tos/copilot-sdk/features/streaming-events,
// source github/copilot-sdk docs/features/streaming-events.md): every record is
// `{id, timestamp, parentId, agentId?, ephemeral?, type, data}`, `agentId` is
// present on events a sub-agent originated, and the members of `data` read below
// belong to the types named here. The CLI reference documents the format only as
// "JSONL, one JSON object per line"; that its prompt mode prints these session
// events is an inference, which is why a check that finds none of them concludes
// INCONCLUSIVE and why every other member of a record is kept in Raw.
const (
	CopilotEventSessionStart     = "session.start"
	CopilotEventAssistantMessage = "assistant.message"
	CopilotEventToolStart        = "tool.execution_start"
	CopilotEventToolComplete     = "tool.execution_complete"
	CopilotEventSubagentStarted  = "subagent.started"
	CopilotEventSubagentDone     = "subagent.completed"
	CopilotEventSubagentFailed   = "subagent.failed"
	CopilotEventSessionIdle      = "session.idle"
	CopilotEventSessionShutdown  = "session.shutdown"
	CopilotEventSessionError     = "session.error"

	// CopilotTaskTool is the tool through which the main agent starts a custom agent.
	CopilotTaskTool = "task"

	// CopilotShutdownRoutine is the shutdownType of a session that ended normally.
	CopilotShutdownRoutine = "routine"

	// Error types of session.error that say the session was refused, not that the
	// harness misbehaved.
	CopilotErrorAuthentication = "authentication"
	CopilotErrorQuota          = "quota"
	CopilotErrorRateLimit      = "rate_limit"
)

// CopilotEvent is one record of the stream. Only the members that are present are
// set.
type CopilotEvent struct {
	Line int
	Type string
	// AgentID is the envelope's agentId: set on events a sub-agent originated.
	AgentID string
	// ToolCallID joins a tool's start to its completion.
	ToolCallID string
	// ToolName, MCPServer and MCPTool are the members of tool.execution_start.
	ToolName  string
	MCPServer string
	MCPTool   string
	// Success is the success member of tool.execution_complete.
	Success *bool
	// AgentName is the name of the agent of a subagent.* event.
	AgentName string
	// SelectedModel is the selectedModel of session.start: the model the session
	// actually ran on.
	SelectedModel string
	// Arguments are the arguments of a tool.execution_start.
	Arguments json.RawMessage
	// ShutdownType is the shutdownType of session.shutdown.
	ShutdownType string
	// ErrorType and Message are the members of session.error.
	ErrorType string
	Message   string
	Raw       json.RawMessage
}

// Mentions reports whether the record's JSON holds the text anywhere.
func (e CopilotEvent) Mentions(text string) bool { return strings.Contains(string(e.Raw), text) }

// CopilotStream is a decoded `copilot --output-format json` stream.
type CopilotStream struct {
	Events []CopilotEvent
}

type rawCopilot struct {
	Type    string         `json:"type"`
	AgentID string         `json:"agentId"`
	Data    rawCopilotData `json:"data"`
}

type rawCopilotData struct {
	ToolCallID    string          `json:"toolCallId"`
	ToolName      string          `json:"toolName"`
	MCPServer     string          `json:"mcpServerName"`
	MCPTool       string          `json:"mcpToolName"`
	Success       json.RawMessage `json:"success"`
	AgentName     string          `json:"agentName"`
	SelectedModel string          `json:"selectedModel"`
	Arguments     json.RawMessage `json:"arguments"`
	ShutdownType  string          `json:"shutdownType"`
	ErrorType     string          `json:"errorType"`
	Message       string          `json:"message"`
}

// DecodeCopilot decodes the stream. A member that does not have the shape above is
// left unset; the record's raw JSON is always kept.
func DecodeCopilot(lines []Line) CopilotStream {
	var stream CopilotStream
	for _, line := range lines {
		var raw rawCopilot
		// A member of an unexpected type is skipped and the others still decode.
		_ = json.Unmarshal(line.Raw, &raw)
		stream.Events = append(stream.Events, CopilotEvent{
			Line: line.Number, Type: raw.Type, AgentID: raw.AgentID, ToolCallID: raw.Data.ToolCallID,
			ToolName: raw.Data.ToolName, MCPServer: raw.Data.MCPServer, MCPTool: raw.Data.MCPTool, Success: boolMember(raw.Data.Success),
			AgentName: raw.Data.AgentName, SelectedModel: raw.Data.SelectedModel, Arguments: raw.Data.Arguments, ShutdownType: raw.Data.ShutdownType, ErrorType: raw.Data.ErrorType,
			Message: raw.Data.Message, Raw: line.Raw,
		})
	}
	return stream
}

// Of returns the events of a type, in order.
func (s CopilotStream) Of(eventType string) []CopilotEvent {
	var found []CopilotEvent
	for _, event := range s.Events {
		if event.Type == eventType {
			found = append(found, event)
		}
	}
	return found
}

// SelectedModel returns the model the session reports it ran on, from the first
// session.start that names one.
func (s CopilotStream) SelectedModel() (CopilotEvent, bool) {
	for _, event := range s.Of(CopilotEventSessionStart) {
		if event.SelectedModel != "" {
			return event, true
		}
	}
	return CopilotEvent{}, false
}

// TaskAgentType returns the agent_type argument of a tool.execution_start of the
// tool that starts a custom agent, and whether the event is such a call.
func (e CopilotEvent) TaskAgentType() (string, bool) {
	if e.Type != CopilotEventToolStart || e.ToolName != CopilotTaskTool {
		return "", false
	}
	var arguments struct {
		AgentType string `json:"agent_type"`
	}
	_ = json.Unmarshal(e.Arguments, &arguments)
	return arguments.AgentType, true
}

// CompletionOf returns the tool.execution_complete that closes a tool call.
func (s CopilotStream) CompletionOf(toolCallID string) (CopilotEvent, bool) {
	for _, event := range s.Events {
		if event.Type == CopilotEventToolComplete && toolCallID != "" && event.ToolCallID == toolCallID {
			return event, true
		}
	}
	return CopilotEvent{}, false
}

// Refusal returns the session.error that says the session was refused
// (authentication, quota or rate limit), if the stream holds one.
func (s CopilotStream) Refusal() (CopilotEvent, bool) {
	for _, event := range s.Of(CopilotEventSessionError) {
		switch event.ErrorType {
		case CopilotErrorAuthentication, CopilotErrorQuota, CopilotErrorRateLimit:
			return event, true
		}
	}
	return CopilotEvent{}, false
}

// Closing returns the event that shows the session ended normally: a routine
// shutdown, or the idle event that follows the last turn.
func (s CopilotStream) Closing() (CopilotEvent, bool) {
	for _, event := range s.Events {
		switch {
		case event.Type == CopilotEventSessionShutdown && event.ShutdownType == CopilotShutdownRoutine:
			return event, true
		case event.Type == CopilotEventSessionIdle:
			return event, true
		}
	}
	return CopilotEvent{}, false
}

// boolMember reads a boolean member, and nothing when it is absent or is not one:
// a value of another type must never read as false.
func boolMember(raw json.RawMessage) *bool {
	var value bool
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return &value
}
