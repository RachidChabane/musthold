package cli_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
)

// What an honest host finds in the Python parcel project: the tests that check
// their criterion are ALIGNED, the test that checks the rejection under the
// multiplication criterion and the test that checks nothing of the surcharge are
// MISALIGNED.
var parcelVerdicts = map[string]string{
	"PAR-1~1 " + testkit.PyMultipliesLabel:   "ALIGNED",
	"PAR-1~1 " + testkit.PySingleUnitLabel:   "ALIGNED",
	"PAR-1~1 " + testkit.PyRejectedLineLabel: "MISALIGNED",
	"PAR-2~1 " + testkit.PyRemoteLabel:       "ALIGNED",
	"PAR-2~1 " + testkit.PyNumberLabel:       "MISALIGNED",
	"PAR-3~1 " + testkit.PyZeroRejectedLabel: "ALIGNED",
}

func verdictsOf(h *harness, output string) map[string]string {
	h.t.Helper()
	found := map[string]string{}
	for key, entry := range results(output) {
		found[key] = entry.Verdict
	}
	return found
}

func TestHonestHostWithDiscriminatingPower(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())

	t.Run("every test gets the verdict the matrix and the judge agree on, and a corroborated ALIGNED rests on the matrix [verifies SP-339~1]", func(t *testing.T) {
		code, output := h.driveAttested(testkit.Oracle{})
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d, want %d because two tests are MISALIGNED:\n%s", code, cli.ExitBlocked, output)
		}
		got := verdictsOf(h, output)
		for key, want := range parcelVerdicts {
			if got[key] != want {
				t.Errorf("%s: %q, want %q\n%s", key, got[key], want, output)
			}
		}
		for key, statement := range h.statements() {
			verdict := statement.Predicate.Verdict
			if verdict.Value == format.VerdictAligned {
				if len(verdict.CorroboratedBy) != 1 || verdict.CorroboratedBy[0].Kind != format.CorroborationSpecificMatrix {
					t.Errorf("%s: ALIGNED without the matrix as evidence: %+v", key, verdict.CorroboratedBy)
				}
			}
			if !statement.Predicate.Executor.Blind || statement.Predicate.Extensions.Decision == nil || len(statement.Predicate.Extensions.Matrix) == 0 {
				t.Errorf("%s: the Statement lacks its executor, decision or matrix", key)
			}
			if !slices.ContainsFunc(statement.Predicate.Inputs, func(input format.Input) bool {
				return input.Role == format.RoleProof && input.Name == "discriminating-power"
			}) {
				t.Errorf("%s: no proof input", key)
			}
		}
		if len(h.statements()) != len(parcelVerdicts) {
			t.Errorf("%d Statements for %d tests", len(h.statements()), len(parcelVerdicts))
		}
	})

	t.Run("the pending file is gone once nothing waits [verifies SP-333~1]", func(t *testing.T) {
		if pending := h.pendingFile(); len(pending.Pending) != 0 {
			t.Fatalf("pending %+v", pending)
		}
	})
}

func TestGoldenStatementsAreRead(t *testing.T) {
	t.Run("the golden discriminating-power Statements of a fixture project give the matrix of every test [verifies SP-329~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		golden := filepath.Join(testkit.FixturesDir(), testkit.PythonParcel, "discriminating-power")
		for _, ref := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			content, err := os.ReadFile(filepath.Join(golden, ref+".json"))
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := format.DecodeAs[format.DiscriminatingPower](content)
			if err != nil {
				t.Fatal(err)
			}
			h.write(".spec-proof/cache/discriminating-power/"+ref+"/"+decoded.Predicate.CacheKey+".json", string(content))
		}
		h.driveAttested(testkit.Oracle{})
		for key, statement := range h.statements() {
			if strings.Contains(string(statement.Predicate.Extensions.Matrix), `"ABSENT"`) {
				t.Errorf("%s read no matrix from the golden Statements", key)
			}
		}
	})
}

func TestVerdictBasis(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.driveAttested(testkit.Oracle{})
	statements := h.statements()

	t.Run("a SPECIFIC test is aligned by System One and the matrix, without the judge [verifies SP-311~1]", func(t *testing.T) {
		statement := statements["PAR-1~1 "+testkit.PyMultipliesLabel]
		decision := statement.Predicate.Extensions.Decision
		if decision == nil || (decision.Basis != format.BasisSystemOne && decision.Basis != format.BasisJudges) {
			t.Fatalf("decision %+v", decision)
		}
		if decision.Basis == format.BasisSystemOne && !slices.Contains(decision.Deciders, "system_one") {
			t.Fatalf("deciders %v", decision.Deciders)
		}
	})

	t.Run("an INERT test is MISALIGNED by the judge alone [verifies SP-312~1]", func(t *testing.T) {
		statement := statements["PAR-2~1 "+testkit.PyNumberLabel]
		decision := statement.Predicate.Extensions.Decision
		if decision == nil || decision.Basis != format.BasisJudges || !strings.Contains(string(statement.Predicate.Extensions.Matrix), `"INERT"`) {
			t.Fatalf("decision %+v matrix %s", decision, statement.Predicate.Extensions.Matrix)
		}
	})
}
