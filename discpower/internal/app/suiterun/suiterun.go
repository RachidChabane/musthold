// Package suiterun is the part of a suite run every ecosystem strategy shares:
// run the runner's command under its deadline, then read the per-test outcomes
// its reporter wrote.
package suiterun

import (
	"context"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
)

const resultsFilePrefix = "results-"

var resultsCounter atomic.Uint64

// ResultsFile returns a results file path inside the workspace's results
// directory that no earlier run of this process used, so a run never reads the
// leftovers of another one.
func ResultsFile(workspace ecosystem.Workspace, extension string) string {
	n := resultsCounter.Add(1)
	return filepath.Join(workspace.ResultsDir, resultsFilePrefix+strconv.FormatUint(n, 10)+extension)
}

// Request describes one suite run.
type Request struct {
	Command ports.Command
	// ResultsPatterns are the glob patterns of the files the reporter writes.
	ResultsPatterns []string
	Format          ecosystem.ReportFormat
	// CollectionFailureExits lists the exit codes that mean the runner could not
	// collect or compile the tests.
	CollectionFailureExits []int
}

// Run executes the request. A timeout and a collection failure are results;
// only a command that cannot start is an error.
func Run(ctx context.Context, process ports.ProcessRunner, reader ports.ResultsReader, request Request) (ecosystem.RunResult, error) {
	executed, err := process.Run(ctx, request.Command)
	if err != nil {
		return ecosystem.RunResult{}, fmt.Errorf("run the suite: %w", err)
	}
	if executed.TimedOut {
		return ecosystem.RunResult{TimedOut: true}, nil
	}
	if slices.Contains(request.CollectionFailureExits, executed.ExitCode) {
		return ecosystem.RunResult{CollectionFailed: true}, nil
	}
	outcomes, err := reader.Read(request.ResultsPatterns, request.Format)
	if err != nil {
		return ecosystem.RunResult{CollectionFailed: true}, nil
	}
	return ecosystem.RunResult{Outcomes: outcomes}, nil
}
