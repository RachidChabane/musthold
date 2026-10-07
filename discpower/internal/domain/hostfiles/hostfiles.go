// Package hostfiles is the single source of the files the module ships to each
// host harness: the plugin of Claude Code, and the agent, skill and hook files of
// Codex and Copilot. The committed trees under `plugin/` and the files `init`
// writes into a project are both generated here, so one definition of every name,
// matcher and command exists. It knows no I/O.
package hostfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"text/template"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Identity of the shipped plugin.
const (
	// PluginVersion is the version of the shipped files.
	PluginVersion = "0.1.0"
	pluginAuthor  = "Rachid Chabane"
	pluginLicense = "Apache-2.0"
	// Binary is the executable the hooks and the server run.
	Binary = "spec-proof-dp"
	// SkillName is the name of the skill that fulfils the pending requests.
	SkillName = "spec-proof-dp-fulfil"
	// HookCommandMarker starts every hook command the module installs; it is how
	// an installed hook is recognized.
	HookCommandMarker = Binary + " hook "
)

// Limits of the generated hook entries.
const (
	hookTimeoutSeconds = 30
	maxTurns           = 4
	hookTypeCommand    = "command"
	indent             = "  "
)

// Matchers of the generated hook entries.
const (
	launchTool        = hostproto.ToolAgent
	claudeToolMatcher = launchTool + "|mcp__plugin_" + hostharness.PluginName + "_" + hostharness.ExecutorServer + "__.*"
	codexToolMatcher  = "mcp__" + hostharness.StandaloneExecutorServer + "__.*"
)

// File is one generated file: its path relative to the root of its tree and its
// bytes.
type File struct {
	Path    string
	Content []byte
}

// Executor roles of the module's plugin, in the order the files list them.
func agents() []hostproto.Agent { return hostproto.Agents() }

var agentDescriptions = map[hostproto.Agent]string{
	hostproto.AgentGenerator: "Executes one generation ticket of spec-proof-dp as a blind executor. Launch it only through the " + SkillName + " skill, with a prompt that is the ticket.",
	hostproto.AgentJudge:     "Executes one judgment ticket of spec-proof-dp as a blind executor. Launch it only through the " + SkillName + " skill, with a prompt that is the ticket.",
	hostproto.AgentDecider:   "Executes one closed-question ticket of spec-proof-dp as a blind executor. Launch it only through the " + SkillName + " skill, with a prompt that is the ticket.",
}

const executorInstructions = "Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt."

// HookCommand is the command line of a hook of a harness.
func HookCommand(kind hostproto.HookKind, harness hostharness.Harness) string {
	return fmt.Sprintf("%s%s --harness %s", HookCommandMarker, kind, harness.Flag)
}

// ExecutorRoleMatcher matches the types the harness reports for the executor
// agents of the plugin, and no other agent type.
func ExecutorRoleMatcher(harness hostharness.Harness) string {
	types := make([]string, 0, len(agents()))
	for _, agent := range agents() {
		types = append(types, regexp.QuoteMeta(harness.AgentType(hostproto.RoleFor(hostharness.PluginName, agent))))
	}
	return "^(" + strings.Join(types, "|") + ")$"
}

// For returns the files shipped for a harness.
func For(harness hostharness.Harness) ([]File, error) {
	switch harness.Flag {
	case hostharness.FlagClaude:
		return claudePlugin(harness)
	case hostharness.FlagCodex:
		return codexFiles(harness)
	case hostharness.FlagCopilot:
		return copilotFiles(harness)
	default:
		return nil, fmt.Errorf("%w: %q", hostharness.ErrUnknownHarness, harness.Flag)
	}
}

func encodeJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func render(name, text string, data any) ([]byte, error) {
	parsed, err := template.New(name).Parse(text)
	if err != nil {
		return nil, fmt.Errorf("parse the %s template: %w", name, err)
	}
	var buffer bytes.Buffer
	if err := parsed.Execute(&buffer, data); err != nil {
		return nil, fmt.Errorf("render %s: %w", name, err)
	}
	return buffer.Bytes(), nil
}

// hookCommandEntry is a command hook of the layout Claude Code and Codex share.
type hookCommandEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout"`
}

// matcherGroup is a matcher with the hooks it runs.
type matcherGroup struct {
	Matcher string             `json:"matcher,omitempty"`
	Hooks   []hookCommandEntry `json:"hooks"`
}

// snakeHooks is the hooks file of Claude Code and Codex.
type snakeHooks struct {
	Hooks map[string][]matcherGroup `json:"hooks"`
}

func group(matcher string, kind hostproto.HookKind, harness hostharness.Harness) []matcherGroup {
	return []matcherGroup{{Matcher: matcher, Hooks: []hookCommandEntry{{
		Type: hookTypeCommand, Command: HookCommand(kind, harness), Timeout: hookTimeoutSeconds,
	}}}}
}

// Event names of the hooks of Claude Code and Codex.
const (
	eventPreToolUse    = "PreToolUse"
	eventSubagentStart = "SubagentStart"
	eventSubagentStop  = "SubagentStop"
	eventPostToolUse   = "PostToolUse"
)

// skillData fills the skill templates.
type skillData struct {
	Binary     string
	Name       string
	Delegation string
	Allowed    string
}

const claudeDelegation = "launch one foreground `Agent` with `subagent_type` set to the role on its line and `prompt` set to `ticket <ticket>`. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt."

const otherDelegation = "ask the subagent named like the role on its line, with its colon replaced by a dash (`spec-proof-dp:judge` is `spec-proof-dp-judge`), to handle `ticket <ticket>`. Run at most 4 at a time and wait for them. Add nothing to the request."

const skillTemplate = `---
name: {{.Name}}
description: Answer the pending host requests of spec-proof-dp (discriminating power) with blind executor subagents and finish the proof run. Use when ` + "`spec-proof-dp prove`" + ` exits with code 3, when Shallnot reports awaiting_host for spec-proof-dp, or when ` + "`spec-proof-dp request list`" + ` shows tickets.
{{- if .Allowed}}
allowed-tools: {{.Allowed}}
{{- end}}
---

# Fulfil the requests of spec-proof-dp

` + "`{{.Binary}} prove`" + ` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket. A blind executor subagent answers it, and ` + "`{{.Binary}} prove --resume`" + ` consumes the answers.

Never answer a ticket yourself, never read what a ticket asks, and give an executor nothing but its ticket.

1. If ` + "`spec-proof-s1`" + ` is on ` + "`PATH`" + ` and ` + "`.spec-proof/config.yaml`" + ` sets ` + "`systemone.endpoint`" + `, run ` + "`spec-proof-s1 drain --module dp`" + `. A failure leaves the requests for the next step.
2. Run ` + "`{{.Binary}} request list`" + `. Each line is ` + "`<ticket> <role>`" + `. When it prints nothing, run ` + "`{{.Binary}} prove --changed`" + ` once and list again.
3. For each ticket, {{.Delegation}} The reply of an executor is a receipt: do not act on it.
4. Run ` + "`{{.Binary}} prove --resume`" + `. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a ` + "`host_unresponsive`" + ` finding, report it to the user and stop. Do not loop.
`

func skillFile(path string, data skillData) (File, error) {
	content, err := render("skill", skillTemplate, data)
	if err != nil {
		return File{}, err
	}
	return File{Path: path, Content: content}, nil
}

// ---- Claude Code ----

const claudeAgentTemplate = `---
name: {{.Agent}}
description: {{.Description}}
tools: {{.Tools}}
model: inherit
omitClaudeMd: true
maxTurns: {{.MaxTurns}}
---
{{.Instructions}}
`

type agentData struct {
	Agent        string
	Description  string
	Tools        string
	MaxTurns     int
	Instructions string
}

type pluginManifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      map[string]string `json:"author"`
	License     string            `json:"license"`
}

type mcpServer struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type mcpFile struct {
	Servers map[string]mcpServer `json:"mcpServers"`
}

func serverArgs(harness hostharness.Harness) []string {
	return []string{"mcp", "--role", "executor", "--harness", harness.Flag}
}

func claudePlugin(harness hostharness.Harness) ([]File, error) {
	manifest, err := encodeJSON(pluginManifest{
		Name: hostharness.PluginName, Version: PluginVersion,
		Description: "Blind executors and hooks that let a host harness answer the model requests of spec-proof-dp, the discriminating-power proof.",
		Author:      map[string]string{"name": pluginAuthor}, License: pluginLicense,
	})
	if err != nil {
		return nil, err
	}
	servers, err := encodeJSON(mcpFile{Servers: map[string]mcpServer{hostharness.ExecutorServer: {Command: Binary, Args: serverArgs(harness)}}})
	if err != nil {
		return nil, err
	}
	hooks, err := encodeJSON(snakeHooks{Hooks: map[string][]matcherGroup{
		eventPreToolUse:    group(claudeToolMatcher, hostproto.HookPreTool, harness),
		eventSubagentStart: group(ExecutorRoleMatcher(harness), hostproto.HookSubagentStart, harness),
		eventSubagentStop:  group(ExecutorRoleMatcher(harness), hostproto.HookSubagentStop, harness),
		eventPostToolUse:   group(launchTool, hostproto.HookPostTool, harness),
	}})
	if err != nil {
		return nil, err
	}
	files := []File{
		{Path: ".claude-plugin/plugin.json", Content: manifest},
		{Path: ".mcp.json", Content: servers},
		{Path: "hooks/hooks.json", Content: hooks},
	}
	for _, agent := range agents() {
		content, err := render("claude-agent", claudeAgentTemplate, agentData{
			Agent: string(agent), Description: agentDescriptions[agent], Tools: strings.Join(harness.ToolNames(), ", "),
			MaxTurns: maxTurns, Instructions: executorInstructions,
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: "agents/" + string(agent) + ".md", Content: content})
	}
	allowed := strings.Join([]string{
		"Bash(" + Binary + " prove *)", "Bash(" + Binary + " request list)", "Bash(spec-proof-s1 drain *)",
	}, ", ")
	skill, err := skillFile("skills/"+SkillName+"/SKILL.md", skillData{Binary: Binary, Name: SkillName, Delegation: claudeDelegation, Allowed: allowed})
	if err != nil {
		return nil, err
	}
	return append(files, skill), nil
}

// ---- Codex ----

const codexAgentTemplate = `name = "{{.Name}}"
description = "{{.Description}}"
developer_instructions = "{{.Instructions}}"
sandbox_mode = "read-only"

[mcp_servers.{{.Server}}]
command = "{{.Command}}"
args = [{{.Args}}]
`

type codexAgentData struct {
	Name         string
	Description  string
	Instructions string
	Server       string
	Command      string
	Args         string
}

// agentName is the name an agent goes by on a harness without plugin scope.
func agentName(agent hostproto.Agent) string { return hostharness.PluginName + "-" + string(agent) }

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}

func codexFiles(harness hostharness.Harness) ([]File, error) {
	var files []File
	for _, agent := range agents() {
		content, err := render("codex-agent", codexAgentTemplate, codexAgentData{
			Name: agentName(agent), Description: agentDescriptions[agent], Instructions: executorInstructions,
			Server: harness.Server(), Command: Binary, Args: quotedList(serverArgs(harness)),
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: ".codex/agents/" + agentName(agent) + ".toml", Content: content})
	}
	skill, err := skillFile(".agents/skills/"+SkillName+"/SKILL.md", skillData{Binary: Binary, Name: SkillName, Delegation: otherDelegation})
	if err != nil {
		return nil, err
	}
	hooks, err := encodeJSON(snakeHooks{Hooks: map[string][]matcherGroup{
		eventPreToolUse:    group(codexToolMatcher, hostproto.HookPreTool, harness),
		eventSubagentStart: group(ExecutorRoleMatcher(harness), hostproto.HookSubagentStart, harness),
		eventSubagentStop:  group(ExecutorRoleMatcher(harness), hostproto.HookSubagentStop, harness),
	}})
	if err != nil {
		return nil, err
	}
	return append(files, skill, File{Path: ".codex/hooks.json", Content: hooks}), nil
}

// ---- Copilot ----

const copilotAgentTemplate = `---
name: {{.Name}}
description: {{.Description}}
tools: [{{.Tools}}]
mcp-servers:
  {{.Server}}:
    type: local
    command: {{.Command}}
    args: [{{.Args}}]
    tools: [{{.ServerTools}}]
---
{{.Instructions}}
`

type copilotAgentData struct {
	Name         string
	Description  string
	Tools        string
	Server       string
	Command      string
	Args         string
	ServerTools  string
	Instructions string
}

// copilotHook is a command hook in the layout of Copilot CLI.
type copilotHook struct {
	Type       string `json:"type"`
	Bash       string `json:"bash"`
	PowerShell string `json:"powershell"`
	TimeoutSec int    `json:"timeoutSec"`
}

type copilotHooks struct {
	Version int                      `json:"version"`
	Hooks   map[string][]copilotHook `json:"hooks"`
}

const copilotHooksVersion = 1

func copilotEntry(kind hostproto.HookKind, harness hostharness.Harness) []copilotHook {
	command := HookCommand(kind, harness)
	return []copilotHook{{Type: hookTypeCommand, Bash: command, PowerShell: command, TimeoutSec: hookTimeoutSeconds}}
}

func copilotFiles(harness hostharness.Harness) ([]File, error) {
	var files []File
	for _, agent := range agents() {
		content, err := render("copilot-agent", copilotAgentTemplate, copilotAgentData{
			Name: agentName(agent), Description: agentDescriptions[agent], Instructions: executorInstructions,
			Tools:  quotedList(harness.ToolNames()),
			Server: harness.Server(), Command: Binary, Args: quotedList(serverArgs(harness)),
			ServerTools: quotedList([]string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse}),
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: ".github/agents/" + agentName(agent) + ".agent.md", Content: content})
	}
	skill, err := skillFile(".github/skills/"+SkillName+"/SKILL.md", skillData{Binary: Binary, Name: SkillName, Delegation: otherDelegation})
	if err != nil {
		return nil, err
	}
	hooks, err := encodeJSON(copilotHooks{Version: copilotHooksVersion, Hooks: map[string][]copilotHook{
		"preToolUse":   copilotEntry(hostproto.HookPreTool, harness),
		"subagentStop": copilotEntry(hostproto.HookSubagentStop, harness),
	}})
	if err != nil {
		return nil, err
	}
	return append(files, skill, File{Path: ".github/hooks/" + hostharness.PluginName + ".json", Content: hooks}), nil
}
