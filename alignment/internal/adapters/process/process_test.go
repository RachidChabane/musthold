//go:build !windows

package process_test

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/process"
	"github.com/RachidChabane/musthold/alignment/internal/app/ports"
)

const (
	shell          = "sh"
	shellFlag      = "-c"
	shortDeadline  = 400 * time.Millisecond
	returnBudget   = 5 * time.Second
	reapBudget     = 3 * time.Second
	reapPoll       = 25 * time.Millisecond
	exitCodeSeven  = 7
	oversizeOutput = "yes x | head -c 6000000"
)

func sh(script string) ports.Command {
	return ports.Command{Argv: []string{shell, shellFlag, script}}
}

func TestRunCapturesOutputAndExitStatus(t *testing.T) {
	runner := process.New()

	t.Run("stdout, stderr and a zero exit status are captured [verifies SP-352~1]", func(t *testing.T) {
		result, err := runner.Run(context.Background(), sh("echo out; echo err >&2"))
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(string(result.Stdout)) != "out" || strings.TrimSpace(string(result.Stderr)) != "err" || result.ExitCode != 0 || result.TimedOut {
			t.Fatalf("result = %+v", result)
		}
	})

	t.Run("a non-zero exit status is a result, not an error [verifies SP-352~1]", func(t *testing.T) {
		result, err := runner.Run(context.Background(), sh("exit "+strconv.Itoa(exitCodeSeven)))
		if err != nil || result.ExitCode != exitCodeSeven {
			t.Fatalf("result = %+v, err = %v", result, err)
		}
	})

	t.Run("the directory and the added environment apply [verifies SP-352~1]", func(t *testing.T) {
		dir := t.TempDir()
		command := sh(`printf '%s' "$MUSTHOLD_PROBE:$(pwd -P)"`)
		command.Dir, command.Env = dir, []string{"MUSTHOLD_PROBE=on"}
		result, err := runner.Run(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		if got := string(result.Stdout); !strings.HasPrefix(got, "on:") || !strings.HasSuffix(got, dir[strings.LastIndex(dir, "/"):]) {
			t.Fatalf("stdout = %q", got)
		}
	})

	t.Run("a command that cannot start is an error [verifies SP-352~1]", func(t *testing.T) {
		if _, err := runner.Run(context.Background(), ports.Command{Argv: []string{"musthold-no-such-binary"}}); err == nil {
			t.Fatal("expected a start error")
		}
		if _, err := runner.Run(context.Background(), ports.Command{}); !errors.Is(err, process.ErrEmptyCommand) {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("output beyond the limit is dropped [verifies SP-352~1]", func(t *testing.T) {
		result, err := runner.Run(context.Background(), sh(oversizeOutput))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Stdout) != process.DefaultOutputLimit {
			t.Fatalf("kept %d bytes", len(result.Stdout))
		}
	})
}

func alive(pid int) bool { return syscall.Kill(pid, 0) == nil }

func TestRunKillsTheProcessGroupAtTheDeadline(t *testing.T) {
	runner := process.New()

	t.Run("a grandchild holding the pipe dies with the group and the call returns at the deadline [verifies SP-353~1]", func(t *testing.T) {
		command := sh("sleep 30 & echo $!; wait")
		command.Timeout = shortDeadline
		started := time.Now()
		result, err := runner.Run(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		if !result.TimedOut {
			t.Fatalf("TimedOut is false: %+v", result)
		}
		if elapsed := time.Since(started); elapsed > returnBudget {
			t.Fatalf("returned after %s", elapsed)
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(result.Stdout)))
		if err != nil {
			t.Fatalf("grandchild pid from %q: %v", result.Stdout, err)
		}
		deadline := time.Now().Add(reapBudget)
		for alive(pid) && time.Now().Before(deadline) {
			time.Sleep(reapPoll)
		}
		if alive(pid) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			t.Fatalf("grandchild %d survived the deadline", pid)
		}
	})

	t.Run("a cancelled context stops the command with an error [verifies SP-353~1]", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(shortDeadline, cancel)
		_, err := runner.Run(ctx, sh("sleep 30"))
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("err = %v", err)
		}
	})
}
