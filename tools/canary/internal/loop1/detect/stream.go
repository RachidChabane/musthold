package detect

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
)

// Tool names of the session's stream.
const (
	ToolEdit         = "Edit"
	ToolWrite        = "Write"
	ToolMultiEdit    = "MultiEdit"
	ToolNotebookEdit = "NotebookEdit"
	ToolBash         = "Bash"
	ToolWebFetch     = "WebFetch"
	ToolWebSearch    = "WebSearch"

	eventAssistant = "assistant"
	blockToolUse   = "tool_use"
	// maxStreamLine bounds one stream line, which can hold a whole file.
	maxStreamLine = 64 << 20
)

// ToolCall is one tool_use block of the stream.
type ToolCall struct {
	// Event is the line of the stream, counted from 1.
	Event int
	Name  string
	// Main is set when the call comes from the main agent: its event has no
	// parent_tool_use_id.
	Main bool
	// Path is the file the call edits; Command is the Bash command.
	Path    string
	Command string
}

type streamEvent struct {
	Type            string  `json:"type"`
	ParentToolUseID *string `json:"parent_tool_use_id"`
	Message         struct {
		Content []contentBlock `json:"content"`
	} `json:"message"`
}

type contentBlock struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Input struct {
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Command      string `json:"command"`
	} `json:"input"`
}

// ParseToolCalls lists the tool calls of a stream in order. A line that is not a
// JSON event, such as the partial last line of a session cut at a cap, holds no
// call.
func ParseToolCalls(stream []byte) ([]ToolCall, error) {
	scanner := bufio.NewScanner(bytes.NewReader(stream))
	scanner.Buffer(nil, maxStreamLine)
	var calls []ToolCall
	for line := 1; scanner.Scan(); line++ {
		var event streamEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != eventAssistant {
			continue
		}
		main := event.ParentToolUseID == nil || *event.ParentToolUseID == ""
		for _, block := range event.Message.Content {
			if block.Type != blockToolUse {
				continue
			}
			calls = append(calls, ToolCall{
				Event: line, Name: block.Name, Main: main,
				Path: editedPath(block), Command: block.Input.Command,
			})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read stream: %w", err)
	}
	return calls, nil
}

func editedPath(block contentBlock) string {
	if block.Input.NotebookPath != "" {
		return block.Input.NotebookPath
	}
	return block.Input.FilePath
}
