// Command spec-proof-dp measures the discriminating power of the tests bound to each requirement, by executing them against mutants.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-dp", os.Args[1:], os.Stdout, os.Stderr))
}
