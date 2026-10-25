package e2e_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

// The projects whose helper stops some test binaries or modules from starting,
// and the tests a scan of each judges.
const (
	goStartUpFixture     = "fixtures/scan/go-rates"
	pytestStartUpFixture = "fixtures/scan/pytest-rates"
	jestStartUpFixture   = "fixtures/scan/jest-rates"
	vitestStartUpFixture = "fixtures/scan/vitest-rates"
	vacuousNodeFare      = "test/billing.test.ts::total accepts any fare"
	stoppedNodeFare      = "test/billing.test.ts::total adds the standard rate"
	verifiesStartUpE2E   = " [verifies SP-2136~2]"
	stoppedSuffix        = " more stopped it from starting"
	stoppedLine          = " proven changes in lines it ran stopped it from starting; they count neither way"
	ratesFile            = "rates/rates.go"
)

// startUpCase is a project of the start-up fixtures: the test that verifies
// nothing, the tests a crash of their start-up stops, and the tests that kill
// those crashes.
type startUpCase struct {
	name, fixture, tool string
	vacuous, stopped    string
	killers             string
}

func scannedStatement(t *testing.T, s *scanned) format.ScanStatement {
	t.Helper()
	short := s.commit()[:7]
	raw, err := os.ReadFile(filepath.Join(s.root, ".spec-proof", "scan", short, "scan.json"))
	if err != nil {
		t.Fatal(err)
	}
	statement, err := format.AdmitScan(raw)
	if err != nil {
		t.Fatalf("the statement does not validate: %v", err)
	}
	return statement
}

func testNamed(statement format.ScanStatement, id string) format.ScanTest {
	index := slices.IndexFunc(statement.Predicate.Tests, func(test format.ScanTest) bool { return test.ID == id })
	if index < 0 {
		return format.ScanTest{}
	}
	return statement.Predicate.Tests[index]
}

func TestScanSetsApartChangesThatStopATest(t *testing.T) {
	for _, c := range []startUpCase{
		{
			name: "go", fixture: goStartUpFixture, tool: "go",
			vacuous: "billing/billing_test.go::TestTotalAcceptsAnyFare", stopped: "billing/billing_test.go::TestTotalAddsTheStandardRate",
			killers: "quote/",
		},
		{
			name: "pytest", fixture: pytestStartUpFixture, tool: "python3",
			vacuous: "tests/test_billing.py::test_total_accepts_any_fare", stopped: "tests/test_billing.py::test_total_adds_the_standard_rate",
		},
		{name: "jest", fixture: jestStartUpFixture, tool: nodeTool, vacuous: vacuousNodeFare, stopped: stoppedNodeFare},
		{name: "vitest", fixture: vitestStartUpFixture, tool: nodeTool, vacuous: vacuousNodeFare, stopped: stoppedNodeFare},
	} {
		t.Run(c.name, func(t *testing.T) {
			if _, err := exec.LookPath(c.tool); err != nil {
				t.Skipf("no %s: %v", c.tool, err)
			}
			s := newScanned(t, c.fixture)
			scan := s.run(s.binary, "scan", "--budget", "0")
			if scan.code != 0 {
				t.Fatalf("exit %d:\n%s", scan.code, scan.output())
			}
			statement := scannedStatement(t, s)
			stopped := testNamed(statement, c.stopped).NotStarted

			t.Run("the tests a change stops from starting list it apart, and it never counts for or against them"+verifiesStartUpE2E, func(t *testing.T) {
				if len(stopped) == 0 {
					t.Fatalf("%s lists nothing that stopped it:\n%s", c.stopped, scan.output())
				}
				for _, id := range []string{c.vacuous, c.stopped} {
					test := testNamed(statement, id)
					if !slices.Equal(test.NotStarted, stopped) {
						t.Fatalf("%s lists %v, %s lists %v", id, test.NotStarted, c.stopped, stopped)
					}
					for _, change := range test.Changes {
						if slices.Contains(stopped, change.Mutant) {
							t.Fatalf("%s counts %s, which stopped it", id, change.Mutant)
						}
					}
				}
			})
			t.Run("the report says how many changes stopped a test, and a test that verifies nothing keeps its sentence"+verifiesStartUpE2E, func(t *testing.T) {
				for _, want := range []string{"#1 " + c.vacuous + " passes on all ", stoppedSuffix, c.stopped + ": "} {
					if !strings.Contains(scan.stdout, want) {
						t.Fatalf("missing %q:\n%s", want, scan.stdout)
					}
				}
				if !strings.Contains(scan.stdout, stoppedLine) {
					t.Fatalf("missing %q:\n%s", stoppedLine, scan.stdout)
				}
			})
			if c.killers != "" {
				t.Run("a test whose binary starts settles a change that stops another binary as usual"+verifiesStartUpE2E, func(t *testing.T) {
					for _, id := range stopped {
						killed := slices.ContainsFunc(statement.Predicate.Tests, func(test format.ScanTest) bool {
							return strings.HasPrefix(test.ID, c.killers) && slices.Contains(test.Changes, format.ScanChange{Mutant: id, Outcome: format.ScanKilled})
						})
						if !killed {
							t.Fatalf("no test of %s kills %s", c.killers, id)
						}
					}
				})
				t.Run("a test that runs a helper only at start-up records no survival on it"+verifiesStartUpE2E, func(t *testing.T) {
					mutants := map[string]format.ScanMutant{}
					for _, mutant := range statement.Predicate.Mutants {
						mutants[mutant.ID] = mutant
					}
					ledger := testNamed(statement, "ledger/ledger_test.go::TestPostSumsTheEntries")
					if ledger.ID == "" {
						t.Fatal("the ledger test is not judged")
					}
					for _, change := range ledger.Changes {
						onRates := slices.ContainsFunc(mutants[change.Mutant].Lines, func(line format.ScanLine) bool { return line.File == ratesFile })
						if onRates && change.Outcome == format.ScanSurvived {
							t.Fatalf("the ledger test counts the survival %s", change.Mutant)
						}
					}
				})
			}
			t.Run("a replay holds the finding of a test that verifies nothing beside the changes that stopped it"+verifiesStartUpE2E, func(t *testing.T) {
				replay := s.run(s.binary, "scan", "replay", ".spec-proof/scan/"+s.commit()[:7])
				if replay.code != 0 || !strings.Contains(replay.stdout, c.vacuous) || strings.Contains(replay.stdout, "NOT held") {
					t.Fatalf("exit %d:\n%s", replay.code, replay.output())
				}
			})
		})
	}
}
