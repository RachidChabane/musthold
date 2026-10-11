package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/capacity"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/gitsource"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/niceness"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/scanreport"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
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
	scanSynopsis = "usage: spec-proof-dp scan [--full] [flags]\n       spec-proof-dp scan replay <dir> [--root <dir>]\n"
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
	budgetUsage  = fmt.Sprintf("stop starting mutants once this long has passed since the scan began, with partial counts (quick: %s, full: no limit; 0: no limit)", scan.QuickBudget)
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
	rest, err := set.ParseArguments(args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	case *outputFormat != formatText && *outputFormat != formatMarkdown && *outputFormat != formatJSON:
		return outcome{err: errScanFormat}
	}
	wired, err := e.openScan(*root)
	if err != nil {
		return outcome{err: err}
	}
	defer wired.close()
	niceness.Lower(scanNiceness)
	depth := scan.DepthQuick
	if *full {
		depth = scan.DepthFull
	}
	given := givenFlags(set.FlagSet)
	preset := scan.PresetOf(depth)
	request := scan.Request{
		Root: wired.root, Base: *base, Seed: *seed, SampleSize: preset.SampleSize, Mutants: preset.Mutants, Budget: preset.Budget,
		Coverage: preset.Coverage, Depth: depth, Workers: *workers, CoverageTimeout: *coverageTimeout,
		Module: format.Module{Name: moduleName, Version: version.Version},
	}
	if given[flagSample] {
		request.SampleSize = *sample
	}
	if given[flagMutants] {
		request.Mutants = *mutants
	}
	if given[flagBudget] {
		request.Budget = *budget
	}
	report, err := wired.service.Run(context.Background(), request)
	if err != nil {
		return outcome{err: scanFailure(err)}
	}
	files, err := scanreport.Files(report)
	if err != nil {
		return outcome{err: err}
	}
	if err := scanreport.Write(wired.root, report, files); err != nil {
		return outcome{err: err}
	}
	switch *outputFormat {
	case formatMarkdown:
		fmt.Fprint(e.Stdout, scanreport.Markdown(report))
	case formatJSON:
		fmt.Fprintln(e.Stdout, string(report.Encoded))
	default:
		fmt.Fprint(e.Stdout, scanreport.Text(report))
	}
	return outcome{code: ExitClean}
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
	wired, err := e.openScan(*root)
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
// runs, its prober and form compiler when it has them, git, and workspaces
// outside the tree.
func (e Env) openScan(rootFlag string) (scanProject, error) {
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
	deps := scan.Deps{
		Ecosystem: strategy, Suite: suite, Workspaces: workspaces, Engines: engines,
		Source: gitsource.Repository{Process: runner, Root: root}, Project: os.DirFS(root), Clock: e.Now,
		Capacity: capacity.New(),
	}
	if compiling, ok := strategy.(ports.Compiling); ok {
		deps.Compiler = compiling.Compiler()
	}
	if probing, ok := strategy.(ports.Probing); ok {
		deps.Prober = probing.Prober()
	}
	return scanProject{root: root, service: scan.New(deps), workspaces: workspaces}, nil
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
