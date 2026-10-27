package runs_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// engineSetup reads what the container tests need, the image that of the
// product's manifest, or skips each check named.
func engineSetup(t *testing.T, checks ...string) (runs.Setup, bool) {
	t.Helper()
	if !dockertest.Enabled() {
		for _, check := range checks {
			t.Run(check, func(t *testing.T) { dockertest.Skip(t) })
		}
		return runs.Setup{}, false
	}
	flags, err := runs.ParseFlags("containers", []string{"--repo", dockertest.RepoRoot(t), "--inputs", filepath.Join(t.TempDir(), study.InputsFile)}, os.Environ(), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := flags.Resolve(adapters.OSFiles{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Options.Image == "" {
		t.Fatalf("the product has no manifest: run %s/script/build", study.StudyDir)
	}
	return setup, true
}

func TestContainers(t *testing.T) {
	const (
		equipCheck = "both arms equip in the image's step containers to trees that differ in shallnot.yaml alone, and the digest is the same in two run directories [verifies SP-2511~1] [verifies SP-2517~1]"
		pinCheck   = "the image's CLI reports the pin [verifies SP-2513~1]"
		boxCheck   = "accept's control arm is equipped through the study's box as a run equips it [verifies SP-2641~1]"
	)
	setup, ready := engineSetup(t, equipCheck, pinCheck, boxCheck)
	if !ready {
		return
	}
	appRunner := runs.AppRunner(os.Environ(), os.Stdout)
	work := dockertest.Scratch(t, "runs-")
	setup.Options.Work = work

	t.Run(boxCheck, func(t *testing.T) {
		runner, err := docker.StudyRunner(setup.Options.RepoRoot, setup.Options.Product, setup.Options.Image)
		if err != nil {
			t.Fatal(err)
		}
		tree := filepath.Join(work, "trees", "equipped")
		if err := (adapters.OSFiles{}).CopyTree(filepath.Join(setup.Options.RepoRoot, setup.Config.Tasks[0].Fixture), tree); err != nil {
			t.Fatal(err)
		}
		task := study.Task{Name: setup.Config.Tasks[0].Name, Prompt: setup.Config.Tasks[0].Prompt, Operations: setup.Config.Tasks[0].Operations}
		equipper := runs.ControlEquipper{Options: setup.Options, Box: runner, Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
		project, err := equipper.EquipControl(context.Background(), tree, task)
		if err != nil {
			t.Fatal(err)
		}
		settings, err := os.ReadFile(filepath.Join(project, "shallnot.yaml"))
		if err != nil || !strings.Contains(string(settings), "require: []") {
			t.Errorf("the control arm's gate: %s, %v", settings, err)
		}
	})

	t.Run(pinCheck, func(t *testing.T) {
		attempt, err := appRunner.Open(setup.Options, work)
		if err != nil {
			t.Fatal(err)
		}
		defer attempt.Close(context.Background())
		if _, err := appRunner.Versions(context.Background(), attempt, setup.Options); err != nil {
			t.Fatal(err)
		}
	})

	t.Run(equipCheck, func(t *testing.T) {
		trees := map[string]string{}
		for _, case_ := range []struct {
			arm  config.Arm
			name string
		}{{config.ArmControl, "control-a"}, {config.ArmTreatment, "treatment-a"}, {config.ArmTreatment, "treatment-b"}} {
			dir := filepath.Join(work, case_.name)
			attempt, err := appRunner.Open(setup.Options, dir)
			if err != nil {
				t.Fatal(err)
			}
			equipper := runs.Equipper{Options: setup.Options, Processes: attempt.Steps(), Files: adapters.OSFiles{}, Tree: runs.OSTree{}}
			equipped, err := equipper.Equip(context.Background(), dir, config.Run{Task: setup.Config.Tasks[0], Arm: case_.arm, Repetition: 1})
			attempt.Close(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			trees[case_.name] = equipped.Tree
		}
		if trees["treatment-a"] != trees["treatment-b"] || trees["control-a"] == trees["treatment-a"] {
			t.Errorf("trees %v", trees)
		}
		control, _ := runs.OSTree{}.Files(filepath.Join(work, "control-a", plan.ProjectDir), []string{".git"})
		treatment, _ := runs.OSTree{}.Files(filepath.Join(work, "treatment-a", plan.ProjectDir), []string{".git"})
		if len(control) != len(treatment) {
			t.Errorf("the arms hold %d and %d files", len(control), len(treatment))
		}
	})
}
