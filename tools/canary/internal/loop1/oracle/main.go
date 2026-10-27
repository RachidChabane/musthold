package oracle

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of Main.
const (
	exitOK     = 0
	exitFailed = 1
	exitUsage  = 2
)

const verbName = "oracle"

// Options are the paths Main resolved from its flags.
type Options struct {
	Repo, Config, Work string
}

// Wiring is what the verb needs from the rest of the tooling: the box that runs
// the project's commands, and the rule that ends the run phase.
type Wiring struct {
	Runner box.Runner
	Phase  Phase
}

// Command is the oracle verb; Wire builds its wiring from the resolved options.
type Command struct {
	Wire func(Options) (Wiring, error)
}

// Main runs the oracle verb. It returns 0 on success, 1 on a refused or failed
// check and 2 on a usage error.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	options, proceed, code := parseFlags(args, stderr)
	if !proceed {
		return code
	}
	wiring, err := c.Wire(options)
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
	flags.StringVar(&options.Repo, "repo", ".", "repository root")
	flags.StringVar(&options.Config, "config", study.ConfigFile, "the study's configuration, beside freeze/")
	flags.StringVar(&options.Work, "work", study.DefaultWork(), "work directory")
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
