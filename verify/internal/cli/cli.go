// Package cli is the composition root of the binary. It parses arguments and
// wires use cases to adapters; it holds no business rule.
package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/verify/internal/adapters/filesource"
	"github.com/RachidChabane/musthold/verify/internal/adapters/verifiedout"
	"github.com/RachidChabane/musthold/verify/internal/app"
	"github.com/RachidChabane/musthold/verify/internal/domain/refusal"
	"github.com/RachidChabane/musthold/verify/internal/version"
)

// Exit codes of the binary (ADR 0002): 0 verified, 1 a bundle or statement was
// rejected, 2 the command line does not parse or the tool failed.
const (
	ExitClean       = 0
	ExitRejected    = 1
	ExitToolFailure = 2
)

const versionCommand = "version"

// helpFlag is the flag the word `help` stands for.
const helpFlag = "-h"

// trustPolicyFileName is the name of the trust policy below the directory that holds
// the verified directory (`.musthold`).
const trustPolicyFileName = "trust-policy.yaml"

const usageText = "usage:\n  %[1]s version\n  %[1]s --bundles DIR [--accept-local DIR] [--offline] --out DIR\n  %[1]s --accept-local DIR --out DIR\n"

// errUsage marks a command line that does not parse.
var errUsage = errors.New("usage")

// Environment is what the binary is given besides its arguments.
type Environment struct {
	// TUFRoot is the TUF root every trusted root is verified back to; nil means
	// the public-good root embedded in the binary.
	TUFRoot []byte
}

// Main runs the binary called binary with the given arguments and returns its
// exit code.
func Main(binary string, args []string, stdout, stderr io.Writer) int {
	return Run(Environment{}, binary, args, stdout, stderr)
}

// Run runs the binary in an environment and returns its exit code.
func Run(environment Environment, binary string, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && args[0] == versionCommand {
		fmt.Fprintln(stdout, version.Line(binary))
		return ExitClean
	}
	if len(args) == 1 && args[0] == cliconv.HelpCommand {
		args = []string{helpFlag}
	}
	err := run(environment, binary, args, stdout)
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
		return ExitRejected
	default:
		fmt.Fprintf(stderr, "%s: %v\n", binary, err)
		return ExitToolFailure
	}
}

type options struct {
	bundles, acceptLocal, out string
	offline                   bool
}

func parseOptions(binary string, args []string) (options, error) {
	var opts options
	flags := cliconv.NewFlagSet(binary, "").WithSynopsis(fmt.Sprintf(usageText, binary))
	flags.StringVar(&opts.bundles, "bundles", "", "directory written by `ledger export`")
	flags.StringVar(&opts.acceptLocal, "accept-local", "", "local cache directory whose unsigned statements are admitted")
	flags.BoolVar(&opts.offline, "offline", false, "read the trusted root from the trust snapshot of --bundles, not from the network")
	flags.StringVar(&opts.out, "out", "", "verified directory to write")
	rest, err := flags.ParseArguments(args)
	if err != nil {
		return options{}, err
	}
	switch {
	case len(rest) > 0:
		return options{}, fmt.Errorf("%w: unexpected argument %q", errUsage, rest[0])
	case opts.out == "":
		return options{}, fmt.Errorf("%w: --out is required", errUsage)
	case opts.bundles == "" && opts.acceptLocal == "":
		return options{}, fmt.Errorf("%w: give --bundles, --accept-local or both", errUsage)
	case opts.offline && opts.bundles == "":
		return options{}, fmt.Errorf("%w: --offline reads the trust snapshot of --bundles", errUsage)
	}
	return opts, nil
}

func run(environment Environment, binary string, args []string, stdout io.Writer) error {
	opts, err := parseOptions(binary, args)
	if err != nil {
		return err
	}
	service := app.VerifyService{
		Version: version.Version,
		Bundles: app.NoBundles{},
		Local:   app.NoLocal{},
		Pending: filesource.PendingDir{Root: filepath.Join(filepath.Dir(opts.out), format.PendingDirectory)},
		Sink:    verifiedout.Directory{Path: opts.out},
	}
	if opts.acceptLocal != "" {
		service.Local = filesource.LocalCache{Root: opts.acceptLocal}
	}
	if opts.bundles != "" {
		trust, err := newBundleTrust(environment, opts, filepath.Join(filepath.Dir(opts.out), trustPolicyFileName))
		if err != nil {
			return err
		}
		service.Signed = true
		service.Bundles = filesource.ExportedLedger{Root: opts.bundles}
		service.Verifier = trust.verifier
		service.TrustedRootDigest = trust.trustedRootDigest
		service.PolicyDigest = trust.policyDigest
	}
	report, err := service.Run()
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "verified %d statements (%d signed, %d local), trust %s, %d pending, written to %s\n",
		report.Statements, report.Signed, report.Local, report.Trust, report.Pending, opts.out)
	return nil
}
