// Package modulecli drives a proof module through its binary: `systemone export`
// to list the closed requests and `systemone import` to hand back answers. It is
// the only way the relay reaches a module.
package modulecli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/s1relay/internal/adapters/systemonewire"
	"github.com/RachidChabane/spec-proof/s1relay/internal/app/ports"
	"github.com/RachidChabane/spec-proof/s1relay/internal/domain/failure"
)

// Names of the command line of a module binary.
const (
	// BinaryPrefix starts the name of every module binary.
	BinaryPrefix = "spec-proof-"

	commandSystemOne = "systemone"
	verbExport       = "export"
	verbImport       = "import"
	flagNonce        = "--nonce"
	flagSource       = "--source"

	// CommandTimeout bounds one invocation of a module binary.
	CommandTimeout = 2 * time.Minute
	// processWaitDelay is how long a killed process gets to release its pipes.
	processWaitDelay = 5 * time.Second
	// maxStderrBytes bounds the module's stderr quoted in a report.
	maxStderrBytes = 300
)

// ErrModuleName is returned for a module name that is not a plain identifier.
var ErrModuleName = errors.New("a module name is lower-case letters, digits and hyphens")

var modulePattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Runner runs one command and returns its output; it is the seam to the operating
// system.
type Runner interface {
	Run(ctx context.Context, dir, name string, args []string, stdin []byte) (stdout, stderr []byte, err error)
}

// ExecRunner runs commands as subprocesses.
type ExecRunner struct{}

// Run runs the command with the given working directory and standard input.
func (ExecRunner) Run(ctx context.Context, dir, name string, args []string, stdin []byte) ([]byte, []byte, error) {
	ctx, cancel := context.WithTimeout(ctx, CommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, name, args...)
	command.Dir = dir
	command.WaitDelay = processWaitDelay
	command.Stdin = bytes.NewReader(stdin)
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

var _ ports.ModuleGateway = (*Gateway)(nil)

// Gateway is the ModuleGateway of one module binary, run from the project root.
type Gateway struct {
	binary string
	dir    string
	runner Runner
}

// BinaryName is the executable of a module: `spec-proof-<module>`.
func BinaryName(module string) (string, error) {
	if !modulePattern.MatchString(module) {
		return "", fmt.Errorf("%w: %q", ErrModuleName, module)
	}
	return BinaryPrefix + module, nil
}

// NewGateway builds the gateway of a module run from dir.
func NewGateway(module, dir string, runner Runner) (*Gateway, error) {
	binary, err := BinaryName(module)
	if err != nil {
		return nil, err
	}
	return &Gateway{binary: binary, dir: dir, runner: runner}, nil
}

// Export runs `systemone export` and reads its list of closed requests.
func (g *Gateway) Export(ctx context.Context) ([]ports.Exported, error) {
	stdout, stderr, err := g.runner.Run(ctx, g.dir, g.binary, []string{commandSystemOne, verbExport}, nil)
	if err != nil {
		return nil, fmt.Errorf("%s %s %s: %w%s", g.binary, commandSystemOne, verbExport, err, excerptOf(stderr))
	}
	entries, err := systemonewire.DecodeExport(stdout)
	if err != nil {
		return nil, err
	}
	exported := make([]ports.Exported, len(entries))
	for index, entry := range entries {
		set, problem := systemonewire.DecodeClosed(entry.Closed)
		exported[index] = ports.Exported{Ticket: entry.Ticket, Nonce: entry.Nonce, Set: set, Problem: problem}
	}
	return exported, nil
}

// Import runs `systemone import` with the reply of the endpoint on its standard
// input. A module that refuses it exits non-zero and says why on stderr.
func (g *Gateway) Import(ctx context.Context, ticket, nonce, source string, reply []byte) error {
	args := []string{commandSystemOne, verbImport, ticket, flagNonce, nonce, flagSource, source}
	_, stderr, err := g.runner.Run(ctx, g.dir, g.binary, args, reply)
	if err != nil {
		return failure.New(failure.KindImport, "%s %s %s refused the reply: %v%s", g.binary, commandSystemOne, verbImport, err, excerptOf(stderr))
	}
	return nil
}

func excerptOf(stderr []byte) string {
	text := strings.TrimSpace(string(stderr))
	if text == "" {
		return ""
	}
	if len(text) > maxStderrBytes {
		text = text[:maxStderrBytes] + "..."
	}
	return ": " + text
}
