package oracle

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of Main.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

// Flags and environment of Main.
const (
	flagRepo       = "repo"
	flagWork       = "work"
	envWork        = "LOOP1_WORK"
	defaultRepo    = "."
	defaultWorkDir = ".musthold-loop-1"
	verbName       = "oracle"
)

// Options are the paths Main resolved from its flags.
type Options struct {
	Repo string
	Work string
}

// Wiring is what the verb needs from the rest of the tooling: the box that runs
// the project's commands, and the rule that ends the run phase.
type Wiring struct {
	Runner box.Runner
	Phase  PhaseEnded
}

// Wire builds the Wiring for Main. The orchestrator replaces it with the docker
// runner of the box and the analysis package's stopping rule.
var Wire = func(Options) (Wiring, error) { return Wiring{}, ErrNotWired }

// Main runs the oracle verb. It returns 0 on success, 1 on a refused or failed
// check and 2 on a usage error.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	options, proceed, code := parseFlags(args, stderr)
	if !proceed {
		return code
	}
	wiring, err := Wire(options)
	if err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verbName, err)
		return exitFailed
	}
	verb := NewVerb(options.Repo, study.Layout{Root: options.Work}, wiring.Runner, wiring.Phase, stdout)
	if err := verb.Execute(ctx); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verbName, err)
		return exitFailed
	}
	return exitOK
}

// parseFlags resolves the options; proceed is false when Main should return code at once.
func parseFlags(args []string, stderr io.Writer) (options Options, proceed bool, code int) {
	flags := flag.NewFlagSet(verbName, flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&options.Repo, flagRepo, defaultRepo, "repository root")
	flags.StringVar(&options.Work, flagWork, defaultWork(), "work directory")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return options, false, exitOK
		}
		return options, false, exitUsage
	}
	if flags.NArg() > 0 {
		fmt.Fprintf(stderr, "%s: unexpected argument %q\n", verbName, flags.Arg(0))
		return options, false, exitUsage
	}
	return options, true, exitOK
}

func defaultWork() string {
	if work := os.Getenv(envWork); work != "" {
		return work
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultWorkDir
	}
	return filepath.Join(home, defaultWorkDir)
}
