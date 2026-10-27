// Package process runs external commands in their own process group, under a
// deadline, with captured output. Timeouts are enforced here and never by a
// runner plugin.
package process

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
)

const (
	// DefaultOutputLimit caps the bytes kept per output stream.
	DefaultOutputLimit = 4 << 20
	// pipeGrace is how long Run waits for the output pipes after the process
	// group was killed.
	pipeGrace = 2 * time.Second
	// killedExitCode is reported for a process that did not exit by itself.
	killedExitCode = -1
)

// ErrEmptyCommand is returned for a command without argv.
var ErrEmptyCommand = errors.New("empty command")

var _ ports.ProcessRunner = (*Runner)(nil)

// Runner implements ports.ProcessRunner with os/exec.
type Runner struct {
	outputLimit int
}

// New returns a Runner keeping DefaultOutputLimit bytes per stream.
func New() *Runner { return &Runner{outputLimit: DefaultOutputLimit} }

// Run starts the command in a new process group and waits for it. When the
// command's timeout elapses, or the context ends, the whole group is killed;
// a timeout is reported through CommandResult.TimedOut, and the signal that
// ended a process nothing here killed through CommandResult.Signal. Where the
// system lets a process volunteer, the command is the out-of-memory killer's
// first choice, ahead of the process that runs it, so that a memory kill
// leaves a result to report.
func (r *Runner) Run(ctx context.Context, command ports.Command) (ports.CommandResult, error) {
	if len(command.Argv) == 0 {
		return ports.CommandResult{}, ErrEmptyCommand
	}
	runCtx, cancel := withDeadline(ctx, command.Timeout)
	defer cancel()

	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = r.outputLimit, r.outputLimit
	cmd := exec.CommandContext(runCtx, command.Argv[0], command.Argv[1:]...)
	cmd.Dir = command.Dir
	cmd.Env = append(os.Environ(), command.Env...)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if command.StdoutFile != "" {
		file, err := os.Create(command.StdoutFile)
		if err != nil {
			return ports.CommandResult{}, fmt.Errorf("create the output file of %q: %w", command.Argv[0], err)
		}
		defer func() { _ = file.Close() }()
		cmd.Stdout = file
	}
	cmd.WaitDelay = pipeGrace
	isolateGroup(cmd)

	waitErr := cmd.Start()
	if waitErr == nil {
		preferAsMemoryVictim(cmd.Process.Pid)
		waitErr = cmd.Wait()
	}
	result := ports.CommandResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: killedExitCode}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		result.ExitCode = cmd.ProcessState.ExitCode()
	}
	if ctx.Err() != nil {
		return result, fmt.Errorf("run %q: %w", command.Argv[0], ctx.Err())
	}
	if errors.Is(runCtx.Err(), context.DeadlineExceeded) {
		result.TimedOut = true
		return result, nil
	}
	result.Signal = signalOf(cmd.ProcessState)
	var exitErr *exec.ExitError
	if waitErr != nil && !errors.As(waitErr, &exitErr) {
		return result, fmt.Errorf("run %q: %w", command.Argv[0], waitErr)
	}
	return result, nil
}

// signalOf is the signal that ended the process, nil when it exited by itself
// or never started.
func signalOf(state *os.ProcessState) os.Signal {
	if state == nil {
		return nil
	}
	if status, ok := state.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return status.Signal()
	}
	return nil
}

func withDeadline(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, timeout)
}

// limitedBuffer keeps the first limit bytes written and drops the rest.
type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.buffer.Len(); room > 0 {
		b.buffer.Write(p[:min(room, len(p))])
	}
	return len(p), nil
}

func (b *limitedBuffer) Bytes() []byte { return b.buffer.Bytes() }
