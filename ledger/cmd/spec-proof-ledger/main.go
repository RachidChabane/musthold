// Command spec-proof-ledger signs proofs and keeps them in the append-only proof ledger.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/ledger/internal/cli"
)

func main() {
	os.Exit(cli.Main("spec-proof-ledger", os.Args[1:], os.Stdout, os.Stderr))
}
