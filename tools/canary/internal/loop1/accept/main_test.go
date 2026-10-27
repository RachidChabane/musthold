package accept_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
)

func runMain(command accept.Command, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := command.Main(context.Background(), args, &stdout, &stderr)
	return code, stdout.String(), stderr.String()
}

func simulation(h *harness) accept.Command {
	return accept.Command{Wire: func(accept.Flags) (accept.Wiring, error) {
		return accept.Wiring{Runner: h.box, Equipper: h.equipper}, nil
	}}
}

var errWiring = errors.New("no engine")

func TestAcceptMain(t *testing.T) {
	flags := func(h *harness) []string {
		return []string{"--repo", h.repo, "--work", h.work}
	}

	t.Run("accept prints the report of each task and exits 0 when all pass [verifies SP-2559~1]", func(t *testing.T) {
		h := newHarness(t)
		code, stdout, stderr := runMain(simulation(h), append(flags(h), sampleName)...)
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
		h.edit("violations/FARE-1/1/zones.py", "# BREAKS: FARE-1", "# harmless")
		code, stdout, _ := runMain(simulation(h), append(flags(h), sampleName)...)
		if code != accept.ExitFailure || !strings.Contains(stdout, "A5 diagonal   FAILED") || !strings.Contains(stdout, "violation-FARE-1-1") {
			t.Fatalf("exit %d:\n%s", code, stdout)
		}
	})

	t.Run("with no task named, every task directory is checked in name order [verifies SP-2559~1]", func(t *testing.T) {
		h := newHarness(t)
		acceptest.NewSample("route").Write(t, h.repo)
		code, stdout, _ := runMain(simulation(h), flags(h)...)
		if code != accept.ExitOK || strings.Index(stdout, "fare: PASS") > strings.Index(stdout, "route: PASS") || !strings.Contains(stdout, "route: PASS") {
			t.Fatalf("exit %d:\n%s", code, stdout)
		}
	})

	t.Run("a wiring that fails refuses to run, and a bad flag is a usage error [verifies SP-2559~1]", func(t *testing.T) {
		failing := accept.Command{Wire: func(accept.Flags) (accept.Wiring, error) { return accept.Wiring{}, errWiring }}
		if code, _, stderr := runMain(failing, "--repo", t.TempDir()); code != accept.ExitFailure || !strings.Contains(stderr, errWiring.Error()) {
			t.Errorf("exit %d: %s", code, stderr)
		}
		if code, _, _ := runMain(failing, "--nonsense"); code != accept.ExitUsage {
			t.Errorf("exit %d", code)
		}
	})
}
