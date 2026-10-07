package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// requestKinds counts the requests the host store holds, by kind.
func (h *harness) requestKinds() map[hostproto.Kind]int {
	h.t.Helper()
	store := h.store()
	entries, err := store.ListEntries()
	if err != nil {
		h.t.Fatal(err)
	}
	seen := map[hostproto.RequestID]bool{}
	kinds := map[hostproto.Kind]int{}
	for _, entry := range entries {
		id := entry.Ticket.RequestID
		if seen[id] {
			continue
		}
		seen[id] = true
		request, err := store.GetRequest(id)
		if err != nil {
			h.t.Fatal(err)
		}
		kinds[request.Kind]++
	}
	return kinds
}

// reportWith serves a copy of the fixture's recorded report after a change.
func (h *harness) reportWith(change func(requirements []any) []any) {
	h.t.Helper()
	content, err := os.ReadFile(h.report)
	if err != nil {
		h.t.Fatal(err)
	}
	var report map[string]any
	if err := json.Unmarshal(content, &report); err != nil {
		h.t.Fatal(err)
	}
	report["requirements"] = change(report["requirements"].([]any))
	changed, err := json.Marshal(report)
	if err != nil {
		h.t.Fatal(err)
	}
	path := filepath.Join(h.t.TempDir(), "report.json")
	if err := os.WriteFile(path, changed, fileMode); err != nil {
		h.t.Fatal(err)
	}
	h.useReport(path)
}

// bind adds a test to a requirement of a report.
func bind(requirements []any, id string, test map[string]any) []any {
	for _, requirement := range requirements {
		fields := requirement.(map[string]any)
		if fields["id"] == id {
			fields["tests"] = append(fields["tests"].([]any), test)
		}
	}
	return requirements
}

func pythonTest(name string) map[string]any {
	return map[string]any{"name": name, "classname": "tests.test_pricing", "source": map[string]any{"file": "tests/test_pricing.py", "line": 1}}
}

func ticketRoles(h *harness) map[string]int {
	h.t.Helper()
	entries, err := h.store().ListEntries()
	if err != nil {
		h.t.Fatal(err)
	}
	roles := map[string]int{}
	for _, entry := range entries {
		roles[entry.Role]++
	}
	return roles
}

func TestWithoutDiscriminatingPower(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	code, output := h.driveAttested(testkit.Oracle{})

	t.Run("ALIGNED comes from the judge alone, uncorroborated, and System One is never asked [verifies SP-356~1]", func(t *testing.T) {
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		got := verdictsOf(h, output)
		for key, want := range parcelVerdicts {
			if got[key] != want {
				t.Errorf("%s: %q, want %q", key, got[key], want)
			}
		}
		for key, statement := range h.statements() {
			decision := statement.Predicate.Extensions.Decision
			if statement.Predicate.Verdict.Value != format.VerdictAligned {
				continue
			}
			if len(statement.Predicate.Verdict.CorroboratedBy) != 0 || decision.EscalationReason != "uncorroborated" || decision.Basis != format.BasisJudges {
				t.Errorf("%s: corroborated %v reason %q basis %q", key, statement.Predicate.Verdict.CorroboratedBy, decision.EscalationReason, decision.Basis)
			}
			if !slices.ContainsFunc(statement.Predicate.Inputs, func(input format.Input) bool { return input.Role == format.RoleProof }) {
				continue
			}
			t.Errorf("%s: a proof input without a proof", key)
		}
		if roles := ticketRoles(h); roles["spec-proof-alignment:decider"] != 0 {
			t.Errorf("System One was asked: %v", roles)
		}
	})

	t.Run("the matrix extension says it is absent [verifies SP-356~1]", func(t *testing.T) {
		for key, statement := range h.statements() {
			if !strings.Contains(string(statement.Predicate.Extensions.Matrix), `"ABSENT"`) {
				t.Errorf("%s: matrix %s", key, statement.Predicate.Extensions.Matrix)
			}
		}
	})
}

func TestForgedAnswers(t *testing.T) {
	t.Run("a forger never gets a corroborated ALIGNED for a test whose matrix is negative [verifies SP-357~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		_, output := h.driveAttested(testkit.Forger{})
		got := verdictsOf(h, output)
		if got["PAR-1~1 "+testkit.PyRejectedLineLabel] == "ALIGNED" {
			t.Errorf("the INERT test was aligned:\n%s", output)
		}
		if reason := results(output)["PAR-1~1 "+testkit.PyRejectedLineLabel].Reason; reason != "judge_matrix_disagreement" {
			t.Errorf("reason %q", reason)
		}
		for _, wrong := range []string{"PAR-2~1 " + testkit.PyRemoteLabel, "PAR-2~1 " + testkit.PyNumberLabel, "PAR-3~1 " + testkit.PyZeroRejectedLabel} {
			if got[wrong] == "ALIGNED" {
				t.Errorf("%s was aligned by a forger", wrong)
			}
		}
	})

	t.Run("without a discriminating-power Statement a forged ALIGNED is never corroborated [verifies SP-357~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.driveAttested(testkit.Forger{})
		aligned := 0
		for key, statement := range h.statements() {
			if statement.Predicate.Verdict.Value == format.VerdictAligned {
				aligned++
				if len(statement.Predicate.Verdict.CorroboratedBy) != 0 {
					t.Errorf("%s: corroborated without a matrix", key)
				}
			}
		}
		if aligned == 0 {
			t.Fatal("the forger aligned nothing, so the test checks nothing")
		}
	})

	t.Run("answers that break the schema are rejected, reissued and end as host_unresponsive [verifies SP-358~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.waves = 30
		code, output := h.drive(hostprototest.PersonaBreaker, hostprototest.SchemaAnswers{})
		if code == cli.ExitClean || code == cli.ExitAwaitingHost {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		for key, entry := range results(output) {
			if entry.Verdict != "UNDETERMINED" || entry.Reason != "host_unresponsive" {
				t.Errorf("%s: %s %s", key, entry.Verdict, entry.Reason)
			}
		}
		abandoned := false
		for _, record := range h.journal() {
			abandoned = abandoned || record.Event.Kind == hostproto.EventAbandoned
		}
		if !abandoned {
			t.Error("no ticket was abandoned")
		}
		pending := h.pendingFile()
		if len(pending.Pending) != 3 || pending.Pending[0].Reason != format.PendingHostUnresponsive {
			t.Errorf("pending %+v", pending)
		}
	})
}

func TestBudget(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withConfig("policy:\n  max_reasoning_calls: 3\n")
	h.withPowers(testkit.PythonPowers())
	_, output := h.driveAttested(testkit.Oracle{})

	t.Run("a test that needs more tickets than the budget is UNDETERMINED with budget_exhausted [verifies SP-358~1]", func(t *testing.T) {
		for _, key := range []string{"PAR-1~1 " + testkit.PyRejectedLineLabel, "PAR-2~1 " + testkit.PyNumberLabel} {
			entry := results(output)[key]
			if entry.Verdict != "UNDETERMINED" || entry.Reason != "budget_exhausted" {
				t.Errorf("%s: %+v", key, entry)
			}
		}
	})

	t.Run("a test that System One settles within the budget keeps its verdict [verifies SP-358~1]", func(t *testing.T) {
		if got := results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel]; got.Verdict != "ALIGNED" {
			t.Fatalf("%+v", got)
		}
	})
}

func TestAwaitingProof(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.write(".spec-proof/pending/discpower.json", `{"module":"discpower","pending":[{"criterion":"PAR-1~1","reason":"awaiting_host"}]}`)
	powers := testkit.PythonPowers()
	h.withPowers(powers[1:])
	code, output := h.driveAttested(testkit.Oracle{})

	t.Run("a test whose proof is still produced is not written and is listed as awaiting_proof [verifies SP-359~1]", func(t *testing.T) {
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		for _, label := range []string{testkit.PyMultipliesLabel, testkit.PySingleUnitLabel, testkit.PyRejectedLineLabel} {
			if got := results(output)["PAR-1~1 "+label]; got.Status != "awaiting_proof" {
				t.Errorf("%s: %+v", label, got)
			}
			if _, written := h.statements()["PAR-1~1 "+label]; written {
				t.Errorf("%s was written before its proof", label)
			}
		}
		pending := h.pendingFile()
		if len(pending.Pending) != 1 || pending.Pending[0] != (format.PendingEntry{Criterion: "PAR-1~1", Reason: format.PendingAwaitingProof}) {
			t.Fatalf("pending %+v", pending)
		}
		if _, written := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel]; !written {
			t.Error("a test that does not wait was not written")
		}
	})

	t.Run("once the proof exists the Statement is written from it [verifies SP-359~1]", func(t *testing.T) {
		h.write(".spec-proof/pending/discpower.json", `{"module":"discpower","pending":[]}`)
		powers[0].Write(t, h.root)
		code, output := h.driveAttested(testkit.Oracle{})
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statement, written := h.statements()["PAR-1~1 "+testkit.PyMultipliesLabel]
		if !written || statement.Predicate.Verdict.Value != format.VerdictAligned || len(statement.Predicate.Verdict.CorroboratedBy) != 1 {
			t.Fatalf("statement %+v written %t", statement.Predicate.Verdict, written)
		}
		if pending := h.pendingFile(); len(pending.Pending) != 0 {
			t.Fatalf("pending %+v", pending)
		}
	})
}

func TestUnmaskableTests(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.write("tests/test_leaky.py", "from parcel.pricing import shipping_cost\n\n\ndef test_remote_zone_surcharge_applies():\n    assert shipping_cost(2, \"remote\") > 0, \"remote zone surcharge applies\"\n")
	h.reportWith(func(requirements []any) []any {
		bind(requirements, "PAR-2", map[string]any{"name": "test_remote_zone_surcharge_applies", "classname": "tests.test_leaky", "source": map[string]any{"file": "tests/test_leaky.py", "line": 4}})
		return bind(requirements, "PAR-3", pythonTest("test_that_is_not_declared"))
	})
	_, output := h.driveAttested(testkit.Oracle{})

	t.Run("a test that is not declared where the report says is UNDETERMINED and no judge is asked [verifies SP-360~1]", func(t *testing.T) {
		entry := results(output)["PAR-3~1 tests.test_pricing.test_that_is_not_declared"]
		if entry.Verdict != "UNDETERMINED" || entry.Reason != "unreadable_test_source" || entry.Status != "proved" {
			t.Fatalf("%+v\n%s", entry, output)
		}
	})

	t.Run("a test whose masked input would leak is UNDETERMINED with masked_input_leaks [verifies SP-360~1]", func(t *testing.T) {
		entry := results(output)["PAR-2~1 tests.test_leaky.test_remote_zone_surcharge_applies"]
		if entry.Verdict != "UNDETERMINED" || entry.Reason != "masked_input_leaks" {
			t.Fatalf("%+v\n%s", entry, output)
		}
	})

	t.Run("the other tests are judged as before [verifies SP-360~1]", func(t *testing.T) {
		if got := results(output)["PAR-1~1 "+testkit.PyMultipliesLabel]; got.Verdict != "ALIGNED" {
			t.Fatalf("%+v", got)
		}
	})
}

// dissenting answers like the oracle, but the last decider of a closed request
// names no criterion.
type dissenting struct{}

func (dissenting) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	ticket, err := hostproto.ParseTicketID(view.Ticket)
	if err != nil {
		return "", err
	}
	if len(view.Closed) > 0 && ticket.Vote == 2 {
		return testkit.Fixed{Word: "no such requirement", Coverage: "NONE"}.Answer(persona, view)
	}
	return testkit.Oracle{}.Answer(persona, view)
}

// relaySource is the model and endpoint a relay says its answers come from.
const relaySource = "system-one:nimble:9b@0123abcd"

func TestSystemOne(t *testing.T) {
	t.Run("host deciders give the modal share of decide_k votes as the confidence [verifies SP-361~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.driveAttested(testkit.Oracle{})
		decision := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Decision
		if decision == nil || decision.Basis != format.BasisSystemOne || decision.RawConfidence != 1 || decision.Threshold != 0.9 {
			t.Fatalf("decision %+v", decision)
		}
		if !slices.Contains(decision.Deciders, "system_one") || !slices.Contains(decision.Deciders, "matrix") {
			t.Fatalf("deciders %v", decision.Deciders)
		}
		if decision.ConfidenceSource != format.HostVotesSource(3) || decision.ConfidenceMapping != format.MappingModalShare {
			t.Fatalf("confidence source %q, mapping %q", decision.ConfidenceSource, decision.ConfidenceMapping)
		}
	})

	t.Run("a dissenting decider lowers the confidence below the route threshold and the judge decides [verifies SP-361~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.driveAttested(dissenting{})
		statement := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		if statement.Predicate.Verdict.Value != format.VerdictAligned || statement.Predicate.Extensions.Decision.Basis != format.BasisJudges || len(statement.Predicate.Verdict.CorroboratedBy) != 1 {
			t.Fatalf("statement %+v", statement.Predicate)
		}
	})

	t.Run("a relayed answer replaces the host votes and carries the endpoint's confidence [verifies SP-361~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.run(nil, proveCommand, allFlag)
		relayed := h.relay(testkit.Oracle{}, 0.93)
		if relayed == 0 {
			t.Fatal("no closed request was relayed")
		}
		code, output := h.driveAttested(testkit.Oracle{}, "--resume")
		if code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statement := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		decision := statement.Predicate.Extensions.Decision
		if decision.Basis != format.BasisSystemOne || decision.RawConfidence != 0.93 || statement.Predicate.Executor.Channel != format.ChannelSystemOneRelay {
			t.Fatalf("decision %+v executor %+v", decision, statement.Predicate.Executor.ExecutorProfile)
		}
		if decision.ConfidenceSource != "system-one:nimble:9b@0123abcd" || decision.ConfidenceMapping != format.MappingSystemOneChoice {
			t.Fatalf("decision %+v does not name the relay source and its mapping", decision)
		}
		if slices.ContainsFunc(decision.Deciders, func(decider string) bool { return strings.Contains(decider, relaySource) }) {
			t.Fatalf("deciders %v carry the confidence source", decision.Deciders)
		}
		entries, err := h.store().ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Role == "spec-proof-alignment:decider" && entry.Ticket.Vote != 0 {
				t.Errorf("a host vote %s was asked next to a relayed answer", entry.Ticket.TicketID)
			}
		}
	})

	t.Run("a relayed answer below the route threshold sends the test to the judge [verifies SP-361~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.run(nil, proveCommand, allFlag)
		h.relay(testkit.Oracle{}, 0.5)
		h.driveAttested(testkit.Oracle{}, "--resume")
		decision := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Decision
		if decision.Basis != format.BasisJudges {
			t.Fatalf("decision %+v", decision)
		}
	})
}

// relay plays a System One relay: it exports the closed requests, answers each
// with the source's reading at the confidence and imports the answers. It
// returns the number of requests relayed.
func (h *harness) relay(answers hostprototest.AnswerSource, confidence float64) int {
	h.t.Helper()
	code, exported, stderr := h.run(nil, "systemone", "export")
	if code != cli.ExitClean {
		h.t.Fatalf("export: %s", stderr)
	}
	var requests []struct {
		Ticket string `json:"ticket"`
		Nonce  string `json:"nonce"`
		Closed json.RawMessage
	}
	if err := json.Unmarshal([]byte(exported), &requests); err != nil {
		h.t.Fatal(err)
	}
	for _, request := range requests {
		view := hostproto.ExecutorView{Ticket: request.Ticket, Closed: request.Closed}
		for _, item := range h.requestsJSON().Items {
			if item.Ticket.TicketID.String() == request.Ticket {
				view = item.View
			}
		}
		answer, err := answers.Answer(hostprototest.PersonaHonest, view)
		if err != nil {
			h.t.Fatal(err)
		}
		set, err := hostproto.ParseClosedSet(request.Closed)
		if err != nil {
			h.t.Fatal(err)
		}
		picked, err := hostproto.ParseClosedAnswers([]byte(answer))
		if err != nil {
			h.t.Fatal(err)
		}
		response, err := hostprototest.SystemOneResponse(set, picked, confidence)
		if err != nil {
			h.t.Fatal(err)
		}
		code, _, stderr := h.run(response, "systemone", "import", request.Ticket, "--nonce", request.Nonce, "--source", relaySource)
		if code != cli.ExitClean {
			h.t.Fatalf("import %s: %s", request.Ticket, stderr)
		}
	}
	return len(requests)
}

func TestShadowAudit(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withConfig("policy:\n  shadow_rate: 1\n")
	h.withPowers(testkit.PythonPowers())
	_, output := h.driveAttested(auditing{})

	t.Run("an audited two-signal verdict is replaced by the judge's outcome [verifies SP-362~1]", func(t *testing.T) {
		entry := results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel]
		if entry.Verdict != "MISALIGNED" {
			t.Fatalf("%+v\n%s", entry, output)
		}
		decision := h.statements()["PAR-3~1 "+testkit.PyZeroRejectedLabel].Predicate.Extensions.Decision
		if decision.Basis != format.BasisJudges || !slices.Contains(decision.Deciders, "shadow_audit") {
			t.Fatalf("decision %+v", decision)
		}
		for key, statement := range h.statements() {
			if basis := statement.Predicate.Extensions.Decision.Basis; basis == format.BasisSystemOne {
				t.Errorf("%s was decided by System One although every verdict is audited", key)
			}
		}
	})
}

// auditing lets System One support every test and the judge name nothing.
type auditing struct{}

func (auditing) Answer(persona string, view hostproto.ExecutorView) (string, error) {
	if len(view.Closed) > 0 {
		return testkit.Oracle{}.Answer(persona, view)
	}
	return testkit.Fixed{Word: "no such requirement", Coverage: "NONE"}.Answer(persona, view)
}

func TestMarkedAnswers(t *testing.T) {
	t.Run("answers submitted outside an executor lose their pass credit and keep their block power [verifies SP-308~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.waves = 20
		_, output := h.drive(hostprototest.PersonaHonest, testkit.Oracle{})
		got := results(output)
		if entry := got["PAR-3~1 "+testkit.PyZeroRejectedLabel]; entry.Verdict != "UNDETERMINED" || entry.Reason != "isolation_below_floor" {
			t.Errorf("an exposed answer aligned a test: %+v\n%s", entry, output)
		}
		if entry := got["PAR-2~1 "+testkit.PyNumberLabel]; entry.Verdict != "MISALIGNED" {
			t.Errorf("an exposed answer lost its block power: %+v", entry)
		}
	})

	t.Run("lowering the isolation floor to inline does not restore the credit of an exposed answer [verifies SP-308~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withConfig("policy:\n  min_isolation: inline\n")
		h.withPowers(testkit.PythonPowers())
		h.waves = 20
		_, output := h.drive(hostprototest.PersonaHonest, testkit.Oracle{})
		if entry := results(output)["PAR-3~1 "+testkit.PyZeroRejectedLabel]; entry.Verdict == "ALIGNED" {
			t.Fatalf("an exposed answer aligned a test: %+v", entry)
		}
	})
}
