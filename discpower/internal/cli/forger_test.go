package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/parcel"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// forgedWaves lets the host answer enough times for every ticket to be rejected
// until it is abandoned.
const forgedWaves = 24

// secondEmission ends the identifier of a ticket issued a second time.
const secondEmission = ".2"

// fabricatedWitness claims that the shipping fee at a total of 50 is 4.5, which the
// original code does not return.
const fabricatedWitness = `{"operation":"shipping_fee","args":{"total":{"type":"integer","value":"50"}},` +
	`"expected":{"kind":"returns","type":"decimal","value":"4.5"}}`

// forging answers like a host that wants every mutant to count: the generated
// mutants are the honest ones, every judge rules VALID with a fabricated witness or
// EQUIVALENT, and every screen answer says the change violates.
type forging struct{ honest oracle }

// Answer implements hostprototest.AnswerSource.
func (f forging) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	switch {
	case len(view.Closed) > 0:
		return strings.ReplaceAll(parcel.ScreenAnswer(view.User), "CONSISTENT", "VIOLATES"), nil
	case strings.Contains(string(view.ResponseSchema), `"mutants"`),
		strings.Contains(string(view.ResponseSchema), `"witnesses"`),
		strings.Contains(string(view.ResponseSchema), `"verdicts"`):
		return f.honest.Answer(persona, view)
	case strings.Contains(string(view.ResponseSchema), `"equivalent"`):
		return `{"equivalent":"EQUIVALENT","confidence":1}`, nil
	default:
		return `{"validity":"VALID","confidence":1,"witness":` + fabricatedWitness + `}`, nil
	}
}

// journal returns the events journaled for the harness's project.
func (h *harness) journal() []hostproto.Record {
	h.t.Helper()
	root, err := filepath.Abs(h.root)
	if err != nil {
		h.t.Fatal(err)
	}
	lookup := func(name string) (string, bool) { return h.state, name == stateVariable }
	store, err := hoststore.Open(xdg.HostDir(lookup, h.state, root))
	if err != nil {
		h.t.Fatal(err)
	}
	records, err := store.Records()
	if err != nil {
		h.t.Fatal(err)
	}
	return records
}

func TestForgedJudgments(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	t.Run("answers that execution refutes are rejected, reissued and never pass a criterion [verifies SP-183~1]", func(t *testing.T) {
		h := newHarness(t, t.TempDir())
		h.waves = forgedWaves
		code, output := h.drive(hostprototest.PersonaHonest, forging{})
		if code == cli.ExitClean {
			t.Errorf("exit code %d for a forging host", code)
		}
		if got := verdicts(output)["PAR-3~1"]; strings.Contains(got, "PASS") {
			t.Errorf("PAR-3~1 passed on forged judgments: %q", got)
		}
		for _, criterion := range []string{"PAR-1~1", "PAR-2~1", "PAR-3~1"} {
			for _, mutant := range h.statement(criterion).Predicate.Mutants {
				if mutant.Validity.Class == format.ClassJudged && mutant.Validity.Value == format.ValidityValid && len(mutant.Validity.CorroboratedBy) == 0 {
					t.Errorf("%s lists a judged VALID mutant without corroboration", criterion)
				}
			}
		}
		rejected, reissued, abandoned := false, false, false
		for _, record := range h.journal() {
			rejected = rejected || record.Event.Kind == hostproto.EventRejected
			reissued = reissued || (record.Event.Kind == hostproto.EventEmitted && strings.HasSuffix(record.Event.Ticket, secondEmission))
			abandoned = abandoned || record.Event.Kind == hostproto.EventAbandoned
		}
		if !rejected || !reissued || !abandoned {
			t.Errorf("journal: rejected %t, reissued %t, abandoned %t", rejected, reissued, abandoned)
		}
	})
}
