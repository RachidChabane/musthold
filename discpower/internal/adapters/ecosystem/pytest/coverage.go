package pytest

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/coverage"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
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
	report, err := runCoverage(ctx, c.deps, workspace, toolchain)
	if err != nil {
		return nil, err
	}
	return unitsOf(report, tests), nil
}

// coverageReport is what the coverage plugin records, by pytest node id.
type coverageReport map[string]nodeCoverage

// nodeCoverage holds the units and the lines one test ran: each unit is a file
// and a qualified name, each line a file and a line number.
type nodeCoverage struct {
	Units [][]string          `json:"units"`
	Lines [][]json.RawMessage `json:"lines"`
}

// runCoverage runs the whole suite once under the coverage plugin, which the
// workspace alone receives, and reads its report.
func runCoverage(ctx context.Context, deps Dependencies, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain) (coverageReport, error) {
	pluginDir, err := installPlugin(workspace, coveragePluginFile, coveragePlugin)
	if err != nil {
		return nil, err
	}
	report := suiterun.ResultsFile(workspace, coverageReportSuffix)
	argv := append(append([]string{}, toolchain.Command...), moduleFlag, pytestModule,
		pluginFlag, coveragePluginModule, noCacheProvider, noCacheProviderID, rootDirFlag)
	pythonPath := pluginPath(pluginDir, workspace)
	if _, err := deps.Process.Run(ctx, ports.Command{
		Argv:    argv,
		Dir:     workspace.Dir,
		Env:     []string{noBytecodeVariable, pythonPathVariable + pythonPath, coverageOutputVariable + environmentAssignment + report},
		Timeout: deps.Timeout,
	}); err != nil {
		return nil, fmt.Errorf("run the coverage suite: %w", err)
	}
	return readCoverage(report)
}

func readCoverage(report string) (coverageReport, error) {
	content, err := os.ReadFile(report)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCoverageFailed, err)
	}
	var executed coverageReport
	if err := json.Unmarshal(content, &executed); err != nil {
		return nil, fmt.Errorf("%w: unreadable report: %v", ErrCoverageFailed, err)
	}
	return executed, nil
}

// unitsOf keeps the units of the requested tests, keyed as the JUnit report
// names them.
func unitsOf(report coverageReport, tests []ecosystem.BoundTest) coverage.Map {
	requested := make(map[ecosystem.TestID]bool, len(tests))
	for _, test := range tests {
		requested[test.ID] = true
	}
	result := coverage.Map{}
	for node, recorded := range report {
		id, ok := testIDOfNode(node)
		if ok && requested[id] {
			result[id] = refsOf(recorded.Units)
		}
	}
	return result
}

// linesOf reads the lines of one test; an entry that is not a file and a
// positive line number is skipped.
func linesOf(recorded nodeCoverage) []vacuity.Line {
	lines := make([]vacuity.Line, 0, len(recorded.Lines))
	for _, entry := range recorded.Lines {
		if len(entry) != lineFieldCount {
			continue
		}
		var line vacuity.Line
		if json.Unmarshal(entry[0], &line.File) != nil || json.Unmarshal(entry[1], &line.Line) != nil || line.Line < 1 {
			continue
		}
		lines = append(lines, line)
	}
	slices.SortFunc(lines, format.CompareScanLines)
	return slices.Compact(lines)
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

// unitFieldCount is the length of a reported unit: file and qualified name;
// lineFieldCount the length of a reported line: file and line number.
const (
	unitFieldCount = 2
	lineFieldCount = 2
)

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
