package e2e_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

// The projects whose tests run shared setup or teardown, and the tests a scan
// of each judges.
const (
	goReceiptsFixture     = "fixtures/scan/go-receipts"
	pytestReceiptsFixture = "fixtures/scan/pytest-receipts"
	jestReceiptsFixture   = "fixtures/scan/jest-receipts"
	vitestReceiptsFixture = "fixtures/scan/vitest-receipts"
	vacuousNodeFormat     = "test/format.test.ts::format accepts any amount"
	sharedNodeBatch       = "test/archive.test.ts::a batch > holds two entries"
	verifiesOwnershipE2E  = " [verifies SP-2143~3]"
)

// ownershipCase is a project of the receipts fixtures: the test that verifies
// nothing, and the test whose only survivals lie in code it shares.
type ownershipCase struct {
	name, fixture, tool string
	vacuous, shared     string
}

func TestScanClaimsOnlyTheLinesATestOwns(t *testing.T) {
	for _, c := range []ownershipCase{
		{
			name: "go", fixture: goReceiptsFixture, tool: "go",
			vacuous: "receipts_test.go::TestFormatAcceptsAnyAmount", shared: "receipts_test.go::TestReceiptLines/total",
		},
		{
			name: "pytest", fixture: pytestReceiptsFixture, tool: "python3",
			vacuous: "tests/test_format.py::test_format_accepts_any_amount", shared: "tests/test_archive.py::test_batch_holds_two_entries",
		},
		{name: "jest", fixture: jestReceiptsFixture, tool: nodeTool, vacuous: vacuousNodeFormat, shared: sharedNodeBatch},
		{name: "vitest", fixture: vitestReceiptsFixture, tool: nodeTool, vacuous: vacuousNodeFormat, shared: sharedNodeBatch},
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

			t.Run("a test whose survivals lie only in setup or teardown it shares makes no claim"+verifiesOwnershipE2E, func(t *testing.T) {
				shared := testNamed(statement, c.shared)
				if shared.ID == "" || shared.Class == format.ScanVerifiesNothing || strings.Contains(scan.stdout, c.shared+" passes on all") {
					t.Fatalf("%s is judged %q:\n%s", c.shared, shared.Class, scan.stdout)
				}
				if testNamed(statement, c.vacuous).Class != format.ScanVerifiesNothing {
					t.Fatalf("%s does not verify nothing:\n%s", c.vacuous, scan.stdout)
				}
			})
			t.Run("a replay holds the findings"+verifiesOwnershipE2E, func(t *testing.T) {
				replay := s.run(s.binary, "scan", "replay", ".spec-proof/scan/"+s.commit()[:7])
				if replay.code != 0 || !strings.Contains(replay.stdout, c.vacuous) || strings.Contains(replay.stdout, "NOT held") {
					t.Fatalf("exit %d:\n%s", replay.code, replay.output())
				}
			})
			if c.name != "pytest" {
				return
			}
			t.Run("a change that stops a test module and a conftest from importing settles the tests outside them"+verifiesOwnershipE2E, func(t *testing.T) {
				killer := testNamed(statement, "tests/test_currency.py::test_parse_currency_upper_cases_the_code")
				for _, id := range []string{"tests/test_home_currency.py::test_home_currency_is_the_euro", "tests/ledger/test_ledger_currency.py::test_ledger_books_in_francs"} {
					stopped := testNamed(statement, id).NotStarted
					if len(stopped) == 0 {
						t.Fatalf("%s lists nothing that stopped it:\n%s", id, scan.stdout)
					}
					for _, mutant := range stopped {
						if !slices.Contains(killer.Changes, format.ScanChange{Mutant: mutant, Outcome: format.ScanKilled}) {
							t.Fatalf("%s does not kill %s, which stopped %s", killer.ID, mutant, id)
						}
					}
				}
			})
		})
	}
}

// The project whose parcel test runs a lane picker through the clause that falls
// into the next one, and what its scan judges.
const (
	goRoutesFixture     = "fixtures/scan/go-routes"
	fallthroughOnlyTest = "routes_test.go::TestFragileExpressParcelGetsALane"
	handlingTest        = "routes_test.go::TestHandlingAcceptsAnyParcel"
)

func TestScanMakesNoClaimOnAFallthroughTargetHeader(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go: %v", err)
	}
	s := newScanned(t, goRoutesFixture)
	scan := s.run(s.binary, "scan", "--budget", "0")
	if scan.code != 0 {
		t.Fatalf("exit %d:\n%s", scan.code, scan.output())
	}
	statement := scannedStatement(t, s)

	t.Run("a test whose only survivals lie in the header of a clause a fallthrough enters makes no claim"+verifiesOwnershipE2E, func(t *testing.T) {
		test := testNamed(statement, fallthroughOnlyTest)
		if test.ID == "" || test.Class == format.ScanVerifiesNothing || len(test.Changes) != 0 || strings.Contains(scan.stdout, fallthroughOnlyTest+" passes on all") {
			t.Fatalf("%s is judged %q with %v:\n%s", fallthroughOnlyTest, test.Class, test.Changes, scan.stdout)
		}
	})
	t.Run("a replay holds the findings"+verifiesOwnershipE2E, func(t *testing.T) {
		if testNamed(statement, handlingTest).Class != format.ScanVerifiesNothing {
			for _, mutant := range statement.Predicate.Mutants {
				for _, change := range testNamed(statement, handlingTest).Changes {
					if change.Outcome == format.ScanKilled && change.Mutant == mutant.ID {
						t.Logf("KILLED %+v", mutant)
					}
				}
			}
			t.Fatalf("%s does not verify nothing:\n%s", handlingTest, scan.stdout)
		}
		replay := s.run(s.binary, "scan", "replay", ".spec-proof/scan/"+s.commit()[:7])
		if replay.code != 0 || !strings.Contains(replay.stdout, handlingTest) || strings.Contains(replay.stdout, "NOT held") {
			t.Fatalf("exit %d:\n%s", replay.code, replay.output())
		}
	})
}
