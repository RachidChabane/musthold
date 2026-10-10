package cli_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const (
	nightJobs        = "../../../../script/night-jobs"
	registryArgument = "--known-inconclusive tools/canary/known-inconclusive.json"
	claudeKnownLog   = "canary: claude known inconclusive: classify_witness_recorded (2.1.295)"
)

func TestNightJobsPassTheRegistry(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("the night jobs use BSD date")
	}

	t.Run("a dry run of the canaries passes the registry to each of them and names the known inconclusive checks [verifies SP-1528~1]", func(t *testing.T) {
		useSecurity(t, securityNotFound)
		logs := t.TempDir()
		command := exec.Command(nightJobs, "canaries")
		command.Env = append(os.Environ(), "NIGHT_JOBS_DRY_RUN=1", "NIGHT_JOBS_LOG_DIR="+logs, "NIGHT_JOBS_SLOTS=0")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("%v\n%s", err, output)
		}
		for _, harness := range []string{"claude", "codex", "copilot"} {
			if !strings.Contains(string(output), "script/canary-"+harness) {
				t.Fatalf("no launch of the %s canary:\n%s", harness, output)
			}
		}
		if got := strings.Count(string(output), registryArgument); got != 3 {
			t.Errorf("the registry was passed %d times, want 3:\n%s", got, output)
		}
		if !strings.Contains(string(output), claudeKnownLog) {
			t.Errorf("no line %q:\n%s", claudeKnownLog, output)
		}
		if _, err := os.Stat(filepath.Join(logs, "canary-nights")); err != nil {
			t.Error(err)
		}
	})
}
