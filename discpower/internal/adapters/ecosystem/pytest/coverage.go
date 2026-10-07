package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
)

var _ ports.Covering = (*Strategy)(nil)

// ErrCoverageFailed is returned when the coverage run leaves no report.
var ErrCoverageFailed = errors.New("coverage run left no report")

const (
	coveragePluginModule   = "spproof_coverage"
	coveragePluginFile     = coveragePluginModule + pythonSuffix
	coverageOutputVariable = "SPEC_PROOF_COVERAGE_OUT"
	pluginFlag             = "-p"
	coverageReportSuffix   = ".json"
	coverageDirPermissions = 0o700
	environmentAssignment  = "="
)

//go:embed scripts/coverage_plugin.py
var coveragePlugin []byte

// Coverage records, per test, the project functions it executes.
func (s *Strategy) Coverage() ports.CoverageRecorder { return coverageRecorder{deps: s.deps} }

type coverageRecorder struct {
	deps Dependencies
}

// Record runs the whole suite once under the coverage plugin and keeps the
// entries of the requested tests. A non-zero pytest exit is not an error as
// long as the plugin wrote its report.
func (c coverageRecorder) Record(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ecosystem.BoundTest) (coverage.Map, error) {
	pluginDir := suiterun.ResultsFile(workspace, "")
	if err := os.MkdirAll(pluginDir, coverageDirPermissions); err != nil {
		return nil, fmt.Errorf("create the coverage plugin directory: %w", err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, coveragePluginFile), coveragePlugin, probeFilePermissions); err != nil {
		return nil, fmt.Errorf("write the coverage plugin: %w", err)
	}
	report := suiterun.ResultsFile(workspace, coverageReportSuffix)
	argv := append(append([]string{}, toolchain.Command...), moduleFlag, pytestModule,
		pluginFlag, coveragePluginModule, noCacheProvider, noCacheProviderID)
	pythonPath := strings.Join([]string{pluginDir, importPath(workspace)}, string(filepath.ListSeparator))
	if _, err := c.deps.Process.Run(ctx, ports.Command{
		Argv:    argv,
		Dir:     workspace.Dir,
		Env:     []string{noBytecodeVariable, pythonPathVariable + pythonPath, coverageOutputVariable + environmentAssignment + report},
		Timeout: c.deps.Timeout,
	}); err != nil {
		return nil, fmt.Errorf("run the coverage suite: %w", err)
	}
	return readCoverage(report, tests)
}

func readCoverage(report string, tests []ecosystem.BoundTest) (coverage.Map, error) {
	content, err := os.ReadFile(report)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCoverageFailed, err)
	}
	var executed map[string][][]string
	if err := json.Unmarshal(content, &executed); err != nil {
		return nil, fmt.Errorf("%w: unreadable report: %v", ErrCoverageFailed, err)
	}
	requested := make(map[ecosystem.TestID]bool, len(tests))
	for _, test := range tests {
		requested[test.ID] = true
	}
	result := coverage.Map{}
	for node, units := range executed {
		id, ok := testIDOfNode(node)
		if ok && requested[id] {
			result[id] = refsOf(units)
		}
	}
	return result, nil
}

func refsOf(units [][]string) []footprint.Ref {
	refs := make([]footprint.Ref, 0, len(units))
	for _, unit := range units {
		if len(unit) == unitFieldCount {
			refs = append(refs, footprint.Ref{File: unit[0], Qualname: unit[1]})
		}
	}
	return refs
}

// unitFieldCount is the length of a reported unit: file and qualified name.
const unitFieldCount = 2

// testIDOfNode maps a pytest node id to the identity the JUnit report gives the
// test: the classname is the module path and the classes, the name is the
// function with its parameter id.
func testIDOfNode(node string) (ecosystem.TestID, bool) {
	file, rest, found := strings.Cut(node, nodeSeparator)
	if !found {
		return "", false
	}
	params := ""
	if open := strings.Index(rest, parameterOpen); open >= 0 {
		rest, params = rest[:open], rest[open:]
	}
	segments := strings.Split(rest, nodeSeparator)
	module := strings.ReplaceAll(strings.TrimSuffix(file, pythonSuffix), pathSeparator, qualnameSeparator)
	classname := strings.Join(append([]string{module}, segments[:len(segments)-1]...), qualnameSeparator)
	return ecosystem.NewTestID(classname, segments[len(segments)-1]+params), true
}
