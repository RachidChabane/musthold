package golang

import (
	"context"
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
)

var _ ports.Scanning = (*Strategy)(nil)

const (
	countFlagName = "-count="
	// testCaseSeparator joins the test file and the subtest path of a test id.
	testCaseSeparator = "::"
	testFileSuffix    = "_test.go"
	scanReproTool     = "go"
	// eventsSuffix names the file that receives the events of one run, whole
	// however much the tests log.
	eventsSuffix = ".jsonl"
	// oneRun is the count of a process that gives each test one observation.
	oneRun = 1
)

// Scan runs the suite for a scan with `go test -json`, and its line coverage
// with test binaries built with coverage in the workspace.
func (s *Strategy) Scan() ports.ScanSuite { return scanSuite{deps: s.deps} }

type scanSuite struct {
	deps Dependencies
}

// Run runs every package, or the packages of the selection with a `-run`
// expression that selects exactly their subtest paths, with `-count` set to the
// number of runs, within the timeout of one run that many times over. When
// that process exits non-zero without timing out before a test of a package
// had every run, the process is one observation of each test of the package,
// and the package runs again in fresh processes of one run each, within the
// timeout of one run, until every test has the requested number of
// observations; no process starts after one timed out. Each run of a leaf is
// read against its ancestors' end events of the same run (see
// testProcess.paired); the leaves of a package with an ambiguous pass and no
// failure run again together, without the others, in one fresh process (see
// rerunAmbiguous).
func (s scanSuite) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return ports.ScanRuns{}, err
	}
	batches, err := batchesOf(request.Tests)
	if err != nil {
		return ports.ScanRuns{}, err
	}
	locator := testLocator{deps: s.deps, workspace: workspace}
	result := ports.ScanRuns{Runs: map[ecosystem.TestID][]vacuity.Run{}}
	for _, batch := range batches {
		first, err := s.runProcess(ctx, workspace, toolchain, batch, request.Runs, request.Timeout*time.Duration(max(request.Runs, oneRun)))
		if err != nil {
			return ports.ScanRuns{}, err
		}
		wanted := batch.tests
		if len(wanted) == 0 {
			for _, key := range sortedKeys(first.leaves) {
				id, file := locator.id(module, key)
				result.Cases = append(result.Cases, ports.ScanCase{ID: id, File: file})
				wanted = append(wanted, id)
			}
		}
		for _, group := range byPackage(wanted, module) {
			var observed groupRuns
			if first.cutShort(group.keys, request.Runs) {
				if observed, err = s.observeApart(ctx, workspace, toolchain, batch.of(group.pkg), group, first, request); err != nil {
					return ports.ScanRuns{}, err
				}
			} else {
				observed = first.groupRuns(group, request.Runs)
			}
			if err := s.rerunAmbiguous(ctx, workspace, toolchain, group, observed, request); err != nil {
				return ports.ScanRuns{}, err
			}
			for test, runs := range observed.runs {
				result.Runs[test] = runs
			}
		}
	}
	return result, nil
}

// groupRuns is what the processes of a package gave its tests: the runs of
// each test, the tests with an ambiguous pass, and whether a process timed
// out.
type groupRuns struct {
	runs      map[ecosystem.TestID][]vacuity.Run
	ambiguous map[ecosystem.TestID]bool
	timedOut  bool
}

func newGroupRuns(group packageGroup) groupRuns {
	return groupRuns{runs: make(map[ecosystem.TestID][]vacuity.Run, len(group.tests)), ambiguous: map[ecosystem.TestID]bool{}}
}

// groupRuns gives each test of the group its runs of this process.
func (p testProcess) groupRuns(group packageGroup, runs int) groupRuns {
	observed := newGroupRuns(group)
	observed.timedOut = p.timedOut
	for index, test := range group.tests {
		observed.runs[test], observed.ambiguous[test] = p.runsOf(group.keys[index], runs)
	}
	return observed
}

// observeApart gives each test of the group the observation of the first
// process, then runs the batch again, one run per fresh process, until every
// test has the requested number of observations. A process that times out
// leaves every later observation timed out, and no later process starts.
func (s scanSuite) observeApart(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, rerun batch, group packageGroup, first testProcess, request ports.ScanRunRequest) (groupRuns, error) {
	observed := newGroupRuns(group)
	process := first
	for observation := range request.Runs {
		if observation > 0 {
			if process.timedOut {
				for _, test := range group.tests {
					observed.runs[test] = append(observed.runs[test], vacuity.RunTimedOut)
				}
				continue
			}
			var err error
			if process, err = s.runProcess(ctx, workspace, toolchain, rerun, oneRun, request.Timeout); err != nil {
				return groupRuns{}, err
			}
		}
		observed.timedOut = observed.timedOut || process.timedOut
		for index, test := range group.tests {
			run, ambiguous := process.observation(group.keys[index])
			observed.runs[test] = append(observed.runs[test], run)
			observed.ambiguous[test] = observed.ambiguous[test] || ambiguous
		}
	}
	return observed, nil
}

// rerunAmbiguous runs again, unless a process of the group timed out, the tests
// of the group that had an ambiguous pass and failed no run: together, without
// the others, in one fresh process with `-count` set to the number of runs,
// within the timeout of one run that many times over. That process gives them
// every run, each read against its ancestors as in any process, an ambiguous
// pass now counting as errored. When it exits non-zero without timing out
// before a test had every run, it gives each test one observation and
// errored runs.
func (s scanSuite) rerunAmbiguous(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, group packageGroup, observed groupRuns, request ports.ScanRunRequest) error {
	if observed.timedOut {
		return nil
	}
	rerun := batch{pattern: group.pkg}
	var keys []resultKey
	for index, test := range group.tests {
		if observed.ambiguous[test] && !slices.Contains(observed.runs[test], vacuity.RunFailed) {
			rerun.tests = append(rerun.tests, test)
			rerun.paths = append(rerun.paths, strings.Split(group.keys[index].test, subtestSeparator))
			keys = append(keys, group.keys[index])
		}
	}
	if len(rerun.tests) == 0 {
		return nil
	}
	process, err := s.runProcess(ctx, workspace, toolchain, rerun, request.Runs, request.Timeout*time.Duration(max(request.Runs, oneRun)))
	if err != nil {
		return err
	}
	cutShort := process.cutShort(keys, request.Runs)
	for index, test := range rerun.tests {
		if !cutShort {
			observed.runs[test], _ = process.runsOf(keys[index], request.Runs)
			continue
		}
		run, _ := process.observation(keys[index])
		observed.runs[test] = append([]vacuity.Run{run}, slices.Repeat([]vacuity.Run{vacuity.RunErrored}, request.Runs-oneRun)...)
	}
	return nil
}

// runProcess runs one `go test -json` process of the batch, each test the given
// number of times, and reads its events.
func (s scanSuite) runProcess(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, batch batch, count int, timeout time.Duration) (testProcess, error) {
	argv := append(append([]string{}, toolchain.Command...), testSubcommand, jsonFlag, fmt.Sprintf("%s%d", countFlagName, count))
	if len(batch.paths) > 0 {
		argv = append(argv, runFlag, exactExpression(batch.paths))
	}
	argv = append(argv, batch.pattern)
	events := suiterun.ResultsFile(workspace, eventsSuffix)
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: timeout, StdoutFile: events})
	if err != nil {
		return testProcess{}, fmt.Errorf("run the suite: %w", err)
	}
	output, err := os.ReadFile(events)
	if err != nil {
		return testProcess{}, fmt.Errorf("read the test events: %w", err)
	}
	return readProcess(output, executed.ExitCode, executed.TimedOut), nil
}

// batch is one `go test` invocation: a package pattern and, for a selection,
// the subtest paths it runs and the tests it reads.
type batch struct {
	pattern string
	paths   [][]string
	tests   []ecosystem.TestID
}

// of is the batch that runs one package of this batch again: the batch itself
// for a selection, which holds one package, and the package alone for a run
// of every package.
func (b batch) of(pkg string) batch {
	if len(b.paths) > 0 {
		return b
	}
	return batch{pattern: pkg}
}

// batchesOf groups a selection by package directory; an empty selection is one
// batch over every package.
func batchesOf(tests []ecosystem.TestID) ([]batch, error) {
	if len(tests) == 0 {
		return []batch{{pattern: allPackages}}, nil
	}
	byDir := map[string]*batch{}
	var dirs []string
	for _, test := range tests {
		file, testPath, found := strings.Cut(string(test), testCaseSeparator)
		if !found || testPath == "" {
			return nil, fmt.Errorf("%w: %s", ErrUnmappedTest, test)
		}
		dir := path.Dir(file)
		if byDir[dir] == nil {
			byDir[dir] = &batch{pattern: packagePath(dir)}
			dirs = append(dirs, dir)
		}
		byDir[dir].paths = append(byDir[dir].paths, strings.Split(testPath, subtestSeparator))
		byDir[dir].tests = append(byDir[dir].tests, test)
	}
	slices.Sort(dirs)
	batches := make([]batch, 0, len(dirs))
	for _, dir := range dirs {
		batches = append(batches, *byDir[dir])
	}
	return batches, nil
}

// packageGroup is the tests of one package, each with its result key.
type packageGroup struct {
	pkg   string
	tests []ecosystem.TestID
	keys  []resultKey
}

// byPackage groups tests by the import path of their package, in order of
// import path, each group keeping the order of the tests.
func byPackage(tests []ecosystem.TestID, module string) []packageGroup {
	index := map[string]int{}
	var groups []packageGroup
	for _, test := range tests {
		key := keyOf(test, module)
		position, known := index[key.pkg]
		if !known {
			position = len(groups)
			index[key.pkg] = position
			groups = append(groups, packageGroup{pkg: key.pkg})
		}
		groups[position].tests = append(groups[position].tests, test)
		groups[position].keys = append(groups[position].keys, key)
	}
	slices.SortFunc(groups, func(a, b packageGroup) int { return strings.Compare(a.pkg, b.pkg) })
	return groups
}

func sortedKeys(results map[resultKey][]vacuity.Run) []resultKey {
	keys := make([]resultKey, 0, len(results))
	for key := range results {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b resultKey) int {
		if byPackage := strings.Compare(a.pkg, b.pkg); byPackage != 0 {
			return byPackage
		}
		return strings.Compare(a.test, b.test)
	})
	return keys
}

// keyOf gives the result key of a test id: the package of its file's directory
// and its subtest path.
func keyOf(test ecosystem.TestID, module string) resultKey {
	file, testPath, _ := strings.Cut(string(test), testCaseSeparator)
	return resultKey{pkg: importPathOf(module, path.Dir(file)), test: testPath}
}

// testLocator finds the test file that declares a top-level test function.
type testLocator struct {
	deps      Dependencies
	workspace ecosystem.Workspace
	declared  map[string]map[string]string
}

// id names a test by the file that declares its top-level function; a test
// whose declaration is not found is named after its package directory.
func (l *testLocator) id(module string, key resultKey) (ecosystem.TestID, string) {
	dir := rootDirectory
	if key.pkg != module {
		dir = strings.TrimPrefix(key.pkg, module+importPathSeparator)
	}
	top, _, _ := strings.Cut(key.test, subtestSeparator)
	file, found := l.files(dir)[top]
	if !found {
		file = path.Join(dir, testFileSuffix)
	}
	return ecosystem.TestID(file + testCaseSeparator + key.test), file
}

// files maps each top-level function name declared in the test files of a
// directory to its file.
func (l *testLocator) files(dir string) map[string]string {
	if l.declared == nil {
		l.declared = map[string]map[string]string{}
	}
	if declared, done := l.declared[dir]; done {
		return declared
	}
	declared := map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(l.workspace.Dir, filepath.FromSlash(dir)))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), testFileSuffix) {
			continue
		}
		file := path.Join(dir, entry.Name())
		source, err := os.ReadFile(filepath.Join(l.workspace.Dir, filepath.FromSlash(file)))
		if err != nil {
			continue
		}
		definitions, err := l.deps.Locator.Locate(file, source)
		if err != nil {
			continue
		}
		for _, definition := range definitions {
			if definition.Kind == ecosystem.KindFunction {
				_, name, _ := strings.Cut(definition.Qualname, qualnameDot)
				declared[name] = file
			}
		}
	}
	l.declared[dir] = declared
	return declared
}

// Operation binds an exported top-level function to `import/path:Function`;
// a method or an unexported function has no binding.
func (scanSuite) Operation(workspace ecosystem.Workspace, file, qualname string) (string, bool) {
	if path.Ext(file) != goSuffix || strings.HasSuffix(file, testFileSuffix) {
		return "", false
	}
	pkg, name, found := strings.Cut(qualname, qualnameDot)
	if !found || strings.Contains(name, qualnameDot) || pkg != packageNameOf(workspace, file) {
		return "", false
	}
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return "", false
	}
	target := importPathOf(module, path.Dir(file)) + targetSeparator + name
	if _, err := parseProbeTarget(target); err != nil {
		return "", false
	}
	return target, true
}

// packageNameOf is the package clause of a file of the workspace, which tells a
// function `pkg.Name` from a method `Type.Name` of a value receiver.
func packageNameOf(workspace ecosystem.Workspace, file string) string {
	parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(workspace.Dir, filepath.FromSlash(file)), nil, parser.PackageClauseOnly)
	if err != nil {
		return ""
	}
	return parsed.Name.Name
}

// Repro runs the one test with `go test`, anchored to its subtest path.
func (scanSuite) Repro(test ecosystem.TestID) []string {
	file, testPath, _ := strings.Cut(string(test), testCaseSeparator)
	return []string{scanReproTool, testSubcommand, countFlag, runFlag, expression([][]string{strings.Split(testPath, subtestSeparator)}), packagePath(path.Dir(file))}
}
