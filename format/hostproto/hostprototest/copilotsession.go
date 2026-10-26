package hostprototest

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

// Event types and members of the session log, as Copilot CLI writes them.
const (
	sessionStartType    = "session.start"
	subagentStartedType = "subagent.started"
	subagentSelectType  = "subagent.selected"
	userMessageType     = "user.message"
	systemMessageType   = "system.message"
	toolStartType       = "tool.execution_start"
	intentToolSpelling  = hostproto.CopilotIntentTool
	// copilotProfileSections is what the CLI appends, after a blank line, to the
	// body of an agent's profile to make its system message.
	copilotProfileSections = "<prohibited_actions>\nThings you *must not* do.\n</prohibited_actions>\n\n" +
		"<environment_context>\nCurrent working directory: /work/project\n</environment_context>"
	mainAgentLogID = ""
)

// CopilotSession is the session log of one Copilot CLI executor run, as the
// fake hosts write it for the stop hook to read.
type CopilotSession struct {
	// AgentID is the agent whose events the log holds besides the main agent's.
	AgentID string
	// CLIVersion is the version `session.start` reports.
	CLIVersion string
	// Selected holds the tool list of every `subagent.selected` event.
	Selected [][]string
	// SystemMessages and UserMessages are the messages the agent was given.
	SystemMessages []string
	UserMessages   []string
	// Calls are the tools the agent started, spelled as the log spells them.
	Calls []hostproto.ToolCall
}

// copilotHarness is the Copilot strategy of the fixture module.
func copilotHarness() harness.Harness { return harness.Copilot(Identity) }

// CopilotObserved is what a Copilot CLI session with the plugin, its server and its
// hooks shows: an allowlist that is written but not verified.
func CopilotObserved() hostproto.Observed {
	return hostproto.Observed{PluginPresent: true, ServerPresent: true, HooksPresent: true, Allowlist: hostproto.AllowlistUnverified}
}

// CopilotAgentType is the agent type Copilot CLI reports for the executor agent of
// a role: the plugin, a colon and the name of the agent file.
func CopilotAgentType(role string) string {
	copilot := copilotHarness()
	plugin, agent, err := hostproto.ParseRole(role)
	if err != nil {
		return ""
	}
	return plugin + hostproto.RoleSeparator + copilot.AgentTypeOf(agent)
}

// HonestCopilotSession is the session log of an executor that was launched with the
// bare ticket, holds exactly the two executor tools on the pinned CLI, fetched,
// submitted `output` once (or `submits` times) and replied with the receipt.
func HonestCopilotSession(agentID string, ticket hostproto.TicketID, submits int, output string) CopilotSession {
	copilot := copilotHarness()
	input, _ := json.Marshal(map[string]string{"ticket": ticket.String(), "cap": ""})
	calls := []hostproto.ToolCall{
		{Name: copilotLogToolName(copilot, hostproto.ToolFetchRequest), Input: input},
	}
	submit, _ := json.Marshal(map[string]any{"ticket": ticket.String(), "cap": "", "output": json.RawMessage(output)})
	for range submits {
		calls = append(calls, hostproto.ToolCall{Name: copilotLogToolName(copilot, hostproto.ToolSubmitResponse), Input: submit})
	}
	return CopilotSession{
		AgentID: agentID, CLIVersion: copilot.Version,
		Selected:       [][]string{copilot.ToolNames()},
		SystemMessages: []string{hostproto.ExecutorInstructions + hostproto.SystemBlockSeparator + copilotProfileSections},
		UserMessages:   []string{hostproto.ExecutorPrompt(ticket)},
		Calls:          calls,
	}
}

// copilotLogToolName is how the session log spells an executor tool: the server and
// the tool joined by a hyphen.
func copilotLogToolName(copilot harness.Harness, tool string) string {
	return copilot.Server() + "-" + tool
}

// Log renders the session as the JSONL the CLI writes: the main agent's events
// first, then the agent's.
func (s CopilotSession) Log() []byte {
	var out bytes.Buffer
	write := func(agentID, eventType string, data map[string]any) {
		entry := map[string]any{"type": eventType, "data": data, "agentId": nil}
		if agentID != mainAgentLogID {
			entry["agentId"] = agentID
		}
		line, err := json.Marshal(entry)
		if err != nil {
			panic(fmt.Sprintf("render the session log: %v", err))
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	write(mainAgentLogID, sessionStartType, map[string]any{"copilotVersion": s.CLIVersion})
	write(mainAgentLogID, userMessageType, map[string]any{"content": "run the proof"})
	write(mainAgentLogID, systemMessageType, map[string]any{"content": "main agent profile"})
	write(s.AgentID, subagentStartedType, map[string]any{"agentName": "executor"})
	for _, tools := range s.Selected {
		write(s.AgentID, subagentSelectType, map[string]any{"tools": tools})
	}
	for _, content := range s.UserMessages {
		write(s.AgentID, userMessageType, map[string]any{"content": content})
	}
	for _, content := range s.SystemMessages {
		write(s.AgentID, systemMessageType, map[string]any{"content": content})
	}
	write(s.AgentID, toolStartType, map[string]any{"toolName": intentToolSpelling, "arguments": map[string]string{"intent": "fetch"}})
	for _, call := range s.Calls {
		write(s.AgentID, toolStartType, map[string]any{"toolName": call.Name, "arguments": call.Input})
	}
	return out.Bytes()
}
