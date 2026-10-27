package oracle

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/phase"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const demoTask = `{
  "name": "demo",
  "fixture": "fixture",
  "spec": "specs/s.md",
  "source": "pkg/mod.py",
  "criterion": "DEMO-2",
  "prompt": "Implement requirement DEMO-2.",
  "operations": {"op": {"python": "pkg.mod:op"}},
  "interface": {"module": "pkg.mod", "operation": "op", "signature": "op(*, n: int) -> int"},
  "truth": {
    "reference": "truth/demo/reference",
    "violations": "truth/demo/violations",
    "refactors": "truth/demo/refactors",
    "oracle": "truth/demo/oracle"
  }
}`

const (
	equippedCommit = "eq"
	runName        = "demo-1-1"
)

// workPhase judges the scenario's work directory, every run that has a directory
// at the call in the order; open refuses as an open run phase does.
type workPhase struct {
	work string
	open bool
}

func (p workPhase) Counted() ([]phase.Attempt, error) {
	if p.open {
		return nil, phase.ErrOpen
	}
	entries, _ := os.ReadDir(filepath.Join(p.work, study.RunsDir))
	var order []string
	for _, entry := range entries {
		run := entry.Name()[:strings.LastIndex(entry.Name(), "-")]
		if !slices.Contains(order, run) {
			order = append(order, run)
		}
	}
	return phase.Reader{Layout: study.Layout{Root: p.work}, Order: order, Now: time.Now}.Counted()
}

// scenario is a repository and a work directory with one counted attempt of a
// task with an oracle of two criteria and three violations.
type scenario struct {
	t      *testing.T
	repo   string
	work   string
	runDir string
	box    *fakeBox
	verb   *Verb
}

func newScenario(t *testing.T) *scenario {
	t.Helper()
	root := t.TempDir()
	s := &scenario{t: t, repo: filepath.Join(root, "repo"), work: filepath.Join(root, "work")}
	s.runDir = study.Layout{Root: s.work}.RunDir("demo-1", 1)
	write(t, s.repo, "experiments/loop-1/tasks/demo/task.json", demoTask)

	write(t, s.repo, "fixture/pytest.ini", "[pytest]\ntestpaths = tests\n")
	write(t, s.repo, "fixture/pkg/mod.py", "FIXTURE\n")
	write(t, s.repo, "fixture/specs/s.md", "spec\n")
	write(t, s.repo, "fixture/tests/test_base.py", "def test_base():\n")

	write(t, s.repo, "truth/demo/reference/mod.py", "REF\n")
	write(t, s.repo, "truth/demo/violations/DEMO-1/1/mod.py", "V11\n")
	write(t, s.repo, "truth/demo/violations/DEMO-1/2/mod.py", "V12\n")
	write(t, s.repo, "truth/demo/violations/DEMO-2/1/mod.py", "V21\n")
	write(t, s.repo, "truth/demo/refactors/1/mod.py", "REFACTOR\n")
	write(t, s.repo, "truth/demo/oracle/pytest.ini", "[pytest]\n")
	write(t, s.repo, "truth/demo/oracle/test_demo_1.py", "def test_a():\ndef test_b():  # flaky FLAKY\n")
	write(t, s.repo, "truth/demo/oracle/test_demo_2.py", "def test_c():  # kills BAD2\n")

	write(t, s.runDir, "project/pkg/mod.py", "SESSION\n")
	write(t, s.runDir, "project/pytest.ini", "[pytest]\n")
	write(t, s.runDir, "project/tests/test_new.py",
		"def test_n1():  # kills V11\ndef test_n2():  # flaky V12\ndef test_n3():  # kills V21\n")
	write(t, s.runDir, "project/.musthold/config.yaml", "x\n")
	write(t, s.runDir, "project/test-results/report.xml", "x\n")
	write(t, s.runDir, "project/.git/HEAD", "ref\n")
	write(t, s.runDir, "project/oracle/test_stale.py", "def test_stale():\n")
	write(t, s.runDir, "records/stops/1/tree.txt", "tree1\n")
	write(t, s.runDir, "records/stops/2/tree.txt", "tree2\n")
	s.writeRun(study.OutcomeCompleted)

	equipped := "def test_e1():  # kills V11\ndef test_e2():  # kills V12\ndef test_e3():  # contradicts\n"
	s.box = &fakeBox{t: t, pytest: s.behave, archives: map[string]map[string]string{
		equippedCommit: {"pytest.ini": "[pytest]\ntestpaths = tests\npythonpath = .\n", "tests/test_eq.py": equipped},
		"tree1":        {"tests/test_eq.py": equipped},
		"tree2":        {"tests/test_eq.py": equipped, "tests/test_s.py": "def test_s():  # kills V21\n"},
	}}
	s.verb = NewVerb(s.repo, study.Layout{Root: s.work}, s.box, workPhase{work: s.work}, io.Discard)
	return s
}

func (s *scenario) writeRun(outcome study.Outcome) {
	s.t.Helper()
	record := study.Run{EquippedCommit: equippedCommit, Attempt: 1, Outcome: outcome}
	record.Task, record.Arm, record.Repetition = "demo", "control", 1
	data, err := json.Marshal(record)
	if err != nil {
		s.t.Fatal(err)
	}
	write(s.t, s.runDir, "records/run.json", string(data))
}

// behave is the deterministic pytest: every `def test_x():` line of a test file
// is a test. A marker `# kills M` makes it fail when the source holds M; `# flaky
// M` fails it in the first run only; `# contradicts` fails it always.
func (s *scenario) behave(call pytestCall) fakeOutcome {
	prefix := testsDirName + "/"
	if call.Kind == kindOracle {
		prefix = oracleDirName + "/"
	}
	source := call.Tree["pkg/mod.py"]
	var outcome fakeOutcome
	for name, content := range call.Tree {
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ".py") {
			continue
		}
		class := strings.ReplaceAll(strings.TrimSuffix(name, ".py"), "/", ".")
		for _, line := range strings.Split(content, "\n") {
			test, marker, found := strings.Cut(line, "# ")
			if !strings.HasPrefix(test, "def test_") {
				continue
			}
			name := strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(test, "def ")), "():")
			c := fakeCase{Classname: class, Name: name}
			kind, arg, _ := strings.Cut(marker, " ")
			switch {
			case !found:
			case kind == "kills":
				c.Failure = strings.Contains(source, arg)
			case kind == "flaky":
				c.Error = strings.Contains(source, arg) && call.Run == 1
			case kind == "contradicts":
				c.Failure = true
			}
			outcome.Cases = append(outcome.Cases, c)
		}
	}
	for _, c := range outcome.Cases {
		if c.Failure || c.Error {
			outcome.Exit = exitSomeFailed
		}
	}
	return outcome
}

func (s *scenario) readOracle() study.Oracle {
	s.t.Helper()
	var oracle study.Oracle
	s.readJSON(OraclePath(s.runDir), &oracle)
	return oracle
}

func (s *scenario) readHeldOut() study.HeldOut {
	s.t.Helper()
	var heldOut study.HeldOut
	s.readJSON(HeldOutPath(s.runDir), &heldOut)
	return heldOut
}

func (s *scenario) readJSON(path string, into any) {
	s.t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		s.t.Fatal(err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		s.t.Fatal(err)
	}
}

// listing maps every file under dir to its content.
func listing(t *testing.T, dir string) map[string]string { return readTree(t, dir) }
