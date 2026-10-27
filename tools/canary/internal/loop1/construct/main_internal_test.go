package construct

import (
	"fmt"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

func TestReportStop(t *testing.T) {
	t.Run("a harness failure exits with its own status and tells that the run resumes [verifies SP-2559~2]", func(t *testing.T) {
		var stderr strings.Builder
		code := reportStop(&stderr, fmt.Errorf("d1 author attempt 1: %w", &HarnessFailure{Kind: HarnessNetwork}))
		if code != ExitHarnessFailure || code == accept.ExitFailure || code == accept.ExitUsage || !strings.Contains(stderr.String(), "resume") {
			t.Fatalf("%d: %s", code, stderr.String())
		}
	})

	t.Run("any other stop exits 1 [verifies SP-2559~2]", func(t *testing.T) {
		var stderr strings.Builder
		if code := reportStop(&stderr, ErrTooFewTasks); code != accept.ExitFailure {
			t.Fatalf("%d", code)
		}
		if code := reportStop(&stderr, ErrFixtureAmendment); code != accept.ExitFailure || !strings.Contains(stderr.String(), "amendment") {
			t.Fatalf("%d", code)
		}
	})
}
