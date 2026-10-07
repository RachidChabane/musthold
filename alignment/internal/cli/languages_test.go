package cli_test

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// verdictsOfStatements reads the verdict of every Statement written, by
// `criterion label`.
func verdictsOfStatements(h *harness) map[string]string {
	h.t.Helper()
	found := map[string]string{}
	for key, statement := range h.statements() {
		found[key] = string(statement.Predicate.Verdict.Value)
	}
	return found
}

// honestHostSettlesTheProject lets an honest attested host answer every ticket of a
// fixture project and checks that each test gets the verdict the matrix and the judge
// agree on, and that a corroborated ALIGNED rests on the matrix.
func honestHostSettlesTheProject(t *testing.T, language testkit.Language) {
	t.Helper()
	h := newHarness(t, language.Fixture)
	h.withPowers(language.Powers())
	code, output := h.driveAttested(testkit.Oracle{})
	if code != cli.ExitBlocked {
		t.Fatalf("exit code %d, want %d because two tests are MISALIGNED:\n%s", code, cli.ExitBlocked, output)
	}
	got, want := verdictsOfStatements(h), language.Verdicts()
	for key, verdict := range want {
		if got[key] != verdict {
			t.Errorf("%s: %q, want %q\n%s", key, got[key], verdict, output)
		}
	}
	if len(got) != len(want) {
		t.Errorf("%d Statements for %d tests", len(got), len(want))
	}
	for key, statement := range h.statements() {
		verdict := statement.Predicate.Verdict
		if verdict.Value == format.VerdictAligned && (len(verdict.CorroboratedBy) != 1 || verdict.CorroboratedBy[0].Kind != format.CorroborationSpecificMatrix) {
			t.Errorf("%s: ALIGNED without the matrix as evidence: %+v", key, verdict.CorroboratedBy)
		}
		if !statement.Predicate.Executor.Blind || len(statement.Predicate.Extensions.Matrix) == 0 {
			t.Errorf("%s: the Statement lacks its blind executor or its matrix", key)
		}
	}
}

// forgersEarnNothingTheMatrixRefutes plays the two forgers against a fixture project:
// a host that answers from the command line without a capability, whose tickets stay
// pending, and an attested host that forges the content of its answers, which earns
// no ALIGNED for a test the matrix refutes.
func forgersEarnNothingTheMatrixRefutes(t *testing.T, language testkit.Language) {
	t.Helper()
	t.Run("without a capability", func(t *testing.T) {
		h := newHarness(t, language.Fixture)
		h.withPowers(language.Powers())
		host := hostOver(h, true)
		if code, _, _ := h.run(nil, proveCommand, allFlag); code != cli.ExitAwaitingHost {
			t.Fatalf("the first prove exited %d", code)
		}
		persona, err := hostprototest.NewPersona(hostprototest.PersonaForger)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range h.requestsJSON().Items {
			host.current = item.Ticket
			request, err := host.store.GetRequest(item.Ticket.RequestID)
			if err != nil {
				t.Fatal(err)
			}
			answer, err := testkit.Oracle{}.Answer(hostprototest.PersonaHonest, item.View)
			if err != nil {
				t.Fatal(err)
			}
			errs := persona.Play(hostprototest.Env{Host: host, Request: request, Ticket: item.Ticket, Answer: json.RawMessage(answer), Out: &hostprototest.Outcome{}})
			if err := hostprototest.Check(persona, host.outcomeOf(item, errs)); err != nil {
				t.Fatalf("%s: %v", item.Ticket.TicketID, err)
			}
		}
		if code, output, _ := h.run(nil, proveCommand, allFlag, resumeFlag); code != cli.ExitAwaitingHost || len(h.statements()) != 0 {
			t.Fatalf("exit %d with %d Statements:\n%s", code, len(h.statements()), output)
		}
	})

	t.Run("with forged answers", func(t *testing.T) {
		h := newHarness(t, language.Fixture)
		h.withPowers(language.Powers())
		host := hostOver(h, true)
		driveThroughHooks(h, host, testkit.Forger{})
		got := verdictsOfStatements(h)
		if len(got) == 0 {
			t.Fatal("no verdict")
		}
		for _, refuted := range []string{
			testkit.Multiply.String() + " " + language.RejectedLine.Label,
			testkit.Surcharge.String() + " " + language.Remote.Label,
			testkit.Surcharge.String() + " " + language.Number.Label,
			testkit.Reject.String() + " " + language.ZeroRejected.Label,
		} {
			if got[refuted] == string(format.VerdictAligned) {
				t.Errorf("%s was aligned by a forger", refuted)
			}
		}
		if !slices.Contains(journalKinds(host), hostproto.EventAttested) {
			t.Error("the forging host was served through the hooks without being attested, so the test checks the wrong path")
		}
	})
}

func TestTypeScriptProject(t *testing.T) {
	t.Run("an honest host settles the tests of a Vitest project with the verdicts the matrix and the judge agree on [verifies SP-1240~1]", func(t *testing.T) {
		honestHostSettlesTheProject(t, testkit.TypeScript())
	})

	t.Run("a forger earns no ALIGNED the matrix refutes in a Vitest project [verifies SP-1241~1]", func(t *testing.T) {
		forgersEarnNothingTheMatrixRefutes(t, testkit.TypeScript())
	})
}

func TestJavaProject(t *testing.T) {
	t.Run("an honest host settles the tests of a JUnit 5 project with the verdicts the matrix and the judge agree on [verifies SP-1242~1]", func(t *testing.T) {
		honestHostSettlesTheProject(t, testkit.Java())
	})

	t.Run("a forger earns no ALIGNED the matrix refutes in a JUnit 5 project [verifies SP-1243~1]", func(t *testing.T) {
		forgersEarnNothingTheMatrixRefutes(t, testkit.Java())
	})
}
