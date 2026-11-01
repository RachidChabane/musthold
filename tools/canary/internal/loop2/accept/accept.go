//go:build loop2wip

// Package accept implements the acceptance checks of the loop-2 study (ADR
// 0017, "Tasks and oracle"): loop-1's A1 to A9 for the loop-2 shape, A10 to
// A12, and the lever checks L1 to L5. It makes no model call and runs no gate:
// every command a check executes runs three times in the offline box through
// loop-1's bench, and every model's answer is a file construction wrote. A task
// that passes every check gets its held-out violations recorded and is appended
// to loop-2.json.
package accept

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

// The checks beyond loop-1's A1 to A9.
const (
	CheckSecondOracle = "A10"
	CheckCitations    = "A11"
	CheckBoundaries   = "A12"
)

// checkNames names each check in reports.
var checkNames = map[string]string{
	l1accept.CheckShape: "shape", l1accept.CheckFixture: "fixture", l1accept.CheckReference: "reference",
	l1accept.CheckBase: "base", l1accept.CheckDiagonal: "diagonal", l1accept.CheckRefactors: "refactors",
	l1accept.CheckRoom: "room", l1accept.CheckLeak: "leak", l1accept.CheckCollection: "collection",
	CheckSecondOracle: "second oracle", CheckCitations: "citations", CheckBoundaries: "boundaries",
}

// CheckOrder lists every check in the order it runs: A1 to A12, then L1 to L5.
func CheckOrder() []string {
	order := append(l1accept.CheckOrder(), CheckSecondOracle, CheckCitations, CheckBoundaries)
	for _, lever := range task.AllLevers() {
		order = append(order, string(lever))
	}
	return order
}

func checkName(id string) string {
	if name, found := checkNames[id]; found {
		return name
	}
	return task.LeverNames[task.Lever(id)]
}

func newResult(id string) *l1accept.Result { return l1accept.NewResult(id, checkName(id)) }

// acceptDir is the staging directory of the checks inside the work directory's
// construction directory: the only host path the containers mount.
const acceptDir = "accept"

const (
	dirMode    = 0o755
	fileMode   = 0o644
	jsonIndent = "  "
)

// Acceptor runs the checks on tasks lying in a repository.
type Acceptor struct {
	// Repo is the repository root; Config loop-2.json, which receives the tasks
	// that pass.
	Repo    string
	Config  string
	Layout  study.Layout
	Runner  box.Runner
	Records records.Writer
}

// NewAcceptor builds the acceptor of the verb's flags.
func NewAcceptor(flags task.Flags, runner box.Runner, writer records.Writer) *Acceptor {
	return &Acceptor{Repo: flags.Repo, Config: flags.ConfigPath(), Layout: flags.Layout(), Runner: runner, Records: writer}
}

// Accept runs every check on the named task, records each, and, when all pass,
// writes the task's held-out violations and appends it to loop-2.json. When A1
// fails the other checks are reported as not run.
func (a *Acceptor) Accept(ctx context.Context, name string, number int) (l1accept.Report, error) {
	m, err := loadMaterial(a.Repo, name)
	if err != nil {
		result := newResult(l1accept.CheckShape)
		result.Fail(records.RoleAuthor, records.TargetTask, "%v", err)
		report := withSkipped(name, result)
		return report, l1accept.RecordReport(a.Records, name, number, report)
	}
	stage := filepath.Join(a.Layout.Construction(), acceptDir, name, fmt.Sprintf("round-%d", number))
	if err := os.RemoveAll(stage); err != nil {
		return l1accept.Report{}, fmt.Errorf("clear the stage of %s: %w", name, err)
	}
	defer os.RemoveAll(stage)
	if err := os.MkdirAll(stage, dirMode); err != nil {
		return l1accept.Report{}, fmt.Errorf("create the stage of %s: %w", name, err)
	}
	r := newRound(l1accept.NewBench(a.Runner, stage), m)
	report, err := r.run(ctx)
	if err != nil {
		return l1accept.Report{}, err
	}
	if err := l1accept.RecordReport(a.Records, name, number, report); err != nil {
		return l1accept.Report{}, err
	}
	if !report.Passed() {
		return report, nil
	}
	return report, a.admit(ctx, r)
}

// withSkipped is a report whose shape check failed: the others did not run.
func withSkipped(name string, shape *l1accept.Result) l1accept.Report {
	report := l1accept.Report{Task: name, Results: []l1accept.Result{*shape}}
	for _, id := range CheckOrder()[1:] {
		report.Results = append(report.Results, l1accept.Result{ID: id, Name: checkName(id), Skipped: true})
	}
	return report
}

type checkFunc func(r *round, ctx context.Context) (*l1accept.Result, error)

func checks() map[string]checkFunc {
	return map[string]checkFunc{
		l1accept.CheckShape: (*round).shape, l1accept.CheckFixture: (*round).fixtureCheck, l1accept.CheckReference: (*round).referenceCheck,
		l1accept.CheckBase: (*round).base, l1accept.CheckDiagonal: (*round).diagonal, l1accept.CheckRefactors: (*round).refactorsCheck,
		l1accept.CheckRoom: (*round).room, l1accept.CheckLeak: (*round).leak, l1accept.CheckCollection: (*round).collection,
		CheckSecondOracle: (*round).secondOracle, CheckCitations: (*round).citationsCheck, CheckBoundaries: (*round).boundaries,
		string(task.LeverInteracting): (*round).interacting, string(task.LeverBoundaries): (*round).boundaryLever,
		string(task.LeverShared): (*round).sharedHelper, string(task.LeverRevision): (*round).revision,
		string(task.LeverSize): (*round).size,
	}
}

// run runs the checks in order; an undeclared lever's check is reported as not
// run, and a failed A1 skips every other check.
func (r *round) run(ctx context.Context) (l1accept.Report, error) {
	table := checks()
	report := l1accept.Report{Task: r.m.task.Name}
	for _, id := range CheckOrder() {
		if lever := task.Lever(id); slices.Contains(task.AllLevers(), lever) && !r.m.levers.Has(lever) {
			report.Results = append(report.Results, l1accept.Result{ID: id, Name: checkName(id), Passed: true, Agreed: true, Skipped: true})
			continue
		}
		r.Log.Reset()
		result, err := table[id](r, ctx)
		if err != nil {
			return l1accept.Report{}, fmt.Errorf("check %s %s: %w", id, checkName(id), err)
		}
		result.Output += r.Log.String()
		report.Results = append(report.Results, *result)
		if id == l1accept.CheckShape && !result.Passed {
			return withSkipped(r.m.task.Name, result), nil
		}
	}
	return report, nil
}

// admit writes the held-out violations of a task that passed and appends it to
// loop-2.json, unless it is listed.
func (a *Acceptor) admit(ctx context.Context, r *round) error {
	heldOut, err := r.heldOutRecords(ctx)
	if err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(heldOut, "", jsonIndent)
	if err != nil {
		return err
	}
	location := filepath.Join(a.Repo, filepath.FromSlash(task.TaskPath(r.m.task.Name, task.HeldOutFile)))
	if err := os.WriteFile(location, append(encoded, '\n'), fileMode); err != nil {
		return err
	}
	data, err := os.ReadFile(a.Config)
	if err != nil {
		return fmt.Errorf("read the run configuration: %w", err)
	}
	updated, err := l1accept.AppendTask(data, r.m.task.ConfigTask())
	if err != nil {
		return err
	}
	if string(updated) == string(data) {
		return nil
	}
	if _, err := task.ParseConfig(updated); err != nil {
		return fmt.Errorf("the run configuration with %s added is refused: %w", r.m.task.Name, err)
	}
	return os.WriteFile(a.Config, updated, fileMode)
}
