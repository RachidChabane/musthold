// Package shallnot is the gateway to Shallnot, the only source of criteria: it
// runs `shallnot gate --advisory --json-out` on the project and reads the report
// Shallnot wrote. It parses no spec itself.
package shallnot

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/RachidChabane/spec-proof/alignment/internal/app/ports"
)

// Shallnot command line.
const (
	// DefaultBinary is the executable run when Gateway.Binary is empty.
	DefaultBinary    = "shallnot"
	gateCommand      = "gate"
	advisoryFlag     = "--advisory"
	jsonOutFlag      = "--json-out"
	reportFilePrefix = "shallnot-report-"
	reportFileSuffix = ".json"
)

// Errors returned by the gateway.
var (
	// ErrGateFailed is returned when Shallnot exits non-zero or is killed.
	ErrGateFailed = errors.New("shallnot gate failed")
	// ErrNoRunner is returned when the gateway has no process runner.
	ErrNoRunner = errors.New("shallnot gateway has no process runner")
)

var _ ports.CriteriaSource = (*Gateway)(nil)

// Gateway implements ports.CriteriaSource with the shallnot executable.
type Gateway struct {
	Process ports.ProcessRunner
	// Binary is the executable to run; empty stands for DefaultBinary.
	Binary string
	// Root is the project root, the directory Shallnot runs in.
	Root string
	// Timeout bounds the run; zero means no deadline beyond the context's.
	Timeout time.Duration
}

// Load runs the gate, reads the report it wrote, removes the report and returns
// its criteria.
func (g Gateway) Load(ctx context.Context) (ports.Report, error) {
	if g.Process == nil {
		return ports.Report{}, ErrNoRunner
	}
	reportPath, err := reserveReportFile()
	if err != nil {
		return ports.Report{}, err
	}
	defer func() { _ = os.Remove(reportPath) }()
	if err := g.runGate(ctx, reportPath); err != nil {
		return ports.Report{}, err
	}
	content, err := os.ReadFile(reportPath)
	if err != nil {
		return ports.Report{}, fmt.Errorf("read shallnot report: %w", err)
	}
	return ParseReport(content)
}

func (g Gateway) runGate(ctx context.Context, reportPath string) error {
	binary := g.Binary
	if binary == "" {
		binary = DefaultBinary
	}
	result, err := g.Process.Run(ctx, ports.Command{
		Argv:    []string{binary, gateCommand, advisoryFlag, jsonOutFlag, reportPath},
		Dir:     g.Root,
		Timeout: g.Timeout,
	})
	switch {
	case err != nil:
		return fmt.Errorf("run %s: %w", binary, err)
	case result.TimedOut:
		return fmt.Errorf("%w: %s timed out after %s", ErrGateFailed, binary, g.Timeout)
	case result.ExitCode != 0:
		return fmt.Errorf("%w: %s exited with status %d: %s", ErrGateFailed, binary, result.ExitCode, result.Stderr)
	}
	return nil
}

// reserveReportFile creates the empty file Shallnot writes its report to.
func reserveReportFile() (string, error) {
	file, err := os.CreateTemp(os.TempDir(), reportFilePrefix+"*"+reportFileSuffix)
	if err != nil {
		return "", fmt.Errorf("create report file: %w", err)
	}
	name := file.Name()
	if err := file.Close(); err != nil {
		_ = os.Remove(name)
		return "", fmt.Errorf("close report file: %w", err)
	}
	return name, nil
}
