// Package dispatch is the `musthold` command line: a dispatcher that holds no
// rule of its own and runs each command in the binary of the module that owns
// it, with the same arguments, streams and exit code. `musthold scan` runs
// `musthold-dp scan`.
package dispatch

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"

	"github.com/RachidChabane/musthold/format/cliconv"
)

// BinaryName is the name of the dispatcher.
const BinaryName = "musthold"

// Commands of the dispatcher.
const (
	commandVersion = "version"
	commandScan    = "scan"
)

// routes maps each command the dispatcher forwards to the binary that runs it.
var routes = map[string]string{commandScan: "musthold-dp"}

// ExitNotInstalled is the exit code when the binary of a command is missing.
const ExitNotInstalled = cliconv.ExitUsage

// ErrNotInstalled is returned when the binary of a command is neither beside
// the dispatcher nor on the PATH.
var ErrNotInstalled = errors.New("is not installed: put it beside musthold or on the PATH")

// Process starts a binary with arguments on the dispatcher's streams and
// returns its exit code.
type Process interface {
	Run(path string, args []string, stdin io.Reader, stdout, stderr io.Writer) (int, error)
}

// Env is what the dispatcher takes from its surroundings.
type Env struct {
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Executable is the path of the running dispatcher, empty when unknown.
	Executable string
	// IsExecutable reports whether a path is an executable file.
	IsExecutable func(path string) bool
	// LookPath finds an executable on the PATH.
	LookPath func(name string) (string, error)
	Process  Process
	Version  string
}

// Run runs one command line and returns the exit code.
func Run(env Env, args []string) int {
	switch {
	case len(args) == 0:
		fmt.Fprint(env.Stderr, usage())
		return cliconv.ExitUsage
	case cliconv.IsVersionRequest(args) || (args[0] == commandVersion && len(args) == 1):
		fmt.Fprintln(env.Stdout, BinaryName+" "+env.Version)
		return 0
	case args[0] == cliconv.HelpCommand && len(args) > 1:
		return Run(env, []string{args[1], "-h"})
	case cliconv.IsHelpRequest(args):
		fmt.Fprint(env.Stdout, usage())
		return cliconv.ExitHelp
	}
	binary, routed := routes[args[0]]
	if !routed {
		fmt.Fprintf(env.Stderr, "%s: unknown command %q\n%s", BinaryName, args[0], usage())
		return cliconv.ExitUsage
	}
	path, err := locate(env, binary)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: %v\n", BinaryName, err)
		return ExitNotInstalled
	}
	code, err := env.Process.Run(path, args, env.Stdin, env.Stdout, env.Stderr)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s: run %s: %v\n", BinaryName, binary, err)
		return ExitNotInstalled
	}
	return code
}

// usage names the commands of the dispatcher.
func usage() string {
	return cliconv.CommandsUsage(BinaryName, append(slices.Sorted(maps.Keys(routes)), commandVersion, cliconv.HelpCommand))
}

// locate finds the binary beside the dispatcher first, then on the PATH.
func locate(env Env, binary string) (string, error) {
	if env.Executable != "" {
		beside := filepath.Join(filepath.Dir(env.Executable), binary)
		if env.IsExecutable != nil && env.IsExecutable(beside) {
			return beside, nil
		}
	}
	if env.LookPath != nil {
		if path, err := env.LookPath(binary); err == nil {
			return path, nil
		}
	}
	return "", fmt.Errorf("%s %w", binary, ErrNotInstalled)
}
