// Command spec-proof produces, signs, stores and replays proofs.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof", os.Args[1:], os.Stdout, os.Stderr))
}
