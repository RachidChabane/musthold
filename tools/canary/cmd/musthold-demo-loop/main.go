// Command musthold-demo-loop runs the recorded demonstration of the agent
// loop: a Claude Code session asked to finish a task in a project equipped with
// Shallnot and the proof modules, recorded and rendered as a transcript. It also
// renders a kept recording without starting any process, and runs each verb of
// the loop-1 study as the subcommand of its name.
package main

import (
	"os"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/cli"
)

func main() { os.Exit(cli.Main()) }
