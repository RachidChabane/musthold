package canary

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
)

// HostPathRelative is the host path C2 tries to read by default: a file of the
// repository, which the engine does not mount.
const HostPathRelative = "experiments/loop-1/PREREG.md"

// Refusals of the canary verb before its first session.
var (
	ErrNoDetectors = errors.New("no cheating detectors are wired for the canary's C3")
	ErrNoImage     = errors.New("--image is required before the inputs freeze names the image")
)

// Command is the canary verb wired to its adapters.
type Command struct {
	// Detectors are the cheating detectors C3 runs; the detect verb provides them.
	Detectors Detectors
	// Environ is the process's environment; nil reads it.
	Environ []string
}

// CanaryMain is the canary verb with no detectors wired: it prints a dry run,
// and refuses to start a session.
func CanaryMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return Command{}.Main(ctx, args, stdout, stderr)
}

// Main runs the canary verb and returns its exit code: 0 when C1 to C3 hold.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	environ := c.Environ
	if environ == nil {
		environ = os.Environ()
	}
	flags, err := runs.ParseFlags("canary", args, environ, stderr)
	if err != nil {
		return runs.ExitUsage
	}
	setup, err := flags.Resolve(adapters.OSFiles{}, true)
	if err != nil {
		return fail(stderr, err)
	}
	if err := c.refuse(setup, flags.DryRun); err != nil {
		return fail(stderr, err)
	}
	canary := Canary{
		Runner:    runs.Runner{App: runs.AppRunner(environ, stdout), Tree: runs.OSTree{}},
		Detectors: c.Detectors, Stdout: stdout,
	}
	output, err := canary.Run(ctx, Invocation{
		Plan: runs.Plan{Options: setup.Options, Inputs: setup.Inputs}, Config: setup.Config, Layout: setup.Layout,
		HostPath: filepath.Join(setup.Options.RepoRoot, HostPathRelative), DryRun: flags.DryRun,
	})
	switch {
	case err != nil:
		return fail(stderr, err)
	case flags.DryRun || output.Held():
		return runs.ExitOK
	}
	return runs.ExitFailure
}

// refuse refuses, before any session spends quota, a canary with no image or no
// detectors; a dry run needs neither.
func (c Command) refuse(setup runs.Setup, dryRun bool) error {
	switch {
	case dryRun:
		return nil
	case setup.Options.Image == "":
		return ErrNoImage
	case c.Detectors == nil:
		return ErrNoDetectors
	}
	return nil
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "loop-1 canary:", err)
	return runs.ExitFailure
}
