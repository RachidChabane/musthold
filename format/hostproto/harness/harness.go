// Package harness describes the host harnesses a proof module serves: how each
// names the executor tools of the module's plugin, what its per-agent tool
// allowlist enforces, which of the module's hooks it can run and which version
// the module was checked against. It is one Strategy per harness over one
// definition, parametrized by the identity of the module's plugin, so that every
// module spells the same rules the same way. It knows no I/O.
package harness

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names an executor server goes by.
const (
	// ExecutorServer is the name of the executor server inside a plugin.
	ExecutorServer = "exec"
	// standaloneSeparator joins a plugin and its server or agent in the names a
	// harness without plugin scope uses.
	standaloneSeparator = "-"
)

// Versions of the harnesses the hooks and the generated files were checked against.
const (
	ClaudeCodeVersion = "2.1.295"
	CodexVersion      = "0.155.1"
	CopilotVersion    = "1.0.92"
	// AgentPluginsVersion is the version of the Agent Plugins specification the
	// portable plugin targets.
	AgentPluginsVersion = "1.0.0"
)

// Flag values of `--harness`.
const (
	FlagClaude  = "claude"
	FlagCodex   = "codex"
	FlagCopilot = "copilot"
	// FlagPortable names the server a client of the Agent Plugins standard starts
	// from the portable `mcp.json`, which carries no `--harness`.
	FlagPortable = "portable"
)

// ErrUnknownHarness is returned for a harness a module does not serve.
var ErrUnknownHarness = errors.New("unknown harness")

// Identity is what a module is on every harness: the name of its plugin and the
// executor agents it ships. Only the agents named here are executors of the
// module.
type Identity struct {
	// Plugin is the name of the module's plugin, such as `spec-proof-dp`.
	Plugin string
	// Agents are the executor agents of the plugin, in the order its files list them.
	Agents []hostproto.Agent
}

// StandaloneExecutorServer is the executor server's name where the harness has
// no plugin scope to put it in.
func (i Identity) StandaloneExecutorServer() string {
	return i.Plugin + standaloneSeparator + ExecutorServer
}

// Roles lists the executor roles of the module, in the order of its agents.
func (i Identity) Roles() []string {
	roles := make([]string, len(i.Agents))
	for index, agent := range i.Agents {
		roles[index] = hostproto.RoleFor(i.Plugin, agent)
	}
	return roles
}

// Behaviours say which of the module's hooks the harness can run.
type Behaviours struct {
	// RewritesLaunch: PreToolUse on the tool that launches a subagent.
	RewritesLaunch bool
	// JournalsStart: a SubagentStart hook.
	JournalsStart bool
	// IssuesCap: PreToolUse on the executor tools adds the capability.
	IssuesCap bool
	// ChecksStop: a SubagentStop hook with the transcript of the run.
	ChecksStop bool
	// ReportsRun: PostToolUse on the launch tool reports the resolved model.
	ReportsRun bool
	// IdentifiesCaller: a tool hook says which agent called the tool.
	IdentifiesCaller bool
	// GuardsSpawn: PreToolUse on the tool that spawns an agent refuses a spawn of
	// an executor that does not ask for a child with none of the parent's history.
	GuardsSpawn bool
	// ObservesSelection: the record the stop hook reads shows the tools the
	// harness selected for the agent and the messages it was given, so the stop
	// hook can verify the allowlist and the launch of the run itself.
	ObservesSelection bool
}

// Harness is one host harness, as one module serves it.
type Harness struct {
	// Identity is the module the harness is described for.
	Identity Identity
	// Flag is how `--harness` spells it.
	Flag string
	// Name is the harness member of an executor profile.
	Name string
	// Version is the harness version the module was checked against.
	Version string
	// Allowlist is what the harness is known to enforce of an agent's tool allowlist.
	Allowlist hostproto.ToolAllowlist
	// AttestationOptOut is set where no capability can be issued, so that the
	// project's requirement of attestation cannot be met.
	AttestationOptOut bool
	// Behaviours are the hooks the harness can run.
	Behaviours Behaviours
	// Channel is how an answer taken through the harness's server reaches the
	// module; the zero value is the subagent channel.
	Channel format.Channel
	// Transcript is the layout of the transcript the stop hook reads.
	Transcript hostproto.TranscriptFormat
	// server is the executor server's name as the harness shows it in tool names.
	server string
	// toolName renders the name the harness gives a tool of the server.
	toolName func(server, tool string) string
	// agentType renders the type the harness reports for an executor agent.
	agentType func(plugin string, agent hostproto.Agent) string
	// acceptedForms are the other spellings a hook payload may use for a tool.
	acceptedForms []func(server, tool string) string
	// otherAgentTypes are the other types a hook payload may report for an executor agent.
	otherAgentTypes []func(plugin string, agent hostproto.Agent) string
	// transcript reads the run of one agent out of the record the stop hook is given.
	transcript func(reader io.Reader, agentID string) (hostproto.SessionRun, error)
}

func standaloneAgentName(plugin string, agent hostproto.Agent) string {
	return plugin + standaloneSeparator + string(agent)
}

// pluginScopedAgentName is the type Copilot CLI reports for a plugin's agent: the
// plugin, a colon and the name of the agent file.
func pluginScopedAgentName(plugin string, agent hostproto.Agent) string {
	return plugin + hostproto.RoleSeparator + standaloneAgentName(plugin, agent)
}

// callsTranscript reads a transcript that records the tool calls of the agent.
func callsTranscript(reader io.Reader, _ string) (hostproto.SessionRun, error) {
	calls, err := hostproto.DecodeTranscript(reader)
	return hostproto.RunOfCalls(calls), err
}

func mcpDoubleUnderscore(server, tool string) string { return "mcp__" + server + "__" + tool }
func slashed(server, tool string) string             { return server + "/" + tool }

// hyphenJoined is how the preToolUse payload of Copilot CLI spells a tool: the
// server and the tool joined by a hyphen.
func hyphenJoined(server, tool string) string { return server + "-" + tool }

// toolNameUnsafe matches every character Codex replaces in a server name when it
// builds the name of a tool.
var toolNameUnsafe = regexp.MustCompile(`[^A-Za-z0-9_]`)

// sanitizedMCP is how Codex spells a tool: the double-underscore form with every
// character of the server name outside `[A-Za-z0-9_]` replaced by `_`.
func sanitizedMCP(server, tool string) string {
	return mcpDoubleUnderscore(toolNameUnsafe.ReplaceAllString(server, "_"), tool)
}

// pluginScopedServer is the server name Claude Code gives a plugin's server.
func pluginScopedServer(plugin string) string { return "plugin_" + plugin + "_" + ExecutorServer }

// Claude is Claude Code, where the executor is a plugin subagent whose allowlist
// is enforced and whose hooks run inside the subagent.
func Claude(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagClaude, Name: hostproto.HarnessClaudeCode, Version: ClaudeCodeVersion,
		Allowlist:  hostproto.AllowlistEnforced,
		Behaviours: Behaviours{RewritesLaunch: true, JournalsStart: true, IssuesCap: true, ChecksStop: true, ReportsRun: true, IdentifiesCaller: true},
		Transcript: hostproto.TranscriptAgentJSONL,
		server:     pluginScopedServer(identity.Plugin),
		toolName:   mcpDoubleUnderscore,
		agentType:  hostproto.RoleFor,
	}
}

// Codex is the Codex CLI, whose agents keep the shell: isolation is
// unrestricted and no capability is required. Its hooks refuse a spawn of an
// executor that would inherit the main agent's history, and its stop hook reads
// the child's rollout for history it inherited all the same.
func Codex(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagCodex, Name: hostproto.HarnessCodex, Version: CodexVersion,
		Allowlist: hostproto.AllowlistAbsent, AttestationOptOut: true,
		Behaviours:    Behaviours{JournalsStart: true, IssuesCap: true, ChecksStop: true, IdentifiesCaller: true, GuardsSpawn: true},
		Transcript:    hostproto.TranscriptCodexRollout,
		server:        identity.StandaloneExecutorServer(),
		toolName:      sanitizedMCP,
		agentType:     standaloneAgentName,
		acceptedForms: []func(string, string) string{mcpDoubleUnderscore, slashed},
	}
}

// Copilot is GitHub Copilot CLI, whose MCP-only allowlist is unverified and
// which has no start hook to issue a capability against. Its stop hook is given
// the session log, from which each run's selection and launch are observed.
func Copilot(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagCopilot, Name: hostproto.HarnessCopilot, Version: CopilotVersion,
		Allowlist: hostproto.AllowlistUnverified, AttestationOptOut: true,
		Behaviours:      Behaviours{ChecksStop: true, ObservesSelection: true},
		Transcript:      hostproto.TranscriptCopilotSession,
		server:          identity.StandaloneExecutorServer(),
		toolName:        slashed,
		agentType:       standaloneAgentName,
		acceptedForms:   []func(string, string) string{mcpDoubleUnderscore, hyphenJoined},
		otherAgentTypes: []func(string, hostproto.Agent) string{pluginScopedAgentName},
		transcript:      hostproto.DecodeCopilotSessionLog,
	}
}

// Portable is a client of the Agent Plugins standard served by the portable
// plugin alone: no hook runs, so no capability is issued and no subagent is
// known to be restricted. Whoever calls the server is the main agent, so its
// answers take the inline channel and isolation, whatever the client enforces.
func Portable(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagPortable, Name: hostproto.HarnessAgentPlugins, Version: AgentPluginsVersion,
		Allowlist: hostproto.AllowlistAbsent, AttestationOptOut: true,
		Channel:       format.ChannelHostInline,
		server:        identity.StandaloneExecutorServer(),
		toolName:      mcpDoubleUnderscore,
		agentType:     standaloneAgentName,
		acceptedForms: []func(string, string) string{slashed},
	}
}

// All lists the harnesses a module serves.
func All(identity Identity) []Harness {
	return []Harness{Claude(identity), Codex(identity), Copilot(identity)}
}

// ByFlag returns the harness a `--harness` value names.
func ByFlag(identity Identity, flag string) (Harness, error) {
	if flag == FlagPortable {
		return Portable(identity), nil
	}
	for _, harness := range All(identity) {
		if harness.Flag == flag {
			return harness, nil
		}
	}
	return Harness{}, fmt.Errorf("%w: %q", ErrUnknownHarness, flag)
}

// Flags lists the values `--harness` takes.
func Flags() []string { return []string{FlagClaude, FlagCodex, FlagCopilot} }

// Server is the executor server's name as the harness names it in tools.
func (h Harness) Server() string { return h.server }

// ToolName is the name the harness gives an executor tool, which is what an
// agent's allowlist and a hook matcher spell.
func (h Harness) ToolName(tool string) string { return h.toolName(h.server, tool) }

// ToolMatcher is the pattern of a hook matcher that selects every tool of the
// server in the form the harness names them.
func (h Harness) ToolMatcher() string { return h.ToolName("") + ".*" }

// PreToolMatcher is the pattern of the PreToolUse hook matcher: every tool of the
// server, and the spawn tool under each name it goes by where the harness guards
// spawns.
func (h Harness) PreToolMatcher() string {
	if !h.Behaviours.GuardsSpawn {
		return h.ToolMatcher()
	}
	spawnTools := hostproto.CodexSpawnToolNames()
	quoted := make([]string, len(spawnTools))
	for index, name := range spawnTools {
		quoted[index] = regexp.QuoteMeta(name)
	}
	return "^(" + strings.Join(quoted, "|") + ")$|" + h.ToolMatcher()
}

// ToolNames lists the names of the two executor tools, fetch first.
func (h Harness) ToolNames() []string {
	return []string{h.ToolName(hostproto.ToolFetchRequest), h.ToolName(hostproto.ToolSubmitResponse)}
}

// ExecutorTool reports which executor tool a hook payload's tool name is.
func (h Harness) ExecutorTool(name string) (string, bool) {
	for _, tool := range []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse, hostproto.ToolDrain} {
		if name == h.toolName(h.server, tool) {
			return tool, true
		}
		for _, form := range h.acceptedForms {
			if name == form(h.server, tool) {
				return tool, true
			}
		}
	}
	return "", false
}

// AgentType is the type this harness reports for the executor agent of a role.
func (h Harness) AgentType(role string) string {
	plugin, agent, err := hostproto.ParseRole(role)
	if err != nil {
		return ""
	}
	return h.agentType(plugin, agent)
}

// AgentTypeOf is the type this harness reports for an executor agent of the module.
func (h Harness) AgentTypeOf(agent hostproto.Agent) string {
	return h.agentType(h.Identity.Plugin, agent)
}

// RoleOf returns the role of the executor agent an agent type names on this
// harness, and false for any other agent, including an executor agent the module
// does not ship.
func (h Harness) RoleOf(agentType string) (string, bool) {
	for _, agent := range h.Identity.Agents {
		if slices.Contains(h.agentTypesOf(agent), agentType) {
			return hostproto.RoleFor(h.Identity.Plugin, agent), true
		}
	}
	return "", false
}

// NamesRole reports whether an agent type is one of the types this harness
// reports for the executor agent of a role.
func (h Harness) NamesRole(role, agentType string) bool {
	if reported, ours := h.RoleOf(agentType); ours {
		return reported == role
	}
	return false
}

// agentTypesOf lists every type the harness may report for an executor agent.
func (h Harness) agentTypesOf(agent hostproto.Agent) []string {
	types := []string{h.AgentTypeOf(agent)}
	for _, form := range h.otherAgentTypes {
		types = append(types, form(h.Identity.Plugin, agent))
	}
	return types
}

// ReadRun reads the run of one agent out of the record the stop hook was given.
func (h Harness) ReadRun(record []byte, agentID string) (hostproto.SessionRun, error) {
	read := h.transcript
	if read == nil {
		read = callsTranscript
	}
	return read(bytes.NewReader(record), agentID)
}

// Observed is what the host shows: the plugin and its server are present, and the
// hooks are present when a capability of a started agent was verified.
func (h Harness) Observed(hooksPresent bool) hostproto.Observed {
	return hostproto.Observed{PluginPresent: true, ServerPresent: true, HooksPresent: hooksPresent, Allowlist: h.Allowlist}
}

// Isolation is the level an answer from this harness reaches before its stop
// hook attests the run.
func (h Harness) Isolation() format.Isolation {
	if h.Channel == format.ChannelHostInline {
		return format.IsolationInline
	}
	selection := hostproto.SelectChannel(h.Observed(false), hostproto.Policy{})
	return selection.Isolation
}

// AnswerChannel is the channel the answers taken through the harness's server travel.
func (h Harness) AnswerChannel() format.Channel {
	if h.Channel == "" {
		return format.ChannelHostSubagent
	}
	return h.Channel
}

// AttestationRequired is the project's requirement of attestation as it applies
// on this harness: a harness that cannot issue capabilities waives it.
func (h Harness) AttestationRequired(projectRequires bool) bool {
	return projectRequires && !h.AttestationOptOut
}
