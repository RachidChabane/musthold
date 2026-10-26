package e2e_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The project the dispatcher scans and the test it names.
const (
	scanFixture     = "discpower/fixtures/scan/pytest"
	vacuousTest     = "tests/test_discount.py::test_discount_accepts_any_percent"
	scanRequirement = "SP-2124~2"
	verifiesScan    = " [verifies SP-2124~2]"
)

// inProject runs a program in a directory under nice and returns how it ended.
func inProject(t *testing.T, dir string, env []string, name string, args ...string) result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), commandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "nice", append([]string{"-n", niceLevel, name}, args...)...)
	command.Dir = dir
	command.Env = append(os.Environ(), env...)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		code = exit.ExitCode()
	case err != nil:
		t.Fatalf("%s %v: %v", name, args, err)
	}
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestMustholdScan(t *testing.T) {
	requirements := []string{scanRequirement}
	if testing.Short() {
		skipRequirements(t, requirements, "builds the binaries and runs a scan")
	}
	if _, err := exec.LookPath("python3"); err != nil {
		skipRequirements(t, requirements, "no python3 to run the pytest project: "+err.Error())
	}
	requireMemory(t, requirements)
	bin := productBinaries(t)
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root, tmp := filepath.Join(base, "project"), filepath.Join(base, "tmp")
	if err := os.MkdirAll(tmp, directoryMode); err != nil {
		t.Fatal(err)
	}
	copyProject(t, filepath.Join(repositoryDir, filepath.FromSlash(scanFixture)), root)
	env := []string{"TMPDIR=" + tmp, "GOMAXPROCS=2", "PATH=" + os.Getenv("PATH")}
	for _, args := range [][]string{
		{"init", "-q"}, {"add", "-A"},
		{"-c", "user.name=scan", "-c", "user.email=scan@example.com", "-c", "commit.gpgsign=false", "commit", "-qm", "first"},
	} {
		if done := inProject(t, root, env, "git", args...); done.code != 0 {
			t.Fatalf("git %v: %s", args, done.output())
		}
	}

	scan := inProject(t, root, env, filepath.Join(bin, dispatcherBinary), "scan", "--budget", "0")
	t.Run("musthold scan runs the module's scan with no configuration and prints the test that verifies nothing"+verifiesScan, func(t *testing.T) {
		if scan.code != 0 || !strings.Contains(scan.stdout, "#1 "+vacuousTest+" passes on all ") || !strings.Contains(scan.stdout, "1/5 verify nothing") {
			t.Fatalf("exit %d:\n%s", scan.code, scan.output())
		}
	})
	t.Run("musthold scan replay re-executes the findings with no model"+verifiesScan, func(t *testing.T) {
		commit := strings.TrimSpace(inProject(t, root, env, "git", "rev-parse", "--short=7", "HEAD").stdout)
		replay := inProject(t, root, env, filepath.Join(bin, dispatcherBinary), "scan", "replay", filepath.Join(".musthold", "scan", commit))
		if replay.code != 0 || !strings.Contains(replay.stdout, vacuousTest) || strings.Contains(replay.stdout, "NOT held") {
			t.Fatalf("exit %d:\n%s", replay.code, replay.output())
		}
	})
}
