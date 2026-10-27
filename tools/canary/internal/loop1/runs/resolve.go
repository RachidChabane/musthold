package runs

import (
	"context"
	"path/filepath"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// ResolveDir is the directory of the work directory the resolve verb prepares the
// tasks in, one fresh directory per task and arm.
const ResolveDir = "resolve"

// ResolvePreparer prepares a task for an arm as a run does, each step a command
// of the study's box, for the resolve verb.
type ResolvePreparer struct {
	// Options lay out the run directories as the study's runs do; Root is the
	// work directory, the engine's only read-write mount.
	Options app.Options
	Root    string
	Box     box.Runner
	Files   canary.Files
	Tree    Tree
}

var _ freeze.Preparer = ResolvePreparer{}

// ControlEquipper equips a tree of the acceptance checks as the control arm's
// project (PREREG §3.4, A2), as a run equips it, every step a command of the
// study's box: the tree is the fixture of a run directory made beside it.
type ControlEquipper struct {
	// Options lay out the run directories as the study's runs do.
	Options app.Options
	Box     box.Runner
	Files   canary.Files
	Tree    Tree
}

// ControlRunSuffix names the run directory made beside a tree.
const ControlRunSuffix = "-control-"

// EquipControl implements the accept package's Equipper: it returns the
// equipped project.
func (c ControlEquipper) EquipControl(ctx context.Context, dir string, task study.Task) (string, error) {
	runDir, err := c.Files.MakeDir(filepath.Dir(dir), filepath.Base(dir)+ControlRunSuffix)
	if err != nil {
		return "", err
	}
	options := c.Options
	options.RepoRoot = filepath.Dir(dir)
	run := config.Run{
		Task: config.Task{Name: task.Name, Fixture: filepath.Base(dir), Prompt: task.Prompt, Operations: task.Operations},
		Arm:  config.ArmControl, Repetition: 1,
	}
	equipper := Equipper{Options: options, Processes: BoxProcesses{Runner: c.Box, RunDir: runDir}, Files: c.Files, Tree: c.Tree}
	if _, err := equipper.Equip(ctx, runDir, run); err != nil {
		return "", err
	}
	return app.LayoutOf(options, runDir).Project(), nil
}

// Prepare implements freeze.Preparer.
func (r ResolvePreparer) Prepare(ctx context.Context, task config.Task, arm config.Arm) (freeze.Prepared, error) {
	dir, err := r.Files.MakeDir(filepath.Join(r.Root, ResolveDir), task.Name+"-"+string(arm)+"-")
	if err != nil {
		return freeze.Prepared{}, err
	}
	equipper := Equipper{Options: r.Options, Processes: BoxProcesses{Runner: r.Box, RunDir: dir}, Files: r.Files, Tree: r.Tree}
	equipped, err := equipper.Equip(ctx, dir, config.Run{Task: task, Arm: arm, Repetition: 1})
	if err != nil {
		return freeze.Prepared{}, err
	}
	return freeze.Prepared{BaseCommit: equipped.BaseCommit, EquippedTree: equipped.Tree, Files: equipped.Files, SessionSettings: equipped.SessionSettings}, nil
}
