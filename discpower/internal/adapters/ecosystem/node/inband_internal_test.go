package node

import (
	"context"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

const verifiesInBand = " [verifies SP-2655~1]"

// shownConfig answers `jest --showConfig` with one project, records every
// command and answers the others with a clean exit.
type shownConfig struct {
	commands [][]string
}

func (s *shownConfig) Run(_ context.Context, command ports.Command) (ports.CommandResult, error) {
	s.commands = append(s.commands, command.Argv)
	if slices.Contains(command.Argv, jestShowConfigFlag) {
		return ports.CommandResult{Stdout: []byte(`{"configs":[{"rootDir":"/work","testEnvironment":"jsdom"}],"globalConfig":{}}`)}, nil
	}
	return ports.CommandResult{}, nil
}

func TestJestRunsInBand(t *testing.T) {
	ctx := context.Background()
	workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
	toolchain := ecosystem.Toolchain{Command: []string{"yarn"}}
	process := &shownConfig{}
	deps := Dependencies{Process: process, Reader: emptyReader{}}

	t.Run("a scan's run of Jest, whole or selected, is one process in band"+verifiesInBand, func(t *testing.T) {
		suite := scanSuite{deps: deps, framework: jest{}, cache: newScanCache()}
		for _, files := range [][]string{nil, {"test/merge.test.ts"}} {
			argv, _, err := jest{}.scanCommand(ctx, suite, workspace, toolchain, t.TempDir(), files)
			if err != nil || !slices.Contains(argv, jestSingleProcessFlag) {
				t.Fatalf("argv %v, error %v", argv, err)
			}
		}
	})
	t.Run("a run of a mutant's tests with Jest is one process in band"+verifiesInBand, func(t *testing.T) {
		if _, err := (jestRunner{deps: deps}).Run(ctx, workspace, toolchain, ecosystem.Selection{}); err != nil {
			t.Fatal(err)
		}
		last := process.commands[len(process.commands)-1]
		if !slices.Contains(last, jestSingleProcessFlag) {
			t.Fatalf("argv %v", last)
		}
	})
	t.Run("the reproduction of a finding runs Jest in band"+verifiesInBand, func(t *testing.T) {
		if argv := (jest{}).reproArguments("test/merge.test.ts", "^merge$"); !slices.Contains(argv, jestSingleProcessFlag) {
			t.Fatalf("argv %v", argv)
		}
	})
}

// emptyReader reads no outcome.
type emptyReader struct{}

func (emptyReader) Read([]string, ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	return map[ecosystem.TestID]ecosystem.TestOutcome{}, nil
}
