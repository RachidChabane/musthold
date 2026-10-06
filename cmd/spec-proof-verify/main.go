// Command spec-proof-verify checks signed proofs and writes the verified set. It
// reads files only: it links no process, model, ecosystem, git or workspace
// adapter.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-verify", os.Args[1:], os.Stdout, os.Stderr))
}
