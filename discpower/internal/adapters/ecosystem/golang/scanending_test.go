package golang_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const (
	verifiesReferenceEnding = " [verifies SP-2108~3]"
	endingRunTimeout        = time.Minute
)

func TestScanRunnerEndings(t *testing.T) {
	suite := golang.New(golang.Dependencies{Process: process.New(), Locator: goparser.NewLocator()}).Scan()
	run := func(t *testing.T, script string) ports.ScanRuns {
		t.Helper()
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/rates\n\ngo 1.26\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		toolchain := ecosystem.Toolchain{Command: []string{"sh", "-c", script, "go"}}
		ran, err := suite.Run(context.Background(), ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}, toolchain, ports.ScanRunRequest{Runs: vacuity.RunsPerOutcome, Timeout: endingRunTimeout})
		if err != nil {
			t.Fatal(err)
		}
		return ran
	}
	t.Run("a go command that fails before naming a test reports no test and how it ended"+verifiesReferenceEnding, func(t *testing.T) {
		ran := run(t, "echo 'rates.go:3:1: syntax error: non-declaration statement outside function body' >&2; exit 1")
		if len(ran.Cases) != 0 || !strings.Contains(ran.Failure, "exited 1") || !strings.Contains(ran.Failure, "syntax error") {
			t.Fatalf("cases %v, failure %q", ran.Cases, ran.Failure)
		}
	})
	t.Run("a module with no test file is no failure"+verifiesReferenceEnding, func(t *testing.T) {
		if ran := run(t, "exit 0"); len(ran.Cases) != 0 || ran.Failure != "" || ran.Killed {
			t.Fatalf("runs %+v", ran)
		}
	})
	t.Run("a go command killed by SIGKILL from outside marks the batch killed"+verifiesReferenceEnding, func(t *testing.T) {
		if ran := run(t, "kill -KILL $$"); !ran.Killed {
			t.Fatalf("runs %+v", ran)
		}
	})
	t.Run("a coverage binary whose build SIGKILL ended from outside fails the recording as a killed runner [verifies SP-2658~1]", func(t *testing.T) {
		dir := t.TempDir()
		files := map[string]string{
			"go.mod":        "module example.com/rates\n\ngo 1.26\n",
			"rates.go":      "package rates\n\nfunc Fee(total int) int { return total / 10 }\n",
			"rates_test.go": "package rates\n\nimport \"testing\"\n\nfunc TestFee(t *testing.T) {\n\tif Fee(100) != 10 {\n\t\tt.Fatal(Fee(100))\n\t}\n}\n",
		}
		for name, content := range files {
			if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		killed := ecosystem.Toolchain{Command: []string{"sh", "-c", "kill -KILL $$", "go"}}
		request := ports.ScanCoverageRequest{Tests: []ports.ScanCase{{ID: "rates_test.go::TestFee", File: "rates_test.go"}}, Timeout: endingRunTimeout, Workers: 1}
		_, err := suite.LineCoverage(context.Background(), ecosystem.Workspace{Dir: dir, ResultsDir: t.TempDir()}, killed, request)
		if !errors.Is(err, ports.ErrRunnerKilled) {
			t.Fatalf("error %v", err)
		}
	})
}
