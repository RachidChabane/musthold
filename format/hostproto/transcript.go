package hostproto

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
)

// Transcript decoding limits and block types.
const (
	// MaxTranscriptLineBytes bounds one line of a transcript.
	MaxTranscriptLineBytes = 4 << 20
	transcriptAssistant    = "assistant"
	transcriptToolUse      = "tool_use"
)

// Errors of the transcript checks.
var (
	// ErrBadTranscript is returned for a transcript line that is not JSON.
	ErrBadTranscript = errors.New("bad transcript")
	// ErrTranscriptDeviation is returned when the executor did more than fetch and submit.
	ErrTranscriptDeviation = errors.New("the transcript deviates from fetch and submit")
)

// ToolCall is one tool call of a subagent transcript.
type ToolCall struct {
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// DecodeTranscript reads the tool calls out of a JSONL transcript: the `tool_use`
// blocks of the assistant messages, in order. Lines of other types are skipped.
func DecodeTranscript(reader io.Reader) ([]ToolCall, error) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(nil, MaxTranscriptLineBytes)
	var calls []ToolCall
	for line := 1; scanner.Scan(); line++ {
		text := bytes.TrimSpace(scanner.Bytes())
		if len(text) == 0 {
			continue
		}
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal(text, &entry); err != nil {
			return nil, fmt.Errorf("%w: line %d: %v", ErrBadTranscript, line, err)
		}
		if entry.Type == transcriptAssistant {
			calls = append(calls, toolUses(entry.Message.Content)...)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadTranscript, err)
	}
	return calls, nil
}

func toolUses(content json.RawMessage) []ToolCall {
	var blocks []struct {
		Type  string          `json:"type"`
		Name  string          `json:"name"`
		Input json.RawMessage `json:"input"`
	}
	if err := json.Unmarshal(content, &blocks); err != nil {
		return nil
	}
	var calls []ToolCall
	for _, block := range blocks {
		if block.Type == transcriptToolUse {
			calls = append(calls, ToolCall{Name: block.Name, Input: block.Input})
		}
	}
	return calls
}

// localToolName strips the `mcp__<server>__` prefix of a tool name.
func localToolName(name string) string {
	if tool, ok := ParseMCPToolName(name); ok {
		return tool.Tool
	}
	return name
}

func callsTicket(call ToolCall) string {
	var input struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(call.Input, &input); err != nil {
		return ""
	}
	return input.Ticket
}

// CheckExecutorTranscript verifies that the executor's tool calls are exactly
// `fetch_request(t)`, `submit_response(t)` and at most one corrective
// `submit_response(t)` for its own ticket.
func CheckExecutorTranscript(calls []ToolCall, ticket TicketID) error {
	expected := []string{ToolFetchRequest, ToolSubmitResponse}
	if len(calls) < len(expected) || len(calls) > len(expected)+MaxCorrections {
		return fmt.Errorf("%w: %d tool calls", ErrTranscriptDeviation, len(calls))
	}
	for index, call := range calls {
		want := ToolSubmitResponse
		if index < len(expected) {
			want = expected[index]
		}
		if localToolName(call.Name) != want {
			return fmt.Errorf("%w: call %d is %q, want %q", ErrTranscriptDeviation, index, call.Name, want)
		}
		if callsTicket(call) != ticket.String() {
			return fmt.Errorf("%w: call %d names another ticket", ErrTranscriptDeviation, index)
		}
	}
	return nil
}

// TranscriptDigest is the digest recorded as an executor's transcript attestation.
func TranscriptDigest(transcript []byte) string { return format.SHA256Hex(transcript) }

// IsExecutorToolOf reports whether an MCP tool name is a tool of an executor
// server of the plugin.
func IsExecutorToolOf(toolName, plugin string) bool {
	tool, ok := ParseMCPToolName(toolName)
	if !ok {
		return false
	}
	owner, _, ok := ParsePluginServer(tool.Server)
	return ok && strings.EqualFold(owner, plugin)
}
