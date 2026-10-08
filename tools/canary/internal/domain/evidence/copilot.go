package evidence

import (
	"encoding/json"
	"strings"
)

// The event stream of `copilot -p --output-format json` is JSON lines. Every
// session event is `{id, timestamp, parentId, agentId?, ephemeral?, type, data}`,
// `agentId` is present on events a sub-agent originated, and the members of
// `data` read below belong to the types named here. The stream has no
// session.start and no session.shutdown: the model of a session is the `model` of
// its model.call_start events, and the session ends with a top-level
// `{"type":"result","exitCode":0,"usage":{"premiumRequests":N}}` record. A check
// that finds none of these events concludes INCONCLUSIVE, and every other member
// of a record is kept in Raw.
const (
	CopilotEventModelCallStart   = "model.call_start"
	CopilotEventAssistantMessage = "assistant.message"
	CopilotEventUserMessage      = "user.message"
	CopilotEventToolStart        = "tool.execution_start"
	CopilotEventToolComplete     = "tool.execution_complete"
	CopilotEventSubagentStarted  = "subagent.started"
	CopilotEventSubagentSelected = "subagent.selected"
	CopilotEventSubagentDone     = "subagent.completed"
	CopilotEventSubagentFailed   = "subagent.failed"
	CopilotEventSessionError     = "session.error"
	// CopilotEventResult is the record that closes the stream.
	CopilotEventResult = "result"

	// CopilotTaskTool is the tool through which the main agent starts a custom agent.
	CopilotTaskTool = "task"

	// CopilotExitSuccess is the exitCode of a session that ended by itself.
	CopilotExitSuccess = 0

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
	// ToolCallID joins a tool's start to its completion. On a subagent.started and a
	// subagent.completed it is the call of the task tool that launched the subagent.
	ToolCallID string
	// ParentToolCallID is the task call whose subagent originated a tool event or a
	// message: the explicit link from a subagent's work to the call that launched it.
	ParentToolCallID string
	// Tools are the tools a subagent.selected lists for the subagent.
	Tools []string
	// ToolName, MCPServer and MCPTool are the members of tool.execution_start.
	ToolName  string
	MCPServer string
	MCPTool   string
	// Success is the success member of tool.execution_complete.
	Success *bool
	// AgentName is the name of the agent of a subagent.* event.
	AgentName string
	// Model is the model of a model.call_start: the model the session actually ran on.
	Model string
	// Arguments are the arguments of a tool.execution_start.
	Arguments json.RawMessage
	// ExitCode and PremiumRequests are the members of the closing result record.
	ExitCode        *int
	PremiumRequests *int
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
	Type     string          `json:"type"`
	AgentID  string          `json:"agentId"`
	ExitCode json.RawMessage `json:"exitCode"`
	Usage    struct {
		PremiumRequests json.RawMessage `json:"premiumRequests"`
	} `json:"usage"`
	Data rawCopilotData `json:"data"`
}

type rawCopilotData struct {
	ToolCallID       string          `json:"toolCallId"`
	ParentToolCallID string          `json:"parentToolCallId"`
	ToolName         string          `json:"toolName"`
	MCPServer        string          `json:"mcpServerName"`
	MCPTool          string          `json:"mcpToolName"`
	Success          json.RawMessage `json:"success"`
	AgentName        string          `json:"agentName"`
	Model            string          `json:"model"`
	Arguments        json.RawMessage `json:"arguments"`
	Tools            json.RawMessage `json:"tools"`
	ErrorType        string          `json:"errorType"`
	Message          string          `json:"message"`
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
			ParentToolCallID: raw.Data.ParentToolCallID, Tools: stringList(raw.Data.Tools),
			ToolName: raw.Data.ToolName, MCPServer: raw.Data.MCPServer, MCPTool: raw.Data.MCPTool, Success: boolMember(raw.Data.Success),
			AgentName: raw.Data.AgentName, Model: raw.Data.Model, Arguments: raw.Data.Arguments, ErrorType: raw.Data.ErrorType,
			ExitCode: intMember(raw.ExitCode), PremiumRequests: intMember(raw.Usage.PremiumRequests),
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
// model.call_start that names one.
func (s CopilotStream) SelectedModel() (CopilotEvent, bool) {
	for _, event := range s.Of(CopilotEventModelCallStart) {
		if event.Model != "" {
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

// Closing returns the result record that shows the session ended by itself: the
// closing record of the stream with exit code 0.
func (s CopilotStream) Closing() (CopilotEvent, bool) {
	for _, event := range s.Of(CopilotEventResult) {
		if event.ExitCode != nil && *event.ExitCode == CopilotExitSuccess {
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

// intMember reads an integer member, and nothing when it is absent or is not one.
func intMember(raw json.RawMessage) *int {
	var value int
	if len(raw) == 0 || json.Unmarshal(raw, &value) != nil {
		return nil
	}
	return &value
}

// stringList reads a list of strings, and nothing when the member is absent or is
// not one: a list that does not have that shape must never read as an empty one.
func stringList(raw json.RawMessage) []string {
	var values []string
	if len(raw) == 0 || json.Unmarshal(raw, &values) != nil {
		return nil
	}
	return values
}
