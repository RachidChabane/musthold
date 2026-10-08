package evidence

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

// The hook log is written by the recording shim that stands in for the module's
// binary on the PATH of the session, and by the probe hooks the canary installs.
// It is the ground truth of what the harness sent to a hook and what the hook
// answered, which no event stream shows.

// Labels of probe records, which are not module commands.
const (
	LabelStopProbe = "Stop-probe"
	// LabelPreToolProbe labels the probe that records every PreToolUse payload.
	LabelPreToolProbe = "PreToolUse-probe"
	// LabelSessionStartProbe and LabelPromptProbe label the probes of the events a
	// session fires before its first tool call; they show that the project's hook
	// settings were loaded, which a silent Stop probe could not tell.
	LabelSessionStartProbe = "SessionStart-probe"
	LabelPromptProbe       = "UserPromptSubmit-probe"
	hookCommand            = "hook"
)

// HookCall is one record of the hook log.
type HookCall struct {
	Line int `json:"-"`
	// Module is the binary the harness ran: spec-proof-dp, for example. It is
	// empty for a probe.
	Module string `json:"module,omitempty"`
	// Label names a probe record.
	Label string   `json:"label,omitempty"`
	Args  []string `json:"args,omitempty"`
	// Stdin is the payload the harness sent, as it sent it.
	Stdin json.RawMessage `json:"stdin,omitempty"`
	// Stdout is the decision the hook answered.
	Stdout   string `json:"stdout,omitempty"`
	ExitCode int    `json:"exitCode"`
}

// HookPayload is the members of a hook payload the checks read. Claude Code and
// Codex spell them alike.
type HookPayload struct {
	EventName           string          `json:"hook_event_name"`
	SessionID           string          `json:"session_id"`
	AgentID             string          `json:"agent_id"`
	AgentType           string          `json:"agent_type"`
	ToolName            string          `json:"tool_name"`
	ToolInput           json.RawMessage `json:"tool_input"`
	AgentTranscriptPath string          `json:"agent_transcript_path"`
	PermissionMode      string          `json:"permission_mode"`
}

// Decision is the members of a PreToolUse answer the checks read.
type Decision struct {
	Permission   string          `json:"permissionDecision"`
	Reason       string          `json:"permissionDecisionReason"`
	UpdatedInput json.RawMessage `json:"updatedInput"`
}

func decodeHooks(data []byte) ([]HookCall, error) {
	lines, err := Lines(HooksFile, data)
	if err != nil {
		return nil, err
	}
	calls := make([]HookCall, 0, len(lines))
	for _, line := range lines {
		var call HookCall
		if err := json.Unmarshal(line.Raw, &call); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", HooksFile, line.Number, err)
		}
		call.Line = line.Number
		calls = append(calls, call)
	}
	return calls, nil
}

// Kind is the hook a module command serves (`pre-tool`, `subagent-stop`), or the
// label of a probe.
func (c HookCall) Kind() string {
	if c.Label != "" {
		return c.Label
	}
	if len(c.Args) >= 2 && c.Args[0] == hookCommand {
		return c.Args[1]
	}
	return ""
}

// IsModuleHook reports whether the record is a module's hook command.
func (c HookCall) IsModuleHook() bool {
	return c.Label == "" && len(c.Args) >= 2 && c.Args[0] == hookCommand
}

// Payload decodes what the harness sent; the zero payload when it is not JSON.
func (c HookCall) Payload() HookPayload {
	var payload HookPayload
	_ = json.Unmarshal(c.Stdin, &payload)
	return payload
}

// PayloadKeys lists, sorted, the top-level members of the payload the harness
// sent; empty when it is not a JSON object.
func (c HookCall) PayloadKeys() []string {
	var members map[string]json.RawMessage
	if json.Unmarshal(c.Stdin, &members) != nil {
		return nil
	}
	keys := make([]string, 0, len(members))
	for key := range members {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// Decision decodes what the hook answered on standard output. The answer of a
// hook with no opinion decodes to the zero decision.
func (c HookCall) Decision() Decision {
	var wrapped struct {
		Output Decision `json:"hookSpecificOutput"`
	}
	_ = json.Unmarshal([]byte(strings.TrimSpace(c.Stdout)), &wrapped)
	return wrapped.Output
}

// HookCalls returns the module hook records of a module binary and a hook kind.
func (e Evidence) HookCalls(module, kind string) []HookCall {
	var found []HookCall
	for _, call := range e.Hooks {
		if call.IsModuleHook() && call.Module == module && call.Kind() == kind {
			found = append(found, call)
		}
	}
	return found
}

// Probes returns the probe records with a label.
func (e Evidence) Probes(label string) []HookCall {
	var found []HookCall
	for _, call := range e.Hooks {
		if call.Label == label {
			found = append(found, call)
		}
	}
	return found
}
