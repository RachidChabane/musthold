// Package cli is the composition root of the binary. It parses arguments and
// wires use cases to adapters; it holds no business rule.
package cli

import (
	"fmt"
	"io"
	"os"

	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/s1relay/internal/adapters/memguard"
	"github.com/RachidChabane/musthold/s1relay/internal/adapters/modulecli"
	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/version"
)

// Exit codes of the binary: 0, 1 and 2 as in ADR 0002, and 4 as script/mem-guard
// returns it for a refused batch.
const (
	// ExitClean: nothing was left unanswered.
	ExitClean = 0
	// ExitUnanswered: some requests were left to the host votes.
	ExitUnanswered = 1
	// ExitToolFailure: the relay could not do its work.
	ExitToolFailure = 2
	// ExitMemoryRefused: the memory rule refused the batch.
	ExitMemoryRefused = 4
)

const (
	versionCommand = "version"
	drainCommand   = "drain"

	// helpFlag is the flag `help <command>` hands to the command.
	helpFlag = "-h"
)

// commands are the commands of the binary, in the order the usage names them.
var commands = []string{versionCommand, drainCommand, cliconv.HelpCommand}

// Env is everything the command line needs from its surroundings.
type Env struct {
	Binary    string
	Stdout    io.Writer
	Stderr    io.Writer
	WorkDir   string
	LookupEnv func(name string) (string, bool)
	// Runner runs module binaries.
	Runner modulecli.Runner
	// NewGuard builds the memory rule for a minimum free percentage.
	NewGuard func(minFreePercent int) ports.MemoryGuard
}

// Main runs the binary called binary with the given arguments in the real
// environment and returns its exit code.
func Main(binary string, args []string, stdout, stderr io.Writer) int {
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", binary, err)
		return ExitToolFailure
	}
	return Run(Env{
		Binary: binary, Stdout: stdout, Stderr: stderr, WorkDir: workDir, LookupEnv: os.LookupEnv,
		Runner:   modulecli.ExecRunner{},
		NewGuard: func(minFree int) ports.MemoryGuard { return memguard.New(minFree) },
	}, args)
}

// Run executes one command line and returns its exit code.
func Run(env Env, args []string) int {
	if len(args) == 1 && args[0] == versionCommand {
		fmt.Fprintln(env.Stdout, version.Line(env.Binary))
		return ExitClean
	}
	if cliconv.IsHelpRequest(args) {
		return env.help(args)
	}
	if len(args) >= 1 && args[0] == drainCommand {
		return env.drain(args[1:])
	}
	fmt.Fprint(env.Stderr, cliconv.CommandsUsage(env.Binary, commands))
	return ExitToolFailure
}

// help answers a request for help: the usage of the binary, or the one of the
// command that `help` names.
func (e Env) help(args []string) int {
	if args[0] == cliconv.HelpCommand && len(args) > 1 {
		return Run(e, []string{args[1], helpFlag})
	}
	fmt.Fprint(e.Stdout, cliconv.CommandsUsage(e.Binary, commands))
	return cliconv.ExitHelp
}
