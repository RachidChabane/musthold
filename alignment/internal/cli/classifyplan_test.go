package cli_test

import (
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func TestSystemOneClassifyPlan(t *testing.T) {
	const model = "claude-haiku-4-5-20251001"
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.run(nil, proveCommand, allFlag)

	t.Run("classify-plan plans every closed request that waits for its first vote [verifies SP-1911~1]", func(t *testing.T) {
		code, stdout, stderr := h.run(nil, "systemone", "classify-plan", "--model", model)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		items := hostprototest.DecodeClassifyPlans(t, stdout)
		listed := 0
		for _, listing := range h.requestsJSON().Items {
			if len(listing.View.Closed) > 0 && listing.Ticket.Vote == 0 {
				if listed >= len(items) {
					t.Fatalf("%d items for more closed requests", len(items))
				}
				hostprototest.CheckClassifyPlan(t, items[listed], listing.Ticket, listing.View.Closed, "host-classify(5,rot1):"+model+"*5")
				listed++
			}
		}
		if listed == 0 || listed != len(items) {
			t.Fatalf("%d closed requests, %d items", listed, len(items))
		}
	})

	t.Run("classify-plan refuses a call that names neither a model nor voters [verifies SP-1911~1]", func(t *testing.T) {
		if code, stdout, stderr := h.run(nil, "systemone", "classify-plan"); code == cli.ExitClean || stdout != "" || stderr == "" {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})
}
