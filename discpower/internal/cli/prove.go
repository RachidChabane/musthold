package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/recordings"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/responsecheck"
	"github.com/RachidChabane/musthold/discpower/internal/app/prove"
	"github.com/RachidChabane/musthold/format/cliconv"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// Flags of `prove` and of the commands that share its project flag.
const (
	flagChanged = "changed"
	flagAll     = "all"
	flagResume  = "resume"
	flagRecord  = "record"
	flagReplay  = "replay"
	flagRoot    = "root"
	flagCI      = "ci"
	flagImport  = "import-host"
)

// Usage errors of `prove`: a command line the binary cannot act on is a tool
// failure, never a verdict.
var (
	errChangedAndAll    = errors.New("--changed and --all exclude each other")
	errRecordAndReplay  = errors.New("--record and --replay exclude each other")
	errNothingToResume  = errors.New("nothing to resume: the prove session holds no request")
	errUnexpectedParams = errors.New("unexpected arguments")
	errImportWithoutCI  = errors.New("--import-host needs --ci")
	errCIAndReplay      = errors.New("--ci excludes --record and --replay")
)

// Usage texts and arguments the flag sets of several commands share.
const (
	usageRoot      = "project root (default: the working directory)"
	usageNonce     = "the nonce of the ticket"
	usageCap       = "the capability of the executor"
	usageSource    = "the model and endpoint the answers come from"
	argumentTicket = "<ticket>"
)

// newFlags builds the flag set of a command; it takes the given positional
// arguments besides its flags.
func (e Env) newFlags(name, arguments string) *cliconv.FlagSet {
	return cliconv.NewFlagSet(e.Binary+" "+name, arguments)
}

// parseInterleaved parses flags that may follow positional arguments, as
// `request submit <ticket> --nonce N` has them, and returns the positionals.
func parseInterleaved(set *cliconv.FlagSet, args []string) ([]string, error) {
	return set.ParseInterleaved(args)
}

func (e Env) prove(args []string) outcome {
	set := e.newFlags(commandProve, "")
	changed := set.Bool(flagChanged, false, "prove the criteria whose inputs changed (the default)")
	all := set.Bool(flagAll, false, "prove every criterion")
	resume := set.Bool(flagResume, false, "continue a run that is awaiting the host")
	record := set.String(flagRecord, "", "directory to record the consumed answers in")
	replay := set.String(flagReplay, "", "directory of recorded answers to serve the requests from")
	root := set.String(flagRoot, "", usageRoot)
	ci := set.Bool(flagCI, false, "run as CI does: draw the seed of the commit and mark the Statements as produced by CI")
	importDir := set.String(flagImport, "", "with --ci: directory holding the bundle a push published")
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
	case *importDir != "" && !*ci:
		return outcome{err: errImportWithoutCI}
	case *ci && (*record != "" || *replay != ""):
		return outcome{err: errCIAndReplay}
	}
	selection := prove.SelectChanged
	if *all {
		selection = prove.SelectAll
	}
	return e.runProve(*root, prove.Options{Selection: selection, Resume: *resume}, *record, *replay, ciRun{enabled: *ci, importDir: *importDir})
}

func (e Env) runProve(rootFlag string, options prove.Options, record, replay string, ci ciRun) outcome {
	project, err := e.openProject(rootFlag)
	if err != nil {
		return outcome{err: err}
	}
	defer project.close()
	if options.Resume {
		if err := e.witnessClassifications(mustRoot(e, rootFlag)); err != nil {
			return outcome{err: err}
		}
		items, err := project.store.Items()
		if err != nil {
			return outcome{err: err}
		}
		if !hostproto.SessionProve.HoldsAny(items) {
			return outcome{err: errNothingToResume}
		}
	}
	if record != "" {
		project.deps.Record = recordings.Directory{Dir: record}
	}
	if replay != "" {
		project.deps.Replay = requests.RecordingReplayer{
			Service:    requests.Service{Plugin: pluginName, Store: project.store, Validator: responsecheck.New(), Clock: e.Now},
			Recordings: recordings.Directory{Dir: replay},
		}
	}
	settings, err := project.proveSettings()
	if err != nil {
		return outcome{err: err}
	}
	finish, err := e.prepareCI(project, &settings, ci)
	if err != nil {
		return outcome{err: err}
	}
	result, err := prove.UseCase{Deps: project.deps, Settings: settings}.Run(context.Background(), options)
	if err != nil {
		return outcome{err: err}
	}
	printResult(e.Stdout, result)
	if err := finish(result); err != nil {
		return outcome{err: err}
	}
	return outcome{code: result.ExitCode()}
}

func printResult(out io.Writer, result prove.Result) {
	fmt.Fprintf(out, "wave %d\n", result.Wave)
	for _, criterion := range result.Criteria {
		fmt.Fprintln(out, cliconv.Line(fmt.Sprint(criterion.Ref), fmt.Sprint(criterion.Status), fmt.Sprint(criterion.Outcome.Result)))
		for _, finding := range criterion.Outcome.Findings {
			fmt.Fprintf(out, "  finding %s\n", finding)
		}
		for _, survivor := range criterion.Survivors {
			for _, line := range survivorLines(survivor) {
				fmt.Fprintln(out, line)
			}
		}
	}
	for _, finding := range result.Findings {
		fmt.Fprintf(out, "module finding %s\n", finding)
	}
	if result.PendingTickets > 0 {
		fmt.Fprintf(out, "awaiting_host %d tickets\n", result.PendingTickets)
	}
}
