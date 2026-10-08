package domain

import "strconv"

// Output is one named value of a plan, written as `key=value` so that a workflow
// can read it from its step output file.
type Output struct{ Key, Value string }

// Keys of the plan outputs.
const (
	OutputTag         = "tag"
	OutputModule      = "module"
	OutputKind        = "kind"
	OutputBinary      = "binary"
	OutputSemver      = "semver"
	OutputShipsBinary = "ships_binary"
	OutputShipsPlugin = "ships_plugin"
	OutputCIModules   = "ci_modules"
	OutputIdentity    = "certificate_identity"
)

// PlanOutputs lists what the workflow and the front door need to know about a
// tag: the module and its kind, the version, which parts the release ships, and
// the modules whose CI gates it. The format library gates every module.
func PlanOutputs(tag Tag) []Output {
	module := tag.Module
	ciModules := FormatModuleName
	if module.Name != FormatModuleName {
		ciModules += " " + module.Name
	}
	return []Output{
		{OutputTag, tag.String()},
		{OutputModule, module.Name},
		{OutputKind, string(module.Kind)},
		{OutputBinary, module.Binary},
		{OutputSemver, tag.Semver()},
		{OutputShipsBinary, strconv.FormatBool(module.ShipsBinary())},
		{OutputShipsPlugin, strconv.FormatBool(module.ShipsPlugin())},
		{OutputCIModules, ciModules},
		{OutputIdentity, CertificateIdentity(tag)},
	}
}
