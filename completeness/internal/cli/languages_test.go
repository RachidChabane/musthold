package cli_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// honestHostAdvisesTheProject lets an honest host answer every ticket of a fixture
// project with its evidence and checks the advisory Statement it gets: a question
// for each changed unit that no probe and no test may execute, one for the
// behaviour no requirement owns and one for the requirement that is vague.
func honestHostAdvisesTheProject(t *testing.T, l ledgerCase) {
	t.Helper()
	h := newHarness(t, l.fixture)
	power, aligned := l.evidence()
	h.withEvidence(power, aligned)
	code, output := h.drive(hostprototest.PersonaHonest, testkit.Honest())
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
		if !slices.ContainsFunc(questionLines(statement), func(line string) bool {
			return strings.Contains(line, "unjustified_change: The change to "+name+" in ")
		}) {
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
}

// forgersGetNoVerdict plays the two forgers against a fixture project: a host that
// answers from the command line without a capability, whose tickets stay pending, and
// an attested host that forges the content of its answers, which gets an advisory
// Statement that carries no verdict.
func forgersGetNoVerdict(t *testing.T, l ledgerCase) {
	t.Helper()
	t.Run("without a capability", func(t *testing.T) {
		h := newHarness(t, l.fixture)
		host := hostOver(h, true)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		persona, err := hostprototest.NewPersona(hostprototest.PersonaForger)
		if err != nil {
			t.Fatal(err)
		}
		playOverPendingTickets(h, host, persona)
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statementBytes()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statementBytes()), output)
		}
	})

	t.Run("with forged answers", func(t *testing.T) {
		h := newHarness(t, l.fixture)
		host := hostOver(h, true)
		code, output := driveThroughHooks(h, host, forgingHost())
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		for path, content := range h.statementBytes() {
			if strings.Contains(content, `"verdict"`) {
				t.Errorf("%s carries a verdict", path)
			}
		}
		if statement := h.mustStatement(); !statement.Predicate.Advisory {
			t.Errorf("the Statement is not advisory: %+v", statement.Predicate)
		}
		if !slices.Contains(journalKinds(host), hostproto.EventAttested) {
			t.Error("the forging host was served through the hooks without being attested, so the test checks the wrong path")
		}
		for _, executor := range acceptedExecutors(h) {
			if executor.Isolation != format.IsolationRestricted || executor.Attestation.AgentID == "" {
				t.Errorf("executor %+v", executor)
			}
		}
	})
}

func ledgerOf(fixture string) ledgerCase {
	for _, l := range ledgerCases {
		if l.fixture == fixture {
			return l
		}
	}
	panic("no ledger case for " + fixture)
}

func TestTypeScriptLedger(t *testing.T) {
	l := ledgerOf(testkit.TypeScriptLedger)
	t.Run("an honest host advises on the change of a Vitest project as it does for a Python project [verifies SP-1320~1]", func(t *testing.T) {
		honestHostAdvisesTheProject(t, l)
	})
	t.Run("a forger gets no answer accepted and no verdict in a Vitest project [verifies SP-1321~1]", func(t *testing.T) {
		forgersGetNoVerdict(t, l)
	})
}

func TestJavaLedger(t *testing.T) {
	l := ledgerOf(testkit.JavaLedger)
	t.Run("an honest host advises on the change of a JUnit 5 project as it does for a Python project [verifies SP-1322~1]", func(t *testing.T) {
		honestHostAdvisesTheProject(t, l)
	})
	t.Run("a forger gets no answer accepted and no verdict in a JUnit 5 project [verifies SP-1323~1]", func(t *testing.T) {
		forgersGetNoVerdict(t, l)
	})
}
