package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/completeness/internal/cli"
	"github.com/RachidChabane/musthold/completeness/internal/testkit"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// ledgerCase is what an honest host finds in a fixture project.
type ledgerCase struct {
	fixture     string
	evidence    func() (testkit.Power, testkit.Aligned)
	unjustified []string
	withHunks   int
	withoutHunk int
}

var ledgerCases = []ledgerCase{
	{fixture: testkit.PythonLedger, evidence: testkit.PythonEvidence, unjustified: []string{"apply_discount", "receipt_line"}, withHunks: 2, withoutHunk: 5},
	{fixture: testkit.GoLedger, evidence: testkit.GoEvidence, unjustified: []string{"ApplyDiscount"}, withHunks: 1, withoutHunk: 4},
	{fixture: testkit.TypeScriptLedger, evidence: testkit.TypeScriptEvidence, unjustified: []string{"applyDiscount", "receiptLine"}, withHunks: 2, withoutHunk: 5},
	{fixture: testkit.JavaLedger, evidence: testkit.JavaEvidence, unjustified: []string{"Totals.applyDiscount", "Receipts.receiptLine"}, withHunks: 2, withoutHunk: 5},
}

func hunkRequests(h *harness) int {
	h.t.Helper()
	store := h.store()
	entries, err := store.ListEntries()
	if err != nil {
		h.t.Fatal(err)
	}
	seen := map[hostproto.RequestID]bool{}
	count := 0
	for _, entry := range entries {
		if seen[entry.Ticket.RequestID] {
			continue
		}
		seen[entry.Ticket.RequestID] = true
		request, err := store.GetRequest(entry.Ticket.RequestID)
		if err != nil {
			h.t.Fatal(err)
		}
		if strings.Contains(string(request.Closed), `"id":"criterion"`) {
			count++
		}
	}
	return count
}

func TestWithEvidence(t *testing.T) {
	for _, l := range ledgerCases {
		h := newHarness(t, l.fixture)
		power, aligned := l.evidence()
		h.withEvidence(power, aligned)
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest())

		t.Run(l.fixture+": an honest host and the Statements of the other modules give the advisory Statement of the spec [verifies SP-548~1]", func(t *testing.T) {
			if code != cli.ExitClean {
				t.Fatalf("exit code %d:\n%s", code, output)
			}
			statement := h.mustStatement()
			if !statement.Predicate.Advisory || statement.Subject[0].Name != "specs/invoicing.md" {
				t.Fatalf("statement %+v", statement)
			}
			if got := hunkRequests(h); got != l.withHunks {
				t.Errorf("%d hunk requests, want %d", got, l.withHunks)
			}
			for _, name := range l.unjustified {
				found := false
				for _, line := range questionLines(statement) {
					found = found || strings.Contains(line, "unjustified_change: The change to "+name+" in ")
				}
				if !found {
					t.Errorf("no unjustified_change question about %s: %v", name, questionLines(statement))
				}
			}
			if got := questionsOf(statement, "unconstrained_behaviour"); len(got) != 1 {
				t.Errorf("unconstrained behaviour %+v", got)
			}
			if got := questionsOf(statement, "clarity"); len(got) != 1 || !slices.Equal(got[0].RequirementIDs, []string{"INV-2"}) {
				t.Errorf("clarity %+v", got)
			}
			if pending := h.pendingFile(); len(pending.Pending) != 0 {
				t.Errorf("pending %+v", pending)
			}
		})
	}
}

func TestWithoutEvidence(t *testing.T) {
	for _, l := range ledgerCases {
		h := newHarness(t, l.fixture)
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest())

		t.Run(l.fixture+": without the Statements of the other modules every changed unit is asked about and nothing is called unconstrained [verifies SP-549~1]", func(t *testing.T) {
			if code != cli.ExitClean {
				t.Fatalf("exit code %d:\n%s", code, output)
			}
			if got := hunkRequests(h); got != l.withoutHunk {
				t.Errorf("%d hunk requests, want %d", got, l.withoutHunk)
			}
			statement := h.mustStatement()
			if got := questionsOf(statement, "unconstrained_behaviour"); len(got) != 0 {
				t.Errorf("unconstrained behaviour %+v", got)
			}
			for _, input := range statement.Predicate.Inputs {
				if input.Role == format.RoleProof {
					t.Errorf("proof input %q", input.Name)
				}
			}
			if got := questionsOf(statement, "unjustified_change"); len(got) != len(l.unjustified) {
				t.Errorf("unjustified changes %+v", got)
			}
		})
	}
}

func TestNoRepository(t *testing.T) {
	t.Run("a project outside version control still gets the dimension and testability questions and says why the diff is missing [verifies SP-507~1]", func(t *testing.T) {
		h := newHarness(t, testkit.GoLedger)
		h.root = testkit.MaterializeWithoutRepository(t, testkit.GoLedger)
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest())
		if code != cli.ExitClean || !strings.Contains(output, "module finding no_base:") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statement := h.mustStatement()
		if got := questionsOf(statement, "unjustified_change"); len(got) != 0 {
			t.Fatalf("unjustified changes %+v", got)
		}
		if len(questionsOf(statement, "error_handling")) != 1 {
			t.Fatalf("questions %v", questionLines(statement))
		}
	})

	t.Run("a base given by flag is measured from, and an unknown one is reported as no base [verifies SP-507~1]", func(t *testing.T) {
		h := newHarness(t, testkit.GoLedger)
		testkit.Git(t, h.root, "add", "-A")
		testkit.Git(t, h.root, "commit", "-q", "-m", "change")
		code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest(), "--base", "HEAD~1")
		if code != cli.ExitClean || hunkRequests(h) != 4 {
			t.Fatalf("exit code %d, %d hunk requests:\n%s", code, hunkRequests(h), output)
		}
		unknown := newHarness(t, testkit.GoLedger)
		code, output = unknown.drive(hostprototest.PersonaHonest, testkit.Honest(), "--base", "no-such-ref")
		if code != cli.ExitClean || !strings.Contains(output, "no_base:") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
	})
}

func TestPersonas(t *testing.T) {
	t.Run("a breaker, whose answers break the schema, is rejected and ends as host_unresponsive without a question made up for it [verifies SP-550~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.waves = 8
		code, output := h.drive(hostprototest.PersonaBreaker, hostprototest.SchemaAnswers{})
		if code != cli.ExitBlocked || !strings.Contains(output, "host_unresponsive") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		statement := h.mustStatement()
		if len(statement.Predicate.Questions) != 0 || statement.Predicate.Extensions.Decision != nil {
			t.Errorf("the Statement rests on answers nobody gave: %+v", statement.Predicate)
		}
		abandoned := false
		for _, record := range h.journal() {
			abandoned = abandoned || record.Event.Kind == hostproto.EventAbandoned
		}
		if !abandoned {
			t.Error("no ticket was abandoned")
		}
		pending := h.pendingFile()
		if len(pending.Pending) != 2 || pending.Pending[0].Reason != format.PendingHostUnresponsive {
			t.Errorf("pending %+v", pending)
		}
	})

	t.Run("a forger that submits without fetching has no answer accepted and the run keeps waiting [verifies SP-550~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.waves = 2
		code, _ := h.drive(hostprototest.PersonaForger, hostprototest.SchemaAnswers{})
		if code != cli.ExitAwaitingHost || len(h.statementBytes()) != 0 {
			t.Fatalf("exit code %d with %d Statements", code, len(h.statementBytes()))
		}
		for _, record := range h.journal() {
			if record.Event.Kind == hostproto.EventAccepted {
				t.Fatal("a forged answer was accepted")
			}
		}
	})

	t.Run("an omitter never answers: the run keeps waiting, writes nothing and lists the criteria as awaiting_host [verifies SP-550~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.waves = 3
		code, output := h.drive(hostprototest.PersonaOmitter, hostprototest.SchemaAnswers{})
		if code != cli.ExitAwaitingHost || !strings.Contains(output, "awaiting_host") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if len(h.statementBytes()) != 0 {
			t.Fatal("a Statement was written for work nobody answered")
		}
		if pending := h.pendingFile(); len(pending.Pending) != 2 || pending.Pending[0].Reason != format.PendingAwaitingHost {
			t.Fatalf("pending %+v", pending)
		}
	})

	t.Run("a stale host answering with old nonces never gets an answer accepted [verifies SP-550~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.waves = 3
		code, _ := h.drive(hostprototest.PersonaStale, testkit.Honest())
		if code == cli.ExitClean {
			t.Fatal("answers with a stale nonce were accepted")
		}
		accepted := false
		for _, record := range h.journal() {
			accepted = accepted || record.Event.Kind == hostproto.EventAccepted
		}
		if accepted {
			t.Fatal("a stale answer was accepted")
		}
	})

	t.Run("every persona leaves only valid advisory Statements with no verdict [verifies SP-550~1]", func(t *testing.T) {
		for _, persona := range []string{hostprototest.PersonaHonest, hostprototest.PersonaSelector, hostprototest.PersonaSlow, hostprototest.PersonaChurner} {
			h := newHarness(t, testkit.GoLedger)
			h.waves = 40
			h.drive(persona, testkit.Honest())
			for path, content := range h.statementBytes() {
				if strings.Contains(content, `"verdict"`) {
					t.Errorf("%s: %s carries a verdict", persona, path)
				}
			}
			if statement, ok := h.statement(); ok && !statement.Predicate.Advisory {
				t.Errorf("%s: the Statement is not advisory", persona)
			}
		}
	})
}

func TestAnswerOrder(t *testing.T) {
	t.Run("the host may answer the tickets in any order and the Statement is the same [verifies SP-551~1]", func(t *testing.T) {
		forward := newHarness(t, testkit.PythonLedger)
		power, aligned := testkit.PythonEvidence()
		forward.withEvidence(power, aligned)
		forward.driveInOrder(testkit.Honest())
		reversed := newHarness(t, testkit.PythonLedger)
		reversed.withEvidence(power, aligned)
		reversed.reverse = true
		reversed.driveInOrder(testkit.Honest())
		got, want := reversed.statementBytes(), forward.statementBytes()
		if len(want) != 1 || len(got) != 1 {
			t.Fatalf("%d and %d Statements", len(got), len(want))
		}
		for path, content := range want {
			if got[path] != content {
				t.Fatalf("the Statement differs when the host answers in reverse order")
			}
		}
	})
}

func TestRecordAndReplay(t *testing.T) {
	t.Run("replaying the recorded answers gives byte-identical Statements, in one command [verifies SP-552~1]", func(t *testing.T) {
		recordings := t.TempDir()
		recorder := newHarness(t, testkit.PythonLedger)
		power, aligned := testkit.PythonEvidence()
		recorder.withEvidence(power, aligned)
		recorder.drive(hostprototest.PersonaHonest, testkit.Honest(), "--record", recordings)
		want := recorder.statementBytes()

		replayer := newHarness(t, testkit.PythonLedger)
		replayer.withEvidence(power, aligned)
		code, output, stderr := replayer.run(nil, proveCommand, allFlag, "--replay", recordings)
		if code != cli.ExitClean {
			t.Fatalf("exit code %d: %s\n%s", code, stderr, output)
		}
		got := replayer.statementBytes()
		if len(got) != 1 || len(want) != 1 {
			t.Fatalf("%d Statements, want %d", len(got), len(want))
		}
		for path, content := range want {
			if got[path] != content {
				t.Errorf("%s differs after the replay", path)
			}
		}
		if listing := replayer.requestsJSON(); len(listing.Items) != 0 {
			t.Errorf("%d tickets are still pending after the replay", len(listing.Items))
		}
	})

	t.Run("a request without a recording is a hard error [verifies SP-552~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, _, stderr := h.run(nil, proveCommand, allFlag, "--replay", t.TempDir())
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "no recorded answer") {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
	})
}
