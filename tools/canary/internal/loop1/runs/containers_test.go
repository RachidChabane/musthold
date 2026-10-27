package runs_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs/runstest"
)

// The container tests run on the study's engine, started by the orchestrator,
// with the product built and the image named.
const (
	containersVariable = "MUSTHOLD_LOOP1_CONTAINERS"
	imageVariable      = "LOOP1_IMAGE"
)

// engineSetup reads what the container tests need, or skips each check named.
func engineSetup(t *testing.T, checks ...string) (runs.Setup, bool) {
	t.Helper()
	if os.Getenv(containersVariable) != "1" {
		for _, check := range checks {
			t.Run(check, func(t *testing.T) {
				t.Skipf("set %s=1, with the study's engine running, to run the container tests", containersVariable)
			})
		}
		return runs.Setup{}, false
	}
	flags, err := runs.ParseFlags("containers", []string{"--repo", runstest.RepoRoot(t), "--image", os.Getenv(imageVariable)}, os.Environ(), os.Stderr)
	if err != nil {
		t.Fatal(err)
	}
	setup, err := flags.Resolve(adapters.OSFiles{}, true)
	if err != nil {
		t.Fatal(err)
	}
	if setup.Options.Image == "" {
		t.Fatalf("set %s to the study's image", imageVariable)
	}
	return setup, true
}

func TestContainers(t *testing.T) {
	const (
		equipCheck = "both arms equip in the image's step containers to trees that differ in shallnot.yaml alone, and the digest is the same in two run directories [verifies SP-2511~1] [verifies SP-2517~1]"
		pinCheck   = "the image's CLI reports the pin [verifies SP-2513~1]"
	)
	setup, ready := engineSetup(t, equipCheck, pinCheck)
	if !ready {
		return
	}
	appRunner := runs.AppRunner(os.Environ(), os.Stdout)
	work := runstest.TempDir(t)
	setup.Options.Work = work

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
