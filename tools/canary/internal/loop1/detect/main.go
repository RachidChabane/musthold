package detect

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of the verb.
const (
	exitOK      = 0
	exitRefused = 1
	exitUsage   = 2
)

// Defaults and names of the verb's flags.
const (
	verbName          = "detect"
	defaultRepo       = "."
	defaultWorkEnv    = "LOOP1_WORK"
	defaultWorkInHome = ".musthold-loop-1"
	inputsRelative    = "experiments/loop-1/freeze/inputs.json"
)

// Errors of the verb.
var (
	ErrRunPhaseOpen = errors.New("the run phase has not ended: detect refuses to look at any run")
	ErrNotWired     = errors.New("detect has no container runner or run-phase check")
)

// PhaseEnded says whether the run phase has ended: every run of the order is
// counted or blocked, or the calendar cap is reached (PREREG §6.6).
type PhaseEnded interface {
	RunPhaseEnded(ctx context.Context, work string) (bool, error)
}

// Dependencies are the adapters the verb needs.
type Dependencies struct {
	Runner box.Runner
	Phase  PhaseEnded
}

// Wire builds the verb's dependencies for a work directory. The composition root
// sets it.
var Wire func(work string) (Dependencies, error)

// Main is the detect verb: it runs the detectors over every counted attempt and
// writes detectors.json into each attempt's records.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return run(ctx, Wire, args, stdout, stderr)
}

func run(ctx context.Context, wire func(string) (Dependencies, error), args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet(verbName, flag.ContinueOnError)
	set.SetOutput(stderr)
	repo := set.String("repo", defaultRepo, "repository root")
	work := set.String("work", defaultWork(), "work directory")
	inputsPath := set.String("inputs", "", "frozen inputs.json (default: under --repo)")
	if err := set.Parse(args); err != nil {
		return exitUsage
	}
	if *inputsPath == "" {
		*inputsPath = filepath.Join(*repo, inputsRelative)
	}
	if err := detectAll(ctx, wire, *work, *inputsPath, stdout); err != nil {
		fmt.Fprintf(stderr, "%s: %v\n", verbName, err)
		return exitRefused
	}
	return exitOK
}

func defaultWork() string {
	if work := os.Getenv(defaultWorkEnv); work != "" {
		return work
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, defaultWorkInHome)
}

func detectAll(ctx context.Context, wire func(string) (Dependencies, error), work, inputsPath string, stdout io.Writer) error {
	if wire == nil {
		return ErrNotWired
	}
	deps, err := wire(work)
	if err != nil {
		return err
	}
	if err := requirePhaseEnded(ctx, deps.Phase, work); err != nil {
		return err
	}
	inputs, err := readInputs(inputsPath)
	if err != nil {
		return err
	}
	attempts, err := countedAttempts(work)
	if err != nil {
		return err
	}
	for _, attempt := range attempts {
		detection, err := Detect(ctx, deps.Runner, attempt, inputs)
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(attempt), err)
		}
		if err := Write(attempt, detection); err != nil {
			return err
		}
		fmt.Fprintf(stdout, "%s: %d hits, %d edits\n", filepath.Base(attempt), len(detection.Hits), len(detection.Edits))
	}
	return nil
}

func requirePhaseEnded(ctx context.Context, phase PhaseEnded, work string) error {
	if phase == nil {
		return ErrNotWired
	}
	ended, err := phase.RunPhaseEnded(ctx, work)
	if err != nil {
		return err
	}
	if !ended {
		return ErrRunPhaseOpen
	}
	return nil
}

func readInputs(path string) (Inputs, error) {
	var inputs Inputs
	data, err := os.ReadFile(path)
	if err != nil {
		return inputs, fmt.Errorf("read inputs: %w", err)
	}
	if err := json.Unmarshal(data, &inputs); err != nil {
		return inputs, fmt.Errorf("decode inputs: %w", err)
	}
	return inputs, nil
}

// countedAttempts lists the attempt directories of the work directory whose
// outcome enters the analysis, in name order.
func countedAttempts(work string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(work, study.RunsDir))
	if err != nil {
		return nil, fmt.Errorf("list runs: %w", err)
	}
	var counted []string
	for _, entry := range entries {
		dir := filepath.Join(work, study.RunsDir, entry.Name())
		data, err := os.ReadFile(filepath.Join(dir, RecordsDir, RunFile))
		if err != nil {
			if entry.IsDir() && errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read %s: %w", dir, err)
		}
		var run runRecord
		if err := json.Unmarshal(data, &run); err != nil {
			return nil, fmt.Errorf("decode run record of %s: %w", dir, err)
		}
		if run.Outcome.Counted() {
			counted = append(counted, dir)
		}
	}
	sort.Strings(counted)
	return counted, nil
}
