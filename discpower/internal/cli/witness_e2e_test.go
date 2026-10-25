package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	verifiesSP1962 = " [verifies SP-1962~1]"
	verifiesSP1963 = " [verifies SP-1963~1]"
	verifiesSP1964 = " [verifies SP-1964~1]"
	verifiesSP1966 = " [verifies SP-1966~2]"
	verifiesSP1967 = " [verifies SP-1967~1]"
	verifiesSP1968 = " [verifies SP-1968~1]"
	verifiesSP1970 = " [verifies SP-1970~1]"
	verifiesSP1972 = " [verifies SP-1972~1]"
	verifiesSP1977 = " [verifies SP-1977~1]"
	verifiesSP1978 = " [verifies SP-1978~1]"

	factsCommand        = "facts"
	showVerb            = "show"
	shippingCriterion   = "PAR-3~1"
	unknownCriterion    = "PAR-9~1"
	criteriaCount       = 3
	writersPerCriterion = 2
	witnessSchemaMarker = `"witnesses"`
	scopeSchemaMarker   = `"verdicts"`
	firstDrawMarker     = " 1 "
	testsFile           = "tests/test_rates.py"
	weakerAssertion     = "assert shipping_fee(10) == 4.5"
	otherAssertion      = "assert shipping_fee(20) == 4.5"
	conflictingExample  = "assert shipping_fee(100) == 0"
	conflictedExample   = "assert shipping_fee(100) == 4.5"
	refactorsDir        = "planted"
	refactorName        = "same_threshold"
	configFile          = ".spec-proof/config.yaml"
	sameThreshold       = `{"edits":[{"file":"parcel/rates.py","qualname":"shipping_fee","new_source":"def shipping_fee(total):\n    if not total < FREE_SHIPPING_FROM:\n        return 0\n    return SHIPPING_FEE"}]}`
	admissionSection    = "admission:\n  refactors: planted\n"
)

// pendingKinds tells, for each pending ticket, whether it asks for witnesses,
// scope verdicts or anything else.
func (h *harness) pendingSchemas() (witnesses, scopes, others int) {
	h.t.Helper()
	_, listed, _ := h.run(nil, hostproto.ListArgs(hostproto.SessionProve)...)
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(listed), &listing); err != nil {
		h.t.Fatal(err)
	}
	for _, item := range listing.Items {
		schema := string(item.View.ResponseSchema)
		switch {
		case strings.Contains(schema, witnessSchemaMarker):
			witnesses++
		case strings.Contains(schema, scopeSchemaMarker):
			scopes++
		default:
			others++
		}
	}
	return witnesses, scopes, others
}

// hostStore opens the host store of the harness's project.
func (h *harness) hostStore() *hoststore.Store {
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
	return store
}

// requestIDsOf lists the ids of the requests of the given kind the store holds.
func (h *harness) requestIDsOf(kind hostproto.Kind) []string {
	h.t.Helper()
	store := h.hostStore()
	entries, err := store.ListEntries()
	if err != nil {
		h.t.Fatal(err)
	}
	var ids []string
	for _, entry := range entries {
		request, err := store.GetRequest(entry.Ticket.RequestID)
		if err != nil {
			h.t.Fatal(err)
		}
		if id := string(entry.Ticket.RequestID); request.Kind == kind && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids
}

// facts reads the facts record of a criterion through `facts show`.
func (h *harness) facts(criterion string) power.Record {
	h.t.Helper()
	code, stdout, stderr := h.run(nil, factsCommand, showVerb, criterion)
	if code != cli.ExitClean {
		h.t.Fatalf("facts show %s exited %d: %s", criterion, code, stderr)
	}
	record, err := power.DecodeRecord([]byte(stdout))
	if err != nil {
		h.t.Fatal(err)
	}
	return record
}

// conflictingOracle writes, for the shipping criterion, an example the
// reference fails.
type conflictingOracle struct{ honest oracle }

func (c conflictingOracle) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	answer, err := c.honest.Answer(persona, view)
	return strings.Replace(answer, conflictingExample, conflictedExample, 1), err
}

func TestWitnessesEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the parcel project's tests against many mutants")
	}
	shared := t.TempDir()

	t.Run("prove asks every criterion's two writers first, and nothing else"+verifiesSP1962, func(t *testing.T) {
		h := newHarness(t, shared)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("exit code %d", code)
		}
		if witnesses, scopes, others := h.pendingSchemas(); witnesses != criteriaCount*writersPerCriterion || scopes+others != 0 {
			t.Errorf("pending: %d witness, %d scope, %d other tickets", witnesses, scopes, others)
		}
	})

	t.Run("no criterion goes past its witnesses while scope judges owe an answer"+verifiesSP1966, func(t *testing.T) {
		h := newHarness(t, shared)
		h.run(nil, proveCommand, allFlag)
		if _, err := hostprototest.PlayCLI(h, hostprototest.PersonaHonest, oracle{}, func(time.Duration) {}); err != nil {
			t.Fatal(err)
		}
		h.run(nil, proveCommand, allFlag, resumeFlag)
		if witnesses, scopes, others := h.pendingSchemas(); scopes == 0 || witnesses+others != 0 {
			t.Errorf("pending: %d witness, %d scope, %d other tickets", witnesses, scopes, others)
		}
	})

	h := newHarness(t, shared)
	_, output := h.drive(hostprototest.PersonaHonest, oracle{})

	t.Run("the digest of every first-draw set is journaled once, before any generation ticket"+verifiesSP1963, func(t *testing.T) {
		store := h.hostStore()
		committed, firstGeneration := map[string]int{}, -1
		for index, record := range h.journal() {
			switch record.Event.Kind {
			case hostproto.EventWitnessSetCommitted:
				committed[record.Event.Detail]++
				if strings.Contains(record.Event.Detail, firstDrawMarker) && firstGeneration >= 0 {
					t.Errorf("a first-draw set was committed after a generation ticket: %s", record.Event.Detail)
				}
			case hostproto.EventEmitted:
				ticket, err := hostproto.ParseTicketID(record.Event.Ticket)
				if err != nil {
					t.Fatal(err)
				}
				request, err := store.GetRequest(ticket.RequestID)
				if err == nil && request.Kind == hostproto.KindGenerateTargeted && firstGeneration < 0 {
					firstGeneration = index
				}
			}
		}
		if len(committed) < criteriaCount || firstGeneration < 0 {
			t.Fatalf("%d sets committed, first generation at %d", len(committed), firstGeneration)
		}
		for detail, count := range committed {
			if count != 1 {
				t.Errorf("%s committed %d times", detail, count)
			}
		}
	})

	t.Run("each witness ran three times on the reference and the scope judges answered about every passing one"+verifiesSP1964, func(t *testing.T) {
		record := h.facts(shippingCriterion)
		passing := 0
		for _, item := range record.Witnesses {
			if len(item.Reference) != witness.ReferenceRuns {
				t.Errorf("witness %s ran %d times", item.Witness.Name, len(item.Reference))
			}
			if item.Reading() == witness.ReadingPasses {
				passing++
				if len(item.Scope) != writersPerCriterion {
					t.Errorf("witness %s has %d scope votes", item.Witness.Name, len(item.Scope))
				}
			}
		}
		if passing == 0 {
			t.Error("no witness passes the reference")
		}
	})

	t.Run("the outcome recomputed from the facts record is the proved one"+verifiesSP1977, func(t *testing.T) {
		for criterion, verdict := range verdicts(output) {
			record := h.facts(criterion)
			recomputed := power.Terminal(record.StandingFor(record.Bindings), record.Rules(""))
			if !strings.HasSuffix(verdict, string(recomputed.Result)) {
				t.Errorf("%s: proved %q, recomputed %s", criterion, verdict, recomputed.Result)
			}
		}
	})

	t.Run("facts show prints the record and refuses a criterion with none"+verifiesSP1978, func(t *testing.T) {
		if record := h.facts(shippingCriterion); record.Criterion.String() != shippingCriterion || len(record.Entries) == 0 {
			t.Errorf("record of %s with %d entries", record.Criterion, len(record.Entries))
		}
		if code, _, _ := h.run(nil, factsCommand, showVerb, unknownCriterion); code != cli.ExitBlocked {
			t.Errorf("facts show %s exited %d, want %d", unknownCriterion, code, cli.ExitBlocked)
		}
	})

	t.Run("an operator survivor that fails a block-bearing witness is in the pool"+verifiesSP1968, func(t *testing.T) {
		record := h.facts(shippingCriterion)
		rules := record.Rules("")
		universe := power.NewUniverse(record.Witnesses, rules)
		found := false
		for _, entry := range record.Entries {
			found = found || (entry.Facts.Origin == power.OriginOperator && len(universe.FailingBlockBearing(entry.Facts, record.Criterion)) > 0)
		}
		if !found {
			t.Error("no witnessed operator mutant in the pool")
		}
	})

	t.Run("two test trees with equal helpers ask the same witness, scope and generation requests"+verifiesSP1962, func(t *testing.T) {
		other := newHarness(t, shared)
		other.rewrite(testsFile, weakerAssertion, otherAssertion)
		other.drive(hostprototest.PersonaHonest, oracle{})
		for _, kind := range []hostproto.Kind{hostproto.KindGenerateWitness, hostproto.KindJudgeScope} {
			if got, want := other.requestIDsOf(kind), h.requestIDsOf(kind); !slices.Equal(got, want) || len(want) == 0 {
				t.Errorf("%s requests differ: %v and %v", kind, got, want)
			}
		}
		t.Run("the generation requests too, since the footprint comes from the witnesses"+verifiesSP1967, func(t *testing.T) {
			if got, want := other.requestIDsOf(hostproto.KindGenerateTargeted), h.requestIDsOf(hostproto.KindGenerateTargeted); !slices.Equal(got, want) || len(want) == 0 {
				t.Errorf("generation requests differ: %v and %v", got, want)
			}
		})
	})

	t.Run("a witness the reference fails is reported as a reading conflict and changes no outcome"+verifiesSP1970, func(t *testing.T) {
		conflicted := newHarness(t, shared)
		_, conflictedOutput := conflicted.drive(hostprototest.PersonaHonest, conflictingOracle{})
		if !strings.Contains(conflictedOutput, "reading_conflict: "+shippingCriterion) {
			t.Errorf("no reading conflict reported:\n%s", conflictedOutput)
		}
		if got, want := verdicts(conflictedOutput)[shippingCriterion], verdicts(output)[shippingCriterion]; got != want {
			t.Errorf("the reading conflict changed the outcome: %q, was %q", got, want)
		}
	})

	t.Run("planted refactors named in the configuration take part in admitting the witnesses"+verifiesSP1972, func(t *testing.T) {
		planted := newHarness(t, shared)
		directory := filepath.Join(planted.root, refactorsDir)
		if err := os.MkdirAll(directory, directoryMode); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, refactorName+".json"), []byte(sameThreshold), fileMode); err != nil {
			t.Fatal(err)
		}
		config, err := os.ReadFile(filepath.Join(planted.root, configFile))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(planted.root, configFile), append(config, []byte(admissionSection)...), fileMode); err != nil {
			t.Fatal(err)
		}
		planted.drive(hostprototest.PersonaHonest, oracle{})
		record := planted.facts(shippingCriterion)
		if record.Admission != witness.AdmissionWithRefactors {
			t.Fatalf("admission %q", record.Admission)
		}
		for _, item := range record.Witnesses {
			if item.Reading() == witness.ReadingPasses && item.Refactors[refactorName] != witness.OutcomePassed {
				t.Errorf("witness %s on the refactor: %q", item.Witness.Name, item.Refactors[refactorName])
			}
		}
	})
}
