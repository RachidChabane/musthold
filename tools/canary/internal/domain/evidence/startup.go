package evidence

import (
	"encoding/json"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

// Event types that show a model turn began, per harness. A session whose stream
// holds none never reached the model: the CLI refused to start, or the model it
// was asked for is not available, and what the checks would then report says
// nothing about the harness.
const (
	copilotAssistantPrefix = "assistant."
	copilotToolPrefix      = "tool."
	copilotSubagentPrefix  = "subagent."
	codexItemPrefix        = "item."
	// codexItemError is the item that reports a failure of the CLI or the model,
	// which is not a step of the model.
	codexItemError = "error"
)

// StartedATurn reports whether the event stream of a harness shows that a model
// turn began.
func StartedATurn(flag string, stream []Line) bool {
	for _, line := range stream {
		var event struct {
			Type string `json:"type"`
			Item struct {
				Type string `json:"type"`
			} `json:"item"`
		}
		if json.Unmarshal(line.Raw, &event) != nil {
			continue
		}
		if beginsATurn(flag, event.Type, event.Item.Type) {
			return true
		}
	}
	return false
}

func beginsATurn(flag, eventType, itemType string) bool {
	switch flag {
	case harness.FlagClaude:
		return eventType == ClaudeTypeAssistant
	case harness.FlagCodex:
		return strings.HasPrefix(eventType, codexItemPrefix) && itemType != codexItemError || eventType == CodexEventTurnCompleted
	case harness.FlagCopilot:
		return strings.HasPrefix(eventType, copilotAssistantPrefix) || eventType == CopilotEventModelCallStart || strings.HasPrefix(eventType, copilotToolPrefix) || strings.HasPrefix(eventType, copilotSubagentPrefix)
	default:
		return false
	}
}

// ErrorLine picks the line of a CLI's standard error that says why it failed: the
// last one that names an error, or else the last non-blank one. It is empty when
// the CLI printed nothing.
func ErrorLine(stderr string) string {
	var last, lastError string
	for _, line := range strings.Split(stderr, "\n") {
		text := strings.TrimSpace(line)
		if text == "" {
			continue
		}
		last = text
		if strings.Contains(strings.ToLower(text), "error") {
			lastError = text
		}
	}
	if lastError != "" {
		return lastError
	}
	return last
}
