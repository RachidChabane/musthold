package ghrun_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/ghrun"
	"github.com/RachidChabane/spec-proof/alignment/internal/adapters/process"
)

// stub writes a gh that records its arguments and exits with the status given.
func stub(t *testing.T, status int, stderr string) (binary, argsFile string) {
	t.Helper()
	dir := t.TempDir()
	argsFile = filepath.Join(dir, "args")
	binary = filepath.Join(dir, "gh")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + argsFile + "\necho '" + stderr + "' >&2\nexit " + string(rune('0'+status)) + "\n"
	if err := os.WriteFile(binary, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return binary, argsFile
}

func TestDownload(t *testing.T) {
	t.Run("the run's artifact is downloaded into the directory with gh [verifies SP-749~1]", func(t *testing.T) {
		binary, argsFile := stub(t, 0, "")
		client := ghrun.Client{Process: process.New(), Binary: binary, Dir: t.TempDir()}
		if err := client.Download(context.Background(), "4242", "spec-proof-host-alignment", "/tmp/out", ""); err != nil {
			t.Fatal(err)
		}
		recorded, _ := os.ReadFile(argsFile)
		if got := strings.Fields(string(recorded)); strings.Join(got, " ") != "run download 4242 --name spec-proof-host-alignment --dir /tmp/out" {
			t.Fatalf("arguments = %v", got)
		}
	})

	t.Run("the repository is passed when one is given [verifies SP-749~1]", func(t *testing.T) {
		binary, argsFile := stub(t, 0, "")
		client := ghrun.Client{Process: process.New(), Binary: binary}
		if err := client.Download(context.Background(), "7", "a", "/tmp/out", "owner/name"); err != nil {
			t.Fatal(err)
		}
		recorded, _ := os.ReadFile(argsFile)
		if !strings.HasSuffix(strings.TrimSpace(string(recorded)), "--repo\nowner/name") {
			t.Fatalf("arguments = %q", recorded)
		}
	})

	t.Run("a failing gh, a missing gh and a client without a runner are errors [verifies SP-749~1]", func(t *testing.T) {
		binary, _ := stub(t, 1, "no such run")
		err := ghrun.Client{Process: process.New(), Binary: binary}.Download(context.Background(), "1", "a", "/tmp/out", "")
		if !errors.Is(err, ghrun.ErrDownloadFailed) || !strings.Contains(err.Error(), "no such run") {
			t.Errorf("failed run: %v", err)
		}
		if err := (ghrun.Client{Process: process.New(), Binary: "/nonexistent/gh"}).Download(context.Background(), "1", "a", "/tmp/out", ""); err == nil {
			t.Error("a missing gh was accepted")
		}
		if err := (ghrun.Client{}).Download(context.Background(), "1", "a", "/tmp/out", ""); !errors.Is(err, ghrun.ErrNoRunner) {
			t.Errorf("no runner: %v", err)
		}
	})
}
