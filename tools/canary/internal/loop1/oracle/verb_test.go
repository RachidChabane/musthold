package oracle

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func TestOracleRun(t *testing.T) {
	t.Run("the oracle tree and its command are those of PREREG 4.1 [verifies SP-2560~1]", func(t *testing.T) {
		s := newScenario(t)
		var trees []map[string]string
		s.box.pytest = func(call pytestCall) fakeOutcome {
			if call.Kind == kindOracle {
				trees = append(trees, call.Tree)
			}
			return s.behave(call)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(trees) != study.OracleRuns {
			t.Fatalf("%d oracle runs", len(trees))
		}
		tree := trees[0]
		for _, kept := range []string{"pkg/mod.py", "pytest.ini", "oracle/pytest.ini", "oracle/test_demo_1.py", "oracle/test_demo_2.py"} {
			if _, ok := tree[kept]; !ok {
				t.Errorf("oracle tree lacks %s", kept)
			}
		}
		for name := range tree {
			for _, left := range []string{"tests/", ".musthold/", "test-results/", ".git/", "oracle/test_stale.py"} {
				if strings.HasPrefix(name, left) {
					t.Errorf("oracle tree holds %s", name)
				}
			}
		}
		var oracleCalls int
		for _, command := range s.box.calls("python") {
			if !strings.HasPrefix(filepath.Base(strings.TrimPrefix(command.Argv[len(command.Argv)-2], "--junitxml=")), "oracle-") {
				continue
			}
			oracleCalls++
			argv := strings.Join(command.Argv, " ")
			want := "python -m pytest -c oracle/pytest.ini --rootdir oracle --noconftest -p no:cacheprovider -q --junitxml="
			if !strings.HasPrefix(argv, want) || !strings.HasSuffix(argv, " oracle") {
				t.Errorf("command %q", argv)
			}
			if command.Limit != 300*time.Second {
				t.Errorf("limit %v", command.Limit)
			}
		}
		if oracleCalls != study.OracleRuns {
			t.Errorf("%d oracle commands", oracleCalls)
		}
	})

	t.Run("a defective and a flaky test reach oracle.json [verifies SP-2560~1, SP-2563~1]", func(t *testing.T) {
		s := newScenario(t)
		write(t, s.runDir, "project/pkg/mod.py", "BAD2 FLAKY\n")
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		oracle := s.readOracle()
		if !oracle.Defective || !slices.Equal(oracle.Violated, []string{"DEMO-2"}) {
			t.Fatalf("%+v", oracle)
		}
		if len(oracle.Flaky) != 1 || !strings.HasSuffix(oracle.Flaky[0], "test_b") {
			t.Fatalf("flaky %v", oracle.Flaky)
		}
		if len(oracle.Tests) != 3 {
			t.Fatalf("tests %+v", oracle.Tests)
		}
	})

	t.Run("a run at its limit fails every test [verifies SP-2562~1]", func(t *testing.T) {
		s := newScenario(t)
		s.box.pytest = func(call pytestCall) fakeOutcome {
			if call.Kind == kindOracle && call.Run >= 2 {
				return fakeOutcome{TimedOut: true}
			}
			return s.behave(call)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		oracle := s.readOracle()
		if !oracle.Defective || oracle.TimedOut != [3]bool{false, true, true} || !slices.Equal(oracle.Violated, []string{"DEMO-1", "DEMO-2"}) {
			t.Fatalf("%+v", oracle)
		}
	})

	t.Run("a collection error fails every test [verifies SP-2561~1]", func(t *testing.T) {
		s := newScenario(t)
		s.box.pytest = func(call pytestCall) fakeOutcome {
			if call.Kind == kindOracle {
				return fakeOutcome{Exit: 2, NoReport: true}
			}
			return s.behave(call)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		oracle := s.readOracle()
		if !oracle.Defective || !slices.Equal(oracle.Violated, []string{"DEMO-1", "DEMO-2"}) || len(oracle.Tests) != 2 {
			t.Fatalf("%+v", oracle)
		}
	})

	t.Run("a container killed for memory is an error, not a verdict [verifies SP-2560~1]", func(t *testing.T) {
		s := newScenario(t)
		s.box.pytest = func(pytestCall) fakeOutcome { return fakeOutcome{OOM: true} }
		err := s.verb.Execute(context.Background())
		if !errors.Is(err, ErrOOMKilled) {
			t.Fatalf("%v", err)
		}
		if _, statErr := os.Stat(OraclePath(s.runDir)); !os.IsNotExist(statErr) {
			t.Fatal("oracle.json written")
		}
	})
}

func TestHeldOut(t *testing.T) {
	t.Run("the equipped, final and stop directories are measured [verifies SP-2564~1, SP-2565~1, SP-2566~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		heldOut := s.readHeldOut()
		labels := slices.Sorted(maps.Keys(heldOut))
		if !slices.Equal(labels, []string{study.DirEquipped, study.DirFinal, study.StopDir(1), study.StopDir(2)}) {
			t.Fatalf("labels %v", labels)
		}
		equipped := heldOut[study.DirEquipped]
		if !slices.Equal(equipped.Valid, []string{"tests.test_eq::test_e1", "tests.test_eq::test_e2"}) ||
			!slices.Equal(equipped.Contradicting, []string{"tests.test_eq::test_e3"}) {
			t.Errorf("equipped %+v", equipped)
		}
		wantEquipped := map[string]bool{"DEMO-1/1": true, "DEMO-1/2": true, "DEMO-2/1": false}
		if !maps.Equal(equipped.Killed, wantEquipped) {
			t.Errorf("equipped kills %v", equipped.Killed)
		}
		final := heldOut[study.DirFinal]
		wantFinal := map[string]bool{"DEMO-1/1": true, "DEMO-1/2": false, "DEMO-2/1": true}
		if len(final.Valid) != 3 || len(final.Contradicting) != 0 || !maps.Equal(final.Killed, wantFinal) {
			t.Errorf("final %+v", final)
		}
		if !maps.Equal(heldOut[study.StopDir(1)].Killed, wantEquipped) {
			t.Errorf("stop 1 kills %v", heldOut[study.StopDir(1)].Killed)
		}
		wantStop2 := map[string]bool{"DEMO-1/1": true, "DEMO-1/2": true, "DEMO-2/1": true}
		if !maps.Equal(heldOut[study.StopDir(2)].Killed, wantStop2) {
			t.Errorf("stop 2 kills %v", heldOut[study.StopDir(2)].Killed)
		}
	})

	t.Run("a test directory runs with the equipped pytest.ini and its own conftest [verifies SP-2564~1]", func(t *testing.T) {
		s := newScenario(t)
		s.box.archives["tree2"]["tests/conftest.py"] = "# conftest\n"
		var seen map[string]string
		s.box.pytest = func(call pytestCall) fakeOutcome {
			if call.Kind == kindHeldOut && call.Tree["tests/test_s.py"] != "" {
				seen = call.Tree
			}
			return s.behave(call)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if seen["pytest.ini"] != s.box.archives[equippedCommit]["pytest.ini"] {
			t.Errorf("pytest.ini %q", seen["pytest.ini"])
		}
		if _, ok := seen["tests/conftest.py"]; !ok {
			t.Error("conftest.py dropped")
		}
		if _, ok := seen["tests/test_base.py"]; ok {
			t.Error("the fixture's tests entered the tree")
		}
	})

	t.Run("a violation is killed at two of three runs and not at one [verifies SP-2565~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if s.readHeldOut()[study.DirFinal].Killed["DEMO-1/2"] {
			t.Fatal("a failure in one run killed the violation")
		}
	})

	t.Run("stop trees come from git archive in the box with the stop object store [verifies SP-2566~1, SP-2568~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		var stopCalls int
		for _, command := range s.box.calls("git") {
			if !slices.Contains(command.Argv, "archive") {
				t.Errorf("unexpected git command %v", command.Argv)
			}
			if command.Argv[len(command.Argv)-2] == "tree1" || command.Argv[len(command.Argv)-2] == "tree2" {
				stopCalls++
				env := strings.Join(command.Env, "\n")
				if !strings.Contains(env, "GIT_OBJECT_DIRECTORY="+filepath.Join(s.runDir, "objects")) ||
					!strings.Contains(env, "GIT_ALTERNATE_OBJECT_DIRECTORIES="+filepath.Join(s.runDir, "project", ".git", "objects")) {
					t.Errorf("stop environment %v", command.Env)
				}
			}
			for _, mount := range command.Mounts {
				if mount.Path == s.runDir && !mount.ReadOnly {
					t.Error("the run directory is mounted read-write")
				}
			}
		}
		if stopCalls != 2 {
			t.Errorf("%d stop archives", stopCalls)
		}
	})

	t.Run("identical test directories are measured once [verifies SP-2566~1]", func(t *testing.T) {
		s := newScenario(t)
		reference := 0
		s.box.pytest = func(call pytestCall) fakeOutcome {
			if call.Kind == kindHeldOut && call.Run == 1 && strings.Contains(call.Tree["pkg/mod.py"], "REF") {
				reference++
			}
			return s.behave(call)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		const distinctDirectories = 3
		if reference != distinctDirectories {
			t.Errorf("%d reference measurements", reference)
		}
	})

	t.Run("a snapshot without tests has no valid test [verifies SP-2566~1]", func(t *testing.T) {
		s := newScenario(t)
		delete(s.box.archives, "tree2")
		s.box.archives["tree2"] = map[string]string{"README": "x"}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		matrix := s.readHeldOut()[study.StopDir(2)]
		killed, total := matrix.Share()
		if len(matrix.Valid) != 0 || killed != 0 || total != 3 {
			t.Fatalf("%+v", matrix)
		}
	})

	t.Run("a violation list is named after its directories [verifies SP-2564~1]", func(t *testing.T) {
		s := newScenario(t)
		violations, err := ListViolations(filepath.Join(s.repo, "truth/demo/violations"))
		if err != nil {
			t.Fatal(err)
		}
		var names []string
		for _, v := range violations {
			names = append(names, v.Name)
		}
		if !slices.Equal(names, []string{"DEMO-1/1", "DEMO-1/2", "DEMO-2/1"}) {
			t.Fatalf("%v", names)
		}
	})
}

func TestPhase(t *testing.T) {
	t.Run("the verb refuses to run before the run phase ends [verifies SP-2567~1]", func(t *testing.T) {
		s := newScenario(t)
		s.verb.Phase = phaseStub{ended: false}
		err := s.verb.Execute(context.Background())
		if !errors.Is(err, ErrPhaseNotEnded) {
			t.Fatalf("%v", err)
		}
		if len(s.box.commands) != 0 {
			t.Error("the box ran a command")
		}
		if _, statErr := os.Stat(OraclePath(s.runDir)); !os.IsNotExist(statErr) {
			t.Error("oracle.json written")
		}
	})

	t.Run("Main exits 1 on a refusal and 2 on a usage error [verifies SP-2567~1]", func(t *testing.T) {
		s := newScenario(t)
		previous := Wire
		defer func() { Wire = previous }()
		Wire = func(Options) (Wiring, error) { return Wiring{Runner: s.box, Phase: phaseStub{ended: false}}, nil }
		var out, errOut bytes.Buffer
		if code := Main(context.Background(), []string{"--repo", s.repo, "--work", s.work}, &out, &errOut); code != 1 {
			t.Fatalf("code %d: %s", code, errOut.String())
		}
		if !strings.Contains(errOut.String(), "run phase has not ended") {
			t.Errorf("stderr %q", errOut.String())
		}
		if code := Main(context.Background(), []string{"--nope"}, &out, &errOut); code != 2 {
			t.Fatalf("code %d", code)
		}
		Wire = func(Options) (Wiring, error) { return Wiring{Runner: s.box, Phase: phaseStub{ended: true}}, nil }
		if code := Main(context.Background(), []string{"--repo", s.repo, "--work", s.work}, &out, &errOut); code != 0 {
			t.Fatalf("code %d: %s", code, errOut.String())
		}
	})
}

func TestIsolation(t *testing.T) {
	t.Run("only oracle.json and heldout.json enter a run directory and truth is untouched [verifies SP-2568~1]", func(t *testing.T) {
		s := newScenario(t)
		runBefore := listing(t, s.runDir)
		truthBefore := listing(t, filepath.Join(s.repo, "truth"))
		fixtureBefore := listing(t, filepath.Join(s.repo, "fixture"))
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		runAfter := listing(t, s.runDir)
		for name, content := range runAfter {
			before, existed := runBefore[name]
			switch {
			case name == "records/oracle.json" || name == "records/heldout.json":
				if existed {
					t.Errorf("%s existed before", name)
				}
			case !existed || before != content:
				t.Errorf("%s changed", name)
			}
		}
		if len(runAfter) != len(runBefore)+2 {
			t.Errorf("run directory holds %d files, want %d", len(runAfter), len(runBefore)+2)
		}
		if !maps.Equal(truthBefore, listing(t, filepath.Join(s.repo, "truth"))) || !maps.Equal(fixtureBefore, listing(t, filepath.Join(s.repo, "fixture"))) {
			t.Error("truth or fixture changed")
		}
		jobs, err := os.ReadDir(filepath.Join(s.work, "oracle", "jobs"))
		if err != nil || len(jobs) != 0 {
			t.Errorf("job directories left: %v %v", jobs, err)
		}
	})
}

func TestResume(t *testing.T) {
	t.Run("a second pass over unchanged inputs runs nothing [verifies SP-2569~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := len(s.box.commands)
		first, _ := os.ReadFile(OraclePath(s.runDir))
		fresh := NewVerb(s.repo, study.Layout{Root: s.work}, s.box, phaseStub{ended: true}, &bytes.Buffer{})
		if err := fresh.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(s.box.commands) != before {
			t.Errorf("%d commands ran again", len(s.box.commands)-before)
		}
		if again, _ := os.ReadFile(OraclePath(s.runDir)); !bytes.Equal(first, again) {
			t.Error("oracle.json rewritten")
		}
	})

	t.Run("a changed source reruns the oracle and keeps the held-out matrices [verifies SP-2569~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		before := len(s.box.commands)
		write(t, s.runDir, "project/pkg/mod.py", "BAD2\n")
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if got := len(s.box.commands) - before; got != study.OracleRuns {
			t.Errorf("%d commands ran, want %d", got, study.OracleRuns)
		}
		if !s.readOracle().Defective {
			t.Error("oracle.json is stale")
		}
	})

	t.Run("a missing output is produced again [verifies SP-2569~1]", func(t *testing.T) {
		s := newScenario(t)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := os.Remove(HeldOutPath(s.runDir)); err != nil {
			t.Fatal(err)
		}
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		if len(s.readHeldOut()) != 4 {
			t.Error("heldout.json not rebuilt")
		}
	})

	t.Run("only counted attempts are judged [verifies SP-2569~1]", func(t *testing.T) {
		s := newScenario(t)
		other := study.Layout{Root: s.work}.RunDir("demo-2", 1)
		write(t, other, "records/run.json", `{"task":"demo","outcome":"harness-error","attempt":1}`)
		write(t, other, "project/pkg/mod.py", "X\n")
		capped := study.Layout{Root: s.work}.RunDir("demo-3", 1)
		write(t, capped, "project/pkg/mod.py", "X\n")
		write(t, capped, "records/run.json", `{"task":"demo","outcome":"capped-time","attempt":1,"equippedCommit":"eq"}`)
		if err := s.verb.Execute(context.Background()); err != nil {
			t.Fatal(err)
		}
		for dir, want := range map[string]bool{s.runDir: true, other: false, capped: true} {
			if _, err := os.Stat(OraclePath(dir)); (err == nil) != want {
				t.Errorf("%s: oracle.json present = %v, want %v", dir, err == nil, want)
			}
		}
	})

	t.Run("a failing attempt does not stop the others [verifies SP-2569~1]", func(t *testing.T) {
		s := newScenario(t)
		broken := study.Layout{Root: s.work}.RunDir("demo-0", 1)
		write(t, broken, "records/run.json", `{"task":"unknown","outcome":"completed","attempt":1}`)
		err := s.verb.Execute(context.Background())
		if !errors.Is(err, ErrTruth) {
			t.Fatalf("%v", err)
		}
		if _, statErr := os.Stat(OraclePath(s.runDir)); statErr != nil {
			t.Error("the good attempt was not judged")
		}
	})
}
