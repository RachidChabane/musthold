// Package cli is the composition root of the binary. It parses arguments and
// wires use cases to adapters; it holds no business rule.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/ledger/internal/app"
	"github.com/RachidChabane/musthold/ledger/internal/domain/refusal"
	"github.com/RachidChabane/musthold/ledger/internal/version"
)

// Exit codes of the binary (ADR 0002).
const (
	ExitClean       = 0
	ExitBlocked     = 1
	ExitToolFailure = 2
)

// helpFlag is the flag `help <command>` hands to the command.
const helpFlag = "-h"

// Commands of the binary.
const (
	versionCommand = "version"
	signCommand    = "sign"
	ledgerCommand  = "ledger"
)

// Subcommands of `ledger`.
const (
	appendSubcommand = "append"
	exportSubcommand = "export"
	auditSubcommand  = "audit"
	seedSubcommand   = "seed"

	fetchTrustSubcommand = "fetch-trust"
)

const usageText = `usage:
  %[1]s version
  %[1]s sign --identity ed25519|sigstore --in DIR --out DIR [--digests FILE] [--key-file FILE] [--fulcio-url URL] [--rekor-url URL]
  %[1]s ledger append --signed DIR [--mutants DIR] [--trust DIR] [--head-sha SHA --run-id ID] [--repo DIR] [--remote NAME]
  %[1]s ledger export --to DIR [--repo DIR]
  %[1]s ledger audit [--repo DIR]
  %[1]s ledger seed --head-sha SHA --run-id ID [--repo DIR]
  %[1]s ledger fetch-trust --out DIR [--mirror URL]
`

// Environment is what the binary reads from the machine it runs on.
type Environment struct {
	// Binary is the name the binary was called by; Run sets it.
	Binary    string
	LookupEnv func(string) (string, bool)
	Clock     app.Clock
	// GitEnvironment is appended to the environment of every git command.
	GitEnvironment []string
	// TUFRoot is the TUF root a trust snapshot is verified back to; nil means the
	// public-good root embedded in the library.
	TUFRoot []byte
}

// errUsage marks a command line that does not parse.
var errUsage = errors.New("usage")

// Main runs the binary called binary with the given arguments on the real machine
// and returns its exit code.
func Main(binary string, args []string, stdout, stderr io.Writer) int {
	return Run(Environment{LookupEnv: os.LookupEnv, Clock: app.SystemClock{}}, binary, args, stdout, stderr)
}

// Run runs the binary in an environment and returns its exit code.
func Run(environment Environment, binary string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == versionCommand {
		fmt.Fprintln(stdout, version.Line(binary))
		return ExitClean
	}
	environment.Binary = binary
	if cliconv.IsHelpRequest(args) {
		return help(binary, args, stdout, stderr, environment)
	}
	err := dispatch(environment, args, stdout, stderr)
	if code, handled := cliconv.Handle(err, binary, stdout, stderr); handled {
		return code
	}
	switch {
	case err == nil:
		return ExitClean
	case errors.Is(err, errUsage):
		fmt.Fprintf(stderr, "%v\n", err)
		fmt.Fprintf(stderr, usageText, binary)
		return ExitToolFailure
	case refusal.Is(err):
		fmt.Fprintf(stderr, "%s: %v\n", binary, err)
		return ExitBlocked
	default:
		fmt.Fprintf(stderr, "%s: %v\n", binary, err)
		return ExitToolFailure
	}
}

// help answers a request for help: the usage of the binary, or the one of the
// command that `help` names.
func help(binary string, args []string, stdout, stderr io.Writer, environment Environment) int {
	if args[0] == cliconv.HelpCommand && len(args) > 1 {
		return Run(environment, binary, []string{args[1], helpFlag}, stdout, stderr)
	}
	fmt.Fprintf(stdout, usageText, binary)
	return cliconv.ExitHelp
}

func dispatch(environment Environment, args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("%w: no command", errUsage)
	}
	command, rest := args[0], args[1:]
	switch command {
	case signCommand:
		return runSign(environment, rest, stdout, stderr)
	case ledgerCommand:
		return runLedger(environment, rest, stdout, stderr)
	default:
		return fmt.Errorf("%w: unknown command %q", errUsage, command)
	}
}

// newFlagSet returns the flag set of a command of the binary; it takes no
// argument besides its flags.
func newFlagSet(environment Environment, command string) *cliconv.FlagSet {
	return cliconv.NewFlagSet(environment.Binary+" "+command, "")
}

// parse parses the flags of a command; a request for help and a flag that does
// not parse come back as the errors of the convention.
func parse(flags *cliconv.FlagSet, args []string) error {
	rest, err := flags.ParseArguments(args)
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return cliconv.UsageError{Err: fmt.Errorf("%s: unexpected argument %q", flags.Name(), rest[0]), Text: flags.Usage()}
	}
	return nil
}

func requireFlag(flags *cliconv.FlagSet, name, value string) error {
	if value == "" {
		return fmt.Errorf("%w: %s: --%s is required", errUsage, flags.Name(), name)
	}
	return nil
}
