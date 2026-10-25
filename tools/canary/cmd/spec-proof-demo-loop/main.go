// Command spec-proof-demo-loop runs the recorded demonstration of the agent
// loop: a Claude Code session asked to finish a task in a project equipped with
// Shallnot and the proof modules, recorded and rendered as a transcript. It also
// renders a kept recording without starting any process.
package main

import (
	"os"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/cli"
)

func main() { os.Exit(cli.Main()) }
