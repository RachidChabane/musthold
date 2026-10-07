package launch

import (
	"encoding/json"
	"fmt"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

// Subcommands of the canary binary that the hooks and shims it installs call.
const (
	ProbeSubcommand = "probe"
	ShimSubcommand  = "shim"
	// FlagRecord names the file a probe or a shim appends its record to.
	FlagRecord = "--record"
	FlagLabel  = "--label"
	FlagReal   = "--real"
	// ClaudeSettingsPath is the project-local settings file of Claude Code.
	ClaudeSettingsPath = ".claude/settings.json"
	// CodexHooksPath is the project-local hooks file of Codex.
	CodexHooksPath = ".codex/hooks.json"

	hookTimeoutSeconds = 30
	hookTypeCommand    = "command"
	eventStop          = "Stop"
	eventPreToolUse    = "PreToolUse"
	matchEveryTool     = ".*"
)

// probeCommand is the hook command that records its payload and answers nothing.
func probeCommand(canary, hooksLog, label string) string {
	return fmt.Sprintf("%s %s %s %s %s %s", Quote(canary), ProbeSubcommand, FlagRecord, Quote(hooksLog), FlagLabel, Quote(label))
}

type hookEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

type hookGroup struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookEntry `json:"hooks"`
}

func probeGroup(canary, hooksLog, label, matcher string) hookGroup {
	return hookGroup{Matcher: matcher, Hooks: []hookEntry{{Type: hookTypeCommand, Command: probeCommand(canary, hooksLog, label), Timeout: hookTimeoutSeconds}}}
}

// ClaudeSettings is the project-local settings of a Claude Code session: the Stop
// probe, which no module installs, and nothing else. The session loads no other
// settings source.
func ClaudeSettings(canary, hooksLog string) ([]byte, error) {
	settings := map[string]any{
		"hooks": map[string][]hookGroup{
			eventStop: {probeGroup(canary, hooksLog, evidence.LabelStopProbe, "")},
		},
	}
	return json.MarshalIndent(settings, "", "  ")
}

// MergeCodexProbes adds the Stop probe and the probe that records every PreToolUse
// payload to a Codex hooks file, keeping every entry the file already holds.
func MergeCodexProbes(existing []byte, canary, hooksLog string) ([]byte, error) {
	document := map[string]json.RawMessage{}
	if len(existing) > 0 {
		if err := json.Unmarshal(existing, &document); err != nil {
			return nil, fmt.Errorf("the generated hooks file is not JSON: %w", err)
		}
	}
	events := map[string][]json.RawMessage{}
	if raw, present := document["hooks"]; present {
		if err := json.Unmarshal(raw, &events); err != nil {
			return nil, fmt.Errorf("the generated hooks file has no hooks table: %w", err)
		}
	}
	for event, group := range map[string]hookGroup{
		eventStop:       probeGroup(canary, hooksLog, evidence.LabelStopProbe, ""),
		eventPreToolUse: probeGroup(canary, hooksLog, evidence.LabelPreToolProbe, matchEveryTool),
	} {
		encoded, err := json.Marshal(group)
		if err != nil {
			return nil, err
		}
		events[event] = append(events[event], encoded)
	}
	table, err := json.Marshal(events)
	if err != nil {
		return nil, err
	}
	document["hooks"] = table
	return json.MarshalIndent(document, "", "  ")
}
