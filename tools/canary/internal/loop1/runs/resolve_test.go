package runs_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs/runstest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestResolvePreparer(t *testing.T) {
	w := runstest.NewWorld(t)
	boxes := &runstest.BoxRunner{Engine: w.Engine}
	preparer := runs.ResolvePreparer{Options: w.Options, Root: w.Work, Box: boxes, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	expect, _ := w.Inputs.Task(w.Config.Tasks[0].Name)

	t.Run("resolve prepares each arm through the box in a fresh directory of the work directory, to the values each run compares with [verifies SP-2517~1]", func(t *testing.T) {
		for _, arm := range w.Config.Arms {
			prepared, err := preparer.Prepare(context.Background(), w.Config.Tasks[0], arm)
			if err != nil {
				t.Fatal(err)
			}
			if prepared.BaseCommit != expect.BaseCommit || prepared.EquippedTree != expect.EquippedTree[string(arm)] || prepared.SessionSettings == "" || len(prepared.Files) == 0 {
				t.Errorf("%s: %+v, frozen %+v", arm, prepared, expect)
			}
		}
		resolveRoot := filepath.Join(w.Work, runs.ResolveDir) + string(filepath.Separator)
		dirs := map[string]bool{}
		for _, mounts := range boxes.Mounts {
			if len(mounts) != 1 || mounts[0].ReadOnly || !strings.HasPrefix(mounts[0].Path, resolveRoot) {
				t.Fatalf("a box command mounts %v", mounts)
			}
			dirs[mounts[0].Path] = true
		}
		if len(dirs) != len(w.Config.Arms) || slices.ContainsFunc(boxes.Mounts, func(m []box.Mount) bool { return m == nil }) {
			t.Errorf("the arms were prepared in %v", dirs)
		}
	})
}

func TestControlEquipper(t *testing.T) {
	w := runstest.NewWorld(t)
	boxes := &runstest.BoxRunner{Engine: w.Engine}
	equipper := runs.ControlEquipper{Options: w.Options, Box: boxes, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
	configured := w.Config.Tasks[0]
	tree := filepath.Join(w.Work, "trees", "equipped")
	if err := (adapters.OSFiles{}).CopyTree(filepath.Join(w.Repo, configured.Fixture), tree); err != nil {
		t.Fatal(err)
	}
	task := study.Task{Name: configured.Name, Prompt: configured.Prompt, Operations: configured.Operations}

	t.Run("a tree of the acceptance checks is equipped beside it as a run of the control arm equips it, every step in the box with its run directory alone mounted [verifies SP-2641~1]", func(t *testing.T) {
		project, err := equipper.EquipControl(context.Background(), tree, task)
		if err != nil {
			t.Fatal(err)
		}
		runDir := filepath.Dir(project)
		if filepath.Dir(runDir) != filepath.Dir(tree) || filepath.Base(project) != study.ProjectDir || !strings.HasPrefix(filepath.Base(runDir), filepath.Base(tree)+runs.ControlRunSuffix) {
			t.Fatalf("the project is %s", project)
		}
		settings := runstest.Read(t, filepath.Join(project, "shallnot.yaml"))
		if !strings.Contains(settings, "require: []") {
			t.Errorf("the control arm's gate requires proofs:\n%s", settings)
		}
		for _, mounts := range boxes.Mounts {
			if len(mounts) != 1 || mounts[0].Path != runDir || mounts[0].ReadOnly {
				t.Fatalf("a box command mounts %v", mounts)
			}
		}
	})
}
