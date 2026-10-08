// Command spec-proof-release plans, checks, builds and verifies the release of one
// module of the repository: its tag, its pre-flight, its archives and checksums,
// its SBOMs, its Homebrew formula and its notes.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/tools/release/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:], os.Stdout, os.Stderr)) }
