// Package cli is the composition root of the binary. It parses arguments and
// wires use cases to adapters; it holds no business rule.
package cli

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/RachidChabane/musthold/completeness/internal/app/prove"
	"github.com/RachidChabane/musthold/completeness/internal/version"
	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// Exit codes of the binary (ADR 0005).
const (
	ExitClean        = prove.ExitClean
	ExitBlocked      = prove.ExitBlocked
	ExitToolFailure  = prove.ExitToolFailure
	ExitAwaitingHost = prove.ExitAwaitingHost
)

// Commands of the binary.
const (
	commandVersion   = "version"
	commandProve     = "prove"
	commandRequest   = "request"
	commandSystemOne = "systemone"
	commandDoctor    = "doctor"
	commandMCP       = "mcp"
	commandHook      = "hook"
)

// commands lists the commands in the order the usage names them.
var commands = []string{commandVersion, commandProve, commandRequest, commandSystemOne, commandDoctor, commandInit, commandEnable, commandDisable, commandMCP, commandHook, commandHost, cliconv.HelpCommand}

// flagHelp is the flag `help <command>` hands to the command.
const flagHelp = "-h"

// Environment variables the binary reads.
const (
	envShallnot = "MUSTHOLD_SHALLNOT"
)

// Env is everything the binary takes from its surroundings, so that a test can
// run the command line in process.
type Env struct {
	Binary    string
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	WorkDir   string
	HomeDir   string
	LookupEnv func(string) (string, bool)
	Now       func() time.Time
	// Entropy is where nonces come from; it defaults to the system's source.
	Entropy io.Reader
	// ClassifyAdmission replaces the shipped admission table of classifications;
	// nil keeps the shipped one.
	ClassifyAdmission *hostproto.ClassifyAdmission
}

// Main runs the binary called binary with the given arguments in the process's
// own environment and returns its exit code.
func Main(binary string, args []string, stdout, stderr io.Writer) int {
	workDir, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", binary, err)
		return ExitToolFailure
	}
	home, _ := os.UserHomeDir()
	return Run(Env{
		Binary: binary, Stdin: os.Stdin, Stdout: stdout, Stderr: stderr,
		WorkDir: workDir, HomeDir: home, LookupEnv: os.LookupEnv, Now: time.Now, Entropy: rand.Reader,
	}, args)
}

// Run runs a command line in an environment and returns its exit code.
func Run(env Env, args []string) int {
	if env.Entropy == nil {
		env.Entropy = rand.Reader
	}
	if env.Now == nil {
		env.Now = time.Now
	}
	if len(args) == 0 {
		return env.usage()
	}
	if cliconv.IsHelpRequest(args) {
		return env.help(args)
	}
	command, rest := args[0], args[1:]
	var err error
	switch {
	case command == commandVersion && len(rest) == 0:
		fmt.Fprintln(env.Stdout, version.Line(env.Binary))
		return ExitClean
	case command == commandVersion && len(rest) == 1 && cliconv.IsHelpArgument(rest[0]):
		return env.finish(outcome{err: cliconv.HelpRequest{Text: env.newFlags(commandVersion, "").Usage()}})
	case command == commandProve:
		return env.finish(env.prove(rest))
	case command == commandRequest:
		return env.finish(env.request(rest))
	case command == commandSystemOne:
		return env.finish(env.systemOne(rest))
	case command == commandDoctor:
		return env.finish(env.doctor(rest))
	case command == commandInit:
		return env.finish(env.init(rest))
	case command == commandEnable:
		return env.finish(env.enable(rest))
	case command == commandDisable:
		return env.finish(env.disable(rest))
	case command == commandHost:
		return env.finish(env.host(rest))
	case command == commandMCP:
		return env.finish(env.mcp(rest))
	case command == commandHook:
		return env.finish(env.hook(rest))
	default:
		err = fmt.Errorf("unknown command %q", command)
	}
	fmt.Fprintf(env.Stderr, "%s: %v\n", env.Binary, err)
	return env.usage()
}

func (e Env) usage() int {
	fmt.Fprint(e.Stderr, cliconv.CommandsUsage(e.Binary, commands))
	return ExitToolFailure
}

// help answers a request for help: the usage of the binary, or the one of the
// command that `help` names.
func (e Env) help(args []string) int {
	if args[0] == cliconv.HelpCommand && len(args) > 1 {
		return Run(e, []string{args[1], flagHelp})
	}
	fmt.Fprint(e.Stdout, cliconv.CommandsUsage(e.Binary, commands))
	return cliconv.ExitHelp
}

// outcome is what a command came to: its exit code and the error that explains a
// failure.
type outcome struct {
	code int
	err  error
}

// refusal is an error the protocol or the user caused: it exits with ExitBlocked,
// not as a tool failure.
type refusal struct{ error }

func (e Env) finish(result outcome) int {
	if result.err != nil {
		if code, handled := cliconv.Handle(result.err, e.Binary, e.Stdout, e.Stderr); handled {
			return code
		}
		fmt.Fprintf(e.Stderr, "%s: %v\n", e.Binary, result.err)
		var refused refusal
		if errors.As(result.err, &refused) {
			return ExitBlocked
		}
		return ExitToolFailure
	}
	return result.code
}

func (e Env) lookupEnv(name string) (string, bool) {
	if e.LookupEnv == nil {
		return "", false
	}
	return e.LookupEnv(name)
}

func (e Env) lookup(name string) string {
	value, _ := e.lookupEnv(name)
	return value
}
