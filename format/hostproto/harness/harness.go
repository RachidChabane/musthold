// Package harness describes the host harnesses a proof module serves: how each
// names the executor tools of the module's plugin, what its per-agent tool
// allowlist enforces, which of the module's hooks it can run and which version
// the module was checked against. It is one Strategy per harness over one
// definition, parametrized by the identity of the module's plugin, so that every
// module spells the same rules the same way. It knows no I/O.
package harness

import (
	"errors"
	"fmt"

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
	ClaudeCodeVersion = "2.1.292"
	CodexVersion      = "0.155.1"
	CopilotVersion    = "1.0.92"
)

// Flag values of `--harness`.
const (
	FlagClaude  = "claude"
	FlagCodex   = "codex"
	FlagCopilot = "copilot"
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
	// server is the executor server's name as the harness shows it in tool names.
	server string
	// toolName renders the name the harness gives a tool of the server.
	toolName func(server, tool string) string
	// agentType renders the type the harness reports for an executor agent.
	agentType func(plugin string, agent hostproto.Agent) string
	// acceptedForms are the other spellings a hook payload may use for a tool.
	acceptedForms []func(server, tool string) string
}

func standaloneAgentName(plugin string, agent hostproto.Agent) string {
	return plugin + standaloneSeparator + string(agent)
}

func mcpDoubleUnderscore(server, tool string) string { return "mcp__" + server + "__" + tool }
func slashed(server, tool string) string             { return server + "/" + tool }

// pluginScopedServer is the server name Claude Code gives a plugin's server.
func pluginScopedServer(plugin string) string { return "plugin_" + plugin + "_" + ExecutorServer }

// Claude is Claude Code, where the executor is a plugin subagent whose allowlist
// is enforced and whose hooks run inside the subagent.
func Claude(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagClaude, Name: hostproto.HarnessClaudeCode, Version: ClaudeCodeVersion,
		Allowlist:  hostproto.AllowlistEnforced,
		Behaviours: Behaviours{RewritesLaunch: true, JournalsStart: true, IssuesCap: true, ChecksStop: true, ReportsRun: true, IdentifiesCaller: true},
		server:     pluginScopedServer(identity.Plugin),
		toolName:   mcpDoubleUnderscore,
		agentType:  hostproto.RoleFor,
	}
}

// Codex is the Codex CLI, whose agents keep the shell: isolation is
// unrestricted and no capability is required.
func Codex(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagCodex, Name: hostproto.HarnessCodex, Version: CodexVersion,
		Allowlist: hostproto.AllowlistAbsent, AttestationOptOut: true,
		Behaviours:    Behaviours{JournalsStart: true, IssuesCap: true, ChecksStop: true, IdentifiesCaller: true},
		server:        identity.StandaloneExecutorServer(),
		toolName:      mcpDoubleUnderscore,
		agentType:     standaloneAgentName,
		acceptedForms: []func(string, string) string{slashed},
	}
}

// Copilot is GitHub Copilot CLI, whose MCP-only allowlist is unverified and
// which has no start hook to issue a capability against.
func Copilot(identity Identity) Harness {
	return Harness{
		Identity: identity, Flag: FlagCopilot, Name: hostproto.HarnessCopilot, Version: CopilotVersion,
		Allowlist: hostproto.AllowlistUnverified, AttestationOptOut: true,
		Behaviours:    Behaviours{ChecksStop: true},
		server:        identity.StandaloneExecutorServer(),
		toolName:      slashed,
		agentType:     standaloneAgentName,
		acceptedForms: []func(string, string) string{mcpDoubleUnderscore},
	}
}

// All lists the harnesses a module serves.
func All(identity Identity) []Harness {
	return []Harness{Claude(identity), Codex(identity), Copilot(identity)}
}

// ByFlag returns the harness a `--harness` value names.
func ByFlag(identity Identity, flag string) (Harness, error) {
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
		if h.AgentTypeOf(agent) == agentType {
			return hostproto.RoleFor(h.Identity.Plugin, agent), true
		}
	}
	return "", false
}

// Observed is what the host shows: the plugin and its server are present, and the
// hooks are present when a capability of a started agent was verified.
func (h Harness) Observed(hooksPresent bool) hostproto.Observed {
	return hostproto.Observed{PluginPresent: true, ServerPresent: true, HooksPresent: hooksPresent, Allowlist: h.Allowlist}
}

// Isolation is the level an answer from this harness reaches before its stop
// hook attests the run.
func (h Harness) Isolation() format.Isolation {
	selection := hostproto.SelectChannel(h.Observed(false), hostproto.Policy{})
	return selection.Isolation
}

// AttestationRequired is the project's requirement of attestation as it applies
// on this harness: a harness that cannot issue capabilities waives it.
func (h Harness) AttestationRequired(projectRequires bool) bool {
	return projectRequires && !h.AttestationOptOut
}
