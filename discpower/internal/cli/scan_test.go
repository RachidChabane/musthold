package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/cli"
)

const verifiesScanCommand = " [verifies SP-2119~3]"

func runScan(t *testing.T, workDir string, arguments ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Run(cli.Env{Binary: helpBinary, Stdout: &out, Stderr: &errOut, WorkDir: workDir}, arguments)
	return code, out.String(), errOut.String()
}

func TestScanCommandLine(t *testing.T) {
	t.Run("an unknown output format is a tool failure that names the formats"+verifiesScanCommand, func(t *testing.T) {
		code, _, stderr := runScan(t, t.TempDir(), "scan", "--format", "yaml")
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "text, md or json") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
	t.Run("a tree without a pytest, Go, Jest or Vitest suite is a tool failure that says what a scan runs"+verifiesScanCommand, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runScan(t, dir, "scan")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "a scan runs pytest, Go, Jest and Vitest test suites") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("a positional argument of scan is refused"+verifiesScanCommand, func(t *testing.T) {
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "extra"); code != cli.ExitToolFailure || !strings.Contains(stderr, "unexpected arguments") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
	t.Run("a pull request number and --base exclude each other, and a second number is refused [verifies SP-2244~1]", func(t *testing.T) {
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "12", "--base", "main"); code != cli.ExitToolFailure || !strings.Contains(stderr, "--base and a pull request number exclude each other") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "--host", "12", "13"); code != cli.ExitToolFailure || !strings.Contains(stderr, "at most one pull request number") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
	t.Run("--host and --resume are flags of scan, and a tree with no suite fails before any session [verifies SP-2244~1]", func(t *testing.T) {
		code, stdout, stderr := runScan(t, t.TempDir(), "scan", "--resume", "7")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "a scan runs pytest, Go, Jest and Vitest test suites") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		_, stdout, stderr = runScan(t, t.TempDir(), "scan", "--help")
		for _, want := range []string{"-host", "-resume", "<PR>"} {
			if !strings.Contains(stdout+stderr, want) {
				t.Fatalf("the help of scan lacks %q:\n%s%s", want, stdout, stderr)
			}
		}
	})
	t.Run("a replay takes one directory that holds a finding"+verifiesScanCommand, func(t *testing.T) {
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "replay"); code != cli.ExitToolFailure || !strings.Contains(stderr, "replay takes one directory") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "replay", t.TempDir()); code != cli.ExitToolFailure || !strings.Contains(stderr, "no finding.json") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
}
