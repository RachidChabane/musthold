package cli_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	classifySource      = "host-classify(5,rot1):" + hostprototest.ClassifySessionModel + "*5"
	floorLoweredConfig  = "policy:\n  min_isolation: restricted-unverified\n"
	exactThresholdEntry = "  tau_route:\n    default: 0.7\n    \"" + classifySource + "\": 0.9\n"
)

// classifyingHost answers like a host whose mod classifies every closed request
// it meets with the honest persona's choices, and whose deciders answer the rest.
func (h *harness) driveClassifying(answers hostprototest.AnswerSource) (code int, output string, imported []hostprototest.ImportedClassification) {
	h.t.Helper()
	args := []string{proveCommand, allFlag}
	code, output, stderr := h.run(nil, args...)
	for wave := 0; code == cli.ExitAwaitingHost && wave < h.waveLimit(); wave++ {
		imported = append(imported, h.classifyHonestly(answers)...)
		h.serveAttested(answers)
		code, output, stderr = h.run(nil, append(args, resumeFlag)...)
	}
	if code == cli.ExitToolFailure {
		h.t.Fatalf("prove failed: %s", stderr)
	}
	return code, output, imported
}

func (h *harness) classifyHonestly(answers hostprototest.AnswerSource) []hostprototest.ImportedClassification {
	h.t.Helper()
	views := map[string]hostproto.ExecutorView{}
	for _, item := range h.requestsJSON().Items {
		views[item.Ticket.TicketID.String()] = item.View
	}
	return hostprototest.ClassifyMod{Run: h.run, Model: hostprototest.ClassifySessionModel, Log: h.rawLog}.Play(h.t, func(item hostproto.ClassifyPlanItem, question string) string {
		answer, err := answers.Answer(hostprototest.PersonaHonest, views[item.Ticket])
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

// witnessing opts the project into the check of a raw API log kept in a directory,
// and has the mod's calls recorded in that log.
func (h *harness) witnessing() {
	h.t.Helper()
	dir := filepath.Join(h.t.TempDir(), "bodies")
	h.write(".spec-proof/config.yaml", "host:\n  classify:\n    witness_dir: "+dir+"\n")
	h.rawLog = hostprototest.NewRawAPILog(h.t, dir, h.now)
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

func (h *harness) admitting() {
	h.t.Helper()
	source, err := format.SingleVoterClassifySource(hostproto.ClassifyCalls, hostproto.ClassifyScheme, hostprototest.ClassifySessionModel)
	if err != nil {
		h.t.Fatal(err)
	}
	admission := hostprototest.ClassifyAdmissionOf(h.t, format.KindAlignment, source, hostprototest.ClassifySessionModel)
	h.admission = &admission
}

// classifications counts the Statements that rest on a classification and the
// records they hold.
func (h *harness) classifications() (statements, records int) {
	h.t.Helper()
	for key, statement := range h.statements() {
		decision := statement.Predicate.Extensions.Decision
		if decision == nil {
			h.t.Fatalf("%s has no decision", key)
		}
		if len(decision.Classify) > 0 {
			statements++
			records += len(decision.Classify)
			if !strings.HasPrefix(string(decision.ConfidenceSource), "host-classify(5,rot1):") || !strings.HasPrefix(decision.ConfidenceMapping, format.MappingHostClassifyPrefix) {
				h.t.Errorf("%s: decision %+v", key, decision)
			}
		}
	}
	return statements, records
}

func (h *harness) rejections() int {
	h.t.Helper()
	count := 0
	for _, record := range h.journal() {
		if record.Event.Kind == hostproto.EventRejected {
			count++
		}
	}
	return count
}

func TestClassifiedAnswersInAnAlignmentProof(t *testing.T) {
	t.Run("a classification the raw API log witnesses is credited at the default floor when its source has a threshold [verifies SP-1918~1] [verifies SP-1915~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.withConfig("policy:\n" + exactThresholdEntry)
		h.admitting()
		h.witnessing()
		code, output, imported := h.driveClassifying(testkit.Oracle{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if len(imported) == 0 || h.journalCount(hostproto.EventClassifyWitnessed) == 0 {
			t.Fatalf("imported %+v, %d witnessed", imported, h.journalCount(hostproto.EventClassifyWitnessed))
		}
		statements, records := h.classifications()
		if statements == 0 || records == 0 {
			t.Fatalf("no Statement rests on the witnessed classifications (%d, %d)", statements, records)
		}
		if h.rejections() != 0 {
			t.Errorf("%d witnessed classifications were handed back", h.rejections())
		}
	})

	t.Run("without a witness the same classification is handed back at the default floor [verifies SP-1918~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.withConfig("policy:\n" + exactThresholdEntry)
		h.admitting()
		code, output, _ := h.driveClassifying(testkit.Oracle{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if statements, _ := h.classifications(); statements != 0 || h.rejections() == 0 {
			t.Fatalf("%d Statements rest on unwitnessed classifications, %d rejections", statements, h.rejections())
		}
	})

	t.Run("an unraised classification has no power, and the host's deciders answer its request [verifies SP-1915~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.admitting()
		code, output, imported := h.driveClassifying(testkit.Oracle{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		answered := 0
		for _, outcome := range imported {
			if outcome.Outcome == "answered" {
				answered++
			}
		}
		if answered == 0 {
			t.Fatalf("nothing was classified: %+v", imported)
		}
		if statements, _ := h.classifications(); statements != 0 {
			t.Errorf("%d Statements rest on an unraised classification", statements)
		}
		if h.rejections() == 0 {
			t.Error("no classification was handed back to the deciders")
		}
	})

	t.Run("a classification at the floor with a threshold of its exact source answers, and the decision records it [verifies SP-1915~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.withConfig(floorLoweredConfig + exactThresholdEntry)
		h.admitting()
		code, output, _ := h.driveClassifying(testkit.Oracle{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statements, records := h.classifications()
		if statements == 0 || records == 0 {
			t.Fatalf("no Statement rests on a classification (%d, %d)", statements, records)
		}
		if h.rejections() != 0 {
			t.Errorf("%d classifications were handed back", h.rejections())
		}
	})

	t.Run("a classification at the floor without a threshold of its exact source is handed to the deciders [verifies SP-1915~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		h.withConfig(floorLoweredConfig + "  tau_route:\n    default: 0.7\n")
		h.admitting()
		code, output, _ := h.driveClassifying(testkit.Oracle{})
		if code != cli.ExitClean && code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if statements, _ := h.classifications(); statements != 0 {
			t.Errorf("%d Statements rest on a classification without a threshold of its source", statements)
		}
		if h.rejections() == 0 {
			t.Error("no classification was handed back to the deciders")
		}
	})

	t.Run("with the shipped, empty admission table nothing is classified [verifies SP-1915~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withPowers(testkit.PythonPowers())
		code, output, imported := h.driveClassifying(testkit.Oracle{})
		if (code != cli.ExitClean && code != cli.ExitBlocked) || len(imported) != 0 {
			t.Fatalf("exit code %d, imported %+v:\n%s", code, imported, output)
		}
		if statements, _ := h.classifications(); statements != 0 {
			t.Errorf("%d Statements rest on a classification", statements)
		}
	})
}
