package calibrate

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// Reacher runs the reach computation of the confirmatory design at a coder's
// pooled control escapes over its pooled control runs.
type Reacher interface {
	Reach(ctx context.Context, escapes, runs int) (string, error)
}

// PowerScript is experiments/loop-2/script/power, run as `power reach ESCAPES
// RUNS`.
type PowerScript struct{ Path string }

// reachCommand is the subcommand of the power script.
const reachCommand = "reach"

// Reach runs the script and returns what it prints.
func (p PowerScript) Reach(ctx context.Context, escapes, runs int) (string, error) {
	var stdout, stderr bytes.Buffer
	command := exec.CommandContext(ctx, p.Path, reachCommand, strconv.Itoa(escapes), strconv.Itoa(runs))
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("%s %s: %w: %s", p.Path, reachCommand, err, stderr.String())
	}
	return stdout.String(), nil
}

// The files the verb writes, and its defaults.
const (
	SelectionFile    = "selection.json"
	ReachFile        = "reach.txt"
	DefaultPower     = "experiments/loop-2/script/power"
	jsonIndent       = "  "
	exitOK           = 0
	exitRefused      = 1
	exitUsage        = 2
	fileMode         = 0o644
	dirMode          = 0o755
	calibrationUsage = "calibration.json: each coder's runs, positive control and P3 replay per calibration phase"
)

// Command is the `calibrate` verb.
type Command struct{ Reacher func(path string) Reacher }

// Calibrate reads calibration.json, selects the coder and writes selection.json in
// out, with reach.txt when a coder is selected.
func Calibrate(ctx context.Context, calibration, out string, reacher Reacher) (Selection, error) {
	data, err := os.ReadFile(calibration)
	if err != nil {
		return Selection{}, err
	}
	var input Calibration
	if err := json.Unmarshal(data, &input); err != nil {
		return Selection{}, fmt.Errorf("%s: %w", calibration, err)
	}
	selection, err := Select(input)
	if err != nil {
		return Selection{}, err
	}
	if err := os.MkdirAll(out, dirMode); err != nil {
		return Selection{}, err
	}
	if selection.Decision == DecisionSelected {
		pooled := selection.Pooled[selection.Coder]
		reach, err := reacher.Reach(ctx, pooled.K, pooled.N)
		if err != nil {
			return Selection{}, err
		}
		if err := os.WriteFile(filepath.Join(out, ReachFile), []byte(reach), fileMode); err != nil {
			return Selection{}, err
		}
	}
	document, err := json.MarshalIndent(selection, "", jsonIndent)
	if err != nil {
		return Selection{}, err
	}
	return selection, os.WriteFile(filepath.Join(out, SelectionFile), append(document, '\n'), fileMode)
}

// Main parses the verb's flags and runs it: 0 when the selection is written, 1
// when it is refused or fails, 2 on a usage error.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	flags := flag.NewFlagSet("calibrate", flag.ContinueOnError)
	flags.SetOutput(stderr)
	repo := flags.String("repo", ".", "repository root")
	calibration := flags.String("calibration", "", calibrationUsage)
	out := flags.String("out", "", "directory that receives selection.json and reach.txt")
	power := flags.String("power", DefaultPower, "the power script, relative to the repository root")
	if err := flags.Parse(args); err != nil {
		return exitUsage
	}
	if flags.NArg() > 0 || *calibration == "" || *out == "" {
		fmt.Fprintln(stderr, "calibrate: --calibration and --out are required, and nothing else")
		return exitUsage
	}
	script := *power
	if !filepath.IsAbs(script) {
		script = filepath.Join(*repo, script)
	}
	selection, err := Calibrate(ctx, *calibration, *out, c.Reacher(script))
	if err != nil {
		fmt.Fprintf(stderr, "calibrate: %v\n", err)
		return exitRefused
	}
	fmt.Fprintf(stdout, "calibrate: %s %s; written to %s\n", selection.Decision, selection.Coder, *out)
	return exitOK
}
