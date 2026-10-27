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
