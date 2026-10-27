// Package accept implements the acceptance checks A1 to A9 of the loop-1 study
// (PREREG §3.4) and the verb that runs them. Every command a check executes goes
// through the box port, three times, offline; the three runs must agree. A task
// that passes every check is appended to the run configuration.
package accept

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// AcceptDir is the staging directory of the checks, inside the construction
// directory of the work directory: the only host path the containers mount.
const AcceptDir = "accept"

// Equipper equips a project as the control arm's project: the equipping of
// PREREG §2 with `proofs.require` empty and the task's operations bound. dir
// holds the fixture's files, inside the stage; the equipped project it returns
// lies inside the stage too.
type Equipper interface {
	EquipControl(ctx context.Context, dir string, task study.Task) (string, error)
}

// Acceptor runs the checks on tasks lying in a repository.
type Acceptor struct {
	// Repo is the repository root; Config the run configuration, which receives
	// the tasks that pass.
	Repo   string
	Config string
	Layout study.Layout
	Runner box.Runner
	// Equipper equips the control arm's project for check A2.
	Equipper Equipper
	// Records receives the output of every check.
	Records records.Writer
}

// round is one evaluation of one task.
type round struct {
	a        *Acceptor
	m        *material
	name     string
	number   int
	stage    string
	log      strings.Builder
	memo     map[string]memoEntry
	criteria []string
	refs     map[string]string
	examples map[string][]Example
	spec     []byte
}

// Accept runs the checks A1 to A9 on the named task, records each check, and
// appends the task to the run configuration when all of them pass. When the shape
// check fails, the other checks are reported as not run.
func (a *Acceptor) Accept(ctx context.Context, name string, number int) (Report, error) {
	m, err := loadMaterial(a.Repo, name)
	if err != nil {
		return a.unreadable(name, number, err)
	}
	stage := filepath.Join(a.Layout.Construction(), AcceptDir, name, fmt.Sprintf("round-%d", number))
	if err := os.RemoveAll(stage); err != nil {
		return Report{}, fmt.Errorf("clear the stage of %s: %w", name, err)
	}
	defer os.RemoveAll(stage)
	if err := os.MkdirAll(stage, treeDirectoryMode); err != nil {
		return Report{}, fmt.Errorf("create the stage of %s: %w", name, err)
	}
	r := &round{a: a, m: m, name: name, number: number, stage: stage, memo: map[string]memoEntry{}, refs: map[string]string{}}
	report, err := r.runChecks(ctx)
	if err != nil {
		return Report{}, err
	}
	if report.Passed() {
		if err := a.admit(m.task); err != nil {
			return Report{}, err
		}
	}
	return report, nil
}

// unreadable reports a task whose description cannot be used: check A1 fails
// and the others do not run.
func (a *Acceptor) unreadable(name string, number int, cause error) (Report, error) {
	result := newResult(CheckShape)
	result.fail(records.RoleAuthor, records.TargetTask, "%v", cause)
	report := withSkipped(name, result)
	return report, a.record(name, number, report)
}

func withSkipped(name string, shape *Result) Report {
	report := Report{Task: name, Results: []Result{*shape}}
	for _, id := range CheckOrder()[1:] {
		report.Results = append(report.Results, Result{ID: id, Name: checkNames[id], Skipped: true})
	}
	return report
}

type checkFunc func(r *round, ctx context.Context) (*Result, error)

func checks() map[string]checkFunc {
	return map[string]checkFunc{
		CheckShape: (*round).shape, CheckFixture: (*round).fixture, CheckReference: (*round).reference,
		CheckBase: (*round).base, CheckDiagonal: (*round).diagonal, CheckRefactors: (*round).refactorsCheck,
		CheckRoom: (*round).room, CheckLeak: (*round).leak, CheckCollection: (*round).collection,
	}
}

func (r *round) runChecks(ctx context.Context) (Report, error) {
	checks := checks()
	report := Report{Task: r.name}
	for _, id := range CheckOrder() {
		r.log.Reset()
		result, err := checks[id](r, ctx)
		if err != nil {
			return Report{}, fmt.Errorf("check %s %s: %w", id, checkNames[id], err)
		}
		result.Output += r.log.String()
		report.Results = append(report.Results, *result)
		if id == CheckShape && !result.Passed {
			report = withSkipped(r.name, result)
			break
		}
	}
	return report, r.a.record(r.name, r.number, report)
}

// record appends the output of each check to the construction records.
func (a *Acceptor) record(name string, number int, report Report) error {
	if a.Records == nil {
		return nil
	}
	for _, result := range report.Results {
		if result.Skipped {
			continue
		}
		entry := records.Check{
			Domain: name, Round: number, ID: result.ID, Name: result.Name,
			Passed: result.Passed, Agreed: result.Agreed, Findings: result.Findings, Output: result.Output,
		}
		if err := a.Records.Append(records.Entry{Check: &entry}); err != nil {
			return err
		}
	}
	return nil
}
