// Package hostharness is the module's view of the host harnesses: the shared
// harness strategies of the host protocol, instantiated for the alignment
// plugin and the executor agents it ships. It knows no I/O.
package hostharness

import (
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// Names the plugin and its executor server go by.
const (
	// PluginName is the name of the module's plugin on every harness.
	PluginName = "spec-proof-alignment"
	// ExecutorServer is the name of the executor server inside the plugin.
	ExecutorServer = harness.ExecutorServer
	// StandaloneExecutorServer is the executor server's name where the harness
	// has no plugin scope to put it in.
	StandaloneExecutorServer = PluginName + "-" + ExecutorServer
)

// Flag values of `--harness`.
const (
	FlagClaude  = harness.FlagClaude
	FlagCodex   = harness.FlagCodex
	FlagCopilot = harness.FlagCopilot
	// FlagPortable is the harness of a client of the Agent Plugins standard.
	FlagPortable = harness.FlagPortable
)

// Harness is one host harness as the module serves it.
type Harness = harness.Harness

// ErrUnknownHarness is returned for a harness the module does not serve.
var ErrUnknownHarness = harness.ErrUnknownHarness

// Identity is what the module is on every harness: its plugin and its two
// executor agents, the judge and the decider.
var Identity = harness.Identity{Plugin: PluginName, Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}}

// Claude is Claude Code.
func Claude() Harness { return harness.Claude(Identity) }

// Codex is the Codex CLI.
func Codex() Harness { return harness.Codex(Identity) }

// Copilot is GitHub Copilot CLI.
func Copilot() Harness { return harness.Copilot(Identity) }

// All lists the harnesses the module serves.
func All() []Harness { return harness.All(Identity) }

// ByFlag returns the harness a `--harness` value names.
func ByFlag(flag string) (Harness, error) { return harness.ByFlag(Identity, flag) }

// Flags lists the values `--harness` takes.
func Flags() []string { return harness.Flags() }
