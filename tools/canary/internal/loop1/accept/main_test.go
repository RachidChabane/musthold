package accept_test

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func runMain(args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := accept.Main(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func wireSimulation(t *testing.T, h *harness) {
	t.Helper()
	accept.Wire = func(string, study.Layout) (accept.Wiring, error) {
		return accept.Wiring{Runner: h.box, Equipper: h.equipper}, nil
	}
	t.Cleanup(func() { accept.Wire = nil })
}

func TestAcceptMain(t *testing.T) {
	flags := func(h *harness) []string {
		return []string{"--repo", h.repo, "--work", h.work}
	}

	t.Run("accept prints the report of each task and exits 0 when all pass [verifies SP-2559~1]", func(t *testing.T) {
		h := newHarness(t)
		wireSimulation(t, h)
		code, stdout, stderr := runMain(append(flags(h), sampleName)...)
		if code != accept.ExitOK {
			t.Fatalf("exit %d:\n%s\n%s", code, stdout, stderr)
		}
		for _, wanted := range []string{"fare: PASS", "A1 shape", "A9 collection"} {
			if !strings.Contains(stdout, wanted) {
				t.Errorf("the report lacks %q:\n%s", wanted, stdout)
			}
		}
	})

	t.Run("accept exits 1 and prints the findings when a check fails [verifies SP-2559~1]", func(t *testing.T) {
		h := newHarness(t)
		wireSimulation(t, h)
		h.edit("violations/FARE-1/1/zones.py", "# BREAKS: FARE-1", "# harmless")
		code, stdout, _ := runMain(append(flags(h), sampleName)...)
		if code != accept.ExitFailure || !strings.Contains(stdout, "A5 diagonal   FAILED") || !strings.Contains(stdout, "violation-FARE-1-1") {
			t.Fatalf("exit %d:\n%s", code, stdout)
		}
	})

	t.Run("with no task named, every task directory is checked in name order [verifies SP-2559~1]", func(t *testing.T) {
		h := newHarness(t)
		wireSimulation(t, h)
		acceptest.NewSample("route").Write(t, h.repo)
		code, stdout, _ := runMain(flags(h)...)
		if code != accept.ExitOK || strings.Index(stdout, "fare: PASS") > strings.Index(stdout, "route: PASS") || !strings.Contains(stdout, "route: PASS") {
			t.Fatalf("exit %d:\n%s", code, stdout)
		}
	})

	t.Run("a build with no container runner wired refuses to run, and a bad flag is a usage error [verifies SP-2559~1]", func(t *testing.T) {
		accept.Wire = nil
		if code, _, stderr := runMain("--repo", t.TempDir()); code != accept.ExitFailure || !strings.Contains(stderr, "no container runner") {
			t.Errorf("exit %d: %s", code, stderr)
		}
		if code, _, _ := runMain("--nonsense"); code != accept.ExitUsage {
			t.Errorf("exit %d", code)
		}
	})
}
