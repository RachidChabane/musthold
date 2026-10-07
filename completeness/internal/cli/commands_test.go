package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/cli"
	"github.com/RachidChabane/spec-proof/completeness/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

func TestProveExitCodes(t *testing.T) {
	h := newHarness(t, testkit.PythonLedger)

	t.Run("prove exits 3 while tickets wait and gives the same requests when run again [verifies SP-543~1]", func(t *testing.T) {
		code, output, _ := h.run(nil, proveCommand, allFlag)
		if code != cli.ExitAwaitingHost || !strings.Contains(output, "awaiting_host") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		first := h.requestsJSON()
		code, _, _ = h.run(nil, proveCommand, allFlag)
		second := h.requestsJSON()
		if code != cli.ExitAwaitingHost || len(first.Items) == 0 || len(first.Items) != len(second.Items) {
			t.Fatalf("exit code %d, %d then %d tickets", code, len(first.Items), len(second.Items))
		}
		for index, item := range first.Items {
			if item.Ticket != second.Items[index].Ticket {
				t.Errorf("ticket %d changed between two runs", index)
			}
		}
	})

	t.Run("prove exits 0 once the host answered, whatever questions the Statement holds [verifies SP-543~1]", func(t *testing.T) {
		answered := newHarness(t, testkit.GoLedger)
		code, output := answered.drive(hostprototest.PersonaHonest, testkit.Honest())
		if code != cli.ExitClean || len(answered.mustStatement().Predicate.Questions) == 0 {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
	})

	t.Run("prove --resume is refused when no request was ever emitted [verifies SP-543~1]", func(t *testing.T) {
		fresh := newHarness(t, testkit.PythonLedger)
		code, _, stderr := fresh.run(nil, proveCommand, resumeFlag)
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "nothing to resume") {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
	})
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t, testkit.PythonLedger)

	t.Run("conflicting flags and unexpected arguments fail without acting [verifies SP-544~1]", func(t *testing.T) {
		for name, args := range map[string][]string{
			"changed and all":      {proveCommand, changedFlag, allFlag},
			"record and replay":    {proveCommand, "--record", "a", "--replay", "b"},
			"an unexpected value":  {proveCommand, "extra"},
			"an unknown flag":      {proveCommand, "--frobnicate"},
			"doctor with an extra": {"doctor", "extra"},
		} {
			code, _, stderr := h.run(nil, args...)
			if code != cli.ExitToolFailure || stderr == "" {
				t.Errorf("%s: exit code %d, stderr %q", name, code, stderr)
			}
		}
		if listing := h.requestsJSON(); len(listing.Items) != 0 {
			t.Errorf("a refused command emitted %d tickets", len(listing.Items))
		}
	})

	t.Run("an unknown command and no command are tool failures with a usage line [verifies SP-544~1]", func(t *testing.T) {
		for _, args := range [][]string{{"frobnicate"}, {}} {
			code, _, stderr := h.run(nil, args...)
			if code != cli.ExitToolFailure || !strings.Contains(stderr, "usage:") {
				t.Errorf("%v: exit code %d, stderr %q", args, code, stderr)
			}
		}
	})
}

func TestRequestCommands(t *testing.T) {
	h := newHarness(t, testkit.PythonLedger)
	h.run(nil, proveCommand, allFlag)
	item := h.requestsJSON().Items[0]
	ticket := item.Ticket.TicketID.String()

	t.Run("request list prints the tickets in state emitted with the role and the view of an executor [verifies SP-545~1]", func(t *testing.T) {
		if item.Role != "spec-proof-completeness:decider" || item.View.User == "" || item.View.Ticket != ticket || item.View.Nonce != item.Ticket.Nonce || len(item.View.ResponseSchema) == 0 || len(item.View.Closed) == 0 {
			t.Fatalf("item %+v", item)
		}
		code, text, _ := h.run(nil, "request", "list")
		if code != cli.ExitClean || !strings.Contains(text, ticket) {
			t.Fatalf("exit code %d:\n%s", code, text)
		}
	})

	t.Run("request show moves the ticket to fetched [verifies SP-545~1]", func(t *testing.T) {
		code, output, stderr := h.run(nil, "request", "show", ticket)
		if code != cli.ExitClean {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
		var view hostproto.ExecutorView
		if err := json.Unmarshal([]byte(output), &view); err != nil || view.Ticket != ticket {
			t.Fatalf("view %+v: %v", view, err)
		}
		for _, listed := range h.requestsJSON().Items {
			if listed.Ticket.TicketID.String() == ticket {
				t.Errorf("a fetched ticket is still listed")
			}
		}
	})

	t.Run("a wrong nonce, a schema violation and an unknown ticket are refusals, not tool failures [verifies SP-545~1]", func(t *testing.T) {
		for name, run := range map[string]func() (int, string){
			"a wrong nonce": func() (int, string) {
				code, _, stderr := h.run([]byte(`{"answers":[]}`), "request", "submit", ticket, "--nonce", strings.Repeat("0", 32))
				return code, stderr
			},
			"a schema violation": func() (int, string) {
				code, _, stderr := h.run([]byte(`{}`), "request", "submit", ticket, "--nonce", item.Ticket.Nonce)
				return code, stderr
			},
			"an unknown ticket": func() (int, string) {
				code, _, stderr := h.run(nil, "request", "show", strings.Repeat("a", 64)+".0.1")
				return code, stderr
			},
			"a malformed ticket": func() (int, string) {
				code, _, stderr := h.run(nil, "request", "show", "not-a-ticket")
				return code, stderr
			},
			"no verb": func() (int, string) {
				code, _, stderr := h.run(nil, "request")
				return code, stderr
			},
			"an unknown verb": func() (int, string) {
				code, _, stderr := h.run(nil, "request", "frobnicate")
				return code, stderr
			},
		} {
			if code, stderr := run(); code != cli.ExitBlocked || stderr == "" {
				t.Errorf("%s: exit code %d, stderr %q", name, code, stderr)
			}
		}
	})
}

// relay plays a System One relay: it exports the closed requests, answers each
// with the honest reading at the confidence and imports the answers. It returns
// the number of requests relayed.
func (h *harness) relay(confidence float64) int {
	h.t.Helper()
	code, exported, stderr := h.run(nil, "systemone", "export")
	if code != cli.ExitClean {
		h.t.Fatalf("export: %s", stderr)
	}
	var requests []struct {
		Ticket string `json:"ticket"`
		Nonce  string `json:"nonce"`
	}
	if err := json.Unmarshal([]byte(exported), &requests); err != nil {
		h.t.Fatal(err)
	}
	answers := testkit.Honest()
	for _, request := range requests {
		var view hostproto.ExecutorView
		for _, item := range h.requestsJSON().Items {
			if item.Ticket.TicketID.String() == request.Ticket {
				view = item.View
			}
		}
		answer, err := answers.Answer(hostprototest.PersonaHonest, view)
		if err != nil {
			h.t.Fatal(err)
		}
		answer = withConfidence(h.t, answer, confidence)
		code, _, stderr := h.run([]byte(answer), "systemone", "import", request.Ticket, "--nonce", request.Nonce, "--source", "systemone:nimble@0123abcd")
		if code != cli.ExitClean {
			h.t.Fatalf("import %s: %s", request.Ticket, stderr)
		}
	}
	return len(requests)
}

// withConfidence states one confidence for every answer of an output.
func withConfidence(t testing.TB, output string, confidence float64) string {
	t.Helper()
	var document struct {
		Answers []map[string]any `json:"answers"`
	}
	if err := json.Unmarshal([]byte(output), &document); err != nil {
		t.Fatal(err)
	}
	for _, answer := range document.Answers {
		answer["confidence"] = confidence
	}
	rewritten, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return string(rewritten)
}

func TestSystemOneCommands(t *testing.T) {
	h := newHarness(t, testkit.PythonLedger)
	h.run(nil, proveCommand, allFlag)

	t.Run("systemone export lists the closed requests that wait for their first vote [verifies SP-546~1]", func(t *testing.T) {
		code, output, stderr := h.run(nil, "systemone", "export")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
		var exported []struct {
			Ticket string          `json:"ticket"`
			Nonce  string          `json:"nonce"`
			Closed json.RawMessage `json:"closed"`
		}
		if err := json.Unmarshal([]byte(output), &exported); err != nil || len(exported) == 0 {
			t.Fatalf("export %q: %v", output, err)
		}
		if len(exported) != len(h.requestsJSON().Items) {
			t.Errorf("%d exported, %d tickets", len(exported), len(h.requestsJSON().Items))
		}
	})

	t.Run("systemone import needs a source and a known ticket and records the answer under the relay channel [verifies SP-546~1]", func(t *testing.T) {
		unknown := strings.Repeat("a", 64) + ".0.1"
		if code, _, _ := h.run([]byte(`{}`), "systemone", "import", unknown, "--nonce", strings.Repeat("0", 32)); code != cli.ExitBlocked {
			t.Errorf("an import without a source: exit code %d", code)
		}
		if code, _, _ := h.run([]byte(`{}`), "systemone", "import", unknown, "--nonce", strings.Repeat("0", 32), "--source", "s1:x@y"); code != cli.ExitBlocked {
			t.Errorf("an import for an unknown ticket: exit code %d", code)
		}
		if code, _, _ := h.run(nil, "systemone"); code != cli.ExitBlocked {
			t.Errorf("no verb: exit code %d", code)
		}
		if code, _, _ := h.run(nil, "systemone", "frobnicate"); code != cli.ExitBlocked {
			t.Errorf("an unknown verb: exit code %d", code)
		}
		if h.relay(0.92) == 0 {
			t.Fatal("nothing was relayed")
		}
		relayed := false
		for _, record := range h.journal() {
			relayed = relayed || record.Event.Kind == hostproto.EventAccepted
		}
		if !relayed {
			t.Error("no answer was accepted")
		}
	})

	t.Run("a relayed answer replaces the host votes, carries the endpoint's confidence and names its source [verifies SP-546~1]", func(t *testing.T) {
		relayed := newHarness(t, testkit.PythonLedger)
		relayed.run(nil, proveCommand, allFlag)
		if relayed.relay(0.93) == 0 {
			t.Fatal("nothing was relayed")
		}
		code, output := relayed.drive(hostprototest.PersonaHonest, testkit.Honest(), "--resume")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		decision := relayed.mustStatement().Predicate.Extensions.Decision
		if decision == nil || decision.RawConfidence != 0.93 || len(decision.Deciders) == 0 || decision.Deciders[0] != "system_one:systemone:nimble@0123abcd" {
			t.Fatalf("decision %+v", decision)
		}
		entries, err := relayed.store().ListEntries()
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Ticket.Vote != 0 {
				t.Errorf("a host vote %s was asked next to a relayed answer", entry.Ticket.TicketID)
			}
		}
	})
}

func TestDoctor(t *testing.T) {
	t.Run("doctor checks Shallnot, git, the configuration, the host store and the journal [verifies SP-547~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		code, output, stderr := h.run(nil, "doctor")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d: %s\n%s", code, stderr, output)
		}
		for _, name := range []string{"ok shallnot", "ok git", "ok policy", "ok journal"} {
			if !strings.Contains(output, name) {
				t.Errorf("the report lacks %q:\n%s", name, output)
			}
		}
	})

	t.Run("doctor exits 2 when a check fails [verifies SP-547~1]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonLedger)
		h.withConfig("policy:\n  decide_k: 0\n")
		code, output, _ := h.run(nil, "doctor")
		if code != cli.ExitToolFailure || !strings.Contains(output, "fail project") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
	})
}
