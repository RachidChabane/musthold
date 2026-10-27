package oracle

import (
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	junitFailure = `<testsuite><testcase classname="oracle.test_box_4" name="test_a"><failure message="x"/></testcase></testsuite>`
	junitError   = `<testsuite><testcase classname="oracle.test_box_4" name="test_a"><error message="x"/></testcase></testsuite>`
	junitPass    = `<testsuite><testcase classname="oracle.test_box_4" name="test_a"/><testcase classname="oracle.test_box_4" name="test_b"><skipped/></testcase></testsuite>`
	junitMixed   = `<testsuites><testsuite><testcase classname="oracle.test_box_4" name="test_a"/><testcase classname="oracle.test_box_5.TestX" name="test_b[1]"><failure/></testcase></testsuite></testsuites>`
)

func TestClassifyRun(t *testing.T) {
	t.Run("a failure or an error child fails the case, a skip does not [verifies SP-2561~1]", func(t *testing.T) {
		for name, tc := range map[string]struct {
			junit string
			want  map[string]bool
		}{
			"failure": {junitFailure, map[string]bool{"oracle.test_box_4::test_a": true}},
			"error":   {junitError, map[string]bool{"oracle.test_box_4::test_a": true}},
			"pass":    {junitPass, map[string]bool{"oracle.test_box_4::test_a": false, "oracle.test_box_4::test_b": false}},
			"mixed":   {junitMixed, map[string]bool{"oracle.test_box_4::test_a": false, "oracle.test_box_5.TestX::test_b[1]": true}},
		} {
			exit := exitAllPassed
			if name != "pass" {
				exit = exitSomeFailed
			}
			report := ClassifyRun(box.Result{ExitCode: exit}, []byte(tc.junit))
			if report.State != RunExecuted || len(report.Failed) != len(tc.want) {
				t.Fatalf("%s: report %+v", name, report)
			}
			for id, failed := range tc.want {
				if report.Failed[id] != failed {
					t.Errorf("%s: %s failed = %v, want %v", name, id, report.Failed[id], failed)
				}
			}
		}
	})

	t.Run("a collection or import failure leaves no result [verifies SP-2561~1]", func(t *testing.T) {
		const interrupted, noTests, internalError = 2, 5, 3
		for name, report := range map[string]RunReport{
			"interrupted by collection": ClassifyRun(box.Result{ExitCode: interrupted}, []byte(junitError)),
			"nothing collected":         ClassifyRun(box.Result{ExitCode: noTests}, nil),
			"internal error":            ClassifyRun(box.Result{ExitCode: internalError}, []byte(junitPass)),
			"missing report":            ClassifyRun(box.Result{ExitCode: exitAllPassed}, nil),
			"unreadable report":         ClassifyRun(box.Result{ExitCode: exitSomeFailed}, []byte("<testsuite>")),
			"empty report":              ClassifyRun(box.Result{ExitCode: exitAllPassed}, []byte("<testsuite/>")),
		} {
			if report.State != RunCollectionFailed || !report.failedTest("any") {
				t.Errorf("%s: %+v", name, report)
			}
		}
	})

	t.Run("a run at its limit is timed out whatever it wrote [verifies SP-2562~2]", func(t *testing.T) {
		report := ClassifyRun(box.Result{TimedOut: true, ExitCode: exitAllPassed}, []byte(junitPass))
		if report.State != RunTimedOut || !report.failedTest("oracle.test_box_4::test_a") {
			t.Fatalf("%+v", report)
		}
	})

	t.Run("a run killed for memory fails every test whatever it wrote [verifies SP-2562~2]", func(t *testing.T) {
		report := ClassifyRun(box.Result{OOMKilled: true, ExitCode: exitAllPassed}, []byte(junitPass))
		if report.State != RunOOMKilled || !report.failedTest("oracle.test_box_4::test_a") {
			t.Fatalf("%+v", report)
		}
	})
}

func TestOracleVerdict(t *testing.T) {
	passing := RunReport{State: RunExecuted, Failed: map[string]bool{"oracle.test_box_4::test_a": false, "oracle.test_box_5::test_b": false}}
	failing := func(ids ...string) RunReport {
		report := RunReport{State: RunExecuted, Failed: map[string]bool{}}
		for id := range passing.Failed {
			report.Failed[id] = slices.Contains(ids, id)
		}
		return report
	}
	const a, b = "oracle.test_box_4::test_a", "oracle.test_box_5::test_b"

	t.Run("a test failing in two of three runs is defective and violates its criterion [verifies SP-2563~1]", func(t *testing.T) {
		verdict := OracleVerdict([3]RunReport{failing(a), passing, failing(a)}, nil)
		if !verdict.Defective || !slices.Equal(verdict.Violated, []string{"BOX-4"}) || len(verdict.Flaky) != 0 {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a test failing in one run is flaky and clean [verifies SP-2563~1]", func(t *testing.T) {
		verdict := OracleVerdict([3]RunReport{passing, failing(b), passing}, nil)
		if verdict.Defective || len(verdict.Violated) != 0 || !slices.Equal(verdict.Flaky, []string{b}) {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a run at its limit counts as every test failing [verifies SP-2562~2]", func(t *testing.T) {
		verdict := OracleVerdict([3]RunReport{passing, {State: RunTimedOut}, {State: RunTimedOut}}, nil)
		if !verdict.Defective || verdict.TimedOut != [3]bool{false, true, true} ||
			!slices.Equal(verdict.Violated, []string{"BOX-4", "BOX-5"}) {
			t.Fatalf("%+v", verdict)
		}
		one := OracleVerdict([3]RunReport{passing, {State: RunTimedOut}, passing}, nil)
		if one.Defective || len(one.Flaky) != 2 {
			t.Fatalf("a single timeout is flaky: %+v", one)
		}
	})

	t.Run("a collection failure in every run fails every oracle file [verifies SP-2561~1]", func(t *testing.T) {
		broken := RunReport{State: RunCollectionFailed}
		files := []string{"oracle/test_box_4.py", "oracle/test_box_5.py"}
		verdict := OracleVerdict([3]RunReport{broken, broken, broken}, files)
		if !verdict.Defective || len(verdict.Tests) != 2 || !slices.Equal(verdict.Violated, []string{"BOX-4", "BOX-5"}) {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a collection failure in some runs fails the tests the other runs listed [verifies SP-2561~1]", func(t *testing.T) {
		broken := RunReport{State: RunCollectionFailed}
		verdict := OracleVerdict([3]RunReport{passing, broken, broken}, nil)
		if !verdict.Defective || len(verdict.Tests) != 2 {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("a test a run did not list did not pass [verifies SP-2561~1]", func(t *testing.T) {
		partial := RunReport{State: RunExecuted, Failed: map[string]bool{a: false}}
		verdict := OracleVerdict([3]RunReport{passing, partial, partial}, nil)
		if !slices.Equal(verdict.Violated, []string{"BOX-5"}) {
			t.Fatalf("%+v", verdict)
		}
	})

	t.Run("the verdict follows the two-of-three rule for any failure pattern [verifies SP-2563~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			var flags [3]bool
			for i := range flags {
				flags[i] = rapid.Bool().Draw(t, "failed")
			}
			var reports [3]RunReport
			failures := 0
			for i, failed := range flags {
				reports[i] = failing()
				reports[i].Failed[a] = failed
				if failed {
					failures++
				}
			}
			verdict := OracleVerdict(reports, nil)
			if verdict.Defective != (failures >= study.OracleQuorum) {
				t.Fatalf("pattern %v: defective = %v", flags, verdict.Defective)
			}
			if slices.Contains(verdict.Flaky, a) != (failures == 1) {
				t.Fatalf("pattern %v: flaky = %v", flags, verdict.Flaky)
			}
		})
	})
}

func TestKillVerdict(t *testing.T) {
	const good, bad = "tests.test_x::test_good", "tests.test_x::test_bad"
	reference := func(badFails bool) RunReport {
		return RunReport{State: RunExecuted, Failed: map[string]bool{good: false, bad: badFails}}
	}
	onViolation := func(failed bool) RunReport {
		return RunReport{State: RunExecuted, Failed: map[string]bool{good: failed, bad: failed}}
	}
	refRuns := [3]RunReport{reference(false), reference(false), reference(false)}

	t.Run("a test failing on the reference contradicts the spec and never kills [verifies SP-2564~2]", func(t *testing.T) {
		matrix := KillVerdict([3]RunReport{reference(true), reference(false), reference(false)},
			map[string][3]RunReport{"C-1/1": {onViolation(false), onViolation(false), onViolation(false)}}, nil)
		if !slices.Equal(matrix.Valid, []string{good}) || !slices.Equal(matrix.Contradicting, []string{bad}) || matrix.Killed["C-1/1"] {
			t.Fatalf("%+v", matrix)
		}
	})

	t.Run("a violation is killed at two of three runs and not at one [verifies SP-2565~2]", func(t *testing.T) {
		matrix := KillVerdict(refRuns, map[string][3]RunReport{
			"C-1/1": {onViolation(true), onViolation(false), onViolation(true)},
			"C-1/2": {onViolation(false), onViolation(true), onViolation(false)},
			"C-1/3": {onViolation(true), onViolation(true), onViolation(true)},
			"C-1/4": {onViolation(false), onViolation(false), onViolation(false)},
		}, nil)
		want := map[string]bool{"C-1/1": true, "C-1/2": false, "C-1/3": true, "C-1/4": false}
		for name, killed := range want {
			if matrix.Killed[name] != killed {
				t.Errorf("%s killed = %v, want %v", name, matrix.Killed[name], killed)
			}
		}
		killed, total := matrix.Share()
		if killed != 2 || total != 4 {
			t.Errorf("share %d/%d", killed, total)
		}
	})

	t.Run("a violation that does not run kills every valid test [verifies SP-2565~2]", func(t *testing.T) {
		broken := RunReport{State: RunCollectionFailed}
		matrix := KillVerdict(refRuns, map[string][3]RunReport{"C-1/1": {broken, broken, {State: RunTimedOut}}}, nil)
		if !matrix.Killed["C-1/1"] {
			t.Fatalf("%+v", matrix)
		}
	})

	t.Run("a directory that cannot run on the reference has no valid test [verifies SP-2564~2]", func(t *testing.T) {
		broken := RunReport{State: RunCollectionFailed}
		matrix := KillVerdict([3]RunReport{broken, broken, broken}, map[string][3]RunReport{"C-1/1": {}}, []string{"tests/test_x.py"})
		if len(matrix.Valid) != 0 || !slices.Equal(matrix.Contradicting, []string{"tests/test_x.py"}) || matrix.Killed["C-1/1"] {
			t.Fatalf("%+v", matrix)
		}
	})
}

func TestCriterionOfFile(t *testing.T) {
	t.Run("an oracle file maps to its criterion [verifies SP-2563~1]", func(t *testing.T) {
		for file, want := range map[string]string{
			"test_box_4.py":                 "BOX-4",
			"oracle/test_late_12.py":        "LATE-12",
			"oracle.test_box_4.TestX::test": "",
		} {
			got, ok := CriterionOfFile(file)
			if got != want || ok != (want != "") {
				t.Errorf("%s -> %q, %v", file, got, ok)
			}
		}
		if got := criterionOfTest("oracle.test_box_4.TestX::test_free[3]"); got != "BOX-4" {
			t.Errorf("test id -> %q", got)
		}
		if got := criterionOfTest("oracle/test_box_4.py"); got != "BOX-4" {
			t.Errorf("file id -> %q", got)
		}
		if got := criterionOfTest("conftest::test_x"); got != "" {
			t.Errorf("unmapped id -> %q", got)
		}
	})
}

func TestParseJUnit(t *testing.T) {
	t.Run("a report that is not JUnit is refused [verifies SP-2561~1]", func(t *testing.T) {
		for _, data := range []string{"", "not xml <", "<testsuite/>"} {
			if _, err := ParseJUnit([]byte(data)); !errors.Is(err, ErrJUnit) {
				t.Errorf("%q: %v", data, err)
			}
		}
	})
}
