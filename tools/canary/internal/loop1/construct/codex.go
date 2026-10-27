package construct

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// The Codex invocation (PREREG §2, §3.3).
const (
	CodexProgram    = "codex"
	CodexModelName  = "gpt-5.6-terra"
	ReasoningEffort = "model_reasoning_effort=high"
	CodexSandbox    = "read-only"

	emptyDirName    = "empty"
	schemaFileName  = "schema.json"
	messageFileName = "last-message.json"
	scratchPattern  = "loop1-codex-*"
	killWaitDelay   = 10 * time.Second
	scratchMode     = 0o700
	fileMode        = 0o600
)

// Errors of a call that produced no usable files.
var (
	ErrCodexVersion = errors.New("codex --version failed")
	ErrCodexExit    = errors.New("codex exec failed")
	ErrBadReply     = errors.New("the final message is not the expected JSON document of files")
)

// Process runs a program to its end with its standard input closed.
type Process interface {
	Run(ctx context.Context, argv []string) (ProcessResult, error)
}

// ProcessResult is how a program ended.
type ProcessResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// ExecProcess runs programs on the host. A program that cannot be started is an
// error; one that ran and failed is a result.
type ExecProcess struct{}

// Run starts the program with the null device as its standard input: codex
// otherwise waits for input on a closed or inherited pipe.
func (ExecProcess) Run(ctx context.Context, argv []string) (ProcessResult, error) {
	command := exec.CommandContext(ctx, argv[0], argv[1:]...)
	command.Stdin = nil
	command.WaitDelay = killWaitDelay
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return ProcessResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: exit.ExitCode()}, nil
	}
	if err != nil {
		return ProcessResult{}, fmt.Errorf("start %s: %w", argv[0], err)
	}
	return ProcessResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, nil
}

// CodexModel is the Model that calls `codex exec`.
type CodexModel struct {
	Process Process
	Now     func() time.Time
	// Timeout bounds one call; zero means CallTimeout.
	Timeout time.Duration
}

func (m CodexModel) timeout() time.Duration {
	if m.Timeout > 0 {
		return m.Timeout
	}
	return CallTimeout
}

// VersionArgs is the command that prints the Codex version.
func VersionArgs() []string { return []string{CodexProgram, "--version"} }

// ExecArgs is the exact command of a call: high effort, a read-only sandbox, no
// user configuration, no session kept, an empty working directory, the output
// schema and the file the final message is written to.
func ExecArgs(emptyDir, schemaFile, messageFile, prompt string) []string {
	return []string{
		CodexProgram, "exec",
		"-m", CodexModelName,
		"-c", ReasoningEffort,
		"--sandbox", CodexSandbox,
		"--ignore-user-config",
		"--ephemeral",
		"--skip-git-repo-check",
		"--cd", emptyDir,
		"--output-schema", schemaFile,
		"-o", messageFile,
		prompt,
	}
}

// Complete records the Codex version, runs one call under its time limit and
// reads the files of its final message. A failure of the harness is an error
// that matches ErrHarnessFailure.
func (m CodexModel) Complete(ctx context.Context, request Request) (Reply, error) {
	callCtx, cancel := context.WithTimeout(ctx, m.timeout())
	defer cancel()
	var reply Reply
	version, err := m.Process.Run(callCtx, VersionArgs())
	if stop := interruption(ctx, callCtx, m.timeout()); stop != nil {
		return reply, stop
	}
	if err != nil || version.ExitCode != 0 {
		unavailable := &HarnessFailure{Kind: HarnessUnavailable, Detail: ErrCodexVersion.Error()}
		return reply, fmt.Errorf("%w: %v (exit %d): %w", ErrCodexVersion, err, version.ExitCode, unavailable)
	}
	reply.CodexVersion = strings.TrimSpace(string(version.Stdout))
	scratch, err := os.MkdirTemp("", scratchPattern)
	if err != nil {
		return reply, fmt.Errorf("create the scratch directory: %w", err)
	}
	defer os.RemoveAll(scratch)
	emptyDir := filepath.Join(scratch, emptyDirName)
	schemaFile := filepath.Join(scratch, schemaFileName)
	messageFile := filepath.Join(scratch, messageFileName)
	if err := os.Mkdir(emptyDir, scratchMode); err != nil {
		return reply, fmt.Errorf("create the empty working directory: %w", err)
	}
	if err := os.WriteFile(schemaFile, request.Schema.JSON, fileMode); err != nil {
		return reply, fmt.Errorf("write the output schema: %w", err)
	}
	reply.Start = m.Now().UTC()
	result, err := m.Process.Run(callCtx, ExecArgs(emptyDir, schemaFile, messageFile, request.Prompt))
	reply.End = m.Now().UTC()
	reply.Stderr, reply.Exit = result.Stderr, result.ExitCode
	reply.Output, _ = os.ReadFile(messageFile)
	if stop := interruption(ctx, callCtx, m.timeout()); stop != nil {
		return reply, stop
	}
	if err != nil {
		return reply, fmt.Errorf("%w: %w", err, &HarnessFailure{Kind: HarnessUnavailable, Detail: err.Error()})
	}
	if failure := ClassifyOutcome(result, reply.Output); failure != nil {
		return reply, failure
	}
	if result.ExitCode != 0 {
		return reply, fmt.Errorf("%w: exit %d", ErrCodexExit, result.ExitCode)
	}
	files, err := ParseFiles(reply.Output)
	reply.Files = files
	return reply, err
}

// ParseFiles reads the final message of a call: one document holding a `files`
// array of path and content, and nothing else.
func ParseFiles(message []byte) ([]File, error) {
	var document struct {
		Files []File `json:"files"`
	}
	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBadReply, err)
	}
	if decoder.More() {
		return nil, fmt.Errorf("%w: more than one document", ErrBadReply)
	}
	return document.Files, nil
}
