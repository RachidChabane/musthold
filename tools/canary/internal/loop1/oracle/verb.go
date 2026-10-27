package oracle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Phase hands the verb the counted attempt of every run once the run phase has
// ended, and refuses before (PREREG §6.6): the rule of analyze/phase.
type Phase interface {
	Counted() ([]phase.Attempt, error)
}

// state remembers the digests of the inputs that produced the outputs of one
// attempt. It lives in the work directory, never in the run directory.
type state struct {
	Oracle  string `json:"oracle"`
	HeldOut string `json:"heldOut"`
}

const stateSuffix = ".json"

// Verb is the oracle verb over a work directory.
type Verb struct {
	Repo   string
	Layout study.Layout
	Judge  *Judge
	Phase  Phase
	Log    io.Writer
}

// NewVerb builds the verb with a judge that keeps its jobs in the work directory.
func NewVerb(repo string, layout study.Layout, runner box.Runner, phase Phase, log io.Writer) *Verb {
	jobs := filepath.Join(layout.Root, verbDir, jobsDir)
	return &Verb{Repo: repo, Layout: layout, Judge: NewJudge(runner, jobs), Phase: phase, Log: log}
}

// Execute judges every counted attempt of the work directory. It refuses to run
// before the run phase ends. An attempt that fails does not stop the others; the
// failures are returned together.
func (v *Verb) Execute(ctx context.Context) error {
	attempts, err := v.Phase.Counted()
	if err != nil {
		return err
	}
	var failures []error
	for _, counted := range attempts {
		a := attempt{name: filepath.Base(counted.Dir), runDir: counted.Dir, record: counted.Run}
		if err := v.judgeAttempt(ctx, a); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", a.name, err))
		}
	}
	return errors.Join(failures...)
}

type attempt struct {
	name   string
	runDir string
	record study.Run
}

// MeasureEquippedAndFinal measures the tests of one attempt at its equipped
// commit and at its end against its task's violations, and writes its
// records/heldout.json: what the canary's C3 gives detector K3.
func (v *Verb) MeasureEquippedAndFinal(ctx context.Context, runDir string, run study.Run) error {
	task, err := v.loadTask(run.Task)
	if err != nil {
		return err
	}
	matrices, err := v.Judge.HeldOut(ctx, HeldOutInput{RunDir: runDir, EquippedCommit: run.EquippedCommit, Truth: TruthOf(v.Repo, task), TaskName: task.Name})
	if err != nil {
		return err
	}
	return writeJSON(HeldOutPath(runDir), matrices)
}

func (v *Verb) loadTask(name string) (study.Task, error) {
	data, err := os.ReadFile(filepath.Join(v.Repo, filepath.FromSlash(study.TaskDir(name)), study.TaskFile))
	if err != nil {
		return study.Task{}, fmt.Errorf("%w: %v", ErrTruth, err)
	}
	return study.ParseTask(data)
}

// readStops reads the snapshots of an attempt, in order of number.
func readStops(runDir string) ([]Stop, error) {
	dir := filepath.Join(recordsPath(runDir), study.StopsDir)
	names, err := listDirs(dir)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecords, err)
	}
	var stops []Stop
	for _, name := range names {
		number, err := strconv.Atoi(name)
		if err != nil {
			return nil, fmt.Errorf("%w: stop directory %q is not a number", ErrRecords, name)
		}
		tree, err := os.ReadFile(filepath.Join(dir, name, study.StopTreeFile))
		if err != nil {
			return nil, fmt.Errorf("%w: %v", ErrRecords, err)
		}
		stops = append(stops, Stop{Number: number, Tree: strings.TrimSpace(string(tree))})
	}
	sortStops(stops)
	return stops, nil
}

func (v *Verb) statePath(name string) string {
	return filepath.Join(v.Layout.Root, verbDir, stateDir, name+stateSuffix)
}

func (v *Verb) readState(name string) state {
	var s state
	if data, err := os.ReadFile(v.statePath(name)); err == nil {
		_ = json.Unmarshal(data, &s)
	}
	return s
}

func (v *Verb) writeState(name string, s state) error {
	if err := os.MkdirAll(filepath.Dir(v.statePath(name)), dirMode); err != nil {
		return err
	}
	data, err := json.Marshal(s)
	if err != nil {
		return err
	}
	return writeFileAtomic(v.statePath(name), data)
}

// judgeAttempt writes oracle.json and heldout.json of one attempt, each only when
// its inputs changed since the output was written.
func (v *Verb) judgeAttempt(ctx context.Context, a attempt) error {
	task, err := v.loadTask(a.record.Task)
	if err != nil {
		return err
	}
	truth := TruthOf(v.Repo, task)
	stops, err := readStops(a.runDir)
	if err != nil {
		return err
	}
	held := HeldOutInput{RunDir: a.runDir, EquippedCommit: a.record.EquippedCommit, Stops: stops, Truth: truth, TaskName: task.Name}
	oracleDigest, err := OracleTreeDigest(a.runDir, truth)
	if err != nil {
		return err
	}
	heldDigest, err := HeldOutDigest(held)
	if err != nil {
		return err
	}
	current := v.readState(a.name)
	if current.Oracle != oracleDigest || !exists(OraclePath(a.runDir)) {
		verdict, err := v.Judge.Oracle(ctx, a.runDir, truth)
		if err != nil {
			return err
		}
		if err := writeJSON(OraclePath(a.runDir), verdict); err != nil {
			return err
		}
		current.Oracle = oracleDigest
		if err := v.writeState(a.name, current); err != nil {
			return err
		}
		fmt.Fprintf(v.Log, "%s: oracle judged\n", a.name)
	}
	if current.HeldOut != heldDigest || !exists(HeldOutPath(a.runDir)) {
		matrices, err := v.Judge.HeldOut(ctx, held)
		if err != nil {
			return err
		}
		if err := writeJSON(HeldOutPath(a.runDir), matrices); err != nil {
			return err
		}
		current.HeldOut = heldDigest
		if err := v.writeState(a.name, current); err != nil {
			return err
		}
		fmt.Fprintf(v.Log, "%s: held-out violations measured\n", a.name)
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}
