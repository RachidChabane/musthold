package cli_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/musthold/discpower/internal/cli"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	verifiesSP1962 = " [verifies SP-1962~1]"
	verifiesSP1963 = " [verifies SP-1963~1]"
	verifiesSP1964 = " [verifies SP-1964~2]"
	verifiesSP1966 = " [verifies SP-1966~2]"
	verifiesSP1967 = " [verifies SP-1967~1]"
	verifiesSP1968 = " [verifies SP-1968~1]"
	verifiesSP1970 = " [verifies SP-1970~2]"
	verifiesSP1972 = " [verifies SP-1972~1]"
	verifiesSP1977 = " [verifies SP-1977~1]"
	verifiesSP1978 = " [verifies SP-1978~1]"
	verifiesSP2701 = " [verifies SP-2701~1]"

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
	configFile          = ".musthold/config.yaml"
	sameThreshold       = `{"edits":[{"file":"parcel/rates.py","qualname":"shipping_fee","new_source":"def shipping_fee(total):\n    if not total < FREE_SHIPPING_FROM:\n        return 0\n    return SHIPPING_FEE"}]}`
	admissionSection    = "admission:\n  refactors: planted\n"
	ratesFile           = "parcel/rates.py"
	inclusiveThreshold  = "if total >= FREE_SHIPPING_FROM:"
	exclusiveThreshold  = "if total > FREE_SHIPPING_FROM:"
	boundaryClause      = "reaches 50"
	boundaryWitness     = "free_from_fifty"
	boundaryAssertion   = "assert shipping_fee(50) == 0"
	// onceThenBoundary passes the first time it runs in a workspace and checks
	// the boundary every later time.
	onceThenBoundary = "assert __import__('pathlib').Path(__file__ + '.ran').exists() is False and __import__('pathlib').Path(__file__ + '.ran').touch() is None or shipping_fee(50) == 0"
	notFixed         = "NOT_FIXED"
)

var scopeLabel = regexp.MustCompile(`(?m)^Witness (w\d+), for the clause`)

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
// reference fails, and its scope judges hold that example NOT_FIXED: the spec
// fixes another output there.
type conflictingOracle struct{ honest oracle }

func (c conflictingOracle) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	answer, err := c.honest.Answer(persona, view)
	if err != nil {
		return "", err
	}
	if strings.Contains(string(view.ResponseSchema), scopeSchemaMarker) {
		return holdNotFixed(answer, view.User, conflictedExample)
	}
	return strings.Replace(answer, conflictingExample, conflictedExample, 1), nil
}

// holdNotFixed turns the scope verdict of every witness whose code holds the
// text into NOT_FIXED.
func holdNotFixed(answer, user, text string) (string, error) {
	var document struct {
		Verdicts map[string]map[string]any `json:"verdicts"`
	}
	if err := json.Unmarshal([]byte(answer), &document); err != nil {
		return "", err
	}
	matches := scopeLabel.FindAllStringSubmatchIndex(user, -1)
	for index, match := range matches {
		end := len(user)
		if index+1 < len(matches) {
			end = matches[index+1][0]
		}
		if label := user[match[2]:match[3]]; strings.Contains(user[match[0]:end], text) {
			document.Verdicts[label]["answer"] = notFixed
		}
	}
	encoded, err := json.Marshal(document)
	return string(encoded), err
}

// flakyBoundaryOracle writes the boundary witness of the shipping criterion so
// that it passes its first run in a workspace and checks the boundary in the
// others.
type flakyBoundaryOracle struct{ honest oracle }

func (f flakyBoundaryOracle) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	answer, err := f.honest.Answer(persona, view)
	return strings.Replace(answer, boundaryAssertion, onceThenBoundary, 1), err
}

// witnessNamed finds the record of the criterion's witness of the name.
func witnessNamed(t *testing.T, record power.Record, name string) witness.Record {
	t.Helper()
	index := slices.IndexFunc(record.Witnesses, func(item witness.Record) bool {
		return item.Witness.Criterion == record.Criterion && item.Witness.Name == name
	})
	if index < 0 {
		t.Fatalf("no witness %s of %s", name, record.Criterion)
	}
	return record.Witnesses[index]
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

	t.Run("the correct change lists no failed witness"+verifiesSP2701, func(t *testing.T) {
		if strings.Contains(output, string(power.FindingSpecWitnessFailed)) {
			t.Errorf("a failed witness on the correct change:\n%s", output)
		}
		for criterion := range verdicts(output) {
			if failed := h.statement(criterion).Predicate.FailedWitnesses; len(failed) != 0 {
				t.Errorf("%s lists %+v", criterion, failed)
			}
		}
	})

	t.Run("a witness the reference fails and a scope judge holds not fixed is a reading conflict and changes no outcome"+verifiesSP1970, func(t *testing.T) {
		conflicted := newHarness(t, shared)
		_, conflictedOutput := conflicted.drive(hostprototest.PersonaHonest, conflictingOracle{})
		if !strings.Contains(conflictedOutput, "reading_conflict: "+shippingCriterion) {
			t.Errorf("no reading conflict reported:\n%s", conflictedOutput)
		}
		if got, want := verdicts(conflictedOutput)[shippingCriterion], verdicts(output)[shippingCriterion]; got != want {
			t.Errorf("the reading conflict changed the outcome: %q, was %q", got, want)
		}
		if strings.Contains(conflictedOutput, string(power.FindingSpecWitnessFailed)) || len(conflicted.statement(shippingCriterion).Predicate.FailedWitnesses) != 0 {
			t.Errorf("a witness held not fixed escalated:\n%s", conflictedOutput)
		}
		t.Run("the scope judges were asked about it, blind to its failure"+verifiesSP1964, func(t *testing.T) {
			record := conflicted.facts(shippingCriterion)
			for _, item := range record.Witnesses {
				if item.Witness.Criterion == record.Criterion && item.Reading() == witness.ReadingConflict && len(item.Scope) != writersPerCriterion {
					t.Errorf("the reading conflict %s has %d scope votes", item.Witness.Name, len(item.Scope))
				}
			}
		})
	})

	t.Run("a change wrong where the spec states a rule in prose, with silent tests, escalates as spec_witness_failed"+verifiesSP2701, func(t *testing.T) {
		wrong := newHarness(t, shared)
		wrong.rewrite(ratesFile, inclusiveThreshold, exclusiveThreshold)
		code, wrongOutput := wrong.drive(hostprototest.PersonaHonest, oracle{})
		if code != cli.ExitBlocked || !slices.Contains(findings(wrongOutput, shippingCriterion), string(power.FindingSpecWitnessFailed)) {
			t.Fatalf("exit %d, findings of %s %v\n%s", code, shippingCriterion, findings(wrongOutput, shippingCriterion), wrongOutput)
		}
		shown := fmt.Sprintf("  spec witness %s, which checks %q, fails in every run", boundaryWitness, boundaryClause)
		for _, want := range []string{shown, boundaryAssertion} {
			if !strings.Contains(wrongOutput, want) {
				t.Errorf("%q is not shown:\n%s", want, wrongOutput)
			}
		}
		failed := wrong.statement(shippingCriterion).Predicate.FailedWitnesses
		if !slices.ContainsFunc(failed, func(item format.FailedWitness) bool {
			return item.Name == boundaryWitness && item.Clause == boundaryClause && strings.Contains(item.Code, boundaryAssertion)
		}) {
			t.Errorf("the Statement lists %+v", failed)
		}
		facts := wrong.facts(shippingCriterion)
		if record := witnessNamed(t, facts, boundaryWitness); !record.FailedAsFixed(facts.Rules("").ScopeRule()) {
			t.Errorf("the boundary witness: runs %v, scope %+v", record.Reference, record.Scope)
		}
		t.Run("a fresh criterion shows the same failed witness"+verifiesSP2701, func(t *testing.T) {
			_, freshOutput, _ := wrong.run(nil, proveCommand, changedFlag)
			if !strings.Contains(freshOutput, shown) || !strings.Contains(freshOutput, boundaryAssertion) {
				t.Errorf("the fresh run does not show the failed witness:\n%s", freshOutput)
			}
		})
	})

	t.Run("a witness that fails the wrong change in fewer than three runs of three never escalates"+verifiesSP2701, func(t *testing.T) {
		flaky := newHarness(t, shared)
		flaky.rewrite(ratesFile, inclusiveThreshold, exclusiveThreshold)
		_, flakyOutput := flaky.drive(hostprototest.PersonaHonest, flakyBoundaryOracle{})
		if record := witnessNamed(t, flaky.facts(shippingCriterion), boundaryWitness); record.Reading() != witness.ReadingFlaky {
			t.Fatalf("the boundary witness reads %s from %v", record.Reading(), record.Reference)
		}
		if slices.Contains(findings(flakyOutput, shippingCriterion), string(power.FindingSpecWitnessFailed)) {
			t.Errorf("a flaky witness escalated:\n%s", flakyOutput)
		}
		for _, item := range flaky.statement(shippingCriterion).Predicate.FailedWitnesses {
			if item.Name == boundaryWitness {
				t.Errorf("the flaky witness is listed: %+v", item)
			}
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
