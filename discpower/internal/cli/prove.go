package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/recordings"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/requests"
)

// Flags of `prove` and of the commands that share its project flag.
const (
	flagChanged = "changed"
	flagAll     = "all"
	flagResume  = "resume"
	flagRecord  = "record"
	flagReplay  = "replay"
	flagRoot    = "root"
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
	changed := set.Bool(flagChanged, false, "prove the criteria whose inputs changed (the default)")
	all := set.Bool(flagAll, false, "prove every criterion")
	resume := set.Bool(flagResume, false, "continue a run that is awaiting the host")
	record := set.String(flagRecord, "", "directory to record the consumed answers in")
	replay := set.String(flagReplay, "", "directory of recorded answers to serve the requests from")
	root := set.String(flagRoot, "", "project root (default: the working directory)")
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
	return e.runProve(*root, prove.Options{Selection: selection, Resume: *resume}, *record, *replay)
}

func (e Env) runProve(rootFlag string, options prove.Options, record, replay string) outcome {
	project, err := e.openProject(rootFlag)
	if err != nil {
		return outcome{err: err}
	}
	defer project.close()
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
	result, err := prove.UseCase{Deps: project.deps, Settings: project.proveSettings()}.Run(context.Background(), options)
	if err != nil {
		return outcome{err: err}
	}
	printResult(e.Stdout, result)
	return outcome{code: result.ExitCode()}
}

func printResult(out io.Writer, result prove.Result) {
	fmt.Fprintf(out, "wave %d\n", result.Wave)
	for _, criterion := range result.Criteria {
		fmt.Fprintf(out, "%s %s %s\n", criterion.Ref, criterion.Status, criterion.Outcome.Result)
		for _, finding := range criterion.Outcome.Findings {
			fmt.Fprintf(out, "  finding %s\n", finding)
		}
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(out, "module finding %s\n", finding)
	}
	if result.PendingTickets > 0 {
		fmt.Fprintf(out, "awaiting_host %d tickets\n", result.PendingTickets)
	}
}
