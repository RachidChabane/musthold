package prove_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
)

func TestAlignedTestsJustifyThroughTheirLabels(t *testing.T) {
	cases := []struct {
		fixture  string
		evidence func() (testkit.Power, testkit.Aligned)
	}{
		{testkit.TypeScriptLedger, testkit.TypeScriptEvidence},
		{testkit.JavaLedger, testkit.JavaEvidence},
	}
	for _, c := range cases {
		_, aligned := c.evidence()

		w := newWorld(t, c.fixture)
		aligned.Write(t, w.root)
		w.mustRun(prove.Options{})
		justified := w.requestKinds()["hunk"]

		unmatched := newWorld(t, c.fixture)
		other := aligned
		other.Test = strings.NewReplacer("multiply", "divide", "multiplies", "divides").Replace(aligned.Test)
		other.Write(t, unmatched.root)
		unmatched.mustRun(prove.Options{})
		unjustified := unmatched.requestKinds()["hunk"]

		t.Run(c.fixture+": an ALIGNED test that is a callback or an annotated method justifies the units it calls through its label alone [verifies SP-1312~1]", func(t *testing.T) {
			if unjustified != 5 {
				t.Fatalf("%d hunk requests when no test unit matches the label, want 5", unjustified)
			}
			if justified != 4 {
				t.Fatalf("%d hunk requests when the label designates the test, want 4: the line amount it calls is justified and the four other changes are not", justified)
			}
		})
	}
}
