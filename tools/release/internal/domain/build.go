package domain

import "fmt"

// The controlled build environment and the flags of every release build.
const (
	ToolchainVersion = "go1.26.4"

	EnvWorkspace = "GOWORK"
	EnvCgo       = "CGO_ENABLED"
	EnvToolchain = "GOTOOLCHAIN"
	EnvFlags     = "GOFLAGS"
	EnvOS        = "GOOS"
	EnvArch      = "GOARCH"

	workspaceOff = "off"
	cgoOff       = "0"
	readonlyMod  = "-mod=readonly"

	LinkedVersionVariable = "main.version"
	LinkedCommitVariable  = "main.commit"
)

// CheckEnvironment lists the variables the build and test checks of the pre-flight
// set: no workspace, the pinned toolchain and read-only modules, so that they
// certify the dependency set that ships.
func CheckEnvironment() []string {
	return []string{EnvWorkspace + "=" + workspaceOff, EnvToolchain + "=" + ToolchainVersion, EnvFlags + "=" + readonlyMod}
}

// BuildSpec is one cross-build of a module's binary.
type BuildSpec struct {
	Tag    Tag
	Commit string
	Target Target
	// Output is the file the binary is written to.
	Output string
}

// Environment lists the variables the build sets: no workspace, no cgo, the pinned
// toolchain, read-only modules and the target.
func (b BuildSpec) Environment() []string {
	return []string{
		EnvWorkspace + "=" + workspaceOff,
		EnvCgo + "=" + cgoOff,
		EnvToolchain + "=" + ToolchainVersion,
		EnvFlags + "=" + readonlyMod,
		EnvOS + "=" + b.Target.OS,
		EnvArch + "=" + b.Target.Arch,
	}
}

// LinkerFlags are the -ldflags of the build: stripped, no build id, and the
// version and commit the binary reports.
func (b BuildSpec) LinkerFlags() string {
	return fmt.Sprintf("-s -w -buildid= -X %s=%s -X %s=%s",
		LinkedVersionVariable, b.Tag.Semver(), LinkedCommitVariable, b.Commit)
}

// Arguments are the arguments of `go`: a trimmed-path build with no VCS stamping,
// of the main package of the module's binary.
func (b BuildSpec) Arguments() []string {
	return []string{
		"build", "-trimpath", "-buildvcs=false",
		"-ldflags", b.LinkerFlags(),
		"-o", b.Output, b.Tag.Module.MainPackage(),
	}
}
