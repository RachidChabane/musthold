package node

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/discpower/internal/platform/workerpool"
)

var _ ports.Scanning = (*Strategy)(nil)

// ErrBrowserMode is returned for a Vitest project that runs its tests in a
// browser: a scan records V8 coverage in Node.
var ErrBrowserMode = errors.New("musthold scan runs Vitest in Node: browser mode is not supported")

// The variables the shipped scripts read, and what a scan's runs set them to.
const (
	scanOutVariable       = "MUSTHOLD_SCAN_OUT"
	scanRootVariable      = "MUSTHOLD_SCAN_ROOT"
	scanSelectionVariable = "MUSTHOLD_SCAN_SELECTION"
	scanCoverageVariable  = "MUSTHOLD_SCAN_COVERAGE"
	scanHollowVariable    = "MUSTHOLD_SCAN_HOLLOW"
	v8CoverageVariable    = "NODE_V8_COVERAGE"
	enabledValue          = "1"
	selectionExtension    = ".json"
	scriptsDirMode        = 0o755
	scriptFileMode        = 0o644
	scriptsDirectory      = "scripts"
	// overlayBasePlaceholder is the line of the shipped Vitest configuration
	// that imports the project's own.
	overlayBasePlaceholder = "// musthold:base"
)

//go:embed scripts/scan-agent.cjs scripts/scan-jest-environment.cjs scripts/scan-jest-reporter.cjs scripts/scan-jest-sequencer.cjs scripts/scan-global-setup.cjs scripts/scan-vitest-runner.mjs scripts/scan-vitest-global-setup.mjs
var scanScripts embed.FS

//go:embed scripts/scan-vitest-config.mjs
var vitestOverlaySource string

// Scan runs the suite for a scan: whole or selected, each run one process of
// the runner under the shipped environment (Jest) or runner (Vitest), with
// the outcome of each test read by its file and titles.
func (s *Strategy) Scan() ports.ScanSuite {
	return scanSuite{deps: s.deps, framework: s.framework, cache: s.scanCache}
}

type scanSuite struct {
	deps      Dependencies
	framework framework
	cache     *scanCache
}

// scanProcess is one process of the runner a scan starts.
type scanProcess struct {
	// tests is the selection; empty runs the whole suite.
	tests []ecosystem.TestID
	// coverage records the V8 coverage of the run; hollow replaces the test
	// function of each selected test by one that does nothing.
	coverage, hollow bool
	// v8Coverage records the run's coverage through Node's NODE_V8_COVERAGE,
	// for a process that cannot open an inspector session.
	v8Coverage bool
	timeout    time.Duration
}

// ranProcess is what one process left: its record and, for a recording
// through NODE_V8_COVERAGE, the directory Node wrote into.
type ranProcess struct {
	record   runRecord
	timedOut bool
	v8Dir    string
}

// runProcess runs the runner once under the shipped scripts, written into a
// fresh directory of the workspace's results, and reads the record the run
// left in another.
func (s scanSuite) runProcess(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, process scanProcess) (ranProcess, error) {
	scripts, err := installScanScripts(workspace)
	if err != nil {
		return ranProcess{}, err
	}
	out := suiterun.ResultsFile(workspace, "")
	if err := os.MkdirAll(out, scriptsDirMode); err != nil {
		return ranProcess{}, fmt.Errorf("create the scan record directory: %w", err)
	}
	env := append(quietEnvironment(), ciVariable,
		scanOutVariable+environmentAssignment+out, scanRootVariable+environmentAssignment+realDirectory(workspace.Dir))
	if len(process.tests) > 0 {
		selection, err := writeScanSelection(workspace, process.tests)
		if err != nil {
			return ranProcess{}, err
		}
		env = append(env, scanSelectionVariable+environmentAssignment+selection)
	}
	if process.coverage {
		env = append(env, scanCoverageVariable+environmentAssignment+enabledValue)
	}
	if process.hollow {
		env = append(env, scanHollowVariable+environmentAssignment+enabledValue)
	}
	ran := ranProcess{}
	if process.v8Coverage {
		ran.v8Dir = suiterun.ResultsFile(workspace, "")
		env = append(env, v8CoverageVariable+environmentAssignment+ran.v8Dir)
	}
	arguments, extra, err := s.framework.scanCommand(ctx, s, workspace, toolchain, scripts, testFiles(process.tests))
	if err != nil {
		return ranProcess{}, err
	}
	argv := append(append([]string{}, toolchain.Command...), arguments...)
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Env: append(env, extra...), Timeout: process.timeout})
	if err != nil {
		return ranProcess{}, fmt.Errorf("run the suite: %w", err)
	}
	if bytes.Contains(executed.Stdout, []byte(ErrBrowserMode.Error())) || bytes.Contains(executed.Stderr, []byte(ErrBrowserMode.Error())) {
		return ranProcess{}, ErrBrowserMode
	}
	ran.timedOut = executed.TimedOut
	if ran.record, err = readRunRecord(out); err != nil {
		return ranProcess{}, err
	}
	return ran, nil
}

// installScanScripts writes the shipped scripts into a fresh directory of the
// workspace's results.
func installScanScripts(workspace ecosystem.Workspace) (string, error) {
	dir := suiterun.ResultsFile(workspace, "")
	if err := os.MkdirAll(dir, scriptsDirMode); err != nil {
		return "", fmt.Errorf("create the scan scripts directory: %w", err)
	}
	err := fs.WalkDir(scanScripts, scriptsDirectory, func(name string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := scanScripts.ReadFile(name)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dir, path.Base(name)), content, scriptFileMode)
	})
	if err != nil {
		return "", fmt.Errorf("write the scan scripts: %w", err)
	}
	return dir, nil
}

// writeScanSelection writes the ids a run keeps, as a JSON array, into a fresh
// file of the workspace's results.
func writeScanSelection(workspace ecosystem.Workspace, tests []ecosystem.TestID) (string, error) {
	content, err := json.Marshal(tests)
	if err != nil {
		return "", fmt.Errorf("encode the selected tests: %w", err)
	}
	selection := suiterun.ResultsFile(workspace, selectionExtension)
	if err := os.WriteFile(selection, content, filePermissions); err != nil {
		return "", fmt.Errorf("write the selected tests: %w", err)
	}
	return selection, nil
}

// realDirectory is the directory with its symbolic links resolved, as the
// runner names the files it loads.
func realDirectory(dir string) string {
	if real, err := filepath.EvalSymlinks(dir); err == nil {
		return real
	}
	return dir
}

// testFiles are the distinct test files of the tests, in the order of their
// first test.
func testFiles(tests []ecosystem.TestID) []string {
	var files []string
	for _, test := range tests {
		if file := fileOfTest(test); !slices.Contains(files, file) {
			files = append(files, file)
		}
	}
	return files
}

// Run runs the runner once per requested run, each run one process, under the
// shipped environment or runner that keeps the selected tests alone. A run
// that times out leaves its tests without an outcome, and every later run,
// timed out, and no later run starts.
func (s scanSuite) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	result := ports.ScanRuns{Runs: map[ecosystem.TestID][]vacuity.Run{}}
	tests := slices.Clone(request.Tests)
	timedOut := false
	for range request.Runs {
		if timedOut {
			suiterun.AppendScanRun(result.Runs, tests, nil, suiterun.Always(vacuity.RunTimedOut))
			continue
		}
		ran, err := s.runProcess(ctx, workspace, toolchain, scanProcess{tests: request.Tests, timeout: request.Timeout})
		if err != nil {
			return ports.ScanRuns{}, err
		}
		timedOut = ran.timedOut
		outcomes := outcomesOf(ran.record, s.framework, len(request.Tests) > 0, ran.timedOut)
		if result.Cases == nil {
			result.Cases = outcomes.cases
		}
		if len(tests) == 0 {
			tests = idsOfCases(result.Cases)
		}
		suiterun.AppendScanRun(result.Runs, tests, outcomes.outcomes, outcomes.missingRun)
	}
	return result, nil
}

func idsOfCases(cases []ports.ScanCase) []ecosystem.TestID {
	ids := make([]ecosystem.TestID, 0, len(cases))
	for _, scanCase := range cases {
		ids = append(ids, scanCase.ID)
	}
	return ids
}

// CoverageOfOneRun is false: each test whose owned lines the scan reads runs
// alone under coverage.
func (scanSuite) CoverageOfOneRun() bool { return false }

// LineCoverage records the coverage of the requested tests. Each owner runs
// alone in RecordingsPerTest processes on the request's workers, and owns the
// lines the ownership rule gives (see ownedLines); its lines are everything
// those processes ran. The other tests run in one process of their files in
// path order, and each gets the lines of its file's segment of that process,
// the start-up included, and owns none. A recording that a process of an
// owner cannot take through an inspector session is taken again through
// NODE_V8_COVERAGE, which gives lines only. Test files are never reported.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	recorder := s.recorder(workspace)
	owners := ownersOf(request)
	var alone, together []ports.ScanCase
	for _, test := range request.Tests {
		if owners == nil || owners[test.ID] {
			alone = append(alone, test)
		} else {
			together = append(together, test)
		}
	}
	covered := map[ecosystem.TestID]vacuity.Coverage{}
	recorded, err := s.recordAlone(ctx, workspace, toolchain, request, alone, recorder)
	if err != nil {
		return nil, err
	}
	for test, coverage := range recorded {
		covered[test] = coverage
	}
	if len(together) == 0 {
		return covered, nil
	}
	lines, err := s.recordTogether(ctx, workspace, toolchain, request, together, recorder)
	if err != nil {
		return nil, err
	}
	for test, coverage := range lines {
		covered[test] = coverage
	}
	return covered, nil
}

// ownersOf is the set of owners of a request; nil when every requested test
// is one.
func ownersOf(request ports.ScanCoverageRequest) map[ecosystem.TestID]bool {
	if request.Owners == nil {
		return nil
	}
	owners := make(map[ecosystem.TestID]bool, len(request.Owners))
	for _, owner := range request.Owners {
		owners[owner] = true
	}
	return owners
}

// coverageRecorder reads the records of a workspace's recordings into lines
// of the project's source files, test files left out.
type coverageRecorder struct {
	shapes *geometries
	layout ecosystem.Layout
	files  projectFiles
}

func (s scanSuite) recorder(workspace ecosystem.Workspace) coverageRecorder {
	files := projectFiles{roots: rootsOf(workspace.Dir), read: func(file string) ([]byte, bool) {
		content, err := os.ReadFile(filepath.Join(workspace.Dir, filepath.FromSlash(file)))
		return content, err == nil
	}}
	return coverageRecorder{shapes: newGeometries(files), layout: (&Strategy{}).Layout(), files: files}
}

// source reports whether a line is of a project source file, not a test file.
func (r coverageRecorder) source(line vacuity.Line) bool { return !r.layout.IsTestFile(line.File) }

// recordAlone records each test alone, RecordingsPerTest times, and reads
// against hollow runs of it the windows whose async hooks were blind.
func (s scanSuite) recordAlone(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest, tests []ports.ScanCase, recorder coverageRecorder) (map[ecosystem.TestID]vacuity.Coverage, error) {
	recordings, err := s.recordings(ctx, workspace, toolchain, request, tests, false)
	if err != nil {
		return nil, err
	}
	covered := make(map[ecosystem.TestID]vacuity.Coverage, len(tests))
	var blind []ports.ScanCase
	views := make([][]processLines, len(tests))
	for index, test := range tests {
		if linesOnly(recordings[index]) {
			lines, err := s.linesThroughNode(ctx, workspace, toolchain, request, test.ID, recorder)
			if err != nil {
				return nil, err
			}
			covered[test.ID] = vacuity.Coverage{Lines: lines}
			continue
		}
		views[index] = viewsOf(recordings[index], test.ID, recorder)
		if slices.ContainsFunc(views[index], func(view processLines) bool { return view.readAgainstHollow }) {
			blind = append(blind, test)
		}
	}
	hollows := map[ecosystem.TestID][]processLines{}
	if len(blind) > 0 {
		hollowRecordings, err := s.recordings(ctx, workspace, toolchain, request, blind, true)
		if err != nil {
			return nil, err
		}
		for index, test := range blind {
			hollows[test.ID] = viewsOf(hollowRecordings[index], test.ID, recorder)
		}
	}
	for index, test := range tests {
		if views[index] == nil {
			continue
		}
		ran := map[vacuity.Line]bool{}
		for _, view := range views[index] {
			for line := range view.ran() {
				ran[line] = true
			}
		}
		owned := slices.DeleteFunc(ownedLines(views[index], hollows[test.ID]), func(line vacuity.Line) bool { return !recorder.source(line) })
		covered[test.ID] = vacuity.Coverage{Lines: sortedLines(ran, recorder.source), Owned: owned}
	}
	return covered, nil
}

// recordings runs each test alone RecordingsPerTest times under coverage, on
// the request's workers; a run past the request's timeout fails the recording.
func (s scanSuite) recordings(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest, tests []ports.ScanCase, hollow bool) ([][]ranProcess, error) {
	recorded := make([][]ranProcess, len(tests))
	for index := range recorded {
		recorded[index] = make([]ranProcess, RecordingsPerTest)
	}
	err := workerpool.Run(ctx, request.Workers, len(tests)*RecordingsPerTest, func(ctx context.Context, job int) error {
		test := tests[job/RecordingsPerTest]
		ran, err := s.runProcess(ctx, workspace, toolchain, scanProcess{tests: []ecosystem.TestID{test.ID}, coverage: true, hollow: hollow, timeout: request.Timeout})
		if err != nil {
			return err
		}
		if ran.timedOut {
			return fmt.Errorf("%w: %s ran past %s", ports.ErrCoverageTimedOut, test.ID, request.Timeout)
		}
		recorded[job/RecordingsPerTest][job%RecordingsPerTest] = ran
		return nil
	})
	if err == nil {
		err = ctx.Err()
	}
	return recorded, err
}

// linesOnly reports whether a recording of a test had a process that could
// not open an inspector session.
func linesOnly(recordings []ranProcess) bool {
	for _, recording := range recordings {
		for _, process := range recording.record.processes {
			if process.recording && !process.capabilities.Inspector {
				return true
			}
		}
	}
	return false
}

// viewsOf reads each recording of a test: the process that ran the test, its
// window owning nothing when the test did not pass there.
func viewsOf(recordings []ranProcess, test ecosystem.TestID, recorder coverageRecorder) []processLines {
	views := make([]processLines, 0, len(recordings))
	for _, recording := range recordings {
		view := processLines{}
		found := false
		for _, process := range recording.record.processes {
			outcome := ""
			for _, recorded := range process.tests {
				if recorded.id == test {
					outcome = recorded.outcome
				}
			}
			if outcome == "" {
				continue
			}
			view, found = analyzeProcess(process, recorder.shapes), true
			view.owning = view.owning && outcome == outcomePassed
			break
		}
		if found {
			views = append(views, view)
		}
	}
	return views
}

// recordTogether runs the tests in one process of their files, and gives each
// the lines of its file's segment, the start-up included.
func (s scanSuite) recordTogether(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest, tests []ports.ScanCase, recorder coverageRecorder) (map[ecosystem.TestID]vacuity.Coverage, error) {
	ids := make([]ecosystem.TestID, 0, len(tests))
	for _, test := range tests {
		ids = append(ids, test.ID)
	}
	ran, err := s.runProcess(ctx, workspace, toolchain, scanProcess{tests: ids, coverage: true, timeout: request.Timeout})
	if err != nil {
		return nil, err
	}
	if ran.timedOut {
		return nil, fmt.Errorf("%w: the recording of %d tests ran past %s", ports.ErrCoverageTimedOut, len(ids), request.Timeout)
	}
	if linesOnly([]ranProcess{ran}) {
		return s.togetherThroughNode(ctx, workspace, toolchain, request, tests, recorder)
	}
	segments := map[string]map[vacuity.Line]bool{}
	for _, process := range ran.record.processes {
		view := analyzeProcess(process, recorder.shapes)
		for _, test := range tests {
			if segments[test.File] == nil {
				segments[test.File] = map[vacuity.Line]bool{}
			}
			for line := range view.segmentOf(test.File) {
				segments[test.File][line] = true
			}
		}
	}
	covered := make(map[ecosystem.TestID]vacuity.Coverage, len(tests))
	for _, test := range tests {
		covered[test.ID] = vacuity.Coverage{Lines: sortedLines(segments[test.File], recorder.source)}
	}
	return covered, nil
}

// Operation binds a top-level function of a module of the project to the
// probe target `path/to/file.ts:function`; a method, a nested function and a
// function of a test file have no binding.
func (scanSuite) Operation(_ ecosystem.Workspace, file, qualname string) (string, bool) {
	if !isScript(file) || strings.HasSuffix(file, declarationFileEnd) || (&Strategy{}).Layout().IsTestFile(file) {
		return "", false
	}
	if qualname == "" || strings.Contains(qualname, qualnameDot) || !isJavaScriptIdentifier(qualname) {
		return "", false
	}
	return file + probeTargetSeparator + qualname, true
}

// Repro runs the one test with the package runner, by its file and an
// anchored pattern over its full title.
func (s scanSuite) Repro(test ecosystem.TestID) []string {
	return append([]string{reproPackageRunner, reproNoInstallFlag}, s.framework.reproArguments(fileOfTest(test), reproPattern(test))...)
}

// isJavaScriptIdentifier reports whether a name is an identifier of ASCII
// letters, digits, underscores and dollar signs that does not start with a
// digit.
func isJavaScriptIdentifier(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return r != '_' && r != '$' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
}
