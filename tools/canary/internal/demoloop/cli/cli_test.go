package cli_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/cli"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
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
		Now: func() time.Time { return time.Date(2026, time.October, 25, 12, 0, 0, 0, time.Local) }, Executable: "/repo/.tools/bin/musthold-demo-loop",
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
		for _, args := range [][]string{nil, {"render"}, {"keep", "--run", "x"}, {"live"}, {"unknown"}} {
			if code, _, _ := run(t, nil, args...); code != cli.ExitUsage {
				t.Errorf("%v exited %d", args, code)
			}
		}
	})
}

func TestDryRun(t *testing.T) {
	t.Run("a dry run of the committed configuration prints every docker command with the token shown as standard input [verifies SP-2403~2]", func(t *testing.T) {
		repo, err := filepath.Abs(repoFromHere)
		if err != nil {
			t.Fatal(err)
		}
		environ := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), launch.OAuthTokenVariable + "=" + token}
		code, stdout, stderr := run(t, environ, "live", "--config", filepath.Join(repo, "demo/loop/demo.json"), "--repo", repo, "--work", t.TempDir(), "--product", t.TempDir(), "--dry-run")
		if code != cli.ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		for _, wanted := range []string{
			"credentials: source environment, read once per session and written to the entry's standard input (<stdin>)",
			"egress allowlist: api.anthropic.com:443",
			"network create --internal",
			"--rm --network none",
			"/opt/musthold/bin/musthold-demo-loop entry -- /opt/musthold/claude/claude -p",
			"(standard input: CLAUDE_CODE_OAUTH_TOKEN=<stdin>)",
			"/shim/shallnot   (records the hook calls of /opt/musthold/bin/shallnot",
		} {
			if !strings.Contains(stdout, wanted) {
				t.Errorf("the plan lacks %q:\n%s", wanted, stdout)
			}
		}
		if strings.Contains(stdout+stderr, token) {
			t.Error("the plan prints the token")
		}
	})

	t.Run("with no work area named, the run directories go under the repository's work area [verifies SP-2418~1]", func(t *testing.T) {
		repo, err := filepath.Abs(repoFromHere)
		if err != nil {
			t.Fatal(err)
		}
		environ := []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir(), launch.OAuthTokenVariable + "=" + token}
		code, stdout, stderr := run(t, environ, "live", "--config", filepath.Join(repo, "demo/loop/demo.json"), "--repo", repo, "--product", t.TempDir(), "--dry-run")
		if code != cli.ExitOK {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		work := filepath.Join(repo, ".tools", "demo-loop", "work")
		if !strings.Contains(stdout, "work: "+work+"\n") || !strings.Contains(stdout, work+"/musthold-demo-loop-boxoffice-four-proofs-1-") {
			t.Errorf("the plan does not use %s:\n%s", work, stdout)
		}
		if tmp := os.TempDir(); strings.Contains(stdout, "work: "+tmp) {
			t.Errorf("the plan works in the system's temporary directory %s", tmp)
		}
	})
}
