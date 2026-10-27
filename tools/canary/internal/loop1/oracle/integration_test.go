package oracle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	containersGate  = "MUSTHOLD_LOOP1_CONTAINERS"
	fixtureFromRepo = "demo/loop/fixture/boxoffice"
	repoFromPackage = "../../../../.."
)

// TestOracleInContainers runs the real command of PREREG §4.1 through the
// runner that Wire builds, on the boxoffice fixture with a hand-written oracle.
// It needs the container engine and the work directory, so it runs only when
// MUSTHOLD_LOOP1_CONTAINERS=1.
func TestOracleInContainers(t *testing.T) {
	if os.Getenv(containersGate) != "1" {
		t.Skipf("%s is not 1", containersGate)
	}
	repo, err := filepath.Abs(repoFromPackage)
	if err != nil {
		t.Fatal(err)
	}
	work := defaultWork()
	wiring, err := Wire(Options{Repo: repo, Work: work})
	if err != nil {
		t.Skipf("no box wired: %v", err)
	}
	scratch, err := os.MkdirTemp(work, "oracle-integration-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(scratch)

	runDir := filepath.Join(scratch, "run")
	if err := copyTree(filepath.Join(repo, fixtureFromRepo), filepath.Join(runDir, ProjectDir), skipNothing); err != nil {
		t.Fatal(err)
	}
	oracleDir := filepath.Join(scratch, "truth", oracleDirName)
	write(t, oracleDir, "pytest.ini", "[pytest]\npythonpath = ..\n")
	write(t, oracleDir, "test_box_1.py", "from boxoffice.prices import ticket_price\n\n\ndef test_adult():\n    assert ticket_price(age=30) == 12\n")
	truth := Truth{Oracle: oracleDir}
	judge := NewJudge(wiring.Runner, filepath.Join(scratch, "jobs"))

	t.Run("the fixture passes the oracle [verifies SP-2560~1]", func(t *testing.T) {
		verdict, err := judge.Oracle(context.Background(), runDir, truth)
		if err != nil {
			t.Fatal(err)
		}
		if verdict.Defective || len(verdict.Tests) != 1 || verdict.Tests[0].Criterion != "BOX-1" {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a broken operation fails the oracle in three runs [verifies SP-2560~1]", func(t *testing.T) {
		source := filepath.Join(runDir, ProjectDir, "boxoffice", "prices.py")
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
