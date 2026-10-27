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

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// PhaseEnded tells whether the run phase has ended: every run of the order is
// counted or blocked, or the calendar cap is reached (PREREG §6.6). The analysis
// package owns the rule; the verb depends on this port alone.
type PhaseEnded interface {
	Ended(ctx context.Context) (bool, error)
}

// attemptRecord is run.json as the oracle verb reads it: the study's record with
// the hash of the equipped commit.
type attemptRecord struct {
	study.Run
	EquippedCommit string `json:"equippedCommit"`
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
	Phase  PhaseEnded
	Log    io.Writer
}

// NewVerb builds the verb with a judge that keeps its jobs in the work directory.
func NewVerb(repo string, layout study.Layout, runner box.Runner, phase PhaseEnded, log io.Writer) *Verb {
	jobs := filepath.Join(layout.Root, verbDir, jobsDir)
	return &Verb{Repo: repo, Layout: layout, Judge: NewJudge(runner, jobs), Phase: phase, Log: log}
}

// Execute judges every counted attempt of the work directory. It refuses to run
// before the run phase ends. An attempt that fails does not stop the others; the
// failures are returned together.
func (v *Verb) Execute(ctx context.Context) error {
	ended, err := v.Phase.Ended(ctx)
	if err != nil {
		return err
	}
	if !ended {
		return ErrPhaseNotEnded
	}
	attempts, err := v.countedAttempts()
	if err != nil {
		return err
	}
	var failures []error
	for _, attempt := range attempts {
		if err := v.judgeAttempt(ctx, attempt); err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", attempt.name, err))
		}
	}
	return errors.Join(failures...)
}

type attempt struct {
	name   string
	runDir string
	record attemptRecord
}

// countedAttempts lists the attempt directories whose outcome enters the
// analysis, in name order.
func (v *Verb) countedAttempts() ([]attempt, error) {
	runs := filepath.Join(v.Layout.Root, study.RunsDir)
	names, err := listDirs(runs)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrRecords, err)
	}
	var counted []attempt
	for _, name := range names {
		runDir := filepath.Join(runs, name)
		record, err := readAttemptRecord(runDir)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if record.Outcome.Counted() {
			counted = append(counted, attempt{name: name, runDir: runDir, record: record})
		}
	}
	return counted, nil
}

func readAttemptRecord(runDir string) (attemptRecord, error) {
	data, err := os.ReadFile(filepath.Join(recordsPath(runDir), runFile))
	if err != nil {
		return attemptRecord{}, fmt.Errorf("%w: %v", ErrRecords, err)
	}
	var record attemptRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return attemptRecord{}, fmt.Errorf("%w: %s: %v", ErrRecords, runFile, err)
	}
	return record, nil
}

func (v *Verb) loadTask(name string) (study.Task, error) {
	data, err := os.ReadFile(filepath.Join(v.Repo, filepath.FromSlash(tasksDir), name, study.TaskFile))
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
