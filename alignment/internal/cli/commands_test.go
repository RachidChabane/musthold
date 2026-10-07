package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/cli"
	"github.com/RachidChabane/spec-proof/alignment/internal/testkit"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestProveExitCodes(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())

	t.Run("prove exits 3 while tickets wait and gives the same requests when run again [verifies SP-334~1]", func(t *testing.T) {
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
			if item.Ticket.RequestID != second.Items[index].Ticket.RequestID || item.Ticket != second.Items[index].Ticket {
				t.Errorf("ticket %d changed between two runs", index)
			}
		}
	})

	t.Run("prove exits 0 when every test is ALIGNED and 1 when one is not [verifies SP-334~1]", func(t *testing.T) {
		aligned := newHarness(t, testkit.PythonParcel)
		aligned.reportWith(func(requirements []any) []any {
			for _, requirement := range requirements {
				fields := requirement.(map[string]any)
				switch fields["id"] {
				case "PAR-1":
					fields["tests"] = fields["tests"].([]any)[:1]
				case "PAR-2":
					fields["tests"] = fields["tests"].([]any)[:1]
				}
			}
			return requirements
		})
		aligned.withPowers(testkit.PythonPowers())
		if code, output := aligned.driveAttested(testkit.Oracle{}); code != cli.ExitClean {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
		if code, output := h.driveAttested(testkit.Oracle{}); code != cli.ExitBlocked {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
	})

	t.Run("prove --resume is refused when no request was ever emitted [verifies SP-334~1]", func(t *testing.T) {
		fresh := newHarness(t, testkit.PythonParcel)
		code, _, stderr := fresh.run(nil, proveCommand, resumeFlag)
		if code != cli.ExitToolFailure || !strings.Contains(stderr, "nothing to resume") {
			t.Fatalf("exit code %d: %s", code, stderr)
		}
	})
}

func TestUsageErrors(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)

	t.Run("conflicting flags and unexpected arguments fail without acting [verifies SP-335~1]", func(t *testing.T) {
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

	t.Run("an unknown command and no command are tool failures with a usage line [verifies SP-335~1]", func(t *testing.T) {
		for _, args := range [][]string{{"frobnicate"}, {}} {
			code, _, stderr := h.run(nil, args...)
			if code != cli.ExitToolFailure || !strings.Contains(stderr, "usage:") {
				t.Errorf("%v: exit code %d, stderr %q", args, code, stderr)
			}
		}
	})
}

func TestRequestCommands(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.run(nil, proveCommand, allFlag)
	item := h.requestsJSON().Items[0]
	ticket := item.Ticket.TicketID.String()

	t.Run("request list prints the tickets in state emitted with the role and the view of an executor [verifies SP-336~1]", func(t *testing.T) {
		if item.Role == "" || item.View.User == "" || item.View.Ticket != ticket || item.View.Nonce != item.Ticket.Nonce || len(item.View.ResponseSchema) == 0 {
			t.Fatalf("item %+v", item)
		}
		code, text, _ := h.run(nil, "request", "list")
		if code != cli.ExitClean || !strings.Contains(text, ticket) {
			t.Fatalf("exit code %d:\n%s", code, text)
		}
	})

	t.Run("request show moves the ticket to fetched [verifies SP-336~1]", func(t *testing.T) {
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

	t.Run("a wrong nonce, a schema violation and an unknown ticket are refusals, not tool failures [verifies SP-336~1]", func(t *testing.T) {
		answer := bytes.NewBufferString(`{"requirement":"NONE","coverage":"NONE","confidence":1}`).Bytes()
		for name, run := range map[string]func() (int, string){
			"a wrong nonce": func() (int, string) {
				code, _, stderr := h.run(answer, "request", "submit", ticket, "--nonce", strings.Repeat("0", 32))
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

func TestSystemOneCommands(t *testing.T) {
	h := newHarness(t, testkit.PythonParcel)
	h.withPowers(testkit.PythonPowers())
	h.run(nil, proveCommand, allFlag)

	t.Run("systemone export lists the closed requests that wait for their first vote [verifies SP-337~2]", func(t *testing.T) {
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
		closed := 0
		for _, item := range h.requestsJSON().Items {
			if len(item.View.Closed) > 0 {
				closed++
			}
		}
		if len(exported) != closed {
			t.Errorf("%d exported, %d closed tickets", len(exported), closed)
		}
	})

	t.Run("systemone import needs a source and a known ticket and records the answer under the relay channel [verifies SP-337~2]", func(t *testing.T) {
		if code, _, _ := h.run([]byte(`{}`), "systemone", "import", strings.Repeat("a", 64)+".0.1", "--nonce", strings.Repeat("0", 32)); code != cli.ExitBlocked {
			t.Errorf("an import without a source: exit code %d", code)
		}
		if code, _, _ := h.run([]byte(`{}`), "systemone", "import", strings.Repeat("a", 64)+".0.1", "--nonce", strings.Repeat("0", 32), "--source", "system-one:x@y"); code != cli.ExitBlocked {
			t.Errorf("an import for an unknown ticket: exit code %d", code)
		}
		for _, source := range []string{"systemone:x@y", "x@y", "system-one:x", "system-one:@y"} {
			if code, _, stderr := h.run([]byte(`{}`), "systemone", "import", strings.Repeat("a", 64)+".0.1", "--nonce", strings.Repeat("0", 32), "--source", source); code != cli.ExitBlocked || !strings.Contains(stderr, "system-one:<model>@<endpointDigest>") {
				t.Errorf("an import under the source %q: exit code %d, stderr %q", source, code, stderr)
			}
		}
		if code, _, _ := h.run(nil, "systemone"); code != cli.ExitBlocked {
			t.Errorf("no verb: exit code %d", code)
		}
		if code, _, _ := h.run(nil, "systemone", "frobnicate"); code != cli.ExitBlocked {
			t.Errorf("an unknown verb: exit code %d", code)
		}
		if h.relay(testkit.Oracle{}, 0.92) == 0 {
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
}

func TestDoctor(t *testing.T) {
	t.Run("doctor checks Shallnot, the configuration, the host store and the journal [verifies SP-338~2]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		code, output, stderr := h.run(nil, "doctor")
		if code != cli.ExitClean {
			t.Fatalf("exit code %d: %s\n%s", code, stderr, output)
		}
		for _, name := range []string{"ok shallnot", "ok policy", "ok journal"} {
			if !strings.Contains(output, name) {
				t.Errorf("the report lacks %q:\n%s", name, output)
			}
		}
	})

	t.Run("doctor exits 2 when a check fails [verifies SP-338~2]", func(t *testing.T) {
		h := newHarness(t, testkit.PythonParcel)
		h.withConfig("policy:\n  align_k: 0\n")
		code, output, _ := h.run(nil, "doctor")
		if code != cli.ExitToolFailure || !strings.Contains(output, "fail project") {
			t.Fatalf("exit code %d:\n%s", code, output)
		}
	})
}
