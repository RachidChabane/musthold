package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/prove"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

func TestPrintResultLeavesNoTrailingSpace(t *testing.T) {
	t.Run("a criterion that awaits the host prints no empty result [verifies SP-1611~1]", func(t *testing.T) {
		var out bytes.Buffer
		printResult(&out, prove.Result{
			Wave:           1,
			Criteria:       []prove.CriterionResult{{Ref: spec.Ref{ID: "PAR-1", Revision: 1}, Status: prove.StatusAwaitingHost}},
			PendingTickets: 2,
		})
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if line != strings.TrimSpace(line) {
				t.Errorf("line %q has a leading or trailing space", line)
			}
		}
		if !strings.Contains(out.String(), "PAR-1~1 awaiting_host\n") || !strings.Contains(out.String(), "awaiting_host 2 tickets\n") {
			t.Errorf("output:\n%s", out.String())
		}
	})
}
