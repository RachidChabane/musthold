package evidence

import (
	"encoding/json"
	"strings"
)

// Event and item types of `codex exec --json` the checks read.
const (
	CodexEventItemCompleted = "item.completed"
	CodexEventItemStarted   = "item.started"
	CodexEventTurnCompleted = "turn.completed"
	CodexEventTurnFailed    = "turn.failed"
	CodexEventError         = "error"

	CodexItemMessage = "agent_message"
	CodexItemCommand = "command_execution"
	CodexItemMCPCall = "mcp_tool_call"
	CodexItemCollab  = "collab_tool_call"

	// CodexStatusFailed is the status of an item that did not succeed.
	CodexStatusFailed = "failed"
	// CodexStatusDeclined is the status of an item the sandbox or the approval
	// policy refused to run.
	CodexStatusDeclined = "declined"
)

// CodexItem is an item of a Codex event. Only the members that are present are set.
type CodexItem struct {
	Line     int
	Event    string
	Type     string
	ID       string
	Status   string
	Server   string
	Tool     string
	Command  string
	ExitCode *int
	Text     string
	Raw      json.RawMessage
}

// Mentions reports whether the item's JSON holds the text anywhere, which is how
// a check finds an agent type in a call whose argument layout the CLI may change.
func (i CodexItem) Mentions(text string) bool { return strings.Contains(string(i.Raw), text) }

// CodexStream is a decoded `codex exec --json` stream.
type CodexStream struct {
	Items []CodexItem
	// Types lists the event types in order, one per line, for counts.
	Types []string
}

type rawCodex struct {
	Type string          `json:"type"`
	Item json.RawMessage `json:"item"`
}

type rawCodexItem struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Status   string `json:"status"`
	Server   string `json:"server"`
	Tool     string `json:"tool"`
	Command  string `json:"command"`
	ExitCode *int   `json:"exit_code"`
	Text     string `json:"text"`
}

// DecodeCodex decodes the stream.
func DecodeCodex(lines []Line) CodexStream {
	var stream CodexStream
	for _, line := range lines {
		var raw rawCodex
		_ = json.Unmarshal(line.Raw, &raw)
		stream.Types = append(stream.Types, raw.Type)
		if len(raw.Item) == 0 {
			continue
		}
		var item rawCodexItem
		_ = json.Unmarshal(raw.Item, &item)
		stream.Items = append(stream.Items, CodexItem{
			Line: line.Number, Event: raw.Type, Type: item.Type, ID: item.ID, Status: item.Status, Server: item.Server,
			Tool: item.Tool, Command: item.Command, ExitCode: item.ExitCode, Text: item.Text, Raw: raw.Item,
		})
	}
	return stream
}

// ItemsOf returns the items of a type.
func (s CodexStream) ItemsOf(itemType string) []CodexItem {
	var found []CodexItem
	for _, item := range s.Items {
		if item.Type == itemType {
			found = append(found, item)
		}
	}
	return found
}
