// Command spec-proof-alignment judges whether the tests bound to a requirement verify what it states.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-alignment", os.Args[1:], os.Stdout, os.Stderr))
}
