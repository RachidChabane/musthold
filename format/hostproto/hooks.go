package hostproto

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

// Harness names, as recorded in an executor's harness member.
const (
	HarnessClaudeCode = "claude-code"
	HarnessCodex      = "codex"
	HarnessCopilot    = "copilot-cli"
	// HarnessAgentPlugins is a client of the Agent Plugins standard that the module
	// serves through the portable plugin alone, with no hook of its own.
	HarnessAgentPlugins = "agent-plugins"
)

// HookKind names the hook a module serves, as the `hook` command spells it.
type HookKind string

// Hook kinds.
const (
	HookPreTool       HookKind = "pre-tool"
	HookSubagentStart HookKind = "subagent-start"
	HookSubagentStop  HookKind = "subagent-stop"
	HookPostTool      HookKind = "post-tool"
)

// Valid reports whether the kind is one of the hooks a module serves.
func (k HookKind) Valid() bool { return slices.Contains(HookKinds(), k) }

// HookKinds lists the hooks a module serves.
func HookKinds() []HookKind {
	return []HookKind{HookPreTool, HookSubagentStart, HookSubagentStop, HookPostTool}
}

// Errors of the hook decoders.
var (
	// ErrUnknownHarness is returned for a harness without a decoder.
	ErrUnknownHarness = errors.New("unknown harness")
	// ErrBadHookPayload is returned for a payload that is not a document of the hook it was given for.
	ErrBadHookPayload = errors.New("bad hook payload")
)

// TokenUsage is the usage a harness reports for a subagent run.
type TokenUsage struct {
	InputTokens  int `json:"inputTokens"`
	OutputTokens int `json:"outputTokens"`
	TotalTokens  int `json:"totalTokens"`
}

// HookEvent is a hook payload normalized across harnesses. Only the fields the
// hook carries are set.
type HookEvent struct {
	Harness        string          `json:"harness"`
	Kind           HookKind        `json:"kind"`
	SessionID      string          `json:"sessionId,omitempty"`
	AgentID        string          `json:"agentId,omitempty"`
	AgentType      string          `json:"agentType,omitempty"`
	ToolName       string          `json:"toolName,omitempty"`
	ToolInput      json.RawMessage `json:"toolInput,omitempty"`
	ToolUseID      string          `json:"toolUseId,omitempty"`
	TranscriptPath string          `json:"transcriptPath,omitempty"`
	LastMessage    string          `json:"lastMessage,omitempty"`
	ResolvedModel  string          `json:"resolvedModel,omitempty"`
	Usage          TokenUsage      `json:"usage"`
	// Reply is the text of an Agent tool's response, which for an executor is
	// what the subagent answered.
	Reply string `json:"reply,omitempty"`
}

// HookDecoder reads the stdin payload of one harness's hook into a HookEvent.
type HookDecoder interface {
	Decode(kind HookKind, payload []byte) (HookEvent, error)
}

// DecoderFor returns the decoder of a harness.
func DecoderFor(harness string) (HookDecoder, error) {
	switch harness {
	case HarnessClaudeCode, HarnessCodex:
		return snakeCaseDecoder{harness: harness}, nil
	case HarnessCopilot:
		return camelCaseDecoder{}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownHarness, harness)
	}
}

// snakeEventNames are the hook_event_name values of the harnesses that share the
// snake_case payload.
var snakeEventNames = map[HookKind]string{
	HookPreTool:       "PreToolUse",
	HookSubagentStart: "SubagentStart",
	HookSubagentStop:  "SubagentStop",
	HookPostTool:      "PostToolUse",
}

// snakeCaseDecoder reads the payloads of Claude Code and Codex, whose documented
// fields coincide.
type snakeCaseDecoder struct{ harness string }

type snakePayload struct {
	SessionID            string          `json:"session_id"`
	HookEventName        string          `json:"hook_event_name"`
	AgentID              string          `json:"agent_id"`
	AgentType            string          `json:"agent_type"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	ToolUseID            string          `json:"tool_use_id"`
	ToolResponse         json.RawMessage `json:"tool_response"`
	AgentTranscriptPath  string          `json:"agent_transcript_path"`
	LastAssistantMessage string          `json:"last_assistant_message"`
}

// Decode implements HookDecoder.
func (d snakeCaseDecoder) Decode(kind HookKind, payload []byte) (HookEvent, error) {
	var raw snakePayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return HookEvent{}, fmt.Errorf("%w: %v", ErrBadHookPayload, err)
	}
	want, known := snakeEventNames[kind]
	if !known {
		return HookEvent{}, fmt.Errorf("%w: unknown hook %q", ErrBadHookPayload, kind)
	}
	if raw.HookEventName != "" && raw.HookEventName != want {
		return HookEvent{}, fmt.Errorf("%w: hook_event_name %q for hook %q", ErrBadHookPayload, raw.HookEventName, kind)
	}
	event := HookEvent{
		Harness: d.harness, Kind: kind, SessionID: raw.SessionID,
		AgentID: raw.AgentID, AgentType: raw.AgentType,
		ToolName: raw.ToolName, ToolInput: raw.ToolInput, ToolUseID: raw.ToolUseID,
		TranscriptPath: raw.AgentTranscriptPath, LastMessage: raw.LastAssistantMessage,
	}
	event.ResolvedModel, event.Usage, event.Reply = readToolResponse(raw.ToolResponse)
	return event, requireFields(event)
}

type camelPayload struct {
	SessionID      string          `json:"sessionId"`
	ToolName       string          `json:"toolName"`
	ToolArgs       json.RawMessage `json:"toolArgs"`
	AgentID        string          `json:"agentId"`
	AgentType      string          `json:"agentType"`
	TranscriptPath string          `json:"transcriptPath"`
	Response       string          `json:"response"`
	ToolResult     json.RawMessage `json:"toolResult"`
}

// camelCaseDecoder reads the payloads of GitHub Copilot CLI.
type camelCaseDecoder struct{}

// Decode implements HookDecoder.
func (camelCaseDecoder) Decode(kind HookKind, payload []byte) (HookEvent, error) {
	var raw camelPayload
	if err := json.Unmarshal(payload, &raw); err != nil {
		return HookEvent{}, fmt.Errorf("%w: %v", ErrBadHookPayload, err)
	}
	if _, known := snakeEventNames[kind]; !known {
		return HookEvent{}, fmt.Errorf("%w: unknown hook %q", ErrBadHookPayload, kind)
	}
	event := HookEvent{
		Harness: HarnessCopilot, Kind: kind, SessionID: raw.SessionID,
		AgentID: raw.AgentID, AgentType: raw.AgentType,
		ToolName: raw.ToolName, ToolInput: unwrapToolArgs(raw.ToolArgs),
		TranscriptPath: raw.TranscriptPath, LastMessage: raw.Response,
	}
	event.ResolvedModel, event.Usage, event.Reply = readToolResponse(raw.ToolResult)
	return event, requireFields(event)
}

// unwrapToolArgs turns the arguments Copilot sends, a JSON document or a string
// holding one, into the JSON document.
func unwrapToolArgs(args json.RawMessage) json.RawMessage {
	var encoded string
	if err := json.Unmarshal(args, &encoded); err == nil && json.Valid([]byte(encoded)) {
		return json.RawMessage(encoded)
	}
	return args
}

func requireFields(event HookEvent) error {
	switch event.Kind {
	case HookPreTool, HookPostTool:
		if event.ToolName == "" {
			return fmt.Errorf("%w: %s without a tool name", ErrBadHookPayload, event.Kind)
		}
	case HookSubagentStart, HookSubagentStop:
		if event.AgentID == "" {
			return fmt.Errorf("%w: %s without an agent id", ErrBadHookPayload, event.Kind)
		}
	}
	return nil
}

// contentTypeText is the type of a text block in a tool response.
const contentTypeText = "text"

// readToolResponse reads the resolved model, the usage and the reply text out of
// the response of an Agent tool. Any other shape yields zero values, since the
// response varies per tool.
func readToolResponse(raw json.RawMessage) (string, TokenUsage, string) {
	var response struct {
		ResolvedModel string `json:"resolvedModel"`
		TotalTokens   int    `json:"totalTokens"`
		Usage         struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return "", TokenUsage{}, ""
	}
	usage := TokenUsage{InputTokens: response.Usage.InputTokens, OutputTokens: response.Usage.OutputTokens, TotalTokens: response.TotalTokens}
	return response.ResolvedModel, usage, replyText(response.Content)
}

// replyText joins the text blocks of a response's content, or reads the content
// itself when it is a string.
func replyText(content json.RawMessage) string {
	var text string
	if err := json.Unmarshal(content, &text); err == nil {
		return text
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, block := range blocks {
		if block.Type == contentTypeText {
			parts = append(parts, block.Text)
		}
	}
	return strings.Join(parts, "")
}

// Names of the tools the hooks reason about.
const (
	// ToolAgent is the tool that launches a subagent.
	ToolAgent = "Agent"
	// ToolTask is the former name of ToolAgent.
	ToolTask = "Task"
	// ToolFetchRequest is the executor tool that returns a request.
	ToolFetchRequest = "fetch_request"
	// ToolSubmitResponse is the executor tool that submits an answer.
	ToolSubmitResponse = "submit_response"
	// ToolDrain is the optional tool that serves pending tickets through sampling.
	ToolDrain = "drain"

	mcpPrefix       = "mcp__"
	mcpSeparator    = "__"
	pluginServer    = "plugin_"
	pluginSeparator = "_"
)

// IsAgentTool reports whether the tool launches a subagent.
func IsAgentTool(toolName string) bool { return toolName == ToolAgent || toolName == ToolTask }

// MCPTool is an MCP tool name split into its server and tool.
type MCPTool struct {
	Server string
	Tool   string
}

// ParseMCPToolName splits `mcp__<server>__<tool>`.
func ParseMCPToolName(name string) (MCPTool, bool) {
	rest, found := strings.CutPrefix(name, mcpPrefix)
	if !found {
		return MCPTool{}, false
	}
	server, tool, found := strings.Cut(rest, mcpSeparator)
	if !found || server == "" || tool == "" {
		return MCPTool{}, false
	}
	return MCPTool{Server: server, Tool: tool}, true
}

// ParsePluginServer splits the server name of a plugin-scoped MCP server,
// `plugin_<plugin>_<server>`.
func ParsePluginServer(server string) (plugin, name string, ok bool) {
	rest, found := strings.CutPrefix(server, pluginServer)
	if !found {
		return "", "", false
	}
	cut := strings.LastIndex(rest, pluginSeparator)
	if cut <= 0 || cut == len(rest)-len(pluginSeparator) {
		return "", "", false
	}
	return rest[:cut], rest[cut+len(pluginSeparator):], true
}

// AgentInput is the input of the Agent tool.
type AgentInput struct {
	SubagentType    string `json:"subagent_type"`
	Description     string `json:"description"`
	Prompt          string `json:"prompt"`
	Model           string `json:"model,omitempty"`
	RunInBackground bool   `json:"run_in_background"`
}

// DecodeAgentInput reads the input of an Agent tool call.
func DecodeAgentInput(raw json.RawMessage) (AgentInput, error) {
	var input AgentInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return AgentInput{}, fmt.Errorf("%w: agent input: %v", ErrBadHookPayload, err)
	}
	return input, nil
}

// ErrNotExecutorRole is returned when an Agent call does not name an executor role of the plugin.
var ErrNotExecutorRole = errors.New("the subagent is not an executor role of the plugin")

// IsExecutorRole reports whether the subagent type is `<plugin>:<agent>` for one
// of the executor agents.
func IsExecutorRole(subagentType, plugin string) bool {
	rolePlugin, _, err := ParseRole(subagentType)
	return err == nil && rolePlugin == plugin
}

// Sanitize rebuilds the input of an executor launch: it keeps the role and the
// description, replaces the prompt by the single ticket the prompt held, runs in
// the foreground and drops the model and any padding. A prompt that holds no
// single ticket is refused.
func (in AgentInput) Sanitize(plugin string) (AgentInput, error) {
	if !IsExecutorRole(in.SubagentType, plugin) {
		return AgentInput{}, fmt.Errorf("%w: %q", ErrNotExecutorRole, in.SubagentType)
	}
	ticket, err := FindTicketID(in.Prompt)
	if err != nil {
		return AgentInput{}, err
	}
	return AgentInput{SubagentType: in.SubagentType, Description: in.Description, Prompt: ExecutorPrompt(ticket)}, nil
}

// executorPromptPrefix starts the only prompt an executor is given.
const executorPromptPrefix = "ticket "

// ExecutorPrompt is the whole prompt of an executor launch.
func ExecutorPrompt(ticket TicketID) string { return executorPromptPrefix + ticket.String() }

// InjectCap sets the `cap` member of an MCP tool input, which is how the hook
// hands the executor's capability to the server.
func InjectCap(toolInput json.RawMessage, capability string) (json.RawMessage, error) {
	members := map[string]json.RawMessage{}
	if len(toolInput) > 0 {
		if err := json.Unmarshal(toolInput, &members); err != nil {
			return nil, fmt.Errorf("%w: tool input: %v", ErrBadHookPayload, err)
		}
	}
	encoded, err := json.Marshal(capability)
	if err != nil {
		return nil, err
	}
	members[CapMember] = encoded
	return json.Marshal(members)
}

// CapMember is the member of the executor tools that carries the capability.
const CapMember = "cap"

// Permission is the decision of a PreToolUse hook.
type Permission string

// Permissions.
const (
	PermissionAllow Permission = "allow"
	PermissionDeny  Permission = "deny"
	PermissionAsk   Permission = "ask"
)

// PreToolDecision is what a PreToolUse hook answers.
type PreToolDecision struct {
	Permission   Permission
	Reason       string
	UpdatedInput json.RawMessage
}

type hookSpecificOutput struct {
	HookEventName string          `json:"hookEventName"`
	Permission    Permission      `json:"permissionDecision"`
	Reason        string          `json:"permissionDecisionReason,omitempty"`
	UpdatedInput  json.RawMessage `json:"updatedInput,omitempty"`
}

type copilotDecision struct {
	Permission   Permission      `json:"permissionDecision"`
	Reason       string          `json:"permissionDecisionReason,omitempty"`
	ModifiedArgs json.RawMessage `json:"modifiedArgs,omitempty"`
}

// EncodePreToolDecision renders the decision in the output format of the harness.
func EncodePreToolDecision(harness string, decision PreToolDecision) ([]byte, error) {
	switch harness {
	case HarnessClaudeCode, HarnessCodex:
		return json.Marshal(struct {
			Output hookSpecificOutput `json:"hookSpecificOutput"`
		}{hookSpecificOutput{
			HookEventName: snakeEventNames[HookPreTool], Permission: decision.Permission,
			Reason: decision.Reason, UpdatedInput: decision.UpdatedInput,
		}})
	case HarnessCopilot:
		return json.Marshal(copilotDecision{Permission: decision.Permission, Reason: decision.Reason, ModifiedArgs: decision.UpdatedInput})
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownHarness, harness)
	}
}

const (
	decisionBlock = "block"
	noDecision    = "{}"
)

// StopDecision is what a SubagentStop hook answers.
type StopDecision struct {
	Block  bool
	Reason string
}

// EncodeStopDecision renders the decision in the output format of the harness.
func EncodeStopDecision(harness string, decision StopDecision) ([]byte, error) {
	switch harness {
	case HarnessClaudeCode, HarnessCodex, HarnessCopilot:
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownHarness, harness)
	}
	if !decision.Block {
		return []byte(noDecision), nil
	}
	return json.Marshal(struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}{Decision: decisionBlock, Reason: decision.Reason})
}
