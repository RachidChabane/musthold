package e2e_test

import (
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/musthold/format"
)

// The projects whose tests read state another test leaves behind, and what a
// scan of each judges.
const (
	pytestAlertsFixture     = "fixtures/scan/pytest-alerts"
	goQuotesFixture         = "fixtures/scan/go-quotes"
	jestAlertsFixture       = "fixtures/scan/jest-alerts"
	vitestAlertsFixture     = "fixtures/scan/vitest-alerts"
	vacuousNodeEscalation   = "test/escalation.test.ts::escalate accepts any repeat"
	dependentNodeRouting    = "test/channels.test.ts::route never raises a level"
	nodeLevels              = "src/levels.ts"
	nodeQuietCondition      = 22
	nodeQuietLowered        = 23
	verifiesConfirmationE2E = " [verifies SP-2145~2]"
	confirmationPhase       = "confirmation "
	alertsLevels            = "alerts/levels.py"
	quietCondition          = 22
	quietLowered            = 23
)

// confirmationCase is a project of the confirmation fixtures: the test that
// verifies nothing alone, the test whose survivals hold only beside another
// test, and why the confirmation leaves that test unproven.
type confirmationCase struct {
	name, fixture, tool string
	vacuous, dependent  string
	// reason is why the dependent test is unproven; empty for a runner that
	// records the test alone, which may judge it in any class but verifies
	// nothing.
	reason format.ScanUnprovenReason
	// premise checks that the scan proved what a claim on the dependent test
	// would have cited.
	premise func(t *testing.T, statement format.ScanStatement, dependent format.ScanTest)
}

// sitesOf are the distinct sites, the first mutated lines, of the mutants.
func sitesOf(mutants []format.ScanMutant) map[format.ScanLine]bool {
	sites := map[format.ScanLine]bool{}
	for _, mutant := range mutants {
		sites[mutant.Lines[0]] = true
	}
	return sites
}

// provenOnQuietLevel requires the proven mutants of the lines the routing test
// owns only once the ops channel is muted: enough of them, on enough sites, for
// a claim.
func provenOnQuietLevel(t *testing.T, statement format.ScanStatement, dependent format.ScanTest) {
	t.Helper()
	provenOn(t, statement, alertsLevels, quietCondition, quietLowered)
}

// provenOnNodeQuietLevel is provenOnQuietLevel for the TypeScript project, in
// which the routing test, recorded alone, never runs the quiet level: none of
// its changes lies there.
func provenOnNodeQuietLevel(t *testing.T, statement format.ScanStatement, dependent format.ScanTest) {
	t.Helper()
	provenOn(t, statement, nodeLevels, nodeQuietCondition, nodeQuietLowered)
	quiet := map[string]bool{}
	for _, mutant := range statement.Predicate.Mutants {
		if slices.ContainsFunc(mutant.Lines, func(line format.ScanLine) bool {
			return line.File == nodeLevels && (line.Line == nodeQuietCondition || line.Line == nodeQuietLowered)
		}) {
			quiet[mutant.ID] = true
		}
	}
	for _, change := range dependent.Changes {
		if quiet[change.Mutant] {
			t.Fatalf("%s cites %s, a change of the quiet level it runs only beside the test that mutes", dependent.ID, change.Mutant)
		}
	}
}

// provenOn requires enough proven mutants of the two lines of the file, on
// enough sites, for a claim.
func provenOn(t *testing.T, statement format.ScanStatement, file string, condition, lowered int) {
	t.Helper()
	var proven []format.ScanMutant
	for _, mutant := range statement.Predicate.Mutants {
		onQuiet := slices.ContainsFunc(mutant.Lines, func(line format.ScanLine) bool {
			return line.File == file && (line.Line == condition || line.Line == lowered)
		})
		if onQuiet && mutant.Proven() && !mutant.Sentinel {
			proven = append(proven, mutant)
		}
	}
	if len(proven) < vacuity.MinChanges || len(sitesOf(proven)) < vacuity.MinSites {
		t.Fatalf("%d proven mutants on %d sites of quiet_level", len(proven), len(sitesOf(proven)))
	}
}

// citesItsChanges requires the dependent test to cite enough changes, on
// enough sites, for a claim, one of them inconclusive alone.
func citesItsChanges(t *testing.T, statement format.ScanStatement, dependent format.ScanTest) {
	t.Helper()
	var cited []format.ScanMutant
	for _, change := range dependent.Changes {
		index := slices.IndexFunc(statement.Predicate.Mutants, func(mutant format.ScanMutant) bool { return mutant.ID == change.Mutant })
		cited = append(cited, statement.Predicate.Mutants[index])
	}
	inconclusive := slices.ContainsFunc(dependent.Changes, func(change format.ScanChange) bool { return change.Outcome == format.ScanInconclusive })
	if len(cited) < vacuity.MinChanges || len(sitesOf(cited)) < vacuity.MinSites || !inconclusive {
		t.Fatalf("changes %+v", dependent.Changes)
	}
}

func TestScanConfirmsEachClaimAlone(t *testing.T) {
	for _, c := range []confirmationCase{
		{
			name: "pytest", fixture: pytestAlertsFixture, tool: "python3",
			vacuous: "tests/test_escalation.py::test_escalate_accepts_any_repeat", dependent: "tests/test_routing.py::test_route_never_raises_a_level",
			reason: format.ScanTooFewChanges, premise: provenOnQuietLevel,
		},
		{
			name: "go", fixture: goQuotesFixture, tool: "go",
			vacuous: "quotes_test.go::TestQuotesAcceptAnySku", dependent: "quotes_test.go::TestPriceOfThePremiumRange",
			reason: format.ScanInconclusiveOutcome, premise: citesItsChanges,
		},
		{name: "jest", fixture: jestAlertsFixture, tool: nodeTool, vacuous: vacuousNodeEscalation, dependent: dependentNodeRouting, premise: provenOnNodeQuietLevel},
		{name: "vitest", fixture: vitestAlertsFixture, tool: nodeTool, vacuous: vacuousNodeEscalation, dependent: dependentNodeRouting, premise: provenOnNodeQuietLevel},
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
				judged := dependent.ID != "" && dependent.Class != format.ScanVerifiesNothing
				if c.reason != "" {
					judged = dependent.Class == format.ScanUnproven && dependent.Reason == c.reason
				}
				if !judged || strings.Contains(scan.stdout, c.dependent+" passes on all") {
					t.Fatalf("%s is judged %q (%s):\n%s", c.dependent, dependent.Class, dependent.Reason, scan.stdout)
				}
				c.premise(t, statement, dependent)
			})
			t.Run("a test that verifies nothing alone keeps its claim, and the confirmation is timed"+verifiesConfirmationE2E, func(t *testing.T) {
				if testNamed(statement, c.vacuous).Class != format.ScanVerifiesNothing || !strings.Contains(scan.stdout, confirmationPhase) {
					t.Fatalf("%s does not verify nothing:\n%s", c.vacuous, scan.stdout)
				}
			})
			t.Run("a replay holds the findings"+verifiesConfirmationE2E, func(t *testing.T) {
				replay := s.run(s.binary, "scan", "replay", ".musthold/scan/"+s.commit()[:7])
				if replay.code != 0 || !strings.Contains(replay.stdout, c.vacuous) || strings.Contains(replay.stdout, "NOT held") {
					t.Fatalf("exit %d:\n%s", replay.code, replay.output())
				}
			})
		})
	}
}
