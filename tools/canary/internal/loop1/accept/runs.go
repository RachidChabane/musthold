package accept

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The program and environment every command of a check runs with.
const (
	PythonProgram   = "python"
	ShallnotProgram = "/opt/musthold/bin/shallnot"
	// ContainerPath is the PATH of every command: the product's binaries, then the
	// image's.
	ContainerPath = "/opt/musthold/bin:/usr/local/bin:/usr/bin:/bin"

	stageTreesDir = "trees"
	stageOutDir   = "out"
	stageHomeDir  = "home"
)

// Runs is how many times a check's command runs.
const Runs = study.OracleRuns

// ordered is a command's three runs and whether they agreed.
type ordered struct {
	results    [Runs]box.Result
	signatures [Runs]string
	agreed     bool
}

// commandFor builds the command of one run; signatureOf reduces one run to what
// the runs must agree on.
type (
	commandFor  func(run int) box.Command
	signatureOf func(run int, result box.Result) string
)

// repeat runs a command Runs times and reports whether the runs agree.
func (r *round) repeat(ctx context.Context, build commandFor, signature signatureOf) (ordered, error) {
	var out ordered
	out.agreed = true
	for run := 0; run < Runs; run++ {
		command := build(run)
		result, err := r.a.Runner.Run(ctx, command)
		if err != nil {
			return out, fmt.Errorf("run %s: %w", strings.Join(command.Argv, " "), err)
		}
		r.transcribe(command, result, run)
		out.results[run] = result
		out.signatures[run] = signature(run, result)
		if out.signatures[run] != out.signatures[0] {
			out.agreed = false
		}
	}
	return out, nil
}

// transcribe appends one run to the evidence of the check that executes it.
func (r *round) transcribe(command box.Command, result box.Result, run int) {
	fmt.Fprintf(&r.log, "$ %s   (dir %s, run %d)\nexit %d timed-out %t oom-killed %t\n",
		strings.Join(command.Argv, " "), command.Dir, run+1, result.ExitCode, result.TimedOut, result.OOMKilled)
	fmt.Fprintf(&r.log, "%s%s\n", result.Stdout, result.Stderr)
}

// command builds a command in a tree directory of the stage.
func (r *round) command(dir string, argv ...string) box.Command {
	return box.Command{
		Argv: argv,
		Env: []string{
			"PATH=" + ContainerPath,
			"HOME=" + filepath.Join(r.stage, stageHomeDir),
			"PYTHONDONTWRITEBYTECODE=1",
			"PYTHONHASHSEED=0",
		},
		Dir:    dir,
		Mounts: []box.Mount{{Path: r.stage}},
		Limit:  study.OracleLimit,
	}
}

// treeDir writes a tree into the stage under a label and returns its directory.
func (r *round) treeDir(label string, files tree) (string, error) {
	dir := filepath.Join(r.stage, stageTreesDir, label)
	if err := files.write(dir); err != nil {
		return "", err
	}
	return dir, nil
}

// outFile is where run k of a labelled command writes its JUnit file.
func (r *round) outFile(label string, run int) string {
	return filepath.Join(r.stage, stageOutDir, label, fmt.Sprintf("run-%d.xml", run+1))
}

// outcome is what a pytest command reported over its runs.
type outcome struct {
	Cases    []CaseResult
	ExitCode int
	TimedOut bool
	// Broken is set when the run left no readable JUnit file, or ended non-zero
	// without a failing case: every test of the run counts as failing.
	Broken bool
}

func (o outcome) signature() string {
	parts := []string{fmt.Sprintf("exit=%d", o.ExitCode), fmt.Sprintf("timeout=%t", o.TimedOut), fmt.Sprintf("broken=%t", o.Broken)}
	for _, c := range o.Cases {
		parts = append(parts, c.ID+"="+string(c.Status))
	}
	return strings.Join(parts, "\n")
}

// failedCases lists the cases that failed.
func (o outcome) failedCases() []CaseResult {
	var failed []CaseResult
	for _, c := range o.Cases {
		if c.Status == StatusFailed {
			failed = append(failed, c)
		}
	}
	return failed
}

// everythingFails is whether the run counts as every test failing.
func (o outcome) everythingFails() bool { return o.Broken || o.TimedOut }

// passesAll is whether the run ended with at least one passing case and no
// failing one.
func (o outcome) passesAll() bool {
	if o.everythingFails() || o.ExitCode != 0 {
		return false
	}
	return len(o.Cases) > 0 && len(o.failedCases()) == 0
}

// failsCriterion is whether a test of the criterion fails in the run.
func (o outcome) failsCriterion(criterion string) bool {
	if o.everythingFails() {
		return true
	}
	for _, c := range o.failedCases() {
		if c.Criterion == criterion {
			return true
		}
	}
	return false
}

// failingOutside lists the failing cases that do not belong to the criterion.
func (o outcome) failingOutside(criterion string) []CaseResult {
	var failed []CaseResult
	for _, c := range o.failedCases() {
		if c.Criterion != criterion {
			failed = append(failed, c)
		}
	}
	return failed
}

// skipped lists the cases that were skipped.
func (o outcome) skipped() []CaseResult {
	var skipped []CaseResult
	for _, c := range o.Cases {
		if c.Status == StatusSkipped {
			skipped = append(skipped, c)
		}
	}
	return skipped
}

// disagreement is the error of a command whose runs differ.
type disagreement struct{ signatures [Runs]string }

func (d disagreement) Error() string {
	return "the three runs disagree:\n" + strings.Join(d.signatures[:], "\n---\n")
}

// pytestOutcome runs pytest three times in a tree and reads its JUnit files.
// The result is kept for the round, so a tree is run once however many checks
// read it. A disagreement is returned as the outcome's error.
func (r *round) pytestOutcome(ctx context.Context, label string, files tree, argv func(junit string) []string, modules map[string]string) (outcome, error) {
	if kept, found := r.memo[label]; found {
		return kept.outcome, kept.err
	}
	dir, err := r.treeDir(label, files)
	if err != nil {
		return outcome{}, err
	}
	if err := os.MkdirAll(filepath.Join(r.stage, stageOutDir, label), treeDirectoryMode); err != nil {
		return outcome{}, fmt.Errorf("create the output directory of %s: %w", label, err)
	}
	var outcomes [Runs]outcome
	runs, err := r.repeat(ctx,
		func(run int) box.Command { return r.command(dir, argv(r.outFile(label, run))...) },
		func(run int, result box.Result) string {
			outcomes[run] = readOutcome(r.outFile(label, run), result, modules)
			return outcomes[run].signature()
		})
	if err != nil {
		return outcome{}, err
	}
	kept := memoEntry{outcome: outcomes[0]}
	if !runs.agreed {
		kept.err = disagreement{signatures: runs.signatures}
	}
	r.memo[label] = kept
	return kept.outcome, kept.err
}

type memoEntry struct {
	outcome outcome
	err     error
}

func readOutcome(junit string, result box.Result, modules map[string]string) outcome {
	found := outcome{ExitCode: result.ExitCode, TimedOut: result.TimedOut || result.OOMKilled}
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
	if result.ExitCode != 0 && len(found.failedCases()) == 0 {
		found.Broken = true
	}
	return found
}

// sortedCaseIDs lists case IDs in order, for findings.
func sortedCaseIDs(cases []CaseResult) []string {
	ids := make([]string, 0, len(cases))
	for _, c := range cases {
		ids = append(ids, c.ID)
	}
	sort.Strings(ids)
	return ids
}
