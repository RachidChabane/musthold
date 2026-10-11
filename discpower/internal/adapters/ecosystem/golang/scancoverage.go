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
	compileOnlyFlag   = "-c"
	coverFlag         = "-cover"
	coverModeFlag     = "-covermode=set"
	coverPackagesFlag = "-coverpkg=" + allPackages
	binaryExtension   = ".test"
	profileExtension  = ".cov"
	binaryRunFlag     = "-test.run"
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
// every package of the module, then runs each test alone, on the workers of
// the request and within its timeout, and reads the lines of the blocks it
// ran. Only the workspace's build cache and results receive the instrumented
// binaries.
func (s scanSuite) LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID][]vacuity.Line, error) {
	module, err := modulePath(os.DirFS(workspace.Dir))
	if err != nil {
		return nil, err
	}
	var runs []coveredRun
	binaries := map[string]string{}
	for _, test := range request.Tests {
		file, testPath, found := strings.Cut(string(test.ID), testCaseSeparator)
		if !found {
			continue
		}
		dir := path.Dir(file)
		if _, built := binaries[dir]; !built {
			if binaries[dir], err = s.buildCoverageBinary(ctx, workspace, toolchain, dir); err != nil {
				return nil, err
			}
		}
		runs = append(runs, coveredRun{test: test.ID, binary: binaries[dir], dir: dir, testPath: testPath})
	}
	return s.runAllCovered(ctx, workspace, module, runs, request)
}

// coveredRun is one test to run alone from its package's coverage binary.
type coveredRun struct {
	test                  ecosystem.TestID
	binary, dir, testPath string
}

// runAllCovered runs the tests on at most the requested workers and stops at
// the first error.
func (s scanSuite) runAllCovered(ctx context.Context, workspace ecosystem.Workspace, module string, runs []coveredRun, request ports.ScanCoverageRequest) (map[ecosystem.TestID][]vacuity.Line, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	covered := make(map[ecosystem.TestID][]vacuity.Line, len(runs))
	var mutex sync.Mutex
	var failure error
	jobs := make(chan coveredRun)
	var wait sync.WaitGroup
	for range max(request.Workers, 1) {
		wait.Go(func() {
			for run := range jobs {
				lines, err := s.runCovered(ctx, workspace, run.binary, run.dir, run.testPath, module, request.Timeout)
				mutex.Lock()
				if err != nil && failure == nil {
					failure = err
					cancel()
				}
				covered[run.test] = lines
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

// runCovered runs one test of a built binary from its package directory, as
// `go test` does, within the timeout, and reads the lines its profile marks as
// run. A test that fails still reports what it ran.
func (s scanSuite) runCovered(ctx context.Context, workspace ecosystem.Workspace, binary, dir, testPath, module string, timeout time.Duration) ([]vacuity.Line, error) {
	profile := suiterun.ResultsFile(workspace, profileExtension)
	argv := []string{binary, binaryRunFlag, expression([][]string{strings.Split(testPath, subtestSeparator)}), binaryCountFlag, binaryProfileFlag + profile}
	executed, err := s.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: filepath.Join(workspace.Dir, filepath.FromSlash(dir)), Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("run %s with coverage: %w", testPath, err)
	}
	if executed.TimedOut {
		return nil, fmt.Errorf("%w: %s ran past %s", ports.ErrCoverageTimedOut, testPath, timeout)
	}
	content, err := os.ReadFile(profile)
	if err != nil {
		return nil, nil
	}
	return profileLines(content, module), nil
}

// profileLines reads the lines of the blocks a cover profile counts as run, as
// paths relative to the module root. A block of a file outside the module is
// left out.
func profileLines(profile []byte, module string) []vacuity.Line {
	var lines []vacuity.Line
	scanner := bufio.NewScanner(bytes.NewReader(profile))
	for scanner.Scan() {
		text := scanner.Text()
		if strings.HasPrefix(text, profileModePrefix) {
			continue
		}
		file, first, last, ran := profileBlock(text)
		if !ran {
			continue
		}
		relative, inside := strings.CutPrefix(file, module+importPathSeparator)
		if !inside {
			continue
		}
		for line := first; line <= last; line++ {
			lines = append(lines, vacuity.Line{File: relative, Line: line})
		}
	}
	slices.SortFunc(lines, format.CompareScanLines)
	return slices.Compact(lines)
}

// profileBlock reads `file.go:12.5,14.2 2 1`: the file, the first and last line
// of the block, and whether its count is above zero.
func profileBlock(text string) (file string, first, last int, ran bool) {
	fields := strings.Fields(text)
	if len(fields) != profileFields || fields[2] == "0" {
		return "", 0, 0, false
	}
	separator := strings.LastIndex(fields[0], positionSeparator)
	if separator < 0 {
		return "", 0, 0, false
	}
	start, end, found := strings.Cut(fields[0][separator+len(positionSeparator):], rangeSeparator)
	if !found {
		return "", 0, 0, false
	}
	first, firstErr := lineOf(start)
	last, lastErr := lineOf(end)
	if firstErr != nil || lastErr != nil || last < first {
		return "", 0, 0, false
	}
	return fields[0][:separator], first, last, true
}

func lineOf(position string) (int, error) {
	line, _, _ := strings.Cut(position, columnSeparator)
	return strconv.Atoi(line)
}
