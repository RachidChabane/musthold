// Package modules lists the proof modules a canary session installs and the
// module whose executor the session runs.
package modules

import (
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/hostinstall"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
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
	// Subject names what the module proves, as its plugin files read it.
	Subject string
	// Required says whether Shallnot accepts the module's proof in
	// `proofs.require`; an advisory proof says so in its block of AGENTS.md.
	Required bool
}

// Identity is the module as the harness strategies spell it.
func (m Module) Identity() harness.Identity {
	return harness.Identity{Plugin: m.Plugin, Agents: m.Agents}
}

// Install is the module as the shared installer renders it: what `enable` writes
// into the AGENTS.md of a project comes from it.
func (m Module) Install() hostinstall.Module {
	return hostinstall.Module{Files: m.Files(), Required: m.Required}
}

// Files is the module as the shared generator of plugin files sees it.
func (m Module) Files() pluginfiles.Module {
	return pluginfiles.Module{Identity: m.Identity(), Binary: m.Binary, Subject: m.Subject}
}

// Installs lists the modules as the shared installer renders them, in order.
func Installs() []hostinstall.Module {
	var installs []hostinstall.Module
	for _, module := range All() {
		installs = append(installs, module.Install())
	}
	return installs
}

// PluginDir is the directory of the module's generated plugin for a harness flag,
// relative to the repository root.
func (m Module) PluginDir(harnessFlag string) string {
	return m.Dir + "/plugin/" + harnessFlag
}

// All lists the modules whose plugins a session loads, the primary one first.
func All() []Module {
	return []Module{
		{Dir: "discpower", Binary: "musthold-dp", Plugin: "musthold-dp", Agents: hostproto.Agents(), Subject: "discriminating power", Required: true},
		{Dir: "alignment", Binary: "musthold-alignment", Plugin: "musthold-alignment", Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}, BuildTags: grammarSubsetTags, Subject: "alignment", Required: true},
		{Dir: "completeness", Binary: "musthold-completeness", Plugin: "musthold-completeness", Agents: []hostproto.Agent{hostproto.AgentDecider}, BuildTags: grammarSubsetTags, Subject: "completeness"},
	}
}

// Classified is the module whose requests the mod classifies in shadow during a
// Claude Code session: the completeness module. Its journal is recorded apart
// from the primary one.
func Classified() Module { return All()[2] }

// Primary is the module whose executor the session launches and whose journal
// the checks read: the discriminating-power module.
func Primary() Module { return All()[0] }
