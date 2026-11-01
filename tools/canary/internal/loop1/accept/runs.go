package accept

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The program and environment every command of a check runs with.
const (
	PythonProgram   = "python"
	ShallnotProgram = productBin + "/" + plan.ShallnotBinary
	// ContainerPath is the PATH of every command: the product's binaries, then the
	// image's.
	ContainerPath = productBin + ":" + container.SystemPath
	productBin    = container.ProductTarget + "/" + plan.ProductBinDir

	stageTreesDir = "trees"
	stageOutDir   = "out"
	stageHomeDir  = "home"
)

// Runs is how many times a check's command runs.
const Runs = study.OracleRuns

// Ordered is a command's three runs and whether they agreed.
type Ordered struct {
	Results    [Runs]box.Result
	Signatures [Runs]string
	Agreed     bool
}

// CommandFor builds the command of one run; SignatureOf reduces one run to what
// the runs must agree on.
type (
	CommandFor  func(run int) box.Command
	SignatureOf func(run int, result box.Result) string
)

// Bench runs the commands of one acceptance round in the offline box: it writes
// each tree under its stage, the only host path a command mounts, runs each
// command Runs times, transcribes every run into Log, and keeps each pytest
// outcome by label for the round.
type Bench struct {
	Runner box.Runner
	Stage  string
	// Log is the evidence of the commands run since it was last reset.
	Log  strings.Builder
	memo map[string]memoEntry
}

// NewBench builds the bench of a round staged at stage.
func NewBench(runner box.Runner, stage string) *Bench {
	return &Bench{Runner: runner, Stage: stage, memo: map[string]memoEntry{}}
}

// Repeat runs a command Runs times and reports whether the runs agree.
func (b *Bench) Repeat(ctx context.Context, build CommandFor, signature SignatureOf) (Ordered, error) {
	var out Ordered
	out.Agreed = true
	for run := 0; run < Runs; run++ {
		command := build(run)
		result, err := b.Runner.Run(ctx, command)
		if err != nil {
			return out, fmt.Errorf("run %s: %w", strings.Join(command.Argv, " "), err)
		}
		b.transcribe(command, result, run)
		out.Results[run] = result
		out.Signatures[run] = signature(run, result)
		if out.Signatures[run] != out.Signatures[0] {
			out.Agreed = false
		}
	}
	return out, nil
}

// transcribe appends one run to the evidence of the check that executes it.
func (b *Bench) transcribe(command box.Command, result box.Result, run int) {
	fmt.Fprintf(&b.Log, "$ %s   (dir %s, run %d)\nexit %d timed-out %t oom-killed %t\n",
		strings.Join(command.Argv, " "), command.Dir, run+1, result.ExitCode, result.TimedOut, result.OOMKilled)
	fmt.Fprintf(&b.Log, "%s%s\n", result.Stdout, result.Stderr)
}

// Command builds a command in a tree directory of the stage.
func (b *Bench) Command(dir string, argv ...string) box.Command {
	return b.CommandWith(dir, nil, argv...)
}

// CommandWith builds a command in a tree directory of the stage with variables
// added to the environment every command has.
func (b *Bench) CommandWith(dir string, variables []string, argv ...string) box.Command {
	env := []string{
		"PATH=" + ContainerPath,
		"HOME=" + filepath.Join(b.Stage, stageHomeDir),
		"PYTHONDONTWRITEBYTECODE=1",
		"PYTHONHASHSEED=0",
	}
	return box.Command{
		Argv:   argv,
		Env:    append(env, variables...),
		Dir:    dir,
		Mounts: []box.Mount{{Path: b.Stage}},
		Limit:  study.OracleLimit,
	}
}

// TreeDir writes a tree into the stage under a label and returns its directory.
func (b *Bench) TreeDir(label string, files Tree) (string, error) {
	dir := filepath.Join(b.Stage, stageTreesDir, label)
	if err := files.Write(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// OutFile is where run k of a labelled command writes its JUnit file.
func (b *Bench) OutFile(label string, run int) string {
	return filepath.Join(b.Stage, stageOutDir, label, fmt.Sprintf("run-%d.xml", run+1))
}

// Outcome is what a pytest command reported over its runs.
type Outcome struct {
	Cases    []CaseResult
	ExitCode int
	TimedOut bool
	// Broken is set when the run left no readable JUnit file, or ended non-zero
	// without a failing case: every test of the run counts as failing.
	Broken bool
}

func (o Outcome) signature() string {
	parts := []string{fmt.Sprintf("exit=%d", o.ExitCode), fmt.Sprintf("timeout=%t", o.TimedOut), fmt.Sprintf("broken=%t", o.Broken)}
	for _, c := range o.Cases {
		parts = append(parts, c.ID+"="+string(c.Status))
	}
	return strings.Join(parts, "\n")
}

// FailedCases lists the cases that failed.
func (o Outcome) FailedCases() []CaseResult {
	var failed []CaseResult
	for _, c := range o.Cases {
		if c.Status == StatusFailed {
			failed = append(failed, c)
		}
	}
	return failed
}

// EverythingFails is whether the run counts as every test failing.
func (o Outcome) EverythingFails() bool { return o.Broken || o.TimedOut }

// PassesAll is whether the run ended with at least one passing case and no
// failing one.
func (o Outcome) PassesAll() bool {
	if o.EverythingFails() || o.ExitCode != 0 {
		return false
	}
	return len(o.Cases) > 0 && len(o.FailedCases()) == 0
}

// FailsCriterion is whether a test of the criterion fails in the run.
func (o Outcome) FailsCriterion(criterion string) bool {
	if o.EverythingFails() {
		return true
	}
	for _, c := range o.FailedCases() {
		if c.Criterion == criterion {
			return true
		}
	}
	return false
}

// FailingOutside lists the failing cases that do not belong to the criterion.
func (o Outcome) FailingOutside(criterion string) []CaseResult {
	var failed []CaseResult
	for _, c := range o.FailedCases() {
		if c.Criterion != criterion {
			failed = append(failed, c)
		}
	}
	return failed
}

// Skipped lists the cases that were skipped.
func (o Outcome) Skipped() []CaseResult {
	var skipped []CaseResult
	for _, c := range o.Cases {
		if c.Status == StatusSkipped {
			skipped = append(skipped, c)
		}
	}
	return skipped
}

// Disagreement is the error of a command whose runs differ.
type Disagreement struct{ Signatures [Runs]string }

func (d Disagreement) Error() string {
	return "the three runs disagree:\n" + strings.Join(d.Signatures[:], "\n---\n")
}

// PytestOutcome runs pytest three times in a tree and reads its JUnit files.
// The result is kept for the round, so a tree is run once however many checks
// read it. A disagreement is returned as the outcome's error.
func (b *Bench) PytestOutcome(ctx context.Context, label string, files Tree, argv func(junit string) []string, modules map[string]string) (Outcome, error) {
	if kept, found := b.memo[label]; found {
		return kept.outcome, kept.err
	}
	dir, err := b.TreeDir(label, files)
	if err != nil {
		return Outcome{}, err
	}
	if err := os.MkdirAll(filepath.Join(b.Stage, stageOutDir, label), treeDirectoryMode); err != nil {
		return Outcome{}, fmt.Errorf("create the output directory of %s: %w", label, err)
	}
	var outcomes [Runs]Outcome
	runs, err := b.Repeat(ctx,
		func(run int) box.Command { return b.Command(dir, argv(b.OutFile(label, run))...) },
		func(run int, result box.Result) string {
			outcomes[run] = readOutcome(b.OutFile(label, run), result, modules)
			return outcomes[run].signature()
		})
	if err != nil {
		return Outcome{}, err
	}
	kept := memoEntry{outcome: outcomes[0]}
	if !runs.Agreed {
		kept.err = Disagreement{Signatures: runs.Signatures}
	}
	b.memo[label] = kept
	return kept.outcome, kept.err
}

type memoEntry struct {
	outcome Outcome
	err     error
}

func readOutcome(junit string, result box.Result, modules map[string]string) Outcome {
	found := Outcome{ExitCode: result.ExitCode, TimedOut: result.TimedOut || result.OOMKilled}
	data, err := os.ReadFile(junit)
	if err != nil {
		found.Broken = true
		return found
	}
	cases, err := ParseJUnit(data, modules)
	if err != nil {
		found.Broken = true
		return found
	}
	found.Cases = cases
	if result.ExitCode != 0 && len(found.FailedCases()) == 0 {
		found.Broken = true
	}
	return found
}

// SortedCaseIDs lists case IDs in order, for findings.
func SortedCaseIDs(cases []CaseResult) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	return ids
}
