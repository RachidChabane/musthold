package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/capacity"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ghpr"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/gitsource"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/localcache"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/niceness"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scansources"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/spec-proof/discpower/internal/version"
	"github.com/RachidChabane/spec-proof/format"
)

// The scan command and its verb.
const (
	commandScan  = "scan"
	verbReplay   = "replay"
	argumentDir  = "<dir>"
	scanSynopsis = "usage: spec-proof-dp scan [--full] [--host [--resume]] [flags] [<PR>]\n       spec-proof-dp scan replay <dir> [--root <dir>]\n"
)

// Flags of `scan`.
const (
	flagBase    = "base"
	flagSeed    = "seed"
	flagSample  = "sample"
	flagMutants = "mutants"
	flagBudget  = "budget"
	flagWorkers = "workers"
	flagFormat  = "format"
	// flagCoverageTimeout bounds each run of tests under coverage.
	flagCoverageTimeout = "coverage-timeout"
	// flagFull asks for the full scan in place of the quick one.
	flagFull = "full"
	// flagHost asks the host for the promises of the code.
	flagHost = "host"
)

// Usage of the flags of a scan that asks a host.
const (
	hostUsage   = "ask the host for the promises of the code, through the exit-3 loop of the scan skill (a scan without it calls no model)"
	resumeUsage = "continue the scan session that awaits the host; implies --host and fails when no open session has this scan's binding"
)

// Output formats of `scan`.
const (
	formatText     = "text"
	formatMarkdown = "md"
	formatJSON     = "json"
)

// coverageTimeoutUsage describes --coverage-timeout and the bound it replaces.
var coverageTimeoutUsage = fmt.Sprintf("bound each run of tests under coverage (0: %d times one reference run of the suite, between %s and %s)",
	scan.CoverageTimeoutFactor, scan.MinCoverageTimeout, scan.MaxCoverageTimeout)

// Usage of the flags a depth presets.
var (
	fullUsage = fmt.Sprintf("judge %d tests on %d mutants with the coverage of every green test and no budget, in place of the quick scan of %d tests within %s",
		scanscope.DefaultSampleSize, scanscope.DefaultMutants, scan.QuickSampleSize, scan.QuickBudget)
	sampleUsage  = fmt.Sprintf("how many green tests to sample (quick: %d, full: %d)", scan.QuickSampleSize, scanscope.DefaultSampleSize)
	mutantsUsage = fmt.Sprintf("how many operator mutants to draw (quick: %d, full: %d)", scan.QuickMutants, scanscope.DefaultMutants)
	budgetUsage  = fmt.Sprintf("run no mutant once this long has passed since the scan began, with partial counts (quick: %s, full: no limit; 0: no limit)", scan.QuickBudget)
	workersUsage = fmt.Sprintf("how many mutants, or tests under coverage, run at once (0: from the processors and the memory above the guard's free share, at most %d)", scan.MaxAutoWorkers)
)

// scanNiceness is the niceness a scan and the test runs it starts take.
const scanNiceness = 10

// Errors of `scan`.
var (
	errScanEcosystem = errors.New("no ecosystem a scan runs was recognized: a scan runs pytest and Go test suites")
	errScanFormat    = errors.New("--format is one of text, md or json")
	errReplayDir     = errors.New("replay takes one directory: a finding folder or the folder of a scan")
	errNoFinding     = errors.New("no finding.json in the directory or its folders")
	errPullArgument  = fmt.Errorf("%w: scan takes at most one pull request number", errUnexpectedParams)
	errBaseAndPull   = errors.New("--base and a pull request number exclude each other: a pull request gives its own base")
)

func (e Env) scan(args []string) outcome {
	if len(args) > 0 && args[0] == verbReplay {
		return e.scanReplay(args[1:])
	}
	set := e.newFlags(commandScan, "").WithSynopsis(scanSynopsis)
	base := set.String(flagBase, "", "scan a change: the tests that run the functions changed since this revision")
	seed := set.Uint64(flagSeed, scanscope.DefaultSeed, "seed of every draw of the scan")
	full := set.Bool(flagFull, false, fullUsage)
	sample := set.Int(flagSample, 0, sampleUsage)
	mutants := set.Int(flagMutants, 0, mutantsUsage)
	budget := set.Duration(flagBudget, 0, budgetUsage)
	workers := set.Int(flagWorkers, 0, workersUsage)
	coverageTimeout := set.Duration(flagCoverageTimeout, 0, coverageTimeoutUsage)
	outputFormat := set.String(flagFormat, formatText, "output: text, md (a pull-request comment) or json (the scan statement)")
	root := set.String(flagRoot, "", usageRoot)
	host := set.Bool(flagHost, false, hostUsage)
	resume := set.Bool(flagResume, false, resumeUsage)
	rest, err := parseInterleaved(set, args)
	if err != nil {
		return outcome{err: err}
	}
	pull, err := pullRequestOf(rest)
	switch {
	case err != nil:
		return outcome{err: err}
	case pull > 0 && *base != "":
		return outcome{err: errBaseAndPull}
	case *outputFormat != formatText && *outputFormat != formatMarkdown && *outputFormat != formatJSON:
		return outcome{err: errScanFormat}
	}
	asksHost := *host || *resume
	wired, err := e.openScan(*root, asksHost)
	if err != nil {
		return outcome{err: err}
	}
	defer wired.close()
	niceness.Lower(scanNiceness)
	request := depthRequest(*full, givenFlags(set.FlagSet), scan.Preset{SampleSize: *sample, Mutants: *mutants, Budget: *budget})
	request.Root, request.Base, request.Seed, request.Workers, request.CoverageTimeout = wired.root, *base, *seed, *workers, *coverageTimeout
	request.PullRequest, request.Resume = pull, *resume
	request.Module = format.Module{Name: moduleName, Version: version.Version}
	if asksHost {
		return e.hostScan(wired, request, *outputFormat)
	}
	report, err := wired.service.Run(context.Background(), request)
	if err != nil {
		return outcome{err: scanFailure(err)}
	}
	return e.reportScan(wired.root, report, *outputFormat)
}

// pullRequestOf reads the one positional argument a scan takes: the number of
// a pull request, or none.
func pullRequestOf(positionals []string) (int, error) {
	switch {
	case len(positionals) == 0:
		return 0, nil
	case len(positionals) > 1:
		return 0, errPullArgument
	}
	number, err := strconv.Atoi(positionals[0])
	if err != nil || number <= 0 {
		return 0, errPullArgument
	}
	return number, nil
}

// hostScan runs one invocation of a scan that asks the host: while tickets
// wait it prints the wave, the session and the tickets and exits 3; once the
// scan finished it prints its only headline and writes its reports.
func (e Env) hostScan(wired scanProject, request scan.Request, outputFormat string) outcome {
	result, err := wired.service.RunHost(context.Background(), request)
	if err != nil {
		return outcome{err: scanFailure(err)}
	}
	if !result.Finished {
		printAwaiting(e.Stdout, result)
		return outcome{code: result.ExitCode()}
	}
	return e.reportScan(wired.root, result.Report, outputFormat)
}

// printAwaiting prints what a scan waits for, as `prove` does: the wave, the
// session whose tickets the skill relays, and how many tickets wait.
func printAwaiting(out io.Writer, result scan.HostResult) {
	fmt.Fprintf(out, "wave %d\n", result.Wave)
	fmt.Fprintf(out, "session %s\n", result.Session)
	fmt.Fprintf(out, "awaiting_host %d tickets\n", len(result.Pending))
}

// reportScan writes the reports of a finished scan and prints it in the
// chosen format.
func (e Env) reportScan(root string, report scan.Report, outputFormat string) outcome {
	files, err := scanreport.Files(report)
	if err != nil {
		return outcome{err: err}
	}
	if err := scanreport.Write(root, report, files); err != nil {
		return outcome{err: err}
	}
	switch outputFormat {
	case formatMarkdown:
		fmt.Fprint(e.Stdout, scanreport.Markdown(report))
	case formatJSON:
		fmt.Fprintln(e.Stdout, string(report.Encoded))
	default:
		fmt.Fprint(e.Stdout, scanreport.Text(report))
	}
	return outcome{code: ExitClean}
}

// depthRequest is the request of a depth, quick unless full: the preset of
// the depth, with the sample, the mutants and the budget the command line
// gave in place of the preset's.
func depthRequest(full bool, given map[string]bool, flags scan.Preset) scan.Request {
	depth := scan.DepthQuick
	if full {
		depth = scan.DepthFull
	}
	preset := scan.PresetOf(depth)
	request := scan.Request{SampleSize: preset.SampleSize, Mutants: preset.Mutants, Budget: preset.Budget, Coverage: preset.Coverage, Depth: depth}
	if given[flagSample] {
		request.SampleSize = flags.SampleSize
	}
	if given[flagMutants] {
		request.Mutants = flags.Mutants
	}
	if given[flagBudget] {
		request.Budget = flags.Budget
	}
	return request
}

// givenFlags names the flags the command line set.
func givenFlags(set *flag.FlagSet) map[string]bool {
	given := map[string]bool{}
	set.Visit(func(f *flag.Flag) { given[f.Name] = true })
	return given
}

// scanFailure is the error a failed scan reports: a coverage run past its
// bound names the flag that raises it.
func scanFailure(err error) error {
	if errors.Is(err, ports.ErrCoverageTimedOut) {
		return fmt.Errorf("%w; raise the bound with --%s", err, flagCoverageTimeout)
	}
	return err
}

func (e Env) scanReplay(args []string) outcome {
	set := e.newFlags(commandScan+" "+verbReplay, argumentDir)
	root := set.String(flagRoot, "", usageRoot)
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) != 1:
		return outcome{err: errReplayDir}
	}
	records, err := findingRecords(rest[0])
	if err != nil {
		return outcome{err: err}
	}
	wired, err := e.openScan(*root, false)
	if err != nil {
		return outcome{err: err}
	}
	defer wired.close()
	niceness.Lower(scanNiceness)
	code := ExitClean
	for _, record := range records {
		replay, err := wired.service.Replay(context.Background(), wired.root, record)
		if err != nil {
			return outcome{err: err}
		}
		fmt.Fprintln(e.Stdout, replay.Test)
		for _, check := range replay.Checks {
			fmt.Fprintf(e.Stdout, "  %s %s\n", replayMark(check.Held), check.Claim)
		}
		if !replay.Held() {
			code = ExitBlocked
		}
	}
	return outcome{code: code}
}

// Marks of a replayed claim.
const (
	markHeld    = "held:"
	markNotHeld = "NOT held:"
)

func replayMark(held bool) string {
	if held {
		return markHeld
	}
	return markNotHeld
}

// findingFile is the record a finding folder holds.
const findingFile = "finding.json"

// findingRecords reads the finding of a finding folder, or every finding of the
// folders of a scan, in the order of their names.
func findingRecords(dir string) ([]scan.FindingRecord, error) {
	paths := []string{filepath.Join(dir, findingFile)}
	if _, err := os.Stat(paths[0]); err != nil {
		matches, globErr := filepath.Glob(filepath.Join(dir, "*", findingFile))
		if globErr != nil || len(matches) == 0 {
			return nil, errNoFinding
		}
		paths = matches
	}
	var records []scan.FindingRecord
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", path, err)
		}
		record, err := scan.DecodeFinding(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		records = append(records, record)
	}
	return records, nil
}

// scanProject is what a scan wires together for one work tree.
type scanProject struct {
	root       string
	service    *scan.Service
	workspaces *workspace.Manager
}

func (p scanProject) close() { _ = p.workspaces.Close() }

// openScan wires a scan of the work tree: the first detected ecosystem a scan
// runs, its prober, form compiler and witness host when it has them, git, the
// pull requests through gh, and workspaces outside the tree; a scan that asks
// the host also gets the host store, the memo of its witness facts and the
// store of its sources. A replay asks no host and holds neither.
func (e Env) openScan(rootFlag string, asksHost bool) (scanProject, error) {
	root, err := e.rootOf(rootFlag)
	if err != nil {
		return scanProject{}, err
	}
	runner := process.New()
	strategies, err := newStrategies(runner, junitxml.NewReader())
	if err != nil {
		return scanProject{}, err
	}
	strategy, suite, err := scanStrategy(strategies, root)
	if err != nil {
		return scanProject{}, err
	}
	engines, err := newEngines()
	if err != nil {
		return scanProject{}, err
	}
	workspaces, err := workspace.New(os.TempDir())
	if err != nil {
		return scanProject{}, fmt.Errorf("create the workspace manager: %w", err)
	}
	repository := gitsource.Repository{Process: runner, Root: root}
	project := os.DirFS(root)
	deps := scan.Deps{
		Ecosystem: strategy, Suite: suite, Workspaces: workspaces, Engines: engines,
		Source: repository, Project: project, Clock: e.Now,
		Capacity:     capacity.New(),
		PullRequests: ghpr.Client{Process: runner, Repository: repository, Dir: root},
	}
	if compiling, ok := strategy.(ports.Compiling); ok {
		deps.Compiler = compiling.Compiler()
	}
	if probing, ok := strategy.(ports.Probing); ok {
		deps.Prober = probing.Prober()
	}
	wireWitnesses(&deps, strategy, project)
	if asksHost {
		if deps.Host, err = e.scanHost(root, repository); err != nil {
			return scanProject{}, err
		}
	}
	return scanProject{root: root, service: scan.New(deps), workspaces: workspaces}, nil
}

// wireWitnesses gives the scan the ecosystem's witness host and the runner of
// its witnesses, with the ecosystem's own runner of witnesses and its coverage
// recorder when it has them.
func wireWitnesses(deps *scan.Deps, strategy ports.Ecosystem, project fs.FS) {
	witnessing, ok := strategy.(ports.Witnessing)
	if !ok {
		return
	}
	deps.WitnessHost = witnessing.Witnesses()
	witnesses := scan.ExecutionWitnesses{Host: deps.WitnessHost, Project: project, Recorder: witnessRecorder(strategy)}
	if running, ok := strategy.(ports.WitnessRunning); ok {
		witnesses.Runner = running.WitnessRunner()
	}
	deps.Witnesses = witnesses
}

// witnessRecorder is the recorder of the coverage of the witnesses: the one
// that runs the witnesses alone when the ecosystem has it, else the one that
// records the whole suite, nil for an ecosystem with neither.
func witnessRecorder(strategy ports.Ecosystem) ports.CoverageRecorder {
	if covering, ok := strategy.(ports.WitnessCovering); ok {
		return covering.WitnessCoverage()
	}
	if covering, ok := strategy.(ports.Covering); ok {
		return covering.Coverage()
	}
	return nil
}

// scanHost wires the ports of a scan that asks the host: the repository's host
// store, shared with `prove`, the memo of the witness facts, the sources
// folder and the README of the head commit.
func (e Env) scanHost(root string, repository gitsource.Repository) (*scan.HostDeps, error) {
	store, err := e.hostStore(root)
	if err != nil {
		return nil, err
	}
	return &scan.HostDeps{
		Store: store, Memo: localcache.NewMemo(xdg.CacheDir(e.lookupEnv, e.HomeDir, root)),
		Sources: scansources.Store{Root: root}, Head: repository, Entropy: e.Entropy, Plugin: pluginName,
	}, nil
}

// scanStrategy is the first ecosystem detected in the tree that a scan runs.
func scanStrategy(strategies *ecosystem.Registry, root string) (ports.Ecosystem, ports.ScanSuite, error) {
	for _, candidate := range strategies.Detect(os.DirFS(root)) {
		if scanning, ok := candidate.Strategy.(ports.Scanning); ok {
			return candidate.Strategy, scanning.Scan(), nil
		}
	}
	return nil, nil, errScanEcosystem
}
