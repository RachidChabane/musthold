//go:build !windows

package process_test

import (
	"context"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
)

// firstVictimScore is the out-of-memory score a process volunteers with to be
// the first one killed.
const firstVictimScore = "1000"

func TestRunReportsAKillFromOutside(t *testing.T) {
	runner := process.New()

	t.Run("a process SIGKILL ends from outside reports the signal and no timeout [verifies SP-2654~1]", func(t *testing.T) {
		result, err := runner.Run(context.Background(), sh("kill -KILL $$"))
		if err != nil || result.Signal != syscall.SIGKILL || result.TimedOut || !result.KilledFromOutside() {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	})

	t.Run("a process the deadline kills is a timeout, not a kill from outside [verifies SP-2654~1]", func(t *testing.T) {
		command := sh("sleep 5")
		command.Timeout = shortDeadline
		result, err := runner.Run(context.Background(), command)
		if err != nil || !result.TimedOut || result.Signal != nil || result.KilledFromOutside() {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	})

	t.Run("a process that exits by itself reports no signal [verifies SP-2654~1]", func(t *testing.T) {
		result, err := runner.Run(context.Background(), sh("exit 3"))
		if err != nil || result.Signal != nil || result.KilledFromOutside() {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	})

	t.Run("on Linux the processes a command starts are the out-of-memory killer's first choice [verifies SP-2654~1]", func(t *testing.T) {
		if runtime.GOOS != "linux" {
			t.Skip("only Linux lets a process volunteer to the out-of-memory killer")
		}
		result, err := runner.Run(context.Background(), sh("sleep 0.3; sh -c 'cat /proc/$$/oom_score_adj'"))
		if err != nil || strings.TrimSpace(string(result.Stdout)) != firstVictimScore {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	})
}
