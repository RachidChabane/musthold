// Package app is the use cases of the agent-loop runner: plan a run, prepare its
// project, conduct its session, collect what it left and render the transcript.
// It reaches processes, files, the clock and the credential only through the
// ports of the canary's application layer, whose adapters it shares.
package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
)

// stepTimeout bounds one preparation command; the gate runs the project's tests.
const stepTimeout = 5 * time.Minute

// ErrPrepare wraps the failure of a preparation step.
var ErrPrepare = errors.New("the project could not be prepared")

// Preparer equips the project of a run the documented way.
type Preparer struct {
	Processes canary.Processes
	Files     canary.Files
	// Env is the environment the steps inherit.
	Env []string
}

// Prepared is a run whose project is equipped and committed.
type Prepared struct {
	Layout plan.Layout
	// Environment is the environment of the steps and of the session's tools,
	// without the credential.
	Environment []string
	// Steps are the outcomes of the preparation, in order.
	Steps []record.Step
}

// Prepare copies the task's fixture into the run, commits it, equips it for the
// arm, binds the task's operations, and commits the result.
func (p Preparer) Prepare(ctx context.Context, l plan.Layout, run config.Run, leading []string) (Prepared, error) {
	prepared := Prepared{Layout: l, Environment: plan.Environment(p.Env, plan.Variables(l, leading))}
	for _, dir := range l.Dirs() {
		if err := p.Files.MkdirAll(dir); err != nil {
			return prepared, err
		}
	}
	if err := p.Files.CopyTree(filepath.Join(l.Repo, run.Task.Fixture), l.Project()); err != nil {
		return prepared, fmt.Errorf("%w: copy the fixture: %v", ErrPrepare, err)
	}
	if err := p.steps(ctx, &prepared, plan.BaseSteps()); err != nil {
		return prepared, err
	}
	if err := p.steps(ctx, &prepared, plan.EquipSteps(l, run.Arm)); err != nil {
		return prepared, err
	}
	if run.Arm.WithModules() && len(run.Task.Operations) > 0 {
		if err := p.bindOperations(l, run.Task.Operations); err != nil {
			return prepared, err
		}
	}
	if kinds, explicit := run.Arm.Require(); explicit {
		if err := p.setRequire(l, kinds); err != nil {
			return prepared, err
		}
	}
	return prepared, p.steps(ctx, &prepared, plan.CommitEquipSteps())
}

// setRequire writes the arm's `proofs.require` into Shallnot's configuration,
// once every module is enabled.
func (p Preparer) setRequire(l plan.Layout, kinds []string) error {
	path := filepath.Join(l.Project(), plan.ShallnotConfig)
	current, err := p.Files.Read(path)
	if err != nil {
		return err
	}
	set, err := plan.SetRequire(current, kinds)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrepare, err)
	}
	return p.Files.Write(path, set, false)
}

func (p Preparer) bindOperations(l plan.Layout, operations map[string]map[string]string) error {
	path := filepath.Join(l.Project(), plan.MustholdConfig)
	current, err := p.Files.Read(path)
	if err != nil {
		return err
	}
	bound, err := plan.BindOperations(current, operations)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrepare, err)
	}
	return p.Files.Write(path, bound, false)
}

func (p Preparer) steps(ctx context.Context, prepared *Prepared, steps []plan.Step) error {
	for _, step := range steps {
		outcome, err := p.Run(ctx, prepared.Layout, prepared.Environment, step)
		prepared.Steps = append(prepared.Steps, outcome)
		if err != nil {
			return err
		}
		if outcome.ExitCode != 0 {
			return fmt.Errorf("%w: %s exited %d: %s", ErrPrepare, strings.Join(step.Argv, " "), outcome.ExitCode, strings.TrimSpace(outcome.Stderr))
		}
	}
	return nil
}

// Run runs one step in the project and returns its record. A git step carries
// the fixed identity of the run's commits.
func (p Preparer) Run(ctx context.Context, l plan.Layout, env []string, step plan.Step) (record.Step, error) {
	if step.Argv[0] == plan.GitBinary {
		env = append(append([]string{}, env...), plan.GitEnvironment()...)
	}
	output, err := p.Processes.Run(ctx, canary.CommandSpec{Argv: step.Argv, Dir: l.Project(), Env: env, Timeout: stepTimeout})
	outcome := record.Step{Argv: step.Argv, Purpose: step.Purpose, ExitCode: output.ExitCode, Stdout: output.Stdout, Stderr: output.Stderr}
	if err != nil {
		return outcome, fmt.Errorf("%w: %s: %v", ErrPrepare, strings.Join(step.Argv, " "), err)
	}
	return outcome, nil
}

// WriteSteps writes the outcomes of the preparation as JSON lines.
func WriteSteps(files canary.Files, path string, steps []record.Step) error {
	var lines []byte
	for _, step := range steps {
		encoded, err := json.Marshal(step)
		if err != nil {
			return err
		}
		lines = append(append(lines, encoded...), '\n')
	}
	return files.Write(path, lines, false)
}
