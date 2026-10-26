package hostproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Event types of the session log GitHub Copilot CLI writes for a session, one JSON
// object per line.
const (
	logEventSessionStart  = "session.start"
	logEventSelected      = "subagent.selected"
	logEventUserMessage   = "user.message"
	logEventSystemMessage = "system.message"
	logEventToolStart     = "tool.execution_start"
)

// CopilotIntentTool is the tool the Copilot CLI calls to narrate its work. It
// reads and runs nothing, so it is not an action of the agent that calls it.
const CopilotIntentTool = "report_intent"

// SystemBlockSeparator ends the leading block of a system message: the harness
// appends its own sections to the body of an agent's profile after a blank line.
const SystemBlockSeparator = "\n\n"

// ErrNoAgentID is returned when the events of one agent are asked for with no agent.
var ErrNoAgentID = errors.New("an agent id is required to read the events of one agent")

// SessionRun is what a harness's record of a session shows of one executor run:
// the version of the CLI, the tool selections the harness made for the agent, the
// messages the agent was given and the tools it started, in order.
type SessionRun struct {
	// CLIVersion is the version of the CLI that wrote the record, empty when the
	// record does not say.
	CLIVersion string
	// Selected holds the tools of every selection event of the agent, one list per event.
	Selected [][]string
	// SystemMessages are the system messages of the agent.
	SystemMessages []string
	// UserMessages are the user messages of the agent, as the agent was given them.
	UserMessages []string
	// Calls are the tools the agent started, in order.
	Calls []ToolCall
}

// RunOfCalls is the run a transcript that records tool calls alone shows.
func RunOfCalls(calls []ToolCall) SessionRun { return SessionRun{Calls: calls} }

// logEntry is the envelope of one line of the session log.
type logEntry struct {
	Type    string `json:"type"`
	AgentID string `json:"agentId"`
	Data    struct {
		CopilotVersion string          `json:"copilotVersion"`
		Tools          *[]string       `json:"tools"`
		Content        string          `json:"content"`
		ToolName       string          `json:"toolName"`
		Arguments      json.RawMessage `json:"arguments"`
	} `json:"data"`
}

// DecodeCopilotSessionLog reads the run of one agent out of the JSONL session log
// of Copilot CLI. The version comes from the `session.start` event, which belongs
// to no agent; every other fact is read from the events that carry the agent's
// `agentId`. A log that the harness is still writing may end with an incomplete
// line, which is ignored; an unreadable line anywhere else is an error.
func DecodeCopilotSessionLog(reader io.Reader, agentID string) (SessionRun, error) {
	if agentID == "" {
		return SessionRun{}, ErrNoAgentID
	}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, MaxTranscriptLineBytes)
	var run SessionRun
	var unreadable error
	for line := 1; scanner.Scan(); line++ {
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		if unreadable != nil {
			return SessionRun{}, unreadable
		}
		var entry logEntry
		if err := json.Unmarshal(text, &entry); err != nil {
			unreadable = fmt.Errorf("%w: line %d: %v", ErrBadTranscript, line, err)
			continue
		}
		run.absorb(entry, agentID)
	}
	if err := scanner.Err(); err != nil {
		return SessionRun{}, fmt.Errorf("%w: %v", ErrBadTranscript, err)
	}
	return run, nil
}

func (r *SessionRun) absorb(entry logEntry, agentID string) {
	if entry.Type == logEventSessionStart && entry.AgentID == "" && r.CLIVersion == "" {
		r.CLIVersion = entry.Data.CopilotVersion
		return
	}
	if entry.AgentID != agentID {
		return
	}
	switch entry.Type {
	case logEventSelected:
		tools := []string{}
		if entry.Data.Tools != nil {
			tools = *entry.Data.Tools
		}
		r.Selected = append(r.Selected, tools)
	case logEventSystemMessage:
		r.SystemMessages = append(r.SystemMessages, entry.Data.Content)
	case logEventUserMessage:
		r.UserMessages = append(r.UserMessages, entry.Data.Content)
	case logEventToolStart:
		if entry.Data.ToolName != CopilotIntentTool {
			r.Calls = append(r.Calls, ToolCall{Name: entry.Data.ToolName, Input: entry.Data.Arguments})
		}
	}
}

// SubmittedOutput is the `output` member of a `submit_response` call. Copilot may
// hand the output as a string that holds the JSON document.
func SubmittedOutput(call ToolCall) (json.RawMessage, bool) {
	var members struct {
		Output json.RawMessage `json:"output"`
	}
	if err := json.Unmarshal(call.Input, &members); err != nil || len(members.Output) == 0 {
		return nil, false
	}
	return unwrapToolArgs(members.Output), true
}
