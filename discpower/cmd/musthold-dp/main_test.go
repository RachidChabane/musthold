package main

import (
	"bytes"
	"runtime/debug"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/cli"
)

const verifiesVersionFlag = " [verifies SP-1851~1]"

func runWith(t *testing.T, linkedVersion, linkedCommit string, info *debug.BuildInfo, args ...string) (int, string) {
	t.Helper()
	previousVersion, previousCommit := version, commit
	version, commit = linkedVersion, linkedCommit
	t.Cleanup(func() { version, commit = previousVersion, previousCommit })
	var stdout, stderr bytes.Buffer
	code := run(args, &stdout, &stderr, info)
	return code, stdout.String()
}

func TestVersionFlag(t *testing.T) {
	t.Run("--version prints exactly the linked version"+verifiesVersionFlag, func(t *testing.T) {
		code, out := runWith(t, "1.2.3", "abc123", nil, "--version")
		if code != cli.ExitClean || out != "1.2.3\n" {
			t.Fatalf("code=%d out=%q", code, out)
		}
	})
	t.Run("--version prints the version go install recorded when none was linked"+verifiesVersionFlag, func(t *testing.T) {
		info := &debug.BuildInfo{Main: debug.Module{Version: "v0.2.0"}}
		if code, out := runWith(t, "", "", info, "--version"); code != cli.ExitClean || out != "0.2.0\n" {
			t.Fatalf("code=%d out=%q", code, out)
		}
	})
	t.Run("--version prints dev for a build that is neither linked nor installed"+verifiesVersionFlag, func(t *testing.T) {
		if code, out := runWith(t, "", "", nil, "--version"); code != cli.ExitClean || out != "dev\n" {
			t.Fatalf("code=%d out=%q", code, out)
		}
	})
	t.Run("the version command reports the same version, and the commit when one was linked"+verifiesVersionFlag, func(t *testing.T) {
		if _, out := runWith(t, "1.2.3", "abc123", nil, "version"); out != "musthold-dp 1.2.3 (abc123)\n" {
			t.Fatalf("out=%q", out)
		}
		if _, out := runWith(t, "1.2.3", "", nil, "version"); out != "musthold-dp 1.2.3\n" {
			t.Fatalf("out=%q", out)
		}
	})
}
