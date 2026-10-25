package adapters

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

const (
	// niceLevel is the scheduling priority of the canary and everything it starts.
	niceLevel = 10
	// terminateGrace is how long a process group gets between SIGTERM and SIGKILL.
	terminateGrace = 5 * time.Second
	// maxStreamLine bounds one line of the harness's event stream.
	maxStreamLine = 16 << 20
)

// Lower puts the process, and so every process it starts, at a low priority:
// niceness niceLevel, or the current niceness when it is already that low or
// lower. Setting a niceness below the current one would raise the priority,
// which the operating system refuses to an unprivileged process.
func Lower() error {
	current, err := currentNiceness()
	if err != nil {
		return err
	}
	if current >= niceLevel {
		return nil
	}
	return syscall.Setpriority(syscall.PRIO_PROCESS, 0, niceLevel)
}

// Exec runs commands as operating-system processes.
type Exec struct{}

// Sessions runs harness CLIs as operating-system processes.
type Sessions struct{}

// Run runs a command to completion in its own process group. A non-zero exit is
// not an error; failing to start or running out of time is.
func (Exec) Run(ctx context.Context, spec app.CommandSpec) (app.Output, error) {
	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}
	command := exec.CommandContext(ctx, spec.Argv[0], spec.Argv[1:]...)
	command.Dir, command.Env = spec.Dir, spec.Env
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Cancel = func() error { return syscall.Kill(-command.Process.Pid, syscall.SIGKILL) }
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	output := app.Output{Stdout: stdout.String(), Stderr: stderr.String()}
	var exit *exec.ExitError
	switch {
	case ctx.Err() != nil:
		return output, fmt.Errorf("%s: %w", spec.Argv[0], ctx.Err())
	case errors.As(err, &exit):
		output.ExitCode = exit.ExitCode()
		return output, nil
	default:
		return output, err
	}
}

// RunSession runs the harness CLI in its own process group, records its output,
// and ends the whole group when the event cap or the timeout is reached, or at
// the first line the spec's Stops gives a reason for. The group
// is also ended when the CLI exits, so that the servers it started do not outlive it.
func (Sessions) Run(spec app.SessionSpec) (app.SessionOutcome, error) {
	stdoutFile, err := os.Create(spec.StdoutPath)
	if err != nil {
		return app.SessionOutcome{}, err
	}
	defer stdoutFile.Close()
	stderrFile, err := os.Create(spec.StderrPath)
	if err != nil {
		return app.SessionOutcome{}, err
	}
	defer stderrFile.Close()
	arrivals, err := openArrivals(spec.TimesPath)
	if err != nil {
		return app.SessionOutcome{}, err
	}
	defer arrivals.Close()

	command := exec.Command(spec.Argv[0], spec.Argv[1:]...)
	command.Dir, command.Env = spec.Dir, spec.Env
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	command.Stderr = stderrFile
	if spec.Stdin != nil {
		command.Stdin = bytes.NewReader(spec.Stdin)
	}
	pipe, err := command.StdoutPipe()
	if err != nil {
		return app.SessionOutcome{}, err
	}
	if err := command.Start(); err != nil {
		return app.SessionOutcome{}, err
	}
	group := command.Process.Pid
	started := time.Now()
	var reason string
	var mutex sync.Mutex
	stop := func(why string) {
		mutex.Lock()
		defer mutex.Unlock()
		if reason == "" {
			reason = why
			terminateGroup(group)
		}
	}
	timer := time.AfterFunc(spec.Timeout, func() { stop(evidence.KilledTimeout) })
	defer timer.Stop()
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM)
	finished := make(chan struct{})
	defer close(finished)
	defer signal.Stop(interrupts)
	go func() {
		select {
		case <-interrupts:
			stop(evidence.KilledInterrupted)
		case <-finished:
		}
	}()

	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(nil, maxStreamLine)
	steps := 0
	for number := 1; scanner.Scan(); number++ {
		line := scanner.Bytes()
		_, _ = stdoutFile.Write(append(append([]byte(nil), line...), '\n'))
		arrivals.record(number, time.Since(started))
		if spec.Stops != nil {
			if why := spec.Stops(line); why != "" {
				stop(why)
			}
		}
		if spec.Counts(line) {
			steps++
			if steps > spec.Cap {
				stop(evidence.KilledTurnCap)
			}
		}
	}
	waitErr := command.Wait()
	terminateGroup(group)
	mutex.Lock()
	defer mutex.Unlock()
	outcome := app.SessionOutcome{Killed: reason}
	var exit *exec.ExitError
	switch {
	case errors.As(waitErr, &exit):
		outcome.ExitCode = exit.ExitCode()
	case waitErr != nil:
		return outcome, waitErr
	}
	return outcome, nil
}

// arrivalLog writes the time each stream line arrived; a nil log writes nothing.
type arrivalLog struct{ file *os.File }

func openArrivals(path string) (*arrivalLog, error) {
	if path == "" {
		return &arrivalLog{}, nil
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, err
	}
	return &arrivalLog{file: file}, nil
}

func (a *arrivalLog) record(line int, since time.Duration) {
	if a.file != nil {
		fmt.Fprintf(a.file, "{\"line\":%d,\"ms\":%d}\n", line, since.Milliseconds())
	}
}

// Close closes the log.
func (a *arrivalLog) Close() error {
	if a.file == nil {
		return nil
	}
	return a.file.Close()
}

// terminateGroup ends a process group: SIGTERM, then SIGKILL after a grace period.
func terminateGroup(group int) {
	if syscall.Kill(-group, syscall.SIGTERM) != nil {
		return
	}
	deadline := time.Now().Add(terminateGrace)
	for time.Now().Before(deadline) {
		if syscall.Kill(-group, 0) != nil {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	_ = syscall.Kill(-group, syscall.SIGKILL)
}
