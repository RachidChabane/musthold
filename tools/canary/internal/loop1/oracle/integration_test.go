package oracle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const fixtureFromRepo = "demo/loop/fixture/boxoffice"

// TestOracleInContainers runs the real command of PREREG §4.1 through the
// study's runner, on the boxoffice fixture with a hand-written oracle.
func TestOracleInContainers(t *testing.T) {
	world := dockertest.New(t)
	runDir := filepath.Join(world.Work, "run")
	if err := copyTree(filepath.Join(world.Repo, fixtureFromRepo), filepath.Join(runDir, study.ProjectDir), skipNothing); err != nil {
		t.Fatal(err)
	}
	oracleDir := filepath.Join(world.Work, "truth", oracleDirName)
	write(t, oracleDir, "pytest.ini", "[pytest]\npythonpath = ..\n")
	write(t, oracleDir, "test_box_1.py", "from boxoffice.prices import ticket_price\n\n\ndef test_adult():\n    assert ticket_price(age=30) == 12\n")
	truth := Truth{Oracle: oracleDir}
	judge := NewJudge(world.Runner, filepath.Join(world.Work, "jobs"))

	t.Run("the fixture passes the oracle [verifies SP-2560~2]", func(t *testing.T) {
		verdict, err := judge.Oracle(context.Background(), runDir, truth)
		if err != nil {
			t.Fatal(err)
		}
		if verdict.Defective || len(verdict.Tests) != 1 || verdict.Tests[0].Criterion != "BOX-1" {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a broken operation fails the oracle in three runs [verifies SP-2560~2]", func(t *testing.T) {
		source := filepath.Join(runDir, study.ProjectDir, "boxoffice", "prices.py")
		if err := os.WriteFile(source, []byte("def ticket_price(age):\n    return 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		verdict, err := judge.Oracle(context.Background(), runDir, truth)
		if err != nil {
			t.Fatal(err)
		}
		if !verdict.Defective || verdict.Tests[0].Failed != [study.OracleRuns]bool{true, true, true} {
			t.Fatalf("%+v", verdict)
		}
	})
}
