package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/cli"
)

const (
	recordedSession = "../domain/checks/testdata/claude-session"
	// claudeChecks is the number of checks of Claude Code.
	claudeChecks = 14

	recordedCopilotSession = "../domain/checks/testdata/copilot-session"
	// copilotChecks is the number of checks of Copilot CLI.
	copilotChecks = 15
)

func run(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code = cli.Run(cli.Env{
		Args: args, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut,
		Environ: []string{"PATH=/usr/bin"}, Now: func() time.Time { return time.Date(2026, time.October, 7, 22, 0, 0, 0, time.Local) },
		Executable: "/bin/canary",
	})
	return code, out.String(), errOut.String()
}

func TestCommandLine(t *testing.T) {
	t.Run("the modules are listed as directory, binary and build tags for the build script [verifies SP-1504~2]", func(t *testing.T) {
		code, stdout, _ := run(t, "", "modules")
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if code != 0 || len(lines) != 3 {
			t.Fatalf("exit %d, output %q", code, stdout)
		}
		fields := strings.Split(lines[0], "\t")
		if len(fields) != 3 || fields[0] != "discpower" || fields[1] != "spec-proof-dp" || fields[2] != "" {
			t.Errorf("first line = %q", lines[0])
		}
		if !strings.Contains(lines[1], "grammar_subset") {
			t.Errorf("second line = %q", lines[1])
		}
	})

	t.Run("a command line the binary cannot act on is a usage error [verifies SP-1504~2]", func(t *testing.T) {
		for _, args := range [][]string{nil, {"unknown"}, {"run", "--bogus"}, {"run", "extra"}, {"evaluate"}} {
			if code, _, _ := run(t, "", args...); code != cli.ExitUsage {
				t.Errorf("%v: exit %d, want %d", args, code, cli.ExitUsage)
			}
		}
	})

	t.Run("a dry run prints its plan and exits 0 [verifies SP-1504~2]", func(t *testing.T) {
		code, stdout, stderr := run(t, "", "run", "--harness", "codex", "--dry-run", "--repo", t.TempDir())
		if code != 0 || !strings.Contains(stdout, "codex exec -m gpt-6-luna") {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})

	t.Run("the model flag replaces the default of every harness in the printed plan [verifies SP-1514~2]", func(t *testing.T) {
		for harness, option := range map[string]string{"claude": "--model", "codex": "-m", "copilot": "--model"} {
			code, stdout, stderr := run(t, "", "run", "--harness", harness, "--dry-run", "--model", "custom-model", "--repo", t.TempDir())
			if code != 0 || !strings.Contains(stdout, option+" custom-model") {
				t.Fatalf("%s: exit %d, stdout %q, stderr %q", harness, code, stdout, stderr)
			}
		}
	})

	t.Run("a dry run of Copilot CLI prints its plan with the shared-home flag and exits 0 [verifies SP-1708~2]", func(t *testing.T) {
		for _, args := range [][]string{
			{"run", "--harness", "copilot", "--dry-run", "--repo", t.TempDir()},
			{"run", "--harness", "copilot", "--dry-run", "--shared-home", "--repo", t.TempDir()},
		} {
			code, stdout, stderr := run(t, "", args...)
			if code != 0 || !strings.Contains(stdout, "copilot -p") || !strings.Contains(stdout, "--model gpt-5.6-luna") {
				t.Fatalf("%v: exit %d, stdout %q, stderr %q", args, code, stdout, stderr)
			}
		}
	})

	t.Run("a recorded Copilot session is evaluated from its directory [verifies SP-1707~2]", func(t *testing.T) {
		records := t.TempDir()
		entries, err := os.ReadDir(recordedCopilotSession)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(recordedCopilotSession, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(records, entry.Name()), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout, stderr := run(t, "", "evaluate", "--harness", "copilot", "--records", records)
		if code != 3 || len(strings.Split(strings.TrimSpace(stdout), "\n")) != copilotChecks || !strings.Contains(stderr, fmt.Sprintf("%d PASS", copilotChecks-1)) {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})

	t.Run("a run of Claude Code with no credential is refused with exit 2 [verifies SP-1504~2]", func(t *testing.T) {
		code, _, stderr := run(t, "", "run", "--harness", "claude", "--force-window", "--repo", t.TempDir())
		if code != cli.ExitRefused || !strings.Contains(stderr, "--shared-home") {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("a synthetic recorded session is evaluated from its directory with one verdict per line [verifies SP-1508~1]", func(t *testing.T) {
		records := t.TempDir()
		entries, err := os.ReadDir(recordedSession)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			data, err := os.ReadFile(filepath.Join(recordedSession, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(records, entry.Name()), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		code, stdout, stderr := run(t, "", "evaluate", "--harness", "claude", "--records", records)
		if code != 0 || len(strings.Split(strings.TrimSpace(stdout), "\n")) != claudeChecks || !strings.Contains(stderr, fmt.Sprintf("%d PASS", claudeChecks)) {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		if _, err := os.Stat(filepath.Join(records, "report.json")); err != nil {
			t.Error("no report.json was written")
		}
	})

	t.Run("a probe records its payload and the shim hands a hook to the real binary [verifies SP-1506~1]", func(t *testing.T) {
		record := filepath.Join(t.TempDir(), "hooks.jsonl")
		if code, _, _ := run(t, `{"hook_event_name":"Stop"}`, "probe", "--record", record, "--label", "Stop-probe"); code != 0 {
			t.Fatalf("probe exit %d", code)
		}
		if code, stdout, _ := run(t, `{}`, "shim", "--real", "/bin/cat", "--record", record, "--", "hook", "pre-tool"); code != 1 && code != 0 {
			t.Fatalf("shim exit %d, stdout %q", code, stdout)
		}
		data, err := os.ReadFile(record)
		if err != nil || strings.Count(string(data), "\n") != 2 {
			t.Fatalf("records = %q, %v", data, err)
		}
	})
}
