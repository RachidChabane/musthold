package cli_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// screeningOut answers like a host that wants every survivor out of the pool:
// the honest answers everywhere, except that every screen answer says the change
// is consistent with the requirement.
type screeningOut struct{ honest oracle }

// Answer implements hostprototest.AnswerSource.
func (s screeningOut) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	answer, err := s.honest.Answer(persona, view)
	if err != nil || len(view.Closed) == 0 {
		return answer, err
	}
	return strings.ReplaceAll(answer, "VIOLATES", "CONSISTENT"), nil
}

// screenTotals sums the operator screen counters over the Statements of the
// parcel criteria.
func (h *harness) screenTotals() (flagged, screenedOut, floorEntries int) {
	h.t.Helper()
	for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
		screen := h.statement(criterion).Predicate.Extensions.OperatorScreen
		if screen == nil {
			h.t.Fatalf("%s reports no operator screen", criterion)
		}
		flagged += screen.Flagged
		screenedOut += screen.ScreenedOut
		floorEntries += screen.FloorEntries
	}
	return flagged, screenedOut, floorEntries
}

func TestScreenIsolationFloor(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	t.Run("a host whose answers are exposed cannot screen a survivor out of the pool [verifies SP-252~1]", func(t *testing.T) {
		h := newHarness(t, "")
		h.drive(hostprototest.PersonaHonest, screeningOut{})
		flagged, screenedOut, floorEntries := h.screenTotals()
		if screenedOut != 0 {
			t.Errorf("%d survivors were screened out by exposed answers", screenedOut)
		}
		if flagged == 0 || floorEntries != flagged {
			t.Errorf("flagged %d, floor entries %d: the survivors did not all enter the pool because of the floor", flagged, floorEntries)
		}
	})
}
