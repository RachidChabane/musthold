package analysis

import (
	"fmt"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
)

// The calendar of the run phase (ADR 0017, "Steps and stops").
const (
	// CalendarCap ends the run phase this long after the start of its first run.
	CalendarCap = 45 * 24 * time.Hour
	// CapTasks is the tasks that need CapRunsPerArm counted runs in each arm for
	// the analysis to run at the cap.
	CapTasks      = 20
	CapRunsPerArm = 6
)

// Calendar is the run phase's calendar.
var Calendar = phase.Calendar{Cap: CalendarCap}

// Feasibility says whether the analysis runs on what was counted.
type Feasibility struct {
	CapReached bool `json:"capReached"`
	// Tasks counts the tasks with at least CapRunsPerArm counted runs in each
	// arm; Needed is CapTasks.
	Tasks  int  `json:"tasks"`
	Needed int  `json:"needed"`
	Runs   bool `json:"runs"`
	// Reason names why the analysis does not run.
	Reason string `json:"reason,omitempty"`
}

// Feasible applies the calendar rule: without the cap the analysis runs; at
// the cap it runs when at least CapTasks tasks hold CapRunsPerArm counted runs
// in each arm.
func Feasible(capReached bool, runs []record.Run) Feasibility {
	found := Feasibility{CapReached: capReached, Needed: CapTasks}
	tasks := map[string]map[string]int{}
	for _, run := range runs {
		if tasks[run.Task] == nil {
			tasks[run.Task] = map[string]int{}
		}
		tasks[run.Task][run.Arm]++
	}
	for _, arms := range tasks {
		if arms[analyze.ArmControl] >= CapRunsPerArm && arms[analyze.ArmTreatment] >= CapRunsPerArm {
			found.Tasks++
		}
	}
	found.Runs = !capReached || found.Tasks >= CapTasks
	if !found.Runs {
		found.Reason = fmt.Sprintf("the calendar cap was reached with %d tasks holding at least %d counted runs in each arm, %d needed",
			found.Tasks, CapRunsPerArm, CapTasks)
	}
	return found
}

// apply makes the outcome INCONCLUSIVE with the calendar's reason when the
// analysis does not run.
func (f Feasibility) apply(primary Primary) Primary {
	if f.Runs {
		return primary
	}
	row := decision.RowOf(decision.Inconclusive)
	primary.Outcome, primary.Row, primary.Reading, primary.Verdict, primary.Reason = row.Outcome, row.Number, row.Reading, row.Verdict, f.Reason
	return primary
}
