// Command musthold-canary runs one short, scripted session of a host harness
// against the proof modules' plugins and concludes, from what the session
// recorded, whether the harness behaves as ADR 0005 relies on.
package main

import (
	"os"

	"github.com/RachidChabane/musthold/tools/canary/internal/cli"
)

func main() { os.Exit(cli.Main()) }
