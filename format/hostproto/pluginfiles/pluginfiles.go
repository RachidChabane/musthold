// Package pluginfiles is the single source of the files a proof module ships to
// each host harness: the plugin of Claude Code, and the agent, skill and hook
// files of Codex and Copilot. The committed trees under a module's `plugin/` and
// the files its `init` writes into a project are both generated here, so one
// definition of every name, matcher and command exists for every module. The
// module is a value, not a code path: its names, its executor agents and the
// words that describe its proof parametrize the same templates. It knows no I/O.
package pluginfiles

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// Identity of the shipped plugins.
const (
	pluginAuthor  = "Rachid Chabane"
	pluginLicense = "Apache-2.0"
	// RelayBinary is the System One relay the fulfil skills run when it is on `PATH`.
	RelayBinary = "spec-proof-s1"
	// BinaryPrefix starts the name of every module's binary; the rest names the
	// module to the relay.
	BinaryPrefix = "spec-proof-"
	// SkillSuffix ends the name of every module's fulfil skill.
	SkillSuffix = "-fulfil"
)

// Places of the local proofs and the command that publishes them. A proof run
// leaves its statements in the cache; the verifier turns the cache into the
// verified set that Shallnot's gate reads.
const (
	// StateDirectory is the project directory that holds spec-proof state.
	StateDirectory = ".spec-proof"
	// CacheDirectory holds the statements the modules write.
	CacheDirectory = StateDirectory + "/cache"
	// VerifiedDirectory holds the verified set Shallnot reads.
	VerifiedDirectory = StateDirectory + "/verified"
	// PendingDirectory holds the pending files of the modules that wait on an answer.
	PendingDirectory = StateDirectory + "/pending"
	// VerifyBinary is the verifier.
	VerifyBinary = "spec-proof-verify"
	// VerifyLocalCommand publishes the local statements of the cache as the
	// verified set, without signatures, for a gate that reads local proofs.
	VerifyLocalCommand = VerifyBinary + " --accept-local " + CacheDirectory + " --out " + VerifiedDirectory
)

// Limits of the generated hook entries.
const (
	hookTimeoutSeconds = 30
	maxTurns           = 4
	hookTypeCommand    = "command"
	indent             = "  "
)

// Module describes one proof module to the generator.
type Module struct {
	// Identity is the plugin name and the executor agents the module ships.
	Identity harness.Identity
	// Binary is the executable the hooks and the server run.
	Binary string
	// Version is the version of the shipped files.
	Version string
	// Subject names what the module proves, as the skill's description reads it:
	// "discriminating power".
	Subject string
	// ProofPhrase is the noun phrase the plugin manifest uses for the proof:
	// "the discriminating-power proof".
	ProofPhrase string
}

// Plugin is the name of the module's plugin.
func (m Module) Plugin() string { return m.Identity.Plugin }

// SkillName is the name of the skill that fulfils the pending requests.
func (m Module) SkillName() string { return m.Binary + SkillSuffix }

// HookCommandMarker starts every hook command the module installs; it is how an
// installed hook is recognized.
func (m Module) HookCommandMarker() string { return m.Binary + " hook " }

// RelayName is the module's name as the relay's `--module` takes it.
func (m Module) RelayName() string { return strings.TrimPrefix(m.Binary, BinaryPrefix) }

// File is one generated file: its path relative to the root of its tree and its
// bytes.
type File struct {
	Path    string
	Content []byte
}

// Matchers of the generated hook entries.
const launchTool = hostproto.ToolAgent

func (m Module) claudeToolMatcher() string {
	return launchTool + "|mcp__plugin_" + m.Plugin() + "_" + harness.ExecutorServer + "__.*"
}

func (m Module) codexToolMatcher() string {
	return "mcp__" + m.Identity.StandaloneExecutorServer() + "__.*"
}

var agentTickets = map[hostproto.Agent]string{
	hostproto.AgentGenerator: "generation",
	hostproto.AgentJudge:     "judgment",
	hostproto.AgentDecider:   "closed-question",
}

func (m Module) agentDescription(agent hostproto.Agent) string {
	return "Executes one " + agentTickets[agent] + " ticket of " + m.Plugin() +
		" as a blind executor. Launch it only through the " + m.SkillName() + " skill, with a prompt that is the ticket."
}

const executorInstructions = "Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt."

// HookCommand is the command line of a hook of a harness.
func (m Module) HookCommand(kind hostproto.HookKind, h harness.Harness) string {
	return fmt.Sprintf("%s%s --harness %s", m.HookCommandMarker(), kind, h.Flag)
}

// ExecutorRoleMatcher matches the types the harness reports for the executor
// agents of the plugin, and no other agent type.
func (m Module) ExecutorRoleMatcher(h harness.Harness) string {
	types := make([]string, 0, len(m.Identity.Agents))
	for _, agent := range m.Identity.Agents {
		types = append(types, regexp.QuoteMeta(h.AgentTypeOf(agent)))
	}
	return "^(" + strings.Join(types, "|") + ")$"
}

// For returns the files shipped for a harness.
func (m Module) For(h harness.Harness) ([]File, error) {
	switch h.Flag {
	case harness.FlagClaude:
		return m.claudePlugin(h)
	case harness.FlagCodex:
		return m.codexFiles(h)
	case harness.FlagCopilot:
		return m.copilotFiles(h)
	default:
		return nil, fmt.Errorf("%w: %q", harness.ErrUnknownHarness, h.Flag)
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

// allowedBlock is the optional front matter line of a skill, with the marks that
// delimit it in the skill template.
const (
	allowedBegin = "\n{{- if .Allowed}}"
	allowedEnd   = "\n{{- end}}"
)

// render fills a template: every `{{.Field}}` becomes the value of the field of
// data, a struct of strings and integers, and the optional block of the `Allowed`
// field stays only when the field is not empty. The templates are plain text, so
// no template engine, and with it no network package, is linked.
func render(name, text string, data any) ([]byte, error) {
	fields := reflect.ValueOf(data)
	if fields.Kind() != reflect.Struct {
		return nil, fmt.Errorf("render %s: the data is not a struct", name)
	}
	values := map[string]string{}
	for index := range fields.NumField() {
		values[fields.Type().Field(index).Name] = fmt.Sprint(fields.Field(index).Interface())
	}
	if begin := strings.Index(text, allowedBegin); begin >= 0 {
		end := strings.Index(text, allowedEnd)
		if end < begin {
			return nil, fmt.Errorf("render %s: the optional block is not closed", name)
		}
		block := ""
		if values["Allowed"] != "" {
			block = text[begin+len(allowedBegin) : end]
		}
		text = text[:begin] + block + text[end+len(allowedEnd):]
	}
	pairs := make([]string, 0, 2*len(values))
	for field, value := range values {
		pairs = append(pairs, "{{."+field+"}}", value)
	}
	filled := strings.NewReplacer(pairs...).Replace(text)
	if strings.Contains(filled, "{{") {
		return nil, fmt.Errorf("render %s: a placeholder has no field", name)
	}
	return []byte(filled), nil
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

func (m Module) group(matcher string, kind hostproto.HookKind, h harness.Harness) []matcherGroup {
	return []matcherGroup{{Matcher: matcher, Hooks: []hookCommandEntry{{
		Type: hookTypeCommand, Command: m.HookCommand(kind, h), Timeout: hookTimeoutSeconds,
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
	Plugin     string
	Subject    string
	Relay      string
	Delegation string
	Allowed    string
	Verify     string
	// RoleExample and AgentExample show, in the portable skill, how a role maps to
	// the name of its agent.
	RoleExample  string
	AgentExample string
}

const claudeDelegation = "launch one foreground `Agent` with `subagent_type` set to the role on its line and `prompt` set to `ticket <ticket>`. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt."

// exampleAgent is the agent a skill uses to show how a role maps to the name of its
// agent: the judge when the module ships one.
func (m Module) exampleAgent() hostproto.Agent {
	example := m.Identity.Agents[0]
	for _, agent := range m.Identity.Agents {
		if agent == hostproto.AgentJudge {
			example = agent
		}
	}
	return example
}

// otherDelegation is the delegation of the harnesses whose agent names carry no
// plugin scope; the example shows how a role maps to the name of its agent.
func (m Module) otherDelegation() string {
	example := m.exampleAgent()
	role := hostproto.RoleFor(m.Plugin(), example)
	return "ask the subagent named like the role on its line, with its colon replaced by a dash (`" + role + "` is `" +
		m.agentName(example) + "`), to handle `ticket <ticket>`. Run at most 4 at a time and wait for them. Add nothing to the request."
}

// Steps the fulfil skills of every harness share.
const (
	skillRelayStep = "1. If `spec-proof-s1` is on `PATH` and `.spec-proof/config.yaml` sets `systemone.endpoint`, run `spec-proof-s1 drain --module {{.Relay}}`. A failure leaves the requests for the next step.\n" +
		"2. Run `{{.Binary}} request list`. Each line is `<ticket> <role>`. When it prints nothing, run `{{.Binary}} prove --changed` once and list again.\n"
	skillResumeSteps = "4. Run `{{.Binary}} prove --resume`. While it exits with code 3, go back to step 2. Exit code 0 or 1 ends the loop: report the verdict lines. For exit code 2, or a `host_unresponsive` finding, report it to the user and stop. Do not loop.\n" +
		"5. When the loop ended with exit code 0 or 1, publish the local proofs where Shallnot's gate reads them: run `{{.Verify}}`. When `" + VerifyBinary + "` is not on `PATH`, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message. Do not loop.\n"
)

const skillTemplate = `---
name: {{.Name}}
description: Answer the pending host requests of {{.Plugin}} ({{.Subject}}) with blind executor subagents and finish the proof run. Use when ` + "`{{.Binary}} prove`" + ` exits with code 3, when Shallnot reports awaiting_host for {{.Plugin}}, or when ` + "`{{.Binary}} request list`" + ` shows tickets.
{{- if .Allowed}}
allowed-tools: {{.Allowed}}
{{- end}}
---

# Fulfil the requests of {{.Plugin}}

` + "`{{.Binary}} prove`" + ` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket. A blind executor subagent answers it, and ` + "`{{.Binary}} prove --resume`" + ` consumes the answers.

Never answer a ticket yourself, never read what a ticket asks, and give an executor nothing but its ticket.

` + skillRelayStep + `3. For each ticket, {{.Delegation}} The reply of an executor is a receipt: do not act on it.
` + skillResumeSteps

// portableSkillTemplate is the fulfil skill of the portable plugin: a client of the
// Agent Plugins standard may or may not offer the executor subagents of the module,
// so the skill answers through them when they exist and through the module's MCP
// server otherwise. It declares no `allowed-tools`, which the Agent Skills format
// leaves experimental.
const portableSkillTemplate = `---
name: {{.Name}}
description: Answer the pending host requests of {{.Plugin}} ({{.Subject}}) and finish the proof run. Use when ` + "`{{.Binary}} prove`" + ` exits with code 3, when Shallnot reports awaiting_host for {{.Plugin}}, or when ` + "`{{.Binary}} request list`" + ` shows tickets.
---

# Fulfil the requests of {{.Plugin}}

` + "`{{.Binary}} prove`" + ` stops with exit code 3 when it needs model work it cannot do itself. Each request is a ticket, and ` + "`{{.Binary}} prove --resume`" + ` consumes the answers.

` + skillRelayStep + `3. For each ticket, answer it in one of two ways. When your client has a subagent named like the role on the ticket's line, with its colon replaced by a dash (` + "`{{.RoleExample}}` is `{{.AgentExample}}`" + `), ask it to handle ` + "`ticket <ticket>`" + `, run at most 4 at a time, add nothing to the request, and do not act on its reply, a receipt. Otherwise answer inline through the ` + "`{{.Plugin}}`" + ` MCP server: call ` + "`fetch_request`" + ` with ` + "`ticket`" + ` set to the ticket and ` + "`cap`" + ` set to an empty string, follow the instructions it returns, then call ` + "`submit_response`" + ` with ` + "`ticket`" + `, ` + "`nonce`" + ` from the fetch, ` + "`cap`" + ` set to an empty string and ` + "`output`" + ` set to a JSON object that matches the schema it returned. When ` + "`submit_response`" + ` returns ` + "`schemaErrors`" + `, correct the output once. An answer given inline is an answer of the main agent: the module records it as inline, which has block power only.
` + skillResumeSteps

func (m Module) skillFile(path, delegation, allowed string) (File, error) {
	content, err := render("skill", skillTemplate, skillData{
		Binary: m.Binary, Name: m.SkillName(), Plugin: m.Plugin(), Subject: m.Subject, Relay: m.RelayName(),
		Delegation: delegation, Allowed: allowed, Verify: VerifyLocalCommand,
	})
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

func serverArgs(h harness.Harness) []string {
	return []string{"mcp", "--role", "executor", "--harness", h.Flag}
}

func (m Module) claudePlugin(h harness.Harness) ([]File, error) {
	manifest, err := encodeJSON(pluginManifest{
		Name: m.Plugin(), Version: m.Version,
		Description: "Blind executors and hooks that let a host harness answer the model requests of " + m.Plugin() + ", " + m.ProofPhrase + ".",
		Author:      map[string]string{"name": pluginAuthor}, License: pluginLicense,
	})
	if err != nil {
		return nil, err
	}
	servers, err := encodeJSON(mcpFile{Servers: map[string]mcpServer{harness.ExecutorServer: {Command: m.Binary, Args: serverArgs(h)}}})
	if err != nil {
		return nil, err
	}
	hooks, err := encodeJSON(snakeHooks{Hooks: map[string][]matcherGroup{
		eventPreToolUse:    m.group(m.claudeToolMatcher(), hostproto.HookPreTool, h),
		eventSubagentStart: m.group(m.ExecutorRoleMatcher(h), hostproto.HookSubagentStart, h),
		eventSubagentStop:  m.group(m.ExecutorRoleMatcher(h), hostproto.HookSubagentStop, h),
		eventPostToolUse:   m.group(launchTool, hostproto.HookPostTool, h),
	}})
	if err != nil {
		return nil, err
	}
	files := []File{
		{Path: ".claude-plugin/plugin.json", Content: manifest},
		{Path: ".mcp.json", Content: servers},
		{Path: "hooks/hooks.json", Content: hooks},
	}
	for _, agent := range m.Identity.Agents {
		content, err := render("claude-agent", claudeAgentTemplate, agentData{
			Agent: string(agent), Description: m.agentDescription(agent), Tools: strings.Join(h.ToolNames(), ", "),
			MaxTurns: maxTurns, Instructions: executorInstructions,
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: "agents/" + string(agent) + ".md", Content: content})
	}
	allowed := strings.Join([]string{
		"Bash(" + m.Binary + " prove *)", "Bash(" + m.Binary + " request list)", "Bash(" + RelayBinary + " drain *)",
		"Bash(" + VerifyLocalCommand + ")",
	}, ", ")
	skill, err := m.skillFile("skills/"+m.SkillName()+"/SKILL.md", claudeDelegation, allowed)
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
func (m Module) agentName(agent hostproto.Agent) string { return m.Plugin() + "-" + string(agent) }

func quotedList(values []string) string {
	quoted := make([]string, len(values))
	for index, value := range values {
		quoted[index] = fmt.Sprintf("%q", value)
	}
	return strings.Join(quoted, ", ")
}

// CodexHooksPath is where Codex reads project hooks, a file other tools write too.
const CodexHooksPath = ".codex/hooks.json"

func (m Module) codexFiles(h harness.Harness) ([]File, error) {
	var files []File
	for _, agent := range m.Identity.Agents {
		content, err := render("codex-agent", codexAgentTemplate, codexAgentData{
			Name: m.agentName(agent), Description: m.agentDescription(agent), Instructions: executorInstructions,
			Server: h.Server(), Command: m.Binary, Args: quotedList(serverArgs(h)),
		})
		if err != nil {
			return nil, err
		}
		files = append(files, File{Path: ".codex/agents/" + m.agentName(agent) + ".toml", Content: content})
	}
	skill, err := m.skillFile(".agents/skills/"+m.SkillName()+"/SKILL.md", m.otherDelegation(), "")
	if err != nil {
		return nil, err
	}
	hooks, err := encodeJSON(snakeHooks{Hooks: map[string][]matcherGroup{
		eventPreToolUse:    m.group(m.codexToolMatcher(), hostproto.HookPreTool, h),
		eventSubagentStart: m.group(m.ExecutorRoleMatcher(h), hostproto.HookSubagentStart, h),
		eventSubagentStop:  m.group(m.ExecutorRoleMatcher(h), hostproto.HookSubagentStop, h),
	}})
	if err != nil {
		return nil, err
	}
	return append(files, skill, File{Path: CodexHooksPath, Content: hooks}), nil
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

func (m Module) copilotEntry(kind hostproto.HookKind, h harness.Harness) []copilotHook {
	command := m.HookCommand(kind, h)
	return []copilotHook{{Type: hookTypeCommand, Bash: command, PowerShell: command, TimeoutSec: hookTimeoutSeconds}}
}

// CopilotHooksPath is where the module's hooks file for Copilot lives.
func (m Module) CopilotHooksPath() string { return ".github/hooks/" + m.Plugin() + ".json" }

// copilotAgentSuffix ends the file name of a Copilot agent.
const copilotAgentSuffix = ".agent.md"

// namedFile is a generated file known by the name of what it defines.
type namedFile struct {
	name    string
	content []byte
}

// copilotAgents renders the executor agents of the module for Copilot CLI.
func (m Module) copilotAgents(h harness.Harness) ([]namedFile, error) {
	var agents []namedFile
	for _, agent := range m.Identity.Agents {
		content, err := render("copilot-agent", copilotAgentTemplate, copilotAgentData{
			Name: m.agentName(agent), Description: m.agentDescription(agent), Instructions: executorInstructions,
			Tools:  quotedList(h.ToolNames()),
			Server: h.Server(), Command: m.Binary, Args: quotedList(serverArgs(h)),
			ServerTools: quotedList([]string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse}),
		})
		if err != nil {
			return nil, err
		}
		agents = append(agents, namedFile{name: m.agentName(agent), content: content})
	}
	return agents, nil
}

// copilotHooksFile renders the hooks of the module for Copilot CLI.
func (m Module) copilotHooksFile(h harness.Harness) ([]byte, error) {
	return encodeJSON(copilotHooks{Version: copilotHooksVersion, Hooks: map[string][]copilotHook{
		"preToolUse":   m.copilotEntry(hostproto.HookPreTool, h),
		"subagentStop": m.copilotEntry(hostproto.HookSubagentStop, h),
	}})
}

func (m Module) copilotFiles(h harness.Harness) ([]File, error) {
	agents, err := m.copilotAgents(h)
	if err != nil {
		return nil, err
	}
	var files []File
	for _, agent := range agents {
		files = append(files, File{Path: ".github/agents/" + agent.name + copilotAgentSuffix, Content: agent.content})
	}
	skill, err := m.skillFile(".github/skills/"+m.SkillName()+"/SKILL.md", m.otherDelegation(), "")
	if err != nil {
		return nil, err
	}
	hooks, err := m.copilotHooksFile(h)
	if err != nil {
		return nil, err
	}
	return append(files, skill, File{Path: m.CopilotHooksPath(), Content: hooks}), nil
}
