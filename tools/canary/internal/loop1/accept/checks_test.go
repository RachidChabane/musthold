package accept_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept/acceptest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	sampleName     = "fare"
	repoFromHere   = "../../../../.."
	committedRun   = "experiments/loop-1/loop-1.json"
	round          = 1
	runsPerCommand = 3
)

// harness is a repository holding the sample task and an acceptor over the
// simulated box.
type harness struct {
	t        *testing.T
	repo     string
	work     string
	box      *acceptest.Box
	equipper *acceptest.Equipper
	acceptor *accept.Acceptor
	log      *records.FileWriter
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	repo, work := t.TempDir(), t.TempDir()
	committed, err := os.ReadFile(filepath.Join(repoFromHere, committedRun))
	if err != nil {
		t.Fatal(err)
	}
	config := filepath.Join(repo, committedRun)
	if err := os.MkdirAll(filepath.Dir(config), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config, committed, 0o644); err != nil {
		t.Fatal(err)
	}
	acceptest.NewSample(sampleName).Write(t, repo)
	layout := study.Layout{Root: work}
	h := &harness{t: t, repo: repo, work: work, box: acceptest.NewBox(t, work), equipper: &acceptest.Equipper{}, log: records.NewFileWriter(layout)}
	h.acceptor = &accept.Acceptor{Repo: repo, Config: config, Layout: layout, Runner: h.box, Equipper: h.equipper, Records: h.log}
	return h
}

func (h *harness) accept() accept.Report {
	h.t.Helper()
	report, err := h.acceptor.Accept(context.Background(), sampleName, round)
	if err != nil {
		h.t.Fatal(err)
	}
	return report
}

func (h *harness) edit(file, from, to string) {
	acceptest.Replace(h.t, h.repo, sampleName, file, from, to)
}

func resultOf(t *testing.T, report accept.Report, id string) accept.Result {
	t.Helper()
	for _, result := range report.Results {
		if result.ID == id {
			return result
		}
	}
	t.Fatalf("the report holds no check %s", id)
	return accept.Result{}
}

func findingTexts(result accept.Result, role records.Role) string {
	var texts []string
	for _, finding := range result.Findings {
		if finding.Role == role {
			texts = append(texts, finding.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func TestTheSampleTaskPasses(t *testing.T) {
	t.Run("a task that meets every condition passes all nine checks, is equipped as the control arm and is appended to the configuration [verifies SP-2558~1]", func(t *testing.T) {
		h := newHarness(t)
		report := h.accept()
		if !report.Passed() {
			var out strings.Builder
			report.Print(&out)
			t.Fatalf("the sample task fails:\n%s", out.String())
		}
		if len(report.Results) != len(accept.CheckOrder()) {
			t.Fatalf("%d results for %d checks", len(report.Results), len(accept.CheckOrder()))
		}
		if len(h.equipper.Equipped) == 0 {
			t.Fatal("the control arm was never equipped")
		}
		gated := false
		for _, command := range h.box.Commands() {
			gated = gated || (command.Argv[0] == accept.ShallnotProgram && command.Argv[1] == "gate" && slices.Contains(h.equipper.Equipped, command.Dir))
		}
		if !gated {
			t.Error("the traceability gate did not run in the equipped project")
		}
		data, err := os.ReadFile(h.acceptor.Config)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), `"name": "fare"`) || !strings.Contains(string(data), `"fixture": "experiments/loop-1/tasks/fare/fixture"`) {
			t.Fatalf("the configuration lacks the task:\n%s", data)
		}
	})

	t.Run("the output of every check is appended to the construction records [verifies SP-2549~1]", func(t *testing.T) {
		h := newHarness(t)
		h.accept()
		log, err := records.Read(records.Path(study.Layout{Root: h.work}))
		if err != nil {
			t.Fatal(err)
		}
		if len(log.Checks) != len(accept.CheckOrder()) {
			t.Fatalf("%d check records", len(log.Checks))
		}
		for _, check := range log.Checks {
			if !check.Passed || check.Domain != sampleName || check.Round != round || (check.Output == "" && check.ID != accept.CheckLeak) {
				t.Errorf("record %+v", check)
			}
		}
	})

	t.Run("a failed task is not appended to the configuration [verifies SP-2558~1]", func(t *testing.T) {
		h := newHarness(t)
		before, _ := os.ReadFile(h.acceptor.Config)
		h.edit("fixture/pytest.ini", "testpaths = tests", "testpaths = spec")
		if h.accept().Passed() {
			t.Fatal("the task passed")
		}
		after, _ := os.ReadFile(h.acceptor.Config)
		if string(before) != string(after) {
			t.Fatal("the configuration changed")
		}
	})
}

func TestEveryCommandRunsThreeTimesInsideTheWorkDirectory(t *testing.T) {
	t.Run("each command of each check runs three times, offline, and only the work directory is mounted [verifies SP-2550~1]", func(t *testing.T) {
		h := newHarness(t)
		h.accept()
		commands := h.box.Commands()
		if len(commands) == 0 || len(commands)%runsPerCommand != 0 {
			t.Fatalf("%d commands, not a multiple of %d", len(commands), runsPerCommand)
		}
		for _, command := range commands {
			if len(command.Mounts) != 1 || command.Mounts[0].ReadOnly || command.Limit != study.OracleLimit {
				t.Errorf("command %v mounts %+v with limit %v", command.Argv, command.Mounts, command.Limit)
			}
		}
	})

	t.Run("a check whose three runs differ fails and is reported as disagreeing [verifies SP-2550~1]", func(t *testing.T) {
		h := newHarness(t)
		h.edit("reference/zones.py", "    return 14\n", "    return 14  # NONDETERMINISTIC\n")
		report := h.accept()
		reference := resultOf(t, report, accept.CheckReference)
		if reference.Passed || reference.Agreed {
			t.Fatalf("A3 %+v", reference)
		}
	})
}

func TestShape(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(h *harness)
		want   string
	}{
		{"a prompt outside the template", func(h *harness) {
			h.edit("task.json", "Implement requirement FARE-4", "Please implement FARE-4")
		}, "the prompt is not"},
		{"a pytest.ini without the test path", func(h *harness) { h.edit("fixture/pytest.ini", "testpaths = tests", "testpaths = spec") }, "testpaths"},
		{"a missing ignore line", func(h *harness) { h.edit("fixture/.gitignore", "test-results/", "results/") }, "test-results/"},
		{"a spec with three criteria", func(h *harness) {
			acceptest.Remove(h.t, h.repo, sampleName, "fixture/specs/fares.md")
			acceptest.Replace(h.t, h.repo, sampleName, "fixture/tests/test_fare.py", "FARE-3~1", "FARE-1~1")
			writeFile(h.t, filepath.Join(h.repo, study.TaskDir(sampleName), "fixture/specs/fares.md"), threeCriteria)
		}, "4 to 6 are required"},
		{"an example that names another operation", func(h *harness) {
			h.edit("fixture/specs/fares.md", "fare_for(distance=5)", "price_of(distance=5)")
		}, `calls "price_of"`},
		{"a task criterion that a test cites", func(h *harness) {
			h.edit("fixture/tests/test_fare.py", `"FARE-3~1"`, `"FARE-4~1"`)
		}, "cites the task criterion"},
		{"a criterion no test cites", func(h *harness) {
			h.edit("fixture/tests/test_fare.py", `"FARE-2~1"`, `"FARE-1~1"`)
		}, "no test of the fixture cites FARE-2~1"},
		{"a criterion with one violation", func(h *harness) {
			acceptest.Remove(h.t, h.repo, sampleName, "violations/FARE-2/2/zones.py")
		}, "criterion FARE-2 has 1 violations"},
		{"a violation that edits a file the project lacks", func(h *harness) {
			writeFile(h.t, filepath.Join(h.repo, study.TaskDir(sampleName), "violations/FARE-2/3/other.py"), "x = 1\n")
		}, "does not hold"},
		{"a module that does not define the operation", func(h *harness) {
			h.edit("fixture/fare/zones.py", "def fare_for(distance):", "def price(distance):")
		}, "holds no callable"},
		{"a second public function", func(h *harness) {
			h.edit("fixture/fare/zones.py", "def fare_for(distance):", "def extra(x):\n    return x\n\n\ndef fare_for(distance):")
		}, "one public operation"},
		{"a signature that lists other parameters", func(h *harness) { h.edit("task.json", "fare_for(distance)", "fare_for(km)") }, "lists"},
	}
	for _, tc := range cases {
		t.Run(tc.name+" fails A1 and the other checks do not run [verifies SP-2551~1]", func(t *testing.T) {
			h := newHarness(t)
			tc.mutate(h)
			report := h.accept()
			shape := resultOf(t, report, accept.CheckShape)
			if shape.Passed || !strings.Contains(findingTexts(shape, records.RoleAuthor), tc.want) {
				t.Fatalf("A1 passed or lacks %q: %+v", tc.want, shape)
			}
			for _, result := range report.Results[1:] {
				if !result.Skipped {
					t.Errorf("%s ran after a failed shape check", result.ID)
				}
			}
		})
	}

	t.Run("a task description that cannot be read fails A1 [verifies SP-2551~1]", func(t *testing.T) {
		h := newHarness(t)
		h.edit("task.json", `"criterion"`, `"criteria"`)
		if report := h.accept(); resultOf(t, report, accept.CheckShape).Passed {
			t.Fatal("A1 passed")
		}
	})
}

const threeCriteria = `# Train fares

## Fares

- **FARE-1~1**: WHEN the distance is 0 to 10 THE SYSTEM SHALL charge 2.
  - Example: fare_for(distance=5) => 2
- **FARE-2~1**: WHEN the distance is 11 to 50 THE SYSTEM SHALL charge 5.
  - Example: fare_for(distance=30) => 5
- **FARE-4~1**: WHEN the distance is more than 200 THE SYSTEM SHALL charge 14.
  - Example: fare_for(distance=250) => 14
`

func writeFile(t *testing.T, location, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(location), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestBehaviourChecks(t *testing.T) {
	cases := []struct {
		tag    string
		check  string
		name   string
		mutate func(h *harness)
		author string
		oracle string
	}{
		{"[verifies SP-2552~1]", accept.CheckFixture, "fixture tests that fail", func(h *harness) {
			h.edit("fixture/fare/zones.py", "return 2\n", "return 2  # BREAKS: FARE-1\n")
		}, "do not pass", ""},
		{"[verifies SP-2553~1]", accept.CheckReference, "a reference that fails the oracle", func(h *harness) {
			h.edit("reference/zones.py", "    return 5\n", "    return 5  # BREAKS: FARE-2\n")
		}, "fails spec tests", "these tests fail"},
		{"[verifies SP-2553~1]", accept.CheckBase, "a fixture that already satisfies the task criterion", func(h *harness) {
			h.edit("fixture/fare/zones.py", "  # BREAKS: FARE-4", "")
		}, "already behaves", "no test of FARE-4 fails"},
		{"[verifies SP-2553~1]", accept.CheckBase, "a fixture that fails the oracle of another criterion", func(h *harness) {
			h.edit("fixture/fare/zones.py", "return 5\n", "return 5  # BREAKS: FARE-2\n")
		}, "fails spec tests of other criteria", "these tests fail"},
		{"[verifies SP-2554~1]", accept.CheckDiagonal, "a violation no oracle test notices", func(h *harness) {
			h.edit("violations/FARE-1/1/zones.py", "# BREAKS: FARE-1", "# harmless")
		}, "fails no spec test of FARE-1", "no test of FARE-1 fails"},
		{"[verifies SP-2554~1]", accept.CheckDiagonal, "a violation that breaks another criterion too", func(h *harness) {
			h.edit("violations/FARE-1/1/zones.py", "# BREAKS: FARE-1", "# BREAKS: FARE-1\n# BREAKS: FARE-3")
		}, "other criteria", "violates only FARE-1"},
		{"[verifies SP-2554~1]", accept.CheckRefactors, "a refactor that breaks a criterion", func(h *harness) {
			h.edit("refactors/1/zones.py", "return 1 + 1", "return 1 + 1  # BREAKS: FARE-1")
		}, "refactor-1 fails spec tests", "these tests fail"},
		{"[verifies SP-2555~1]", accept.CheckRoom, "no violation of the task criterion that the inline examples miss", func(h *harness) {
			h.edit("violations/FARE-4/2/zones.py", "  # INLINE-MISS: FARE-4", "")
		}, "inline examples", ""},
		{"[verifies SP-2555~1]", accept.CheckRoom, "no violation of an implemented criterion that the tests miss", func(h *harness) {
			for _, criterion := range []string{"FARE-1", "FARE-2", "FARE-3"} {
				h.edit("violations/"+criterion+"/2/zones.py", "  # TESTS-MISS: "+criterion, "")
			}
		}, "passes the tests of the fixture", ""},
		{"[verifies SP-2557~1]", accept.CheckCollection, "an oracle file with one test", func(h *harness) {
			writeFile(h.t, filepath.Join(h.repo, study.TaskDir(sampleName), "oracle/test_fare_2.py"), "def test_only():\n    assert True\n")
		}, "", "collects 1 tests"},
		{"[verifies SP-2557~1]", accept.CheckCollection, "a skipped test", func(h *harness) {
			h.edit("oracle/test_fare_3.py", "def test_three_example", "# SKIPPED\ndef test_three_example")
		}, "", "skipped"},
		{"[verifies SP-2557~1]", accept.CheckCollection, "an oracle file that does not collect", func(h *harness) {
			h.edit("oracle/test_fare_3.py", "def test_three_example", "# SYNTAX_ERROR\ndef test_three_example")
		}, "", "cannot be collected"},
	}
	for _, tc := range cases {
		t.Run(tc.check+" fails for "+tc.name+" "+tc.tag, func(t *testing.T) {
			h := newHarness(t)
			tc.mutate(h)
			report := h.accept()
			result := resultOf(t, report, tc.check)
			if result.Passed {
				t.Fatalf("%s passed", tc.check)
			}
			if got := findingTexts(result, records.RoleAuthor); !strings.Contains(got, tc.author) {
				t.Errorf("the author's findings lack %q:\n%s", tc.author, got)
			}
			if got := findingTexts(result, records.RoleOracle); !strings.Contains(got, tc.oracle) {
				t.Errorf("the test writer's findings lack %q:\n%s", tc.oracle, got)
			}
		})
	}
}

func TestOracleFindingsNeverQuoteTheProject(t *testing.T) {
	t.Run("what the test writer is told about a failing check names tests and criteria and no code of the project [verifies SP-2547~1]", func(t *testing.T) {
		h := newHarness(t)
		h.edit("reference/zones.py", "    return 5\n", "    return 5  # BREAKS: FARE-2\n")
		report := h.accept()
		for _, result := range report.Results {
			for _, finding := range result.Findings {
				if finding.Role == records.RoleOracle && (strings.Contains(finding.Text, "return") || strings.Contains(finding.Text, "assert failed")) {
					t.Errorf("%s tells the test writer %q", result.ID, finding.Text)
				}
			}
		}
	})
}
