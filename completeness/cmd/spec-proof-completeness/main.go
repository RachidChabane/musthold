// Command spec-proof-completeness advises on behaviour the specification leaves unstated.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-completeness", os.Args[1:], os.Stdout, os.Stderr))
}
