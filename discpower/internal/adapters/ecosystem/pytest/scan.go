package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

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
	scanReportSuffix   = ".json"
	rootDirFlag        = "--rootdir=."
	// continueFlag keeps a module that does not import from stopping the run
	// of every other test.
	continueFlag    = "--continue-on-collection-errors"
	quietFlag       = "-q"
	targetSeparator = ":"
	initModule      = "__init__"
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
}

// Run runs pytest once per requested run, under the scan plugin that the
// workspace alone receives. A run that times out leaves its tests and every
// later run timed out, and no later run starts.
func (s scanSuite) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	pluginDir, err := installPlugin(workspace, scanPluginFile, scanPlugin)
	if err != nil {
		return ports.ScanRuns{}, err
	}
	result := ports.ScanRuns{Runs: map[ecosystem.TestID][]vacuity.Run{}}
	tests := slices.Clone(request.Tests)
	timedOut := false
	for range request.Runs {
		if timedOut {
			suiterun.AppendScanRun(result.Runs, tests, nil, vacuity.RunTimedOut)
			continue
		}
		var report scanReport
		report, timedOut, err = s.runOnce(ctx, workspace, toolchain, pluginDir, request)
		if err != nil {
			return ports.ScanRuns{}, err
		}
		if result.Cases == nil {
			result.Cases = casesOf(report)
		}
		if len(tests) == 0 {
			tests = idsOf(result.Cases)
		}
		missing := vacuity.RunErrored
		if timedOut {
			missing = vacuity.RunTimedOut
		}
		suiterun.AppendScanRun(result.Runs, tests, outcomesOf(report), missing)
	}
	return result, nil
}

func (s scanSuite) runOnce(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, pluginDir string, request ports.ScanRunRequest) (scanReport, bool, error) {
	output := suiterun.ResultsFile(workspace, scanReportSuffix)
	argv := append(append([]string{}, toolchain.Command...), moduleFlag, pytestModule,
		noCacheProvider, noCacheProviderID, pluginFlag, scanPluginModule, rootDirFlag, continueFlag, quietFlag)
	for _, test := range request.Tests {
		argv = append(argv, string(test))
	}
	executed, err := s.deps.Process.Run(ctx, ports.Command{
		Argv:    argv,
		Dir:     workspace.Dir,
		Env:     []string{noBytecodeVariable, pythonPathVariable + pluginPath(pluginDir, workspace), scanOutputVariable + environmentAssignment + output},
		Timeout: request.Timeout,
	})
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

func outcomesOf(report scanReport) map[ecosystem.TestID]ecosystem.TestOutcome {
	outcomes := make(map[ecosystem.TestID]ecosystem.TestOutcome, len(report.Outcomes))
	for node, outcome := range report.Outcomes {
		outcomes[ecosystem.TestID(node)] = ecosystem.TestOutcome(outcome)
	}
	return outcomes
}

// LineCoverage runs the suite once under the coverage plugin and keeps the
// lines of the requested tests, by node id.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ports.ScanCase) (map[ecosystem.TestID][]vacuity.Line, error) {
	report, err := runCoverage(ctx, s.deps, workspace, toolchain)
	if err != nil {
		return nil, err
	}
	covered := map[ecosystem.TestID][]vacuity.Line{}
	for _, test := range tests {
		if recorded, found := report[string(test.ID)]; found {
			covered[test.ID] = linesOf(recorded)
		}
	}
	return covered, nil
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

// installPlugin writes a plugin module into a fresh directory of the
// workspace's results, which is put on the import path of the run.
func installPlugin(workspace ecosystem.Workspace, file string, content []byte) (string, error) {
	pluginDir := suiterun.ResultsFile(workspace, "")
	if err := os.MkdirAll(pluginDir, coverageDirPermissions); err != nil {
		return "", fmt.Errorf("create the plugin directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, file), content, probeFilePermissions); err != nil {
		return "", fmt.Errorf("write the plugin: %w", err)
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
