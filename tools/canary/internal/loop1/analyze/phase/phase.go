// Package phase judges the run phase of the loop-1 study (PREREG §6.4, §6.6, as
// amended): the state of each run of the order from its attempts, and whether the
// phase has ended, which `oracle`, `detect` and `analyze` wait for. A quota stop
// never uses up one of a run's three attempts; only harness errors do. Judge is
// pure; Load reads the attempts of the work directory.
package phase

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// CalendarCap ends the run phase this long after the start of the first run.
const CalendarCap = 28 * 24 * time.Hour

// RecordsDir is the directory of an attempt that holds its records and the
// analysis's files.
const RecordsDir = plan.RecordsDir

// Records is the records directory of an attempt directory.
func Records(attemptDir string) string { return filepath.Join(attemptDir, RecordsDir) }

// Attempt is one attempt of a run: its number, from 1, its directory and its
// run.json.
type Attempt struct {
	Number int
	Dir    string
	Run    study.Run
}

// Records is the directory of the attempt's records.
func (a Attempt) Records() string { return Records(a.Dir) }

// RunState is a run of the order with its attempts and its state.
type RunState struct {
	Name     string
	Attempts []Attempt
	// Counted is the index in Attempts of the counted attempt, or -1.
	Counted int
	// State is the counted attempt's outcome, quota-blocked, harness-blocked or
	// not-run; it is empty while the run is open.
	State study.Outcome
}

// CountedAttempt returns the counted attempt.
func (r RunState) CountedAttempt() (Attempt, bool) {
	if r.Counted < 0 {
		return Attempt{}, false
	}
	return r.Attempts[r.Counted], true
}

// Closed reports whether the run is counted or blocked.
func (r RunState) Closed() bool { return r.State != "" && r.State != study.OutcomeNotRun }

// StateOf derives a run's state from its attempts, ordered by number.
func StateOf(name string, attempts []Attempt, capReached bool) RunState {
	state := RunState{Name: name, Attempts: attempts, Counted: -1}
	harnessErrors := 0
	for index, attempt := range attempts {
		if attempt.Run.Outcome.Counted() {
			state.Counted, state.State = index, attempt.Run.Outcome
			return state
		}
		if attempt.Run.Outcome == study.OutcomeHarnessError {
			harnessErrors++
		}
	}
	switch {
	case harnessErrors >= study.MaxAttempts:
		state.State = study.OutcomeHarnessBlocked
	case !capReached:
	case len(attempts) == 0:
		state.State = study.OutcomeNotRun
	case attempts[len(attempts)-1].Run.Outcome == study.OutcomeQuotaStop:
		state.State = study.OutcomeQuotaBlocked
	default:
		state.State = study.OutcomeHarnessBlocked
	}
	return state
}

// Phase is the judgement of the run phase.
type Phase struct {
	// Ended says whether the phase has ended, and CapReached whether the calendar
	// cap ended it.
	Ended, CapReached bool
	// FirstStart is the earliest start of any attempt, and Cap the time the
	// calendar cap falls; both are zero before the first attempt.
	FirstStart, Cap time.Time
	// Runs holds every run of the order, in order.
	Runs []RunState
	// Open names the runs neither counted nor blocked.
	Open []string
}

// Judge judges the run phase at now, from the order and the attempts of each
// run. The phase ends when every run of the order is counted or blocked, or once
// the calendar cap is reached.
func Judge(order []string, attempts map[string][]Attempt, now time.Time) Phase {
	judged := Phase{FirstStart: firstStart(attempts)}
	if !judged.FirstStart.IsZero() {
		judged.Cap = judged.FirstStart.Add(CalendarCap)
		judged.CapReached = !now.Before(judged.Cap)
	}
	for _, name := range order {
		state := StateOf(name, attempts[name], judged.CapReached)
		judged.Runs = append(judged.Runs, state)
		if !state.Closed() && state.State != study.OutcomeNotRun {
			judged.Open = append(judged.Open, name)
		}
	}
	judged.Ended = judged.CapReached || len(judged.Open) == 0
	return judged
}

func firstStart(attempts map[string][]Attempt) time.Time {
	var first time.Time
	for _, list := range attempts {
		for _, attempt := range list {
			started := attempt.Run.StartedAt
			if !started.IsZero() && (first.IsZero() || started.Before(first)) {
				first = started
			}
		}
	}
	return first
}

// ErrOpen refuses a verb that waits for the end of the run phase.
var ErrOpen = errors.New("the run phase has not ended")

// Require returns ErrOpen, naming the open runs and the cap, when the phase has
// not ended.
func (p Phase) Require() error {
	if p.Ended {
		return nil
	}
	limit := "the first run has not started"
	if !p.Cap.IsZero() {
		limit = "the calendar cap falls at " + p.Cap.UTC().Format(time.RFC3339)
	}
	return fmt.Errorf("%w: %d run(s) neither counted nor blocked (%s); %s", ErrOpen, len(p.Open), strings.Join(p.Open, ", "), limit)
}

// attemptSeparator joins a run's name and its attempt number in a directory name.
const attemptSeparator = "-"

// Load reads the attempts of every run of the order from the work directory: each
// directory of runs/ named <run>-<attempt>, ordered by attempt number, with the
// run.json of its records.
func Load(work study.Layout, order []string) (map[string][]Attempt, error) {
	root := filepath.Join(work.Root, study.RunsDir)
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return map[string][]Attempt{}, nil
	}
	if err != nil {
		return nil, err
	}
	found := map[string][]Attempt{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name, number, ok := attemptOf(entry.Name(), order)
		if !ok {
			continue
		}
		dir := filepath.Join(root, entry.Name())
		run, err := readRun(Records(dir))
		if err != nil {
			return nil, err
		}
		found[name] = append(found[name], Attempt{Number: number, Dir: dir, Run: run})
	}
	for name := range found {
		slices.SortFunc(found[name], func(a, b Attempt) int { return a.Number - b.Number })
	}
	return found, nil
}

// attemptOf splits a directory name into a run of the order and an attempt
// number.
func attemptOf(dir string, order []string) (string, int, bool) {
	cut := strings.LastIndex(dir, attemptSeparator)
	if cut < 0 {
		return "", 0, false
	}
	name, suffix := dir[:cut], dir[cut+len(attemptSeparator):]
	number, err := strconv.Atoi(suffix)
	if err != nil || number < 1 || strconv.Itoa(number) != suffix || !slices.Contains(order, name) {
		return "", 0, false
	}
	return name, number, true
}

// readRun reads the run.json of a records directory; an attempt that has not written it yet
// has no outcome.
func readRun(dir string) (study.Run, error) {
	var run study.Run
	data, err := os.ReadFile(filepath.Join(dir, record.MetaFile))
	if errors.Is(err, os.ErrNotExist) {
		return run, nil
	}
	if err != nil {
		return run, err
	}
	if err := json.Unmarshal(data, &run); err != nil {
		return run, fmt.Errorf("%s: %w", filepath.Join(dir, record.MetaFile), err)
	}
	return run, nil
}
