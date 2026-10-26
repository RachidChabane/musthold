package golang

import (
	"context"
	"fmt"
	"os"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

var _ ports.WitnessRunning = (*Strategy)(nil)

// WitnessRunner runs a scan's witnesses with `go test -json`, which needs no
// tool the module declares, where the suite runner of `prove` runs gotestsum.
func (s *Strategy) WitnessRunner() ports.SuiteRunner { return eventsRunner{deps: s.deps} }

// eventsRunner is the suite runner that reads each test's outcome from the
// events of `go test -json`, by the package import path and the test name a
// test's id is made of.
type eventsRunner struct {
	deps Dependencies
}

// Run executes `go test -json -count=1 <selection>`, every package for an
// empty selection, within the strategy's timeout. A test binary that does not
// compile fails the collection; a test named by the events with no end event
// errored.
func (r eventsRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	args := selection.Args
	if len(args) == 0 {
		args = []string{allPackages}
	}
	argv := append(append([]string{}, toolchain.Command...), testSubcommand, jsonFlag, countFlag)
	argv = append(argv, args...)
	events := suiterun.ResultsFile(workspace, eventsSuffix)
	executed, err := r.deps.Process.Run(ctx, ports.Command{Argv: argv, Dir: workspace.Dir, Timeout: r.deps.Timeout, StdoutFile: events})
	if err != nil {
		return ecosystem.RunResult{}, fmt.Errorf("run the tests: %w", err)
	}
	output, err := os.ReadFile(events)
	if err != nil {
		return ecosystem.RunResult{}, fmt.Errorf("read the test events: %w", err)
	}
	return outcomesOf(readProcess(output, executed.ExitCode, executed.TimedOut)), nil
}

// testOutcomes are the outcomes of a test's last end event, a skip ending its
// run as an errored one.
var testOutcomes = map[vacuity.Run]ecosystem.TestOutcome{
	vacuity.RunPassed:  ecosystem.TestPassed,
	vacuity.RunFailed:  ecosystem.TestFailed,
	vacuity.RunErrored: ecosystem.TestSkipped,
}

// outcomesOf reads the outcome of every test the events name, a leaf or a
// test with subtests, by its package and name.
func outcomesOf(process testProcess) ecosystem.RunResult {
	result := ecosystem.RunResult{Outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{}, TimedOut: process.timedOut, CollectionFailed: process.buildFailed}
	for key := range process.named {
		runs := process.leaves[key]
		if parent, isParent := process.parents[key]; isParent {
			runs = parent
		}
		outcome := ecosystem.TestErrored
		if len(runs) > 0 {
			outcome = testOutcomes[runs[len(runs)-1]]
		}
		result.Outcomes[ecosystem.NewTestID(key.pkg, key.test)] = outcome
	}
	return result
}
