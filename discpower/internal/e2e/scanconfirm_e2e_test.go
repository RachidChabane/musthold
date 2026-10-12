package e2e_test

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
)

// The projects whose tests read state another test leaves behind, and what a
// scan of each judges.
const (
	pytestAlertsFixture     = "fixtures/scan/pytest-alerts"
	goQuotesFixture         = "fixtures/scan/go-quotes"
	verifiesConfirmationE2E = " [verifies SP-2145~1]"
	confirmationPhase       = "confirmation "
)

// confirmationCase is a project of the confirmation fixtures: the test that
// verifies nothing alone, and the test whose survivals hold only beside
// another test.
type confirmationCase struct {
	name, fixture, tool string
	vacuous, dependent  string
}

func TestScanConfirmsEachClaimAlone(t *testing.T) {
	for _, c := range []confirmationCase{
		{
			name: "pytest", fixture: pytestAlertsFixture, tool: "python3",
			vacuous: "tests/test_escalation.py::test_escalate_accepts_any_repeat", dependent: "tests/test_routing.py::test_route_never_raises_a_level",
		},
		{
			name: "go", fixture: goQuotesFixture, tool: "go",
			vacuous: "quotes_test.go::TestQuotesAcceptAnySku", dependent: "quotes_test.go::TestPriceOfThePremiumRange",
		},
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

			t.Run("a test whose survivals hold only beside another test makes no claim"+verifiesConfirmationE2E, func(t *testing.T) {
				dependent := testNamed(statement, c.dependent)
				if dependent.ID == "" || dependent.Class == format.ScanVerifiesNothing || strings.Contains(scan.stdout, c.dependent+" passes on all") {
					t.Fatalf("%s is judged %q:\n%s", c.dependent, dependent.Class, scan.stdout)
				}
			})
			t.Run("a test that verifies nothing alone keeps its claim, and the confirmation is timed"+verifiesConfirmationE2E, func(t *testing.T) {
				if testNamed(statement, c.vacuous).Class != format.ScanVerifiesNothing || !strings.Contains(scan.stdout, confirmationPhase) {
					t.Fatalf("%s does not verify nothing:\n%s", c.vacuous, scan.stdout)
				}
			})
			t.Run("a replay holds the findings"+verifiesConfirmationE2E, func(t *testing.T) {
				replay := s.run(s.binary, "scan", "replay", ".spec-proof/scan/"+s.commit()[:7])
				if replay.code != 0 || !strings.Contains(replay.stdout, c.vacuous) || strings.Contains(replay.stdout, "NOT held") {
					t.Fatalf("exit %d:\n%s", replay.code, replay.output())
				}
			})
		})
	}
}
