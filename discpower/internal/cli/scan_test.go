package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
)

const verifiesScanCommand = " [verifies SP-2119~2]"

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
	t.Run("a tree without a pytest or Go suite is a tool failure that says what a scan runs"+verifiesScanCommand, func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# notes\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		code, stdout, stderr := runScan(t, dir, "scan")
		if code != cli.ExitToolFailure || stdout != "" || !strings.Contains(stderr, "a scan runs pytest and Go test suites") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
	t.Run("a positional argument of scan is refused"+verifiesScanCommand, func(t *testing.T) {
		if code, _, stderr := runScan(t, t.TempDir(), "scan", "extra"); code != cli.ExitToolFailure || !strings.Contains(stderr, "unexpected arguments") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
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
