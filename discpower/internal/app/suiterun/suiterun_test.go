package suiterun_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/app/suiterun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

type fakeProcess struct {
	result ports.CommandResult
	err    error
}

func (f fakeProcess) Run(context.Context, ports.Command) (ports.CommandResult, error) {
	return f.result, f.err
}

type fakeReader struct {
	outcomes map[ecosystem.TestID]ecosystem.TestOutcome
	err      error
	called   *bool
}

func (f fakeReader) Read([]string, ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error) {
	if f.called != nil {
		*f.called = true
	}
	return f.outcomes, f.err
}

const collectionExit = 2

func request() suiterun.Request {
	return suiterun.Request{Format: ecosystem.ReportPytest, CollectionFailureExits: []int{collectionExit}}
}

func TestRun(t *testing.T) {
	id := ecosystem.NewTestID("c", "n")
	outcomes := map[ecosystem.TestID]ecosystem.TestOutcome{id: ecosystem.TestPassed}

	t.Run("outcomes are read after a normal run [verifies SP-66~1]", func(t *testing.T) {
		got, err := suiterun.Run(context.Background(), fakeProcess{}, fakeReader{outcomes: outcomes}, request())
		if err != nil || got.Outcomes[id] != ecosystem.TestPassed || got.TimedOut || got.CollectionFailed {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("a timeout is a result and nothing is read [verifies SP-67~1]", func(t *testing.T) {
		called := false
		got, err := suiterun.Run(context.Background(), fakeProcess{result: ports.CommandResult{TimedOut: true}}, fakeReader{called: &called}, request())
		if err != nil || !got.TimedOut || called {
			t.Fatalf("%+v %v read=%v", got, err, called)
		}
	})

	t.Run("a collection exit code is a collection failure [verifies SP-66~1]", func(t *testing.T) {
		got, err := suiterun.Run(context.Background(), fakeProcess{result: ports.CommandResult{ExitCode: collectionExit}}, fakeReader{outcomes: outcomes}, request())
		if err != nil || !got.CollectionFailed {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("unreadable results are a collection failure [verifies SP-66~1]", func(t *testing.T) {
		got, err := suiterun.Run(context.Background(), fakeProcess{}, fakeReader{err: errors.New("bad xml")}, request())
		if err != nil || !got.CollectionFailed {
			t.Fatalf("%+v %v", got, err)
		}
	})

	t.Run("a command that cannot start is an error [verifies SP-66~1]", func(t *testing.T) {
		if _, err := suiterun.Run(context.Background(), fakeProcess{err: errors.New("no such file")}, fakeReader{}, request()); err == nil {
			t.Fatal("expected an error")
		}
	})
}

func TestResultsFileIsUniquePerCall(t *testing.T) {
	t.Run("each call names a new file inside the results directory [verifies SP-66~1]", func(t *testing.T) {
		workspace := ecosystem.Workspace{ResultsDir: "/work/results/reference"}
		first, second := suiterun.ResultsFile(workspace, ".xml"), suiterun.ResultsFile(workspace, ".xml")
		if first == second || !strings.HasPrefix(first, workspace.ResultsDir) || !strings.HasSuffix(first, ".xml") {
			t.Fatalf("%q %q", first, second)
		}
	})
}
