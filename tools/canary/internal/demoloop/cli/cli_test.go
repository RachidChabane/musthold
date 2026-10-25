package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/cli"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
)

const (
	repoFromHere = "../../../../.."
	recording    = "../app/testdata/recording"
	token        = "sk-ant-oat01-dry-run-token"
)

func run(t *testing.T, environ []string, args ...string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := cli.Run(cli.Env{
		Args: args, Stdin: strings.NewReader(""), Stdout: &stdout, Stderr: &stderr, Environ: environ,
		Now: func() time.Time { return time.Date(2026, time.October, 25, 12, 0, 0, 0, time.Local) }, Executable: "/repo/.tools/bin/spec-proof-demo-loop",
	})
	return code, stdout.String(), stderr.String()
}

func TestRender(t *testing.T) {
	t.Run("a recording renders into a directory and then passes the check, a changed transcript fails it [verifies SP-2406~1]", func(t *testing.T) {
		out := t.TempDir()
		if code, _, stderr := run(t, nil, "render", "--records", recording, "--out", out); code != cli.ExitOK {
			t.Fatalf("render exited %d: %s", code, stderr)
		}
		if code, _, stderr := run(t, nil, "render", "--records", recording, "--out", out, "--check"); code != cli.ExitOK {
			t.Fatalf("check exited %d: %s", code, stderr)
		}
		if err := os.WriteFile(filepath.Join(out, "transcript.md"), []byte("edited"), 0o644); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := run(t, nil, "render", "--records", recording, "--out", out, "--check"); code != cli.ExitFailure || !strings.Contains(stderr, "differs") {
			t.Fatalf("check of a changed transcript exited %d: %s", code, stderr)
		}
	})

	t.Run("a command line without its arguments is a usage error [verifies SP-2406~1]", func(t *testing.T) {
		for _, args := range [][]string{nil, {"render"}, {"keep", "--run", "x"}, {"run"}, {"unknown"}} {
			if code, _, _ := run(t, nil, args...); code != cli.ExitUsage {
				t.Errorf("%v exited %d", args, code)
			}
		}
	})
}

func TestDryRun(t *testing.T) {
	t.Run("a dry run of the committed configuration prints the plan with a placeholder for the token [verifies SP-2403~1]", func(t *testing.T) {
		repo, err := filepath.Abs(repoFromHere)
		if err != nil {
			t.Fatal(err)
		}
		environ := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), launch.OAuthTokenVariable + "=" + token}
		code, stdout, stderr := run(t, environ, "run", "--config", filepath.Join(repo, "demo/loop/demo.json"), "--repo", repo, "--out", t.TempDir(), "--dry-run")
		if code != cli.ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		for _, wanted := range []string{
			"credentials: source environment (CLAUDE_CODE_OAUTH_TOKEN=<set>, given to the session only)",
			"/.tools/claude-code/" + strings.TrimSpace(readFile(t, filepath.Join(repo, "script/claude-code.pin"))) + "/bin/claude -p",
			"/shim/shallnot   (records the hook calls of " + filepath.Join(repo, ".tools/bin/shallnot"),
		} {
			if !strings.Contains(stdout, wanted) {
				t.Errorf("the plan lacks %q:\n%s", wanted, stdout)
			}
		}
		if strings.Contains(stdout+stderr, token) {
			t.Error("the plan prints the token")
		}
	})
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}
