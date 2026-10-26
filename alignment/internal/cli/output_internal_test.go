package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/app/prove"
	"github.com/RachidChabane/musthold/alignment/internal/domain/criterion"
)

func TestPrintResultLeavesNoTrailingSpace(t *testing.T) {
	t.Run("a test that awaits the host prints no empty verdict [verifies SP-1621~1]", func(t *testing.T) {
		var out bytes.Buffer
		printResult(&out, prove.Result{
			Wave: 1,
			Tests: []prove.TestResult{
				{Test: "test_total", Criterion: criterion.Ref{ID: "PAR-1", Revision: 1}, Status: prove.StatusAwaitingHost},
				{Test: "test_zone", Criterion: criterion.Ref{ID: "PAR-2", Revision: 1}, Status: prove.StatusAwaitingProof, Reason: "awaiting_proof"},
			},
			PendingTickets: 2,
		})
		for _, line := range strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n") {
			if line != strings.TrimSpace(line) {
				t.Errorf("line %q has a leading or trailing space", line)
			}
		}
		if !strings.Contains(out.String(), "PAR-1~1 test_total awaiting_host\n") || !strings.Contains(out.String(), "PAR-2~1 test_zone awaiting_proof awaiting_proof\n") {
			t.Errorf("output:\n%s", out.String())
		}
	})
}
