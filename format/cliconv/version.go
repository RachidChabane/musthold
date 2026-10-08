package cliconv

import (
	"runtime/debug"
	"strings"
)

// VersionFlag is the lone argument that asks a binary for its version.
const VersionFlag = "--version"

// DevelopmentVersion is what a binary reports when nothing says which release it
// is: a build with no linked version that is not an installed module version.
const DevelopmentVersion = "dev"

const (
	versionPrefix       = "v"
	unreleasedBuildInfo = "(devel)"
)

// IsVersionRequest reports whether a command line is the lone argument
// VersionFlag.
func IsVersionRequest(arguments []string) bool {
	return len(arguments) == 1 && arguments[0] == VersionFlag
}

// ResolveVersion returns the version a binary reports. The value the linker set
// wins; without one the module version `go install` recorded in the build
// information is used, without its leading "v"; a build that has neither reports
// DevelopmentVersion.
func ResolveVersion(linked string, info *debug.BuildInfo) string {
	if linked != "" {
		return linked
	}
	if info == nil {
		return DevelopmentVersion
	}
	installed := strings.TrimPrefix(info.Main.Version, versionPrefix)
	if installed == "" || info.Main.Version == unreleasedBuildInfo {
		return DevelopmentVersion
	}
	return installed
}
