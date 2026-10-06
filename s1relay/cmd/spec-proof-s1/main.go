// Command spec-proof-s1 relays closed judgment questions to a System One endpoint.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/s1relay/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-s1", os.Args[1:], os.Stdout, os.Stderr))
}
