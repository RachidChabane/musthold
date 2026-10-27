package detect

import (
	"context"
	"flag"
	"fmt"
	"io"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of the verb.
const (
	exitOK      = 0
	exitRefused = 1
	exitUsage   = 2
)

const verbName = "detect"

// Phase hands the verb the counted attempt of every run once the run phase has
// ended, and refuses before (PREREG §6.6): the rule of analyze/phase.
type Phase interface {
	Counted() ([]phase.Attempt, error)
}

// Flags are the paths the verb resolved from its command line.
type Flags struct {
	Repo, Config, Work string
}

// Dependencies are what the verb takes from the rest of the tooling: the box
// that reads the syntax trees, the run-phase rule, and every host the canary saw
// the CLI contact, from the inputs freeze.
type Dependencies struct {
	Runner         box.Runner
	Phase          Phase
	EgressObserved []string
}

// Command is the detect verb; Wire builds its dependencies from the resolved
// options.
type Command struct {
	Wire func(Flags) (Dependencies, error)
}

// Main is the detect verb: it runs the detectors over every counted attempt and
// writes detectors.json into each attempt's records.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet(verbName, flag.ContinueOnError)
	set.SetOutput(stderr)
	var options Flags
	set.StringVar(&options.Repo, "repo", ".", "repository root")
	set.StringVar(&options.Config, "config", study.ConfigFile, "the study's configuration, beside freeze/")
	set.StringVar(&options.Work, "work", study.DefaultWork(), "work directory")
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return exitUsage
	}
	if err := c.detectAll(ctx, options, stdout); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verbName, err)
		return exitRefused
	}
	return exitOK
}

func (c Command) detectAll(ctx context.Context, options Flags, stdout io.Writer) error {
	deps, err := c.Wire(options)
	if err != nil {
		return err
	}
	attempts, err := deps.Phase.Counted()
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		detection, err := Detect(ctx, deps.Runner, attempt.Dir, deps.EgressObserved)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(attempt.Dir), err)
		}
		if err := Write(attempt.Dir, detection); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %d hits, %d edits\n", filepath.Base(attempt.Dir), len(detection.Hits), len(detection.Edits))
	}
	return nil
}
