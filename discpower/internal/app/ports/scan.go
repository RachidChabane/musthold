package ports

import (
	"context"
	"errors"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

// ScanCase is one test of a suite as a scan addresses it.
type ScanCase struct {
	// ID is the runner-native address of the test: the pytest node id, or for Go
	// the test file and the subtest path, `pkg/x_test.go::TestX/sub`.
	ID ecosystem.TestID
	// File is the test file, relative POSIX.
	File string
}

// ScanRunRequest is one batch of runs of a selection.
type ScanRunRequest struct {
	// Tests are the tests to run; empty runs the whole suite.
	Tests []ecosystem.TestID
	// Runs is how many times each test runs.
	Runs int
	// Timeout bounds one run of the selection; a run it stops leaves every
	// test without a result timed out, and no later run starts.
	Timeout time.Duration
}

// ErrCoverageTimedOut is returned when a run of tests under coverage outlives
// the timeout of its recording.
var ErrCoverageTimedOut = errors.New("a run of tests under coverage outlived its timeout")

// ScanCoverageRequest is one recording of the line coverage of tests.
type ScanCoverageRequest struct {
	Tests []ScanCase
	// Timeout bounds each run of tests under coverage; a run it stops fails the
	// recording with ErrCoverageTimedOut.
	Timeout time.Duration
	// Workers bounds how many runs under coverage go at once, for a runner that
	// runs each test alone; below one, one.
	Workers int
	// Alone records each requested test in a run that selects it and no other
	// test, even for a runner whose recording of the suite is one run.
	Alone bool
	// Owners are the requested tests whose owned lines the scan reads; nil
	// stands for every requested test. A runner that records the owned lines
	// of a test in runs of its own may give each other test its lines only.
	Owners []ecosystem.TestID
}

// ScanRuns is what a batch of runs did: the tests the runner collected and,
// per test, the result of each run, in order. A run is not_started only on
// the runner's positive evidence that its process stopped before any test
// began.
type ScanRuns struct {
	Cases []ScanCase
	Runs  map[ecosystem.TestID][]vacuity.Run
}

// ScanSuite is the Strategy through which a scan runs the tests of one
// ecosystem. Its instrumentation ships with the tool and is applied only inside
// the workspace.
type ScanSuite interface {
	// Run runs the selection the requested number of times.
	Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ScanRunRequest) (ScanRuns, error)
	// CoverageOfOneRun reports whether LineCoverage records every requested
	// test in one run of the suite, so that recording fewer tests saves no
	// time.
	CoverageOfOneRun() bool
	// LineCoverage records, per requested test, the lines of the project's
	// source files its run executed and, among them, the lines the test itself
	// ran beyond the start-up of its process: the lines it owns. Test files are
	// never reported.
	LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, request ScanCoverageRequest) (map[ecosystem.TestID]vacuity.Coverage, error)
	// Operation binds a definition of the workspace to the probe target that
	// calls it without a spec, when the ecosystem can call it as a function.
	Operation(workspace ecosystem.Workspace, file, qualname string) (string, bool)
	// Repro is the command line that runs one test from the project root, with
	// the interpreter or tool the shell finds.
	Repro(test ecosystem.TestID) []string
}

// Scanning is implemented by an ecosystem strategy a scan can run.
type Scanning interface {
	Scan() ScanSuite
}

// SourceControl reads the version-control state of the scanned project.
type SourceControl interface {
	// Head is the full id of the commit the work tree is at.
	Head(ctx context.Context) (string, error)
	// Resolve gives the full commit id a revision names.
	Resolve(ctx context.Context, revision string) (string, error)
	// Dirty reports whether a tracked file differs from the head commit.
	Dirty(ctx context.Context) (bool, error)
	// TrackedFiles lists the tracked files of the head commit, relative POSIX.
	TrackedFiles(ctx context.Context) ([]string, error)
	// ChangedLines lists, per file, the lines of the head commit that differ
	// from the base commit.
	ChangedLines(ctx context.Context, base string) (map[string][]int, error)
	// Name names the repository: `owner/name` of its origin, or its directory.
	Name(ctx context.Context) string
}
