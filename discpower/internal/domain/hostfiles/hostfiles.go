// Package hostfiles is the module's definition of the files it ships to each host
// harness: the values that parametrize the shared generator of the host protocol.
// The committed trees under `plugin/` and the files `init` writes into a project
// are both generated from it. It knows no I/O.
package hostfiles

import (
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

// Identity of the shipped plugin.
const (
	// PluginVersion is the version of the shipped files.
	PluginVersion = "0.1.0"
	// Binary is the executable the hooks and the server run.
	Binary = "spec-proof-dp"
)

// Module describes the module to the shared generator.
var Module = pluginfiles.Module{
	Identity:    hostharness.Identity,
	Binary:      Binary,
	Version:     PluginVersion,
	Subject:     "discriminating power",
	ProofPhrase: "the discriminating-power proof",
}

// For returns the files shipped for a harness.
func For(harness hostharness.Harness) ([]pluginfiles.File, error) { return Module.For(harness) }

// Portable returns the files of the module's portable plugin, the one every client
// of the Agent Plugins standard loads.
func Portable() ([]pluginfiles.File, error) { return Module.Portable() }
