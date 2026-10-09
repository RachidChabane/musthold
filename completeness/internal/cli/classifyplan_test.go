package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const classifyModel = "claude-haiku-4-5-20251001"

func TestSystemOneClassifyPlan(t *testing.T) {
	h := newHarness(t, testkit.PythonLedger)
	h.run(nil, proveCommand, allFlag)
	h.write(".spec-proof/config.yaml", "systemone:\n  classify: shadow\n")

	t.Run("classify-plan plans every closed request that waits for its first vote [verifies SP-1912~1]", func(t *testing.T) {
		code, stdout, stderr := h.run(nil, "systemone", "classify-plan", "--model", classifyModel)
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
				hostprototest.CheckClassifyPlan(t, items[listed], listing.Ticket, listing.View.Closed, "host-classify(5,rot1):"+classifyModel+"*5")
				listed++
			}
		}
		if listed == 0 || listed != len(items) {
			t.Fatalf("%d closed requests, %d items", listed, len(items))
		}
	})

	t.Run("classify-plan refuses a call that names neither a model nor voters [verifies SP-1912~1]", func(t *testing.T) {
		if code, stdout, stderr := h.run(nil, "systemone", "classify-plan"); code == cli.ExitClean || stdout != "" || stderr == "" {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
	})

	t.Run("classify-plan plans nothing while the switch is off or nothing is admitted [verifies SP-1912~1]", func(t *testing.T) {
		for name, config := range map[string]string{"off": "systemone:\n  classify: off\n", "auto with the shipped table": "systemone:\n  classify: auto\n", "unset": ""} {
			h.write(".spec-proof/config.yaml", config)
			if code, stdout, _ := h.run(nil, "systemone", "classify-plan", "--model", classifyModel); code != cli.ExitClean || strings.TrimSpace(stdout) != "[]" {
				t.Errorf("%s: exit %d, stdout %q", name, code, stdout)
			}
		}
	})
}

// classifyHonestly plays the host mod over the closed requests of the harness,
// every call returning the choice the honest persona would answer.
func (h *harness) classifyHonestly() []hostprototest.ImportedClassification {
	h.t.Helper()
	honest := testkit.Honest()
	views := map[string]hostproto.ExecutorView{}
	for _, item := range h.requestsJSON().Items {
		views[item.Ticket.TicketID.String()] = item.View
	}
	return hostprototest.ClassifyAsTheMod(h.t, h.run, hostprototest.ClassifySessionModel, func(item hostproto.ClassifyPlanItem, question string) string {
		answer, err := honest.Answer(hostprototest.PersonaHonest, views[item.Ticket])
		if err != nil {
			h.t.Fatal(err)
		}
		parsed, err := hostproto.ParseClosedAnswers([]byte(answer))
		if err != nil {
			h.t.Fatal(err)
		}
		return parsed[question].Choice
	})
}

func (h *harness) admitting() {
	h.t.Helper()
	source, err := format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel)
	if err != nil {
		h.t.Fatal(err)
	}
	admission := hostprototest.ClassifyAdmissionOf(h.t, format.KindCompleteness, source, hostprototest.ClassifySessionModel)
	h.admission = &admission
}

func TestClassifiedAnswersInACompletenessProof(t *testing.T) {
	t.Run("a classification replaces the host votes, carries the table's confidence and is recorded in the decision [verifies SP-1914~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		h.run(nil, proveCommand, allFlag)
		imported := h.classifyHonestly()
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		for _, outcome := range imported {
			if outcome.Outcome != "answered" {
				t.Fatalf("outcome %+v", outcome)
			}
		}
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		decision := h.mustStatement().Predicate.Extensions.Decision
		if decision == nil || decision.ConfidenceSource != "host-classify(5,rot1):"+hostprototest.ClassifySessionModel+"*5" ||
			!strings.HasPrefix(decision.ConfidenceMapping, format.MappingHostClassifyPrefix) || decision.RawConfidence != 0.99 {
			t.Fatalf("decision %+v", decision)
		}
		if len(decision.Classify) == 0 {
			t.Fatal("the decision records no classification")
		}
		for _, record := range decision.Classify {
			if len(record.Calls) == 0 || record.CLIVersion != hostprototest.ClassifyCLIVersion || record.Questions[0].SeedInput == "" {
				t.Fatalf("record %+v", record)
			}
		}
		entries, err := h.store().ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Ticket.Vote != 0 {
				t.Errorf("a host vote %s was asked next to a classification", entry.Ticket.TicketID)
			}
		}
	})

	t.Run("a classification below the gate leaves the requests to the host votes [verifies SP-1914~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		h.run(nil, proveCommand, allFlag)
		imported := hostprototest.ClassifyAsTheMod(t, h.run, hostprototest.ClassifySessionModel, func(hostproto.ClassifyPlanItem, string) string { return "" })
		for _, outcome := range imported {
			if outcome.Outcome != "fell-through" || outcome.Reason != string(hostproto.GateNoVotes) {
				t.Fatalf("outcome %+v", outcome)
			}
		}
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		decision := h.mustStatement().Predicate.Extensions.Decision
		if decision == nil || !strings.HasPrefix(string(decision.ConfidenceSource), "host-votes(") || len(decision.Classify) != 0 {
			t.Fatalf("decision %+v", decision)
		}
	})

	t.Run("in shadow the classifications are journaled and the host votes answer [verifies SP-1914~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		h.write(".spec-proof/config.yaml", "systemone:\n  classify: shadow\n")
		h.run(nil, proveCommand, allFlag)
		imported := h.classifyHonestly()
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		for _, outcome := range imported {
			if outcome.Outcome != "shadow" {
				t.Fatalf("outcome %+v", outcome)
			}
		}
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		shadowed := false
		for _, record := range h.journal() {
			shadowed = shadowed || record.Event.Kind == hostproto.EventClassifyShadow
		}
		decision := h.mustStatement().Predicate.Extensions.Decision
		if !shadowed || decision == nil || !strings.HasPrefix(string(decision.ConfidenceSource), "host-votes(") {
			t.Fatalf("shadowed %v, decision %+v", shadowed, decision)
		}
	})

	t.Run("doctor reports the switch, the admitted entries and a host version that differs from the one they were measured on [verifies SP-1914~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		h.run(nil, proveCommand, allFlag)
		h.classifyHonestly()
		if _, output, _ := h.run(nil, "doctor"); !strings.Contains(output, "ok classify mode auto, 1 admitted, host "+hostprototest.ClassifyCLIVersion) {
			t.Fatalf("doctor:\n%s", output)
		}
		admission := *h.admission
		admission.Entries[0].CLIVersion = "2.1.100"
		h.admission = &admission
		code, output, _ := h.run(nil, "doctor")
		if code != cli.ExitToolFailure || !strings.Contains(output, "fail classify") || !strings.Contains(output, "host version mismatch") {
			t.Fatalf("exit %d, doctor:\n%s", code, output)
		}
	})
}

func (h *harness) journalCount(kind hostproto.EventKind) int {
	h.t.Helper()
	count := 0
	for _, record := range h.journal() {
		if record.Event.Kind == kind {
			count++
		}
	}
	return count
}

// witnessing opts the project into the check of a raw API log kept in a directory
// and opens that log.
func (h *harness) witnessing() *hostprototest.RawAPILog {
	h.t.Helper()
	dir := filepath.Join(h.t.TempDir(), "bodies")
	h.write(".spec-proof/config.yaml", "host:\n  classify:\n    witness_dir: "+dir+"\n")
	return hostprototest.NewRawAPILog(h.t, dir, h.now)
}

func TestWitnessedClassificationsAtResume(t *testing.T) {
	pick := func(h *harness) func(hostproto.ClassifyPlanItem, string) string {
		honest := testkit.Honest()
		views := map[string]hostproto.ExecutorView{}
		for _, item := range h.requestsJSON().Items {
			views[item.Ticket.TicketID.String()] = item.View
		}
		return func(item hostproto.ClassifyPlanItem, question string) string {
			answer, err := honest.Answer(hostprototest.PersonaHonest, views[item.Ticket])
			if err != nil {
				h.t.Fatal(err)
			}
			parsed, err := hostproto.ParseClosedAnswers([]byte(answer))
			if err != nil {
				h.t.Fatal(err)
			}
			return parsed[question].Choice
		}
	}

	t.Run("a resumed run witnesses the classifications the raw API log holds and consumes the entries [verifies SP-1919~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		log := h.witnessing()
		h.run(nil, proveCommand, allFlag)
		imported := hostprototest.ClassifyMod{Run: h.run, Model: hostprototest.ClassifySessionModel, Log: log}.Play(t, pick(h))
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if witnessed := h.journalCount(hostproto.EventClassifyWitnessed); witnessed != len(imported) || h.journalCount(hostproto.EventWitnessConsumed) != len(imported) {
			t.Fatalf("%d witnessed, %d consumed for %d classifications", witnessed, h.journalCount(hostproto.EventWitnessConsumed), len(imported))
		}
	})

	t.Run("a resumed run does not witness a classification no log holds [verifies SP-1919~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		h.witnessing()
		h.run(nil, proveCommand, allFlag)
		if imported := hostprototest.ClassifyAsTheMod(t, h.run, hostprototest.ClassifySessionModel, pick(h)); len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		if code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume"); code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if witnessed := h.journalCount(hostproto.EventClassifyWitnessed); witnessed != 0 {
			t.Fatalf("%d classifications were witnessed without a log", witnessed)
		}
	})

	t.Run("a log that holds other labels contradicts the classification and raises nothing [verifies SP-1919~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.admitting()
		log := h.witnessing()
		h.run(nil, proveCommand, allFlag)
		other := func(call hostproto.ClassifiedCall, reported string) string {
			for _, label := range call.Labels {
				if label != reported {
					return label
				}
			}
			return reported
		}
		imported := hostprototest.ClassifyMod{Run: h.run, Model: hostprototest.ClassifySessionModel, Log: log, LoggedAs: other}.Play(t, pick(h))
		if len(imported) == 0 {
			t.Fatal("nothing was classified")
		}
		if code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume"); code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if h.journalCount(hostproto.EventClassifyLabelContradicted) != len(imported) || h.journalCount(hostproto.EventClassifyWitnessed) != 0 {
			t.Fatalf("%d contradicted, %d witnessed for %d classifications", h.journalCount(hostproto.EventClassifyLabelContradicted), h.journalCount(hostproto.EventClassifyWitnessed), len(imported))
		}
	})
}
