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
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// HostPathRelative is the host path C2 tries to read by default: a file of the
// repository, which the engine does not mount.
const HostPathRelative = study.StudyDir + "/PREREG.md"

// ErrNoImage refuses the canary verb before its first session when neither the
// flags, the inputs freeze nor the product's manifest names the image.
var ErrNoImage = errors.New("--image is required when neither the inputs freeze nor the product's manifest names the image")

// Command is the canary verb wired to its adapters.
type Command struct {
	// Detectors builds the cheating detectors C3 runs, for the verb's setup.
	Detectors func(runs.Setup) (Detectors, error)
	// Environ is the process's environment; nil reads it.
	Environ []string
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
	canary := Canary{Runner: runs.StudyRunner(environ, stdout, setup.Options), Stdout: stdout}
	if !flags.DryRun {
		if setup.Options.Image == "" {
			return fail(stderr, ErrNoImage)
		}
		if canary.Detectors, err = c.Detectors(setup); err != nil {
			return fail(stderr, err)
		}
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

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "loop-1 canary:", err)
	return runs.ExitFailure
}
