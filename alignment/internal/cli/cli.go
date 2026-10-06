// Package cli is the composition root of the binary. It parses arguments and
// wires use cases to adapters; it holds no business rule.
package cli

import (
	"fmt"
	"io"

	"github.com/RachidChabane/spec-proof/alignment/internal/version"
)

// Exit codes of the binary (ADR 0002).
const (
	ExitClean       = 0
	ExitToolFailure = 2
)

const versionCommand = "version"

// Main runs the binary called binary with the given arguments and returns its
// exit code.
func Main(binary string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == versionCommand {
		fmt.Fprintln(stdout, version.Line(binary))
		return ExitClean
	}
	fmt.Fprintf(stderr, "usage: %s %s\n", binary, versionCommand)
	return ExitToolFailure
}
