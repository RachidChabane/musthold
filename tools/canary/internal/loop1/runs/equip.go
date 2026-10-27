package runs

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
)

// Equipped are a task's start commits in one arm (PREREG §3.5).
type Equipped struct {
	// BaseCommit is the hash of the commit of the copied fixture.
	BaseCommit string
	// Commit is the hash of the equipped commit, and Tree the digest of its
	// normalised tree.
	Commit string
	Tree   string
}

// Equipper prepares a task to its equipped state, every step a command of the
// study's image.
type Equipper struct {
	// Options lay out the run directory as the study's runs do.
	Options app.Options
	// Processes runs each step; BoxProcesses runs them through the study's box.
	Processes canary.Processes
	Files     canary.Files
	Tree      Tree
}

// Equip copies the task's fixture into the project of the run directory given,
// commits it, equips it for the arm as a run does, and returns its base commit
// and the digest of its equipped tree. The resolve verb records these values in
// the inputs freeze; each run compares its own with them.
func (e Equipper) Equip(ctx context.Context, dir string, run config.Run) (Equipped, error) {
	l := app.LayoutOf(e.Options, dir)
	preparer := app.Preparer{Processes: e.Processes, Files: e.Files, Env: app.ContainerBase(l)}
	prepared, err := preparer.Prepare(ctx, l, run, app.LeadingDirs(l))
	if err != nil {
		return Equipped{}, err
	}
	return equippedFacts(ctx, preparer, &prepared, e.Tree)
}

// equippedFacts reads the commits of a prepared project and the digest of its
// equipped tree.
func equippedFacts(ctx context.Context, preparer app.Preparer, prepared *app.Prepared, tree Tree) (Equipped, error) {
	var answers []string
	for _, argv := range [][]string{plan.BaseCommitArgv(), plan.HeadCommitArgv(), plan.TrackedFilesArgv()} {
		outcome, err := preparer.Run(ctx, prepared.Layout, prepared.Environment, plan.Step{Argv: argv, Purpose: "read the equipped commit"})
		prepared.Steps = append(prepared.Steps, outcome)
		if err != nil {
			return Equipped{}, err
		}
		if outcome.ExitCode != 0 {
			return Equipped{}, fmt.Errorf("%w: %s exited %d: %s", app.ErrPrepare, strings.Join(argv, " "), outcome.ExitCode, strings.TrimSpace(outcome.Stderr))
		}
		answers = append(answers, outcome.Stdout)
	}
	project := prepared.Layout.Project()
	files := map[string][]byte{}
	for _, path := range TrackedFiles(answers[2]) {
		content, err := tree.Read(filepath.Join(project, filepath.FromSlash(path)))
		if err != nil {
			return Equipped{}, err
		}
		files[path] = content
	}
	return Equipped{
		BaseCommit: strings.TrimSpace(answers[0]),
		Commit:     strings.TrimSpace(answers[1]),
		Tree:       EquippedDigest(files, prepared.Layout.Run),
	}, nil
}

// BoxProcesses runs each command in a fresh offline container of the study's
// image through the box port, with the run directory as its only mount besides
// the product.
type BoxProcesses struct {
	Runner box.Runner
	// RunDir is the run directory, mounted read-write.
	RunDir string
}

// Run implements canary.Processes.
func (b BoxProcesses) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	result, err := b.Runner.Run(ctx, box.Command{
		Argv: spec.Argv, Env: spec.Env, Dir: spec.Dir, Limit: spec.Timeout,
		Mounts: []box.Mount{{Path: b.RunDir}},
	})
	if err != nil {
		return canary.Output{}, err
	}
	return canary.Output{Stdout: string(result.Stdout), Stderr: string(result.Stderr), ExitCode: result.ExitCode}, nil
}
