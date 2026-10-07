package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/recordings"
	"github.com/RachidChabane/spec-proof/completeness/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/app/requests"
)

// Flags of `prove` and of the commands that share its project flag.
const (
	flagChanged = "changed"
	flagAll     = "all"
	flagResume  = "resume"
	flagRecord  = "record"
	flagReplay  = "replay"
	flagRoot    = "root"
	flagBase    = "base"
)

// Usage errors of `prove`: a command line the binary cannot act on is a tool
// failure, never a verdict.
var (
	errChangedAndAll    = errors.New("--changed and --all exclude each other")
	errRecordAndReplay  = errors.New("--record and --replay exclude each other")
	errNothingToResume  = errors.New("nothing to resume: no request was ever emitted")
	errUnexpectedParams = errors.New("unexpected arguments")
)

// newFlags builds a flag set that reports to stderr and never exits.
func (e Env) newFlags(name string) *flag.FlagSet {
	set := flag.NewFlagSet(e.Binary+" "+name, flag.ContinueOnError)
	set.SetOutput(e.Stderr)
	return set
}

// parseInterleaved parses flags that may follow positional arguments, as
// `request submit <ticket> --nonce N` has them, and returns the positionals.
func parseInterleaved(set *flag.FlagSet, args []string) ([]string, error) {
	var positionals []string
	for {
		if err := set.Parse(args); err != nil {
			return nil, err
		}
		if set.NArg() == 0 {
			return positionals, nil
		}
		positionals = append(positionals, set.Arg(0))
		args = set.Args()[1:]
	}
}

func (e Env) prove(args []string) outcome {
	set := e.newFlags(commandProve)
	changed := set.Bool(flagChanged, false, "prove the specs whose inputs changed (the default)")
	all := set.Bool(flagAll, false, "prove every spec")
	resume := set.Bool(flagResume, false, "continue a run that is awaiting the host")
	record := set.String(flagRecord, "", "directory to record the consumed answers in")
	replay := set.String(flagReplay, "", "directory of recorded answers to serve the requests from")
	root := set.String(flagRoot, "", "project root (default: the working directory)")
	base := set.String(flagBase, "", "reference the change is measured from (default: the merge base with the default branch)")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	case *changed && *all:
		return outcome{err: errChangedAndAll}
	case *record != "" && *replay != "":
		return outcome{err: errRecordAndReplay}
	}
	selection := prove.SelectChanged
	if *all {
		selection = prove.SelectAll
	}
	return e.runProve(*root, prove.Options{Selection: selection, Resume: *resume, Base: *base}, *record, *replay)
}

func (e Env) runProve(rootFlag string, options prove.Options, record, replay string) outcome {
	project, err := e.openProject(rootFlag)
	if err != nil {
		return outcome{err: err}
	}
	if options.Resume {
		items, err := project.store.Items()
		if err != nil {
			return outcome{err: err}
		}
		if len(items) == 0 {
			return outcome{err: errNothingToResume}
		}
	}
	if record != "" {
		project.deps.Record = recordings.Directory{Dir: record}
	}
	if replay != "" {
		project.deps.Replay = requests.Replayer{
			Service:    requests.Service{Store: project.store, Validator: responsecheck.New(), Clock: e.Now},
			Recordings: recordings.Directory{Dir: replay},
		}
	}
	result, err := prove.UseCase{Deps: project.deps, Settings: project.settings}.Run(context.Background(), options)
	if err != nil {
		return outcome{err: err}
	}
	printResult(e.Stdout, result)
	return outcome{code: result.ExitCode()}
}

func printResult(out io.Writer, result prove.Result) {
	fmt.Fprintf(out, "wave %d\n", result.Wave)
	for _, spec := range result.Specs {
		fmt.Fprintf(out, "%s %s %d questions", spec.Spec, spec.Status, spec.Questions)
		if spec.Unresponsive {
			fmt.Fprint(out, " host_unresponsive")
		}
		fmt.Fprintln(out)
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(out, "module finding %s\n", finding)
	}
	if result.PendingTickets > 0 {
		fmt.Fprintf(out, "awaiting_host %d tickets\n", result.PendingTickets)
	}
}
