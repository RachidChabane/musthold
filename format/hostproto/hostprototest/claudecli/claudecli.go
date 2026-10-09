// Package claudecli runs the offline checks of the Claude Code CLI over a plugin
// folder: `claude plugin validate`, which reads the manifest and the hooks
// module the way the engine does, and `claude plugin test`, which runs the
// `*.test.ts` files of a mod against the engine. Both need no session and no
// model. A test that calls them skips, with a named reason, where the CLI is not
// installed.
package claudecli

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

const (
	// Binary is the Claude Code CLI.
	Binary = "claude"
	// commandTimeout bounds one CLI run.
	commandTimeout = 2 * time.Minute
	// offlineVariable keeps the CLI from any traffic its commands do not need.
	offlineVariable = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"
	pluginCommand   = "plugin"
	validateCommand = "validate"
	testCommand     = "test"
	// skipFormat names why a test is skipped.
	skipFormat = "the %s CLI is not on PATH: the offline plugin checks need it"
)

// Validate fails the test when `claude plugin validate` reports an error for the
// plugin folder. A warning does not fail it.
func Validate(t *testing.T, pluginDirectory string) {
	t.Helper()
	run(t, pluginCommand, validateCommand, pluginDirectory)
}

// Test fails the test when a `*.test.ts` file under the plugin folder fails.
func Test(t *testing.T, pluginDirectory string) string {
	t.Helper()
	return run(t, pluginCommand, testCommand, pluginDirectory)
}

func run(t *testing.T, arguments ...string) string {
	t.Helper()
	path, err := exec.LookPath(Binary)
	if err != nil {
		t.Skipf(skipFormat, Binary)
	}
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, path, arguments...)
	command.Env = append(os.Environ(), offlineVariable)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s: %v\n%s", Binary, strings.Join(arguments, " "), err, output)
	}
	return string(output)
}
