// Package hostharness describes the host harnesses the module serves: how each
// names the executor tools, what its per-agent tool allowlist enforces, which of
// the module's hooks it can run and which version the module was checked against.
// It is a Strategy per harness over one definition; it knows no I/O.
package hostharness

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Names the plugin and its executor server go by.
const (
	// PluginName is the name of the module's plugin on every harness.
	PluginName = "spec-proof-dp"
	// ExecutorServer is the name of the executor server inside the plugin.
	ExecutorServer = "exec"
	// StandaloneExecutorServer is the executor server's name where the harness
	// has no plugin scope to put it in.
	StandaloneExecutorServer = PluginName + "-" + ExecutorServer
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

// ErrUnknownHarness is returned for a harness the module does not serve.
var ErrUnknownHarness = errors.New("unknown harness")

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

// Harness is one host harness.
type Harness struct {
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
	// acceptedForms are the other spellings a hook payload may use for a tool.
	acceptedForms []func(server, tool string) string
}

func mcpDoubleUnderscore(server, tool string) string { return "mcp__" + server + "__" + tool }
func slashed(server, tool string) string             { return server + "/" + tool }

// Claude is Claude Code, where the executor is a plugin subagent whose allowlist
// is enforced and whose hooks run inside the subagent.
func Claude() Harness {
	return Harness{
		Flag: FlagClaude, Name: hostproto.HarnessClaudeCode, Version: ClaudeCodeVersion,
		Allowlist:  hostproto.AllowlistEnforced,
		Behaviours: Behaviours{RewritesLaunch: true, JournalsStart: true, IssuesCap: true, ChecksStop: true, ReportsRun: true, IdentifiesCaller: true},
		server:     "plugin_" + PluginName + "_" + ExecutorServer,
		toolName:   mcpDoubleUnderscore,
	}
}

// Codex is the Codex CLI, whose agents keep the shell: isolation is
// unrestricted and no capability is required.
func Codex() Harness {
	return Harness{
		Flag: FlagCodex, Name: hostproto.HarnessCodex, Version: CodexVersion,
		Allowlist: hostproto.AllowlistAbsent, AttestationOptOut: true,
		Behaviours:    Behaviours{JournalsStart: true, IssuesCap: true, ChecksStop: true, IdentifiesCaller: true},
		server:        StandaloneExecutorServer,
		toolName:      mcpDoubleUnderscore,
		acceptedForms: []func(string, string) string{slashed},
	}
}

// Copilot is GitHub Copilot CLI, whose MCP-only allowlist is unverified and
// which has no start hook to issue a capability against.
func Copilot() Harness {
	return Harness{
		Flag: FlagCopilot, Name: hostproto.HarnessCopilot, Version: CopilotVersion,
		Allowlist: hostproto.AllowlistUnverified, AttestationOptOut: true,
		Behaviours:    Behaviours{ChecksStop: true},
		server:        StandaloneExecutorServer,
		toolName:      slashed,
		acceptedForms: []func(string, string) string{mcpDoubleUnderscore},
	}
}

// All lists the harnesses the module serves.
func All() []Harness { return []Harness{Claude(), Codex(), Copilot()} }

// ByFlag returns the harness a `--harness` value names.
func ByFlag(flag string) (Harness, error) {
	for _, harness := range All() {
		if harness.Flag == flag {
			return harness, nil
		}
	}
	return Harness{}, fmt.Errorf("%w: %q", ErrUnknownHarness, flag)
}

// Flags lists the values `--harness` takes.
func Flags() []string {
	var flags []string
	for _, harness := range All() {
		flags = append(flags, harness.Flag)
	}
	return flags
}

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

// IsExecutorRole reports whether an agent type is one of the executor roles of
// the module's plugin.
func IsExecutorRole(agentType string) bool {
	return hostproto.IsExecutorRole(agentType, PluginName)
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
func (h Harness) AttestationRequired(host projectconfig.HostSettings) bool {
	return host.AttestationRequired() && !h.AttestationOptOut
}
