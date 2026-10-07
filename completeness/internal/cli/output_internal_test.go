package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/prove"
)

func TestPrintResultLeavesNoTrailingSpace(t *testing.T) {
	t.Run("a spec that awaits the host prints no empty field [verifies SP-1631~1]", func(t *testing.T) {
		var out bytes.Buffer
		printResult(&out, prove.Result{
			Wave:           1,
			Specs:          []prove.SpecResult{{Spec: "specs/parcel.md", Status: prove.StatusAwaitingHost}, {Spec: "specs/zone.md", Status: prove.StatusProved, Unresponsive: true}},
			PendingTickets: 2,
		})
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if line != strings.TrimSpace(line) {
				t.Errorf("line %q has a leading or trailing space", line)
			}
		}
		if !strings.Contains(out.String(), "specs/parcel.md awaiting_host 0 questions\n") {
			t.Errorf("output:\n%s", out.String())
		}
	})
}
