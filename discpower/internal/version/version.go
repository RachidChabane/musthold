// Package version holds the product version.
package version

import (
	"runtime/debug"

	"github.com/RachidChabane/spec-proof/format/cliconv"
)

// Version is the product version. Stamp sets it from the linker values of the
// binary, which a release build passes as `-X main.version=<semver>`.
var Version = cliconv.DevelopmentVersion

// Commit is the commit a release build was made from, passed as
// `-X main.commit=<sha>`; it is empty for a build that did not link one.
var Commit string

// Stamp sets Version and Commit from the values the linker gave the binary and
// from its build information, which supplies the version of a module installed
// with `go install`.
func Stamp(linkedVersion, linkedCommit string, info *debug.BuildInfo) {
	Version = cliconv.ResolveVersion(linkedVersion, info)
	Commit = linkedCommit
}

// Line renders the one-line version output of a binary: its name, its version and,
// when the build linked one, its commit.
func Line(binary string) string {
	return cliconv.Line(binary, Version, commitSuffix())
}

func commitSuffix() string {
	if Commit == "" {
		return ""
	}
	return "(" + Commit + ")"
}
