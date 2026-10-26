package scan

import (
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/scanscope"
	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
)

const verifiesDispatch = " [verifies SP-2139~1]"

// The judged tests of the dispatch scenario: the first one kills the first
// mutant of its line, the second one survives everything.
const (
	catcher  = ecosystem.TestID("tests/test_rates.py::test_rate_of_zero")
	survivor = ecosystem.TestID("tests/test_rates.py::test_rate_is_read")
	rates    = "fares/rates.py"
)

func ratesLine(number int) vacuity.Line { return vacuity.Line{File: rates, Line: number} }

func passed() []vacuity.Run {
	return []vacuity.Run{vacuity.RunPassed, vacuity.RunPassed, vacuity.RunPassed}
}
func failed() []vacuity.Run {
	return []vacuity.Run{vacuity.RunFailed, vacuity.RunFailed, vacuity.RunFailed}
}

// dispatchScenario draws a sentinel and four mutants: three on the line only
// the catcher runs, one on the line only the survivor runs, in the start order
// sentinel, kill, two on the catcher's line, then the survivor's.
func dispatchScenario() (reference, plan, drawn, map[int]map[ecosystem.TestID][]vacuity.Run) {
	ref := reference{
		runs: map[ecosystem.TestID][]vacuity.Run{catcher: passed(), survivor: passed()},
		coverage: map[ecosystem.TestID]vacuity.Coverage{
			catcher:  {Lines: []vacuity.Line{ratesLine(3)}, Owned: []vacuity.Line{ratesLine(3)}},
			survivor: {Lines: []vacuity.Line{ratesLine(7)}, Owned: []vacuity.Line{ratesLine(7)}},
		},
	}
	candidates := []Candidate{
		{ID: "sentinel", Sentinel: true, Lines: []vacuity.Line{ratesLine(3)}},
		{ID: "kill", Lines: []vacuity.Line{ratesLine(3)}},
		{ID: "after-kill", Lines: []vacuity.Line{ratesLine(3)}},
		{ID: "also-after-kill", Lines: []vacuity.Line{ratesLine(3)}},
		{ID: "survived", Lines: []vacuity.Line{ratesLine(7)}},
	}
	results := map[int]map[ecosystem.TestID][]vacuity.Run{
		0: {catcher: passed()}, 1: {catcher: failed()}, 2: {catcher: passed()}, 3: {catcher: passed()}, 4: {survivor: passed()},
	}
	return ref, plan{sampled: []ecosystem.TestID{catcher, survivor}}, drawn{candidates: candidates, order: []int{0, 1, 2, 3, 4}}, results
}

// steppedClock advances by one step at every reading.
type steppedClock struct {
	now  time.Time
	step time.Duration
}

func (c *steppedClock) read() time.Time {
	c.now = c.now.Add(c.step)
	return c.now
}

// dispatchOn runs the scenario's coordinator on one worker that finishes each
// candidate with the scenario's runs, and lists the candidates in the order
// they started.
func dispatchOn(t *testing.T, deadline time.Time, clock func() time.Time) ([]int, []outcome) {
	t.Helper()
	ref, scanPlan, scanDrawn, results := dispatchScenario()
	c := coordinator{
		service: &Service{deps: Deps{Clock: clock}}, deadline: deadline, drawn: scanDrawn,
		queue: scanscope.NewQueue(scanDrawn.order), progress: vacuity.NewProgress(progressInput(ref, scanPlan, scanDrawn)),
		outcomes: make([]outcome, len(scanDrawn.candidates)),
	}
	jobs, finished := make(chan int), make(chan finishedRun)
	var started []int
	go func() {
		for index := range jobs {
			started = append(started, index)
			finished <- finishedRun{index: index, result: outcome{state: stateRan, runs: results[index]}}
		}
	}()
	if err := c.dispatch(1, jobs, finished); err != nil {
		t.Fatal(err)
	}
	close(jobs)
	return started, c.outcomes
}

func TestDispatch(t *testing.T) {
	t.Run("once a test is settled, the mutants only it runs start after those an open test runs"+verifiesDispatch, func(t *testing.T) {
		started, outcomes := dispatchOn(t, time.Time{}, time.Now)
		if !slices.Equal(started, []int{0, 1, 4, 2, 3}) {
			t.Fatalf("started %v", started)
		}
		for index, executed := range outcomes {
			if executed.state != stateRan {
				t.Fatalf("candidate %d did not run with no deadline", index)
			}
		}
	})
	t.Run("a deadline leaves unrun the mutants no open test runs, not those it needs"+verifiesDispatch, func(t *testing.T) {
		clock := &steppedClock{now: time.Unix(0, 0), step: time.Second}
		deadline := clock.now.Add(3*time.Second + time.Second/2)
		started, outcomes := dispatchOn(t, deadline, clock.read)
		if !slices.Equal(started, []int{0, 1, 4}) || outcomes[2].state != stateNotRun || outcomes[3].state != stateNotRun {
			t.Fatalf("started %v, outcomes %+v", started, outcomes)
		}
	})
}
