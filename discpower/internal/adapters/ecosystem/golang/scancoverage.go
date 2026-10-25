package golang

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/discpower/internal/platform/workerpool"
	"github.com/RachidChabane/spec-proof/format"
)

// ErrCoverageFailed is returned when a package's test binary cannot be built
// with coverage.
var ErrCoverageFailed = errors.New("the coverage build failed")

const (
	compileOnlyFlag = "-c"
	coverFlag       = "-cover"
	// coverModeFlag counts how often each block runs, exactly across
	// goroutines, so that a test's count can be told from its baselines'.
	coverModeFlag     = "-covermode=atomic"
	coverPackagesFlag = "-coverpkg=" + allPackages
	binaryExtension   = ".test"
	profileExtension  = ".cov"
	binaryRunFlag     = "-test.run"
	// startUpOnly runs no test: the binary's start-up alone.
	startUpOnly       = matchNothing
	binaryCountFlag   = "-test.count=1"
	binaryProfileFlag = "-test.coverprofile="
	// profileModePrefix starts the first line of a cover profile.
	profileModePrefix = "mode:"
	// profileFields are the fields of a block line: position, statements, count.
	profileFields     = 3
	positionSeparator = ":"
	rangeSeparator    = ","
	columnSeparator   = "."
	// noSubtest is the name of a level that matches no subtest.
	noSubtest = ""
)

// CoverageOfOneRun is false: each test runs alone under coverage.
func (scanSuite) CoverageOfOneRun() bool { return false }

// LineCoverage builds the test binary of each package once, with coverage of
// every package of the module in atomic mode, on the workers of the request;
// then runs, on the same workers and within the request's timeout, the
// baselines of each binary three times each: its start-up, with no test, and
// each distinct ancestor of a leaf it records, with that ancestor alone and no
// subtest; and each test alone. It reads the lines of the blocks each test ran
// and, among them, the lines it owns against its baselines (see leafCoverage).
// Each test already runs alone, so a request for each test alone records the
// same. Only the workspace's build cache and results receive the instrumented
// binaries.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return nil, err
	}
	var leaves []coveredRun
	var dirs []string
	for _, test := range request.Tests {
		file, testPath, found := strings.Cut(string(test.ID), testCaseSeparator)
		if !found {
			continue
		}
		if dir := path.Dir(file); !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
		}
		leaves = append(leaves, coveredRun{test: test.ID, dir: path.Dir(file), elements: strings.Split(testPath, subtestSeparator)})
	}
	binaries := make([]*coverageBinary, len(dirs))
	err = workerpool.Run(ctx, request.Workers, len(dirs), func(ctx context.Context, index int) error {
		built, err := s.buildCoverageBinary(ctx, workspace, toolchain, dirs[index])
		binaries[index] = &coverageBinary{path: built, dir: dirs[index]}
		return err
	})
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, err
	}
	for index := range leaves {
		leaves[index].binary = binaries[slices.Index(dirs, leaves[index].dir)]
	}
	plan := planCoverageRuns(binaries, leaves)
	if err := s.runBaselines(ctx, workspace, plan, request); err != nil {
		return nil, err
	}
	covered, err := s.runLeaves(ctx, workspace, plan, module, request)
	if err != nil {
		return nil, err
	}
	byTest := make(map[ecosystem.TestID]vacuity.Coverage, len(leaves))
	for index, leaf := range leaves {
		byTest[leaf.test] = covered[index]
	}
	return byTest, nil
}

// coverageBinary is the coverage binary of one package.
type coverageBinary struct {
	path, dir string
}

// coveredRun is one test to run alone from its package's coverage binary, by
// the elements of its subtest path.
type coveredRun struct {
	test     ecosystem.TestID
	dir      string
	binary   *coverageBinary
	elements []string
}

// baselineKey names a baseline: the package directory of its binary and the
// subtest path of the ancestor it runs, empty for the start-up.
type baselineKey struct {
	dir, ancestor string
}

// baselineKeys are the baselines of a leaf, from the start-up of its binary
// down to its immediate baseline, its parent's.
func (r coveredRun) baselineKeys() []baselineKey {
	keys := []baselineKey{{dir: r.dir}}
	for depth := 1; depth < len(r.elements); depth++ {
		keys = append(keys, baselineKey{dir: r.dir, ancestor: strings.Join(r.elements[:depth], subtestSeparator)})
	}
	return keys
}

// selection is the `-test.run` expression of a baseline: no test for the
// start-up, the ancestor alone with no subtest otherwise.
func (k baselineKey) selection() string {
	if k.ancestor == "" {
		return startUpOnly
	}
	return expression([][]string{append(strings.Split(k.ancestor, subtestSeparator), noSubtest)})
}

// coveragePlan is every run of a recording: the runs of each distinct
// baseline, with what each left, then each leaf.
type coveragePlan struct {
	baselineKeys []baselineKey
	binaryOf     map[baselineKey]*coverageBinary
	baselineRuns map[baselineKey][]baselineRun
	baselines    map[baselineKey]baseline
	leaves       []coveredRun
}

// planCoverageRuns lists the start-up of each binary, then each other distinct
// baseline of the leaves once, in the order of the leaves.
func planCoverageRuns(binaries []*coverageBinary, leaves []coveredRun) *coveragePlan {
	plan := &coveragePlan{
		binaryOf: map[baselineKey]*coverageBinary{}, baselineRuns: map[baselineKey][]baselineRun{},
		baselines: map[baselineKey]baseline{}, leaves: leaves,
	}
	add := func(key baselineKey, binary *coverageBinary) {
		if _, planned := plan.binaryOf[key]; !planned {
			plan.baselineKeys = append(plan.baselineKeys, key)
			plan.binaryOf[key] = binary
			plan.baselineRuns[key] = make([]baselineRun, baselineRunsPerPath)
		}
	}
	for _, binary := range binaries {
		add(baselineKey{dir: binary.dir}, binary)
	}
	for _, leaf := range leaves {
		for _, key := range leaf.baselineKeys() {
			add(key, leaf.binary)
		}
	}
	return plan
}

// baselineJobs is the number of baseline runs of the plan.
func (p *coveragePlan) baselineJobs() int { return len(p.baselineKeys) * baselineRunsPerPath }

// chainOf is the baseline chain of a leaf, once the baselines have run.
func (p *coveragePlan) chainOf(leaf coveredRun) baselineChain {
	keys := leaf.baselineKeys()
	chain := make(baselineChain, 0, len(keys))
	for _, key := range keys {
		chain = append(chain, p.baselines[key])
	}
	return chain
}

// runBaselines runs every run of every baseline of the plan on at most the
// requested workers, stops at the first error, and reads each baseline. A
// baseline run that times out is not usable, never an error.
func (s scanSuite) runBaselines(ctx context.Context, workspace ecosystem.Workspace, plan *coveragePlan, request ports.ScanCoverageRequest) error {
	err := workerpool.Run(ctx, request.Workers, plan.baselineJobs(), func(ctx context.Context, index int) error {
		key := plan.baselineKeys[index/baselineRunsPerPath]
		ran, err := s.runBinary(ctx, workspace, plan.binaryOf[key], key.selection(), request.Timeout)
		if err != nil {
			return err
		}
		plan.baselineRuns[key][index%baselineRunsPerPath] = baselineRun{counts: countsRun(readBlockCounts(ran.profile)), usable: ran.usable()}
		return nil
	})
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return err
	}
	for _, key := range plan.baselineKeys {
		plan.baselines[key] = baselineOf(plan.baselineRuns[key])
	}
	return nil
}

// runLeaves runs each leaf of the plan alone on at most the requested workers,
// reads its coverage against its baselines as soon as it ends, and stops at the
// first error; a leaf run past the request's timeout is one.
func (s scanSuite) runLeaves(ctx context.Context, workspace ecosystem.Workspace, plan *coveragePlan, module string, request ports.ScanCoverageRequest) ([]vacuity.Coverage, error) {
	source := newSourceReader(workspace.Dir)
	covered := make([]vacuity.Coverage, len(plan.leaves))
	err := workerpool.Run(ctx, request.Workers, len(plan.leaves), func(ctx context.Context, index int) error {
		leaf := plan.leaves[index]
		selection := expression([][]string{leaf.elements})
		ran, err := s.runBinary(ctx, workspace, leaf.binary, selection, request.Timeout)
		if err != nil {
			return err
		}
		if ran.timedOut {
			return fmt.Errorf("%w: %s of %s ran past %s", ports.ErrCoverageTimedOut, selection, leaf.dir, request.Timeout)
		}
		if ran.profile != nil {
			covered[index] = leafCoverage(readBlockCounts(ran.profile), plan.chainOf(leaf), source, module)
		}
		return nil
	})
	return covered, err
}

func (s scanSuite) buildCoverageBinary(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, dir string) (string, error) {
	binary := suiterun.ResultsFile(workspace, binaryExtension)
	argv := append(append([]string{}, toolchain.Command...), testSubcommand, compileOnlyFlag, outputFlag, binary,
		coverFlag, coverModeFlag, coverPackagesFlag, packagePath(dir))
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: s.deps.Timeout})
	if err != nil {
		return "", fmt.Errorf("build the coverage binary of %s: %w", dir, err)
	}
	if executed.TimedOut || executed.ExitCode != 0 {
		return "", fmt.Errorf("%w: %s: %s", ErrCoverageFailed, dir, strings.TrimSpace(string(executed.Stderr)))
	}
	return binary, nil
}

// binaryRun is what one run of a coverage binary left: its cover profile, nil
// when it left none, and how it ended.
type binaryRun struct {
	profile  []byte
	exitCode int
	timedOut bool
}

// usable reports whether the run counts as a baseline: it left a profile,
// exited zero and did not time out.
func (r binaryRun) usable() bool {
	return r.profile != nil && r.exitCode == 0 && !r.timedOut
}

// runBinary runs the tests of a coverage binary that the expression selects,
// once, from its package directory, as `go test` does, within the timeout. A
// test that fails still leaves what it ran.
func (s scanSuite) runBinary(ctx context.Context, workspace ecosystem.Workspace, binary *coverageBinary, selection string, timeout time.Duration) (binaryRun, error) {
	profile := suiterun.ResultsFile(workspace, profileExtension)
	argv := []string{binary.path, binaryRunFlag, selection, binaryCountFlag, binaryProfileFlag + profile}
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: filepath.Join(workspace.Dir, filepath.FromSlash(binary.dir)), Timeout: timeout})
	if err != nil {
		return binaryRun{}, fmt.Errorf("run %s of %s with coverage: %w", selection, binary.dir, err)
	}
	ran := binaryRun{exitCode: executed.ExitCode, timedOut: executed.TimedOut}
	if content, err := os.ReadFile(profile); err == nil {
		ran.profile = content
	}
	return ran, nil
}

// profileBlock is one block of a cover profile: its file, its first and last
// line, the column it starts at on its first line and the column it ends
// before on its last.
type profileBlock struct {
	file                    string
	first, last             int
	firstColumn, lastColumn int
}

// countedBlock is a block and how often it ran.
type countedBlock struct {
	block profileBlock
	count int64
}

// blockCounts maps the position of each block of a profile to the block and
// its count.
type blockCounts map[string]countedBlock

// readBlockCounts reads a cover profile, summing the counts of a block the
// profile lists more than once; an unreadable line is skipped.
func readBlockCounts(profile []byte) blockCounts {
	counts := blockCounts{}
	scanner := bufio.NewScanner(bytes.NewReader(profile))
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, profileModePrefix) {
			continue
		}
		position, block, count, ok := parseBlock(text)
		if !ok {
			continue
		}
		entry := counts[position]
		entry.block = block
		entry.count += count
		counts[position] = entry
	}
	return counts
}

// lines are the lines of the block, relative to the module root; none for a
// file outside the module.
func (b profileBlock) lines(module string) []vacuity.Line {
	relative, inside := b.relativeFile(module)
	if !inside {
		return nil
	}
	lines := make([]vacuity.Line, 0, b.last-b.first+1)
	for line := b.first; line <= b.last; line++ {
		lines = append(lines, vacuity.Line{File: relative, Line: line})
	}
	return lines
}

// relativeFile gives the file of the block relative to the module root, and
// whether it lies inside the module.
func (b profileBlock) relativeFile(module string) (string, bool) {
	return strings.CutPrefix(b.file, module+importPathSeparator)
}

func sortedLines(lines []vacuity.Line) []vacuity.Line {
	slices.SortFunc(lines, format.CompareScanLines)
	return slices.Compact(lines)
}

// parseBlock reads `file.go:12.5,14.2 2 1`: the position, the file with the
// first and last line and column of the block, and its count.
func parseBlock(text string) (string, profileBlock, int64, bool) {
	fields := strings.Fields(text)
	if len(fields) != profileFields {
		return "", profileBlock{}, 0, false
	}
	count, countErr := strconv.ParseInt(fields[2], decimalBase, countBits)
	separator := strings.LastIndex(fields[0], positionSeparator)
	if countErr != nil || count < 0 || separator < 0 {
		return "", profileBlock{}, 0, false
	}
	start, end, found := strings.Cut(fields[0][separator+len(positionSeparator):], rangeSeparator)
	if !found {
		return "", profileBlock{}, 0, false
	}
	first, firstColumn, firstErr := positionOf(start)
	last, lastColumn, lastErr := positionOf(end)
	if firstErr != nil || lastErr != nil || last < first {
		return "", profileBlock{}, 0, false
	}
	block := profileBlock{file: fields[0][:separator], first: first, last: last, firstColumn: firstColumn, lastColumn: lastColumn}
	return fields[0], block, count, true
}

// The count of a block is a decimal 64-bit integer.
const (
	decimalBase = 10
	countBits   = 64
)

// positionOf reads `12.5`: a line and a column.
func positionOf(position string) (int, int, error) {
	lineText, columnText, found := strings.Cut(position, columnSeparator)
	if !found {
		return 0, 0, strconv.ErrSyntax
	}
	line, err := strconv.Atoi(lineText)
	if err != nil {
		return 0, 0, err
	}
	column, err := strconv.Atoi(columnText)
	if err != nil {
		return 0, 0, err
	}
	return line, column, nil
}
