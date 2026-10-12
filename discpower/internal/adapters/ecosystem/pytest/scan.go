package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"maps"
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
	scanPluginModule   = "spproof_scan"
	scanPluginFile     = scanPluginModule + pythonSuffix
	scanOutputVariable = "SPEC_PROOF_SCAN_OUT"
	// scanSelectionVariable names the file that lists, as a JSON array, the
	// node ids a session of selected tests keeps.
	scanSelectionVariable = "SPEC_PROOF_SCAN_SELECTION"
	scanReportSuffix      = ".json"
	scanSelectionSuffix   = ".json"
	rootDirFlag           = "--rootdir=."
	// continueFlag keeps a module that does not import from stopping the run
	// of every other test of the files a session collects.
	continueFlag = "--continue-on-collection-errors"
	// noMaxFailFlag keeps a failure from stopping the session, whatever the
	// project's own options say.
	noMaxFailFlag   = "--maxfail=0"
	quietFlag       = "-q"
	targetSeparator = ":"
	initModule      = "__init__"
	// initFile, like conftestFile, is a file whose import starts the
	// collection of every module below their directory.
	initFile        = initModule + pythonSuffix
	parentDirectory = "."
	// reproInterpreter is the interpreter a reproduction script calls.
	reproInterpreter = "python3"
)

//go:embed scripts/scan_plugin.py
var scanPlugin []byte

// Scan runs the suite for a scan: whole or selected, several times, with the
// outcome of each test read by node id.
func (s *Strategy) Scan() ports.ScanSuite { return scanSuite{deps: s.deps} }

type scanSuite struct {
	deps Dependencies
}

// scanReport is what the scan plugin writes for one run.
type scanReport struct {
	Collected []struct {
		ID   string `json:"id"`
		File string `json:"file"`
	} `json:"collected"`
	Outcomes map[string]string `json:"outcomes"`
	// CollectFailures are the node ids of the failed collect reports, and the
	// paths of the conftest files that failed to import, relative to the root.
	CollectFailures []string `json:"collectFailures"`
	// StoppedBeforeCollection is set when a conftest file loaded before the
	// session began failed to import, so that the session ran nothing.
	StoppedBeforeCollection bool `json:"stoppedBeforeCollection"`
}

// covers reports whether a failed collect report of the session covers the
// test (see coversFile).
func (r scanReport) covers(test ecosystem.TestID) bool {
	file := fileOf(test)
	return slices.ContainsFunc(r.CollectFailures, func(failure string) bool { return coversFile(failure, file) })
}

// fileOf is the test file of a node id.
func fileOf(test ecosystem.TestID) string {
	file, _, _ := strings.Cut(string(test), nodeSeparator)
	return file
}

// Run runs pytest once per requested run, under the scan plugin that the
// workspace alone receives: each run is one run index, in one session or more
// (see runIndex), and gives each test one run. A run that times out leaves its
// tests that no earlier session of it covers, and every later run, timed out,
// and no later session starts.
func (s scanSuite) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	pluginDir, err := installPlugin(workspace, pluginModule{file: scanPluginFile, content: scanPlugin})
	if err != nil {
		return ports.ScanRuns{}, err
	}
	result := ports.ScanRuns{Runs: map[ecosystem.TestID][]vacuity.Run{}}
	tests := slices.Clone(request.Tests)
	timedOut := false
	for range request.Runs {
		if timedOut {
			suiterun.AppendScanRun(result.Runs, tests, nil, suiterun.Always(vacuity.RunTimedOut))
			continue
		}
		index, err := s.runIndex(ctx, workspace, toolchain, pluginDir, request)
		if err != nil {
			return ports.ScanRuns{}, err
		}
		timedOut = index.timedOut
		if result.Cases == nil {
			result.Cases = casesOf(index.first())
		}
		if len(tests) == 0 {
			tests = idsOf(result.Cases)
		}
		suiterun.AppendScanRun(result.Runs, tests, index.outcomes(), index.missingRun)
	}
	return result, nil
}

// runIndex is what the sessions of one run index reported, in order, and
// whether its last session timed out.
type runIndex struct {
	sessions []scanReport
	timedOut bool
}

// first is the report of the first session; empty when it left none.
func (r runIndex) first() scanReport {
	if len(r.sessions) == 0 {
		return scanReport{}
	}
	return r.sessions[0]
}

// outcomes are the outcomes the sessions reported, by node id.
func (r runIndex) outcomes() map[ecosystem.TestID]ecosystem.TestOutcome {
	outcomes := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for _, report := range r.sessions {
		for node, outcome := range report.Outcomes {
			outcomes[ecosystem.TestID(node)] = ecosystem.TestOutcome(outcome)
		}
	}
	return outcomes
}

// covers reports whether a failed collect report of one of the sessions covers
// the test.
func (r runIndex) covers(test ecosystem.TestID) bool {
	return slices.ContainsFunc(r.sessions, func(report scanReport) bool { return report.covers(test) })
}

// missingRun is the run of a test no session reported: not started when a
// failed collect report of one of them covers it, timed out when the last
// session timed out, errored otherwise, a test absent from a module that
// collected included.
func (r runIndex) missingRun(test ecosystem.TestID) vacuity.Run {
	switch {
	case r.covers(test):
		return vacuity.RunNotStarted
	case r.timedOut:
		return vacuity.RunTimedOut
	}
	return vacuity.RunErrored
}

// runIndex runs one run index: a session of the requested tests, then, while a
// session stops before collection on a conftest file that failed to import, a
// session of the requested tests that no failure so far covers, until a
// session runs, no test is left, or a session covers no more of them. A
// session that times out ends the run index.
func (s scanSuite) runIndex(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, pluginDir string, request ports.ScanRunRequest) (runIndex, error) {
	var index runIndex
	remaining := request.Tests
	for {
		report, timedOut, err := s.runOnce(ctx, workspace, toolchain, pluginDir, remaining, request.Timeout)
		if err != nil {
			return runIndex{}, err
		}
		if timedOut {
			index.timedOut = true
			return index, nil
		}
		index.sessions = append(index.sessions, report)
		if !report.StoppedBeforeCollection || len(remaining) == 0 {
			return index, nil
		}
		left := slices.DeleteFunc(slices.Clone(remaining), index.covers)
		if len(left) == 0 || len(left) == len(remaining) {
			return index, nil
		}
		remaining = left
	}
}

// coversFile reports whether a failed collect report stops the collection of a
// test file: the report is of the file itself, of a conftest.py or __init__.py
// in a directory on its path, or of such a directory.
func coversFile(failure, file string) bool {
	failure = strings.TrimSuffix(failure, pathSeparator)
	switch base := path.Base(failure); {
	case failure == file:
		return true
	case base == conftestFile || base == initFile:
		return isUnder(path.Dir(failure), file)
	case path.Ext(failure) == pythonSuffix:
		return false
	}
	return isUnder(failure, file)
}

// isUnder reports whether the file lies in the directory, the root holding
// every file.
func isUnder(dir, file string) bool {
	return dir == "" || dir == parentDirectory || strings.HasPrefix(file, dir+pathSeparator)
}

// runOnce runs one session: the whole suite, or the test files of the tests, in
// order, the plugin keeping the tests alone, and reads its report; an empty
// report when the session left none or an unreadable one.
func (s scanSuite) runOnce(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, pluginDir string, tests []ecosystem.TestID, timeout time.Duration) (scanReport, bool, error) {
	output := suiterun.ResultsFile(workspace, scanReportSuffix)
	argv := append(append([]string{}, toolchain.Command...), moduleFlag, pytestModule,
		noCacheProvider, noCacheProviderID, pluginFlag, scanPluginModule, rootDirFlag, continueFlag, noMaxFailFlag, quietFlag)
	env := []string{noBytecodeVariable, pythonPathVariable + pluginPath(pluginDir, workspace), scanOutputVariable + environmentAssignment + output}
	if len(tests) > 0 {
		selection, err := writeSelection(workspace, tests)
		if err != nil {
			return scanReport{}, false, err
		}
		env = append(env, scanSelectionVariable+environmentAssignment+selection)
		argv = append(argv, filesOf(tests)...)
	}
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Env: env, Timeout: timeout})
	if err != nil {
		return scanReport{}, false, fmt.Errorf("run the suite: %w", err)
	}
	if executed.TimedOut {
		return scanReport{}, true, nil
	}
	var report scanReport
	content, err := os.ReadFile(output)
	if err != nil {
		return scanReport{}, false, nil
	}
	if err := json.Unmarshal(content, &report); err != nil {
		return scanReport{}, false, nil
	}
	return report, false, nil
}

// writeSelection writes the node ids a session keeps, as a JSON array, into a
// fresh file of the workspace's results.
func writeSelection(workspace ecosystem.Workspace, tests []ecosystem.TestID) (string, error) {
	content, err := json.Marshal(tests)
	if err != nil {
		return "", fmt.Errorf("encode the selected tests: %w", err)
	}
	selection := suiterun.ResultsFile(workspace, scanSelectionSuffix)
	if err := os.WriteFile(selection, content, probeFilePermissions); err != nil {
		return "", fmt.Errorf("write the selected tests: %w", err)
	}
	return selection, nil
}

// filesOf are the distinct test files of the tests, in the order of their first
// test.
func filesOf(tests []ecosystem.TestID) []string {
	var files []string
	for _, test := range tests {
		if file := fileOf(test); !slices.Contains(files, file) {
			files = append(files, file)
		}
	}
	return files
}

func casesOf(report scanReport) []ports.ScanCase {
	cases := make([]ports.ScanCase, 0, len(report.Collected))
	for _, item := range report.Collected {
		cases = append(cases, ports.ScanCase{ID: ecosystem.TestID(item.ID), File: item.File})
	}
	return cases
}

func idsOf(cases []ports.ScanCase) []ecosystem.TestID {
	ids := make([]ecosystem.TestID, 0, len(cases))
	for _, scanCase := range cases {
		ids = append(ids, scanCase.ID)
	}
	return ids
}

// CoverageOfOneRun is true: one run of the suite records every test.
func (scanSuite) CoverageOfOneRun() bool { return true }

// LineCoverage runs the suite once under the coverage plugin, within the
// timeout of the request, and keeps the lines of the requested tests, by node
// id; a request for each test alone runs, for each, a session that selects it
// and no other test. The plugin records only inside the run of each test, its
// setup, call and teardown, never during collection; a test owns the lines
// among them that ran on the thread of its run, in its setup or call, outside
// the setup of a fixture of a scope wider than the function and outside the
// body of a module its run imported.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	if !request.Alone {
		report, err := runCoverage(ctx, s.deps, workspace, toolchain, request.Timeout, nil)
		if err != nil {
			return nil, err
		}
		return coverageOfTests(report, request.Tests), nil
	}
	covered := map[ecosystem.TestID]vacuity.Coverage{}
	for _, test := range request.Tests {
		report, err := runCoverage(ctx, s.deps, workspace, toolchain, request.Timeout, []ecosystem.TestID{test.ID})
		if err != nil {
			return nil, err
		}
		maps.Copy(covered, coverageOfTests(report, []ports.ScanCase{test}))
	}
	return covered, nil
}

// coverageOfTests keeps the lines of the tests a coverage report records, by
// node id.
func coverageOfTests(report coverageReport, tests []ports.ScanCase) map[ecosystem.TestID]vacuity.Coverage {
	covered := map[ecosystem.TestID]vacuity.Coverage{}
	for _, test := range tests {
		if recorded, found := report[string(test.ID)]; found {
			covered[test.ID] = lineCoverageOf(recorded)
		}
	}
	return covered
}

// Operation binds a top-level function of a module to the target
// `package.module:function`; a method or a nested function has no binding.
func (scanSuite) Operation(_ ecosystem.Workspace, file, qualname string) (string, bool) {
	if path.Ext(file) != pythonSuffix || qualname == "" || strings.Contains(qualname, qualnameSeparator) || !isIdentifier(qualname) {
		return "", false
	}
	module := strings.TrimSuffix(strings.TrimPrefix(file, srcLayoutDirectory+pathSeparator), pythonSuffix)
	module = strings.TrimSuffix(strings.TrimSuffix(module, initModule), pathSeparator)
	parts := strings.Split(module, pathSeparator)
	if module == "" || slices.ContainsFunc(parts, func(part string) bool { return !isIdentifier(part) }) {
		return "", false
	}
	return strings.Join(parts, moduleSeparator) + targetSeparator + qualname, true
}

// Repro runs the one test by its node id.
func (scanSuite) Repro(test ecosystem.TestID) []string {
	return []string{reproInterpreter, moduleFlag, pytestModule, noCacheProvider, noCacheProviderID, string(test)}
}

// pluginModule is a plugin a run loads: its module file and its source.
type pluginModule struct {
	file    string
	content []byte
}

// installPlugin writes plugin modules into a fresh directory of the
// workspace's results, which is put on the import path of the run.
func installPlugin(workspace ecosystem.Workspace, plugins ...pluginModule) (string, error) {
	pluginDir := suiterun.ResultsFile(workspace, "")
	if err := os.MkdirAll(pluginDir, coverageDirPermissions); err != nil {
		return "", fmt.Errorf("create the plugin directory: %w", err)
	}
	for _, plugin := range plugins {
		if err := os.WriteFile(filepath.Join(pluginDir, plugin.file), plugin.content, probeFilePermissions); err != nil {
			return "", fmt.Errorf("write the plugin %s: %w", plugin.file, err)
		}
	}
	return pluginDir, nil
}

func pluginPath(pluginDir string, workspace ecosystem.Workspace) string {
	return strings.Join([]string{pluginDir, importPath(workspace)}, string(filepath.ListSeparator))
}

// isIdentifier reports whether a name is a Python identifier of ASCII letters,
// digits and underscores that does not start with a digit.
func isIdentifier(name string) bool {
	if name == "" || (name[0] >= '0' && name[0] <= '9') {
		return false
	}
	return !strings.ContainsFunc(name, func(r rune) bool {
		return r != '_' && (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9')
	})
}
