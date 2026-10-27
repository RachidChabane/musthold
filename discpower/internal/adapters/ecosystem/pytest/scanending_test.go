package pytest_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const (
	verifiesReferenceEnding = " [verifies SP-2108~3]"
	// polyglotFixture declares pytest for tooling whose conftest.py imports a
	// package the project does not install.
	polyglotFixture = "../../../../fixtures/scan/vitest-polyglot"
)

// interpreterExiting is an interpreter that prints a line to its standard
// error and exits with the code, whatever pytest arguments it is given.
func interpreterExiting(message, code string) ecosystem.Toolchain {
	return ecosystem.Toolchain{Command: []string{"sh", "-c", "echo '" + message + "' >&2; exit " + code, "python3"}}
}

func TestScanRunnerEndings(t *testing.T) {
	suite := pytest.New(pytest.Dependencies{Process: process.New(), Locator: treesitter.NewLocator()}).Scan()
	run := func(t *testing.T, toolchain ecosystem.Toolchain) ports.ScanRuns {
		t.Helper()
		workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
		ran, err := suite.Run(context.Background(), workspace, toolchain, ports.ScanRunRequest{Runs: vacuity.RunsPerOutcome, Timeout: scanRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		return ran
	}
	t.Run("an interpreter without pytest reports no test and how it ended"+verifiesReferenceEnding, func(t *testing.T) {
		ran := run(t, interpreterExiting("No module named pytest", "1"))
		if len(ran.Cases) != 0 || !strings.Contains(ran.Failure, "exited 1") || !strings.Contains(ran.Failure, "No module named pytest") {
			t.Fatalf("cases %v, failure %q", ran.Cases, ran.Failure)
		}
	})
	t.Run("a session that collected no test is no failure"+verifiesReferenceEnding, func(t *testing.T) {
		if ran := run(t, interpreterExiting("no tests ran", "5")); len(ran.Cases) != 0 || ran.Failure != "" {
			t.Fatalf("cases %v, failure %q", ran.Cases, ran.Failure)
		}
	})
	t.Run("a session a conftest.py stopped before collection reports no test and no failure"+verifiesReferenceEnding, func(t *testing.T) {
		project := openScanFixture(t, polyglotFixture)
		ran, err := project.suite.Run(context.Background(), project.workspace, project.toolchain, ports.ScanRunRequest{Runs: vacuity.RunsPerOutcome, Timeout: scanRunTimeout})
		if err != nil || len(ran.Cases) != 0 || ran.Failure != "" || ran.Killed {
			t.Fatalf("runs %+v, error %v", ran, err)
		}
	})
	t.Run("an interpreter killed by SIGKILL from outside marks the batch killed"+verifiesReferenceEnding, func(t *testing.T) {
		killed := ecosystem.Toolchain{Command: []string{"sh", "-c", "kill -KILL $$", "python3"}}
		if ran := run(t, killed); !ran.Killed {
			t.Fatalf("runs %+v", ran)
		}
	})
	t.Run("a recording of the coverage whose interpreter SIGKILL ended from outside fails as a killed runner [verifies SP-2658~1]", func(t *testing.T) {
		killed := ecosystem.Toolchain{Command: []string{"sh", "-c", "kill -KILL $$", "python3"}}
		workspace := ecosystem.Workspace{Dir: t.TempDir(), ResultsDir: t.TempDir()}
		request := ports.ScanCoverageRequest{Tests: []ports.ScanCase{{ID: takesPercentOff, File: discountTests}}, Timeout: scanRunTimeout, Workers: 1}
		if _, err := suite.LineCoverage(context.Background(), workspace, killed, request); !errors.Is(err, ports.ErrRunnerKilled) {
			t.Fatalf("error %v", err)
		}
	})
}
