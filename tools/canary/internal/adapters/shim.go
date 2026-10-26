// Package adapters holds the canary's edges: the recording shim and probes the
// harness runs, the process runner, the single-session lock, the stdio client of
// the executor server and the recorded-session files.
package adapters

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
)

const (
	recordMode       os.FileMode = 0o644
	hookSubcommand   string      = "hook"
	exitShimFailure              = 127
	maxCapturedBytes             = 8 << 20
)

// ErrNoRealBinary is returned when a shim is started without the binary it stands for.
var ErrNoRealBinary = errors.New("the shim needs --real")

// AppendRecord appends one record to the hook log with a single write, so that the
// hooks of concurrent processes do not interleave their lines.
func AppendRecord(path string, call evidence.HookCall) error {
	line, err := json.Marshal(call)
	if err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, recordMode)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(line, '\n'))
	return errors.Join(writeErr, file.Close())
}

// asJSON keeps a payload that is JSON as it is and a payload that is not as a string.
func asJSON(payload []byte) json.RawMessage {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 {
		return nil
	}
	if json.Valid(trimmed) {
		return json.RawMessage(trimmed)
	}
	quoted, _ := json.Marshal(string(trimmed))
	return quoted
}

// RunProbe records the payload a harness hook received under a label and answers
// nothing. It always exits 0: a probe must never change what the harness does.
func RunProbe(record, label string, stdin io.Reader) int {
	payload, _ := io.ReadAll(io.LimitReader(stdin, maxCapturedBytes))
	_ = AppendRecord(record, evidence.HookCall{Label: label, Stdin: asJSON(payload)})
	return 0
}

// RunShim stands in for a module binary. A `hook` command is run for real with the
// payload on its standard input, and the payload, the decision and the exit code
// are recorded; any other command, the executor server above all, is replaced by
// the real binary so that the harness talks to it directly.
func RunShim(real, record string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if real == "" {
		fmt.Fprintln(stderr, ErrNoRealBinary)
		return exitShimFailure
	}
	call := evidence.HookCall{Module: filepath.Base(real), Args: args}
	if len(args) == 0 || args[0] != hookSubcommand {
		_ = AppendRecord(record, call)
		err := syscall.Exec(real, append([]string{real}, args...), launch.WithoutOAuthToken(os.Environ()))
		fmt.Fprintln(stderr, err)
		return exitShimFailure
	}
	payload, _ := io.ReadAll(io.LimitReader(stdin, maxCapturedBytes))
	command := exec.Command(real, args...)
	command.Env = launch.WithoutOAuthToken(os.Environ())
	command.Stdin = bytes.NewReader(payload)
	var captured, capturedErr bytes.Buffer
	command.Stdout = &captured
	command.Stderr = io.MultiWriter(stderr, &capturedErr)
	started := time.Now()
	err := command.Run()
	call.StartedAt, call.DurationMillis = started.UTC().Format(time.RFC3339Nano), time.Since(started).Milliseconds()
	call.Stdin = asJSON(payload)
	call.Stdout = captured.String()
	call.Stderr = capturedErr.String()
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		call.ExitCode = exit.ExitCode()
	case err != nil:
		call.ExitCode = exitShimFailure
		fmt.Fprintln(stderr, err)
	}
	_ = AppendRecord(record, call)
	_, _ = stdout.Write(captured.Bytes())
	return call.ExitCode
}
