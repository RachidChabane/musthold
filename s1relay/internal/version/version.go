// Package version holds the product version.
package version

// Version is the product version. Release builds override it with
// `-ldflags "-X github.com/RachidChabane/spec-proof/s1relay/internal/version.Version=<v>"`.
var Version = "dev"

// Line renders the one-line version output of a binary.
func Line(binary string) string {
	return binary + " " + Version
}
