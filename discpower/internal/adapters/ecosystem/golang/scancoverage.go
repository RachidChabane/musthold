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
	"sync"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/suiterun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// ErrCoverageFailed is returned when a package's test binary cannot be built
// with coverage.
var ErrCoverageFailed = errors.New("the coverage build failed")

const (
	compileOnlyFlag = "-c"
	coverFlag       = "-cover"
	// coverModeFlag counts how often each block runs, exactly across
	// goroutines, so that a test's count can be told from its binary's
	// start-up.
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
)

// CoverageOfOneRun is false: each test runs alone under coverage.
func (scanSuite) CoverageOfOneRun() bool { return false }

// LineCoverage builds the test binary of each package once, with coverage of
// every package of the module in atomic mode, runs it once with no test to
// count the blocks its start-up runs, then runs each test alone, on the workers
// of the request and within its timeout, and reads the lines of the blocks it
// ran and, among them, the lines of the blocks it ran more often than the
// start-up did: the lines it owns. A test whose binary gives no start-up count
// owns no line. Only the workspace's build cache and results receive the
// instrumented binaries.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return nil, err
	}
	var runs []coveredRun
	binaries := map[string]*coverageBinary{}
	for _, test := range request.Tests {
		file, testPath, found := strings.Cut(string(test.ID), testCaseSeparator)
		if !found {
			continue
		}
		dir := path.Dir(file)
		if _, built := binaries[dir]; !built {
			if binaries[dir], err = s.prepareCoverageBinary(ctx, workspace, toolchain, dir, request.Timeout); err != nil {
				return nil, err
			}
		}
		runs = append(runs, coveredRun{test: test.ID, binary: binaries[dir], testPath: testPath})
	}
	return s.runAllCovered(ctx, workspace, module, runs, request)
}

// coverageBinary is the coverage binary of one package, with the counts of the
// blocks its start-up runs; nil counts when the start-up left no profile.
type coverageBinary struct {
	path, dir string
	startUp   blockCounts
}

// coveredRun is one test to run alone from its package's coverage binary.
type coveredRun struct {
	test     ecosystem.TestID
	binary   *coverageBinary
	testPath string
}

// prepareCoverageBinary builds the coverage binary of a package and counts the
// blocks its start-up runs, within the timeout of one run under coverage.
func (s scanSuite) prepareCoverageBinary(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, dir string, timeout time.Duration) (*coverageBinary, error) {
	built, err := s.buildCoverageBinary(ctx, workspace, toolchain, dir)
	if err != nil {
		return nil, err
	}
	binary := &coverageBinary{path: built, dir: dir}
	profile, err := s.runBinary(ctx, workspace, binary, startUpOnly, timeout)
	if err != nil {
		return nil, err
	}
	if profile != nil {
		binary.startUp = readBlockCounts(profile)
	}
	return binary, nil
}

// runAllCovered runs the tests on at most the requested workers and stops at
// the first error.
func (s scanSuite) runAllCovered(ctx context.Context, workspace ecosystem.Workspace, module string, runs []coveredRun, request ports.ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	covered := make(map[ecosystem.TestID]vacuity.Coverage, len(runs))
	var mutex sync.Mutex
	var failure error
	jobs := make(chan coveredRun)
	var wait sync.WaitGroup
	for range max(request.Workers, 1) {
		wait.Go(func() {
			for run := range jobs {
				coverage, err := s.runCovered(ctx, workspace, run, module, request.Timeout)
				mutex.Lock()
				if err != nil && failure == nil {
					failure = err
					cancel()
				}
				covered[run.test] = coverage
				mutex.Unlock()
			}
		})
	}
	for _, run := range runs {
		if ctx.Err() != nil {
			break
		}
		jobs <- run
	}
	close(jobs)
	wait.Wait()
	if failure != nil {
		return nil, failure
	}
	return covered, nil
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

// runCovered runs one test of a built binary and reads what it ran and what it
// owns. A test that fails still reports what it ran.
func (s scanSuite) runCovered(ctx context.Context, workspace ecosystem.Workspace, run coveredRun, module string, timeout time.Duration) (vacuity.Coverage, error) {
	profile, err := s.runBinary(ctx, workspace, run.binary, expression([][]string{strings.Split(run.testPath, subtestSeparator)}), timeout)
	if err != nil || profile == nil {
		return vacuity.Coverage{}, err
	}
	return profileCoverage(profile, run.binary.startUp, module), nil
}

// runBinary runs the tests of a coverage binary that the expression selects,
// once, from its package directory, as `go test` does, within the timeout, and
// returns its cover profile; nil when it left none.
func (s scanSuite) runBinary(ctx context.Context, workspace ecosystem.Workspace, binary *coverageBinary, selection string, timeout time.Duration) ([]byte, error) {
	profile := suiterun.ResultsFile(workspace, profileExtension)
	argv := []string{binary.path, binaryRunFlag, selection, binaryCountFlag, binaryProfileFlag + profile}
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: filepath.Join(workspace.Dir, filepath.FromSlash(binary.dir)), Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("run %s of %s with coverage: %w", selection, binary.dir, err)
	}
	if executed.TimedOut {
		return nil, fmt.Errorf("%w: %s of %s ran past %s", ports.ErrCoverageTimedOut, selection, binary.dir, timeout)
	}
	content, err := os.ReadFile(profile)
	if err != nil {
		return nil, nil
	}
	return content, nil
}

// profileBlock is one block of a cover profile: its file and its first and
// last line.
type profileBlock struct {
	file        string
	first, last int
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

// profileCoverage reads a test's cover profile against its binary's start-up:
// the lines of every block it ran, and the lines of every block it ran more
// often than the start-up did, as paths relative to the module root. A block of
// a file outside the module is left out; with no start-up counts the test owns
// no line.
func profileCoverage(profile []byte, startUp blockCounts, module string) vacuity.Coverage {
	var coverage vacuity.Coverage
	for position, entry := range readBlockCounts(profile) {
		if entry.count == 0 {
			continue
		}
		lines := entry.block.lines(module)
		coverage.Lines = append(coverage.Lines, lines...)
		if startUp != nil && entry.count > startUp[position].count {
			coverage.Owned = append(coverage.Owned, lines...)
		}
	}
	return vacuity.Coverage{Lines: sortedLines(coverage.Lines), Owned: sortedLines(coverage.Owned)}
}

// lines are the lines of the block, relative to the module root; none for a
// file outside the module.
func (b profileBlock) lines(module string) []vacuity.Line {
	relative, inside := strings.CutPrefix(b.file, module+importPathSeparator)
	if !inside {
		return nil
	}
	lines := make([]vacuity.Line, 0, b.last-b.first+1)
	for line := b.first; line <= b.last; line++ {
		lines = append(lines, vacuity.Line{File: relative, Line: line})
	}
	return lines
}

func sortedLines(lines []vacuity.Line) []vacuity.Line {
	slices.SortFunc(lines, format.CompareScanLines)
	return slices.Compact(lines)
}

// parseBlock reads `file.go:12.5,14.2 2 1`: the position, the file with the
// first and last line of the block, and its count.
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
	first, firstErr := lineOf(start)
	last, lastErr := lineOf(end)
	if firstErr != nil || lastErr != nil || last < first {
		return "", profileBlock{}, 0, false
	}
	return fields[0], profileBlock{file: fields[0][:separator], first: first, last: last}, count, true
}

// The count of a block is a decimal 64-bit integer.
const (
	decimalBase = 10
	countBits   = 64
)

func lineOf(position string) (int, error) {
	line, _, _ := strings.Cut(position, columnSeparator)
	return strconv.Atoi(line)
}
