package runs

import (
	"context"
	"path/filepath"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
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
