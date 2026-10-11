package ports

import (
	"context"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
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

// ScanRuns is what a batch of runs did: the tests the runner collected and,
// per test, the result of each run, in order. A test that was asked for and
// never reported has no entry.
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
	// LineCoverage records, per test, the lines of the project's source files
	// the test itself ran. Test files are never reported.
	LineCoverage(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, tests []ScanCase) (map[ecosystem.TestID][]vacuity.Line, error)
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
