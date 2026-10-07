// Package modules lists the proof modules a canary session installs and the
// module whose executor the session runs.
package modules

import (
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// Grammar build tags of the modules that parse several languages; the end-to-end
// tests build them the same way, which keeps the binaries small.
const grammarSubsetTags = "grammar_subset,grammar_subset_python,grammar_subset_javascript,grammar_subset_typescript,grammar_subset_tsx,grammar_subset_java,grammar_subset_kotlin"

// Module is one proof module as the canary builds, installs and runs it.
type Module struct {
	// Dir is the module's directory in the repository.
	Dir string
	// Binary is the executable the module's plugin files name.
	Binary string
	// Plugin is the plugin's name.
	Plugin string
	// Agents are the executor agents of the plugin.
	Agents []hostproto.Agent
	// BuildTags are the Go build tags of the binary, comma-separated.
	BuildTags string
}

// Identity is the module as the harness strategies spell it.
func (m Module) Identity() harness.Identity {
	return harness.Identity{Plugin: m.Plugin, Agents: m.Agents}
}

// PluginDir is the directory of the module's generated plugin for a harness flag,
// relative to the repository root.
func (m Module) PluginDir(harnessFlag string) string {
	return m.Dir + "/plugin/" + harnessFlag
}

// All lists the modules whose plugins a session loads, the primary one first.
func All() []Module {
	return []Module{
		{Dir: "discpower", Binary: "spec-proof-dp", Plugin: "spec-proof-dp", Agents: hostproto.Agents()},
		{Dir: "alignment", Binary: "spec-proof-alignment", Plugin: "spec-proof-alignment", Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}, BuildTags: grammarSubsetTags},
		{Dir: "completeness", Binary: "spec-proof-completeness", Plugin: "spec-proof-completeness", Agents: []hostproto.Agent{hostproto.AgentDecider}, BuildTags: grammarSubsetTags},
	}
}

// Primary is the module whose executor the session launches and whose journal
// the checks read: the discriminating-power module.
func Primary() Module { return All()[0] }
