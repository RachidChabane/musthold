package cli_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/xdg"
	"github.com/RachidChabane/spec-proof/discpower/internal/cli"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	honestAnswer      = `{"validity":"VALID"}`
	conflictingAnswer = `{"validity":"EQUIVALENT"}`
	brokenAnswer      = `{"verdict":"VALID"}`
	staleNonce        = "00000000000000000000000000000000"
	relaySource       = "system-one:model-x"
	tamperedRecord    = `{"seq":0,"prev":"x","event":{"kind":"emitted"},"digest":"y"}` + "\n"
	missingShallnot   = "/nonexistent/shallnot"
	correctionNotice  = "may be corrected once"
	refusalExit       = cli.ExitBlocked
	toolFailureExit   = cli.ExitToolFailure
)

// hostFixture is a harness whose host store holds the tickets a test put there.
type hostFixture struct {
	*harness
	store *hoststore.Store
}

func newHostFixture(t *testing.T) *hostFixture {
	t.Helper()
	h := newHarness(t, "")
	lookup := func(name string) (string, bool) {
		if name == stateVariable {
			return h.state, true
		}
		return "", false
	}
	store, err := hoststore.Open(xdg.HostDir(lookup, h.state, h.root))
	if err != nil {
		t.Fatal(err)
	}
	return &hostFixture{harness: h, store: store}
}

// emit stores the request and the first emission of one vote of it.
func (f *hostFixture) emit(request hostproto.Request, vote int) hostproto.Ticket {
	f.t.Helper()
	if _, err := f.store.PutRequest(request); err != nil {
		f.t.Fatal(err)
	}
	digest, err := request.SchemaDigest()
	if err != nil {
		f.t.Fatal(err)
	}
	ticket := hostprototest.Ticket(request, vote, hostproto.FirstEmission)
	if err := f.store.PutEntry(hostproto.NewEntry(ticket, request.Role, digest, hostproto.DefaultMaxEmissions)); err != nil {
		f.t.Fatal(err)
	}
	return ticket
}

func (f *hostFixture) show(ticket hostproto.Ticket) {
	f.t.Helper()
	if code, _, stderr := f.run(nil, hostproto.ShowArgs(ticket.TicketID)...); code != cli.ExitClean {
		f.t.Fatalf("show exited %d: %s", code, stderr)
	}
}

func (f *hostFixture) submit(ticket hostproto.Ticket, nonce, output string) (int, string, string) {
	return f.run([]byte(output), hostproto.SubmitArgs(ticket.TicketID, nonce)...)
}

func (f *hostFixture) stateOf(ticket hostproto.Ticket) hostproto.State {
	f.t.Helper()
	entry, err := f.store.GetEntry(ticket.Key())
	if err != nil {
		f.t.Fatal(err)
	}
	return entry.State
}

func (f *hostFixture) journaled(kind hostproto.EventKind) bool {
	f.t.Helper()
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	for _, record := range records {
		if record.Event.Kind == kind {
			return true
		}
	}
	return false
}

// runWithEnv runs the command line with some environment variables replaced.
func (f *hostFixture) runWithEnv(overrides map[string]string, args ...string) (int, string, string) {
	f.t.Helper()
	env, stdout, stderr := f.env(nil)
	inner := env.LookupEnv
	env.LookupEnv = func(name string) (string, bool) {
		if value, found := overrides[name]; found {
			return value, true
		}
		return inner(name)
	}
	code := cli.Run(env, args)
	return code, stdout.String(), stderr.String()
}

func TestRequestList(t *testing.T) {
	t.Run("list --json prints the emitted tickets with the view an executor sees [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		code, stdout, stderr := f.run(nil, hostproto.ListArgs()...)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		var listing hostproto.Listing
		if err := json.Unmarshal([]byte(stdout), &listing); err != nil {
			t.Fatal(err)
		}
		if len(listing.Items) != 1 || listing.Items[0].Ticket != ticket || listing.Items[0].View.Nonce != ticket.Nonce || listing.Items[0].Role != hostprototest.Request().Role {
			t.Fatalf("listing %+v", listing)
		}
	})

	t.Run("list without --json prints one line per ticket [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		code, stdout, _ := f.run(nil, hostproto.CommandRequest, hostproto.SubcommandList)
		if want := ticket.TicketID.String() + " " + hostprototest.Request().Role + "\n"; code != cli.ExitClean || stdout != want {
			t.Fatalf("exit %d, stdout %q, want %q", code, stdout, want)
		}
	})

	t.Run("an empty store lists an empty array of items [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		code, stdout, _ := f.run(nil, hostproto.ListArgs()...)
		if code != cli.ExitClean || strings.TrimSpace(stdout) != `{"items":[]}` {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})
}

func TestRequestShow(t *testing.T) {
	t.Run("show prints the view, fetches the ticket and removes it from the listing [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		code, stdout, stderr := f.run(nil, hostproto.ShowArgs(ticket.TicketID)...)
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		var view hostproto.ExecutorView
		if err := json.Unmarshal([]byte(stdout), &view); err != nil || view.Ticket != ticket.TicketID.String() || view.Nonce != ticket.Nonce {
			t.Fatalf("view %s (%v)", stdout, err)
		}
		if f.stateOf(ticket) != hostproto.StateFetched {
			t.Fatalf("state %s", f.stateOf(ticket))
		}
		if _, listed, _ := f.run(nil, hostproto.ListArgs()...); !strings.Contains(listed, `"items":[]`) {
			t.Fatalf("listing after show: %s", listed)
		}
	})

	refusals := []struct {
		name string
		args []string
	}{
		{"no ticket", []string{hostproto.CommandRequest, hostproto.SubcommandShow}},
		{"two tickets", []string{hostproto.CommandRequest, hostproto.SubcommandShow, "a", "b"}},
		{"a text that is no ticket", []string{hostproto.CommandRequest, hostproto.SubcommandShow, "not-a-ticket"}},
		{"a ticket nobody emitted", hostproto.ShowArgs(hostprototest.Ticket(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "unknown" }), 0, 1).TicketID)},
	}
	for _, tt := range refusals {
		t.Run("show of "+tt.name+" is a refusal [verifies SP-192~1]", func(t *testing.T) {
			f := newHostFixture(t)
			f.emit(hostprototest.Request(), 0)
			if code, _, stderr := f.run(nil, tt.args...); code != refusalExit || stderr == "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
		})
	}
}

func TestRequestSubmit(t *testing.T) {
	t.Run("a valid answer is accepted with a receipt and kept exposed, as it came without a capability [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		f.show(ticket)
		code, stdout, stderr := f.submit(ticket, ticket.Nonce, honestAnswer)
		if code != cli.ExitClean || !strings.Contains(stdout, ticket.TicketID.String()) {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		response, err := f.store.GetResponse(ticket.Key())
		if err != nil || string(response.Output) != honestAnswer || response.Executor.Channel != format.ChannelHostInline {
			t.Fatalf("response %+v (%v)", response, err)
		}
		if f.stateOf(ticket) != hostproto.StateAccepted || !f.journaled(hostproto.EventExposed) {
			t.Fatalf("state %s", f.stateOf(ticket))
		}
	})

	t.Run("the same answer submitted again is accepted again [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		f.show(ticket)
		f.submit(ticket, ticket.Nonce, honestAnswer)
		if code, _, stderr := f.submit(ticket, ticket.Nonce, honestAnswer); code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
	})

	t.Run("a different second answer is a refusal [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		f.show(ticket)
		f.submit(ticket, ticket.Nonce, honestAnswer)
		if code, _, stderr := f.submit(ticket, ticket.Nonce, conflictingAnswer); code != refusalExit || !strings.Contains(stderr, hostproto.ErrResubmissionConflict.Error()) {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("an answer with the nonce of another issue is a refusal [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		f.show(ticket)
		if code, _, stderr := f.submit(ticket, staleNonce, honestAnswer); code != refusalExit || !strings.Contains(stderr, hostproto.ErrNonceMismatch.Error()) {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})

	t.Run("an answer for a ticket that was not shown is a refusal [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		if code, _, _ := f.submit(ticket, ticket.Nonce, honestAnswer); code != refusalExit {
			t.Fatalf("exit %d", code)
		}
	})

	t.Run("an answer that breaks the schema is a refusal that says it may be corrected once [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(hostprototest.Request(), 0)
		f.show(ticket)
		code, _, stderr := f.submit(ticket, ticket.Nonce, brokenAnswer)
		if code != refusalExit || !strings.Contains(stderr, correctionNotice) {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
		if code, _, stderr := f.submit(ticket, ticket.Nonce, honestAnswer); code != cli.ExitClean {
			t.Fatalf("the correction exited %d: %s", code, stderr)
		}
	})

	t.Run("a submission without a ticket is a refusal [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run([]byte(honestAnswer), hostproto.CommandRequest, hostproto.SubcommandSubmit, hostproto.FlagNonce, staleNonce); code != refusalExit {
			t.Fatalf("exit %d", code)
		}
	})
}

func TestRequestVerbs(t *testing.T) {
	refusals := []struct {
		name string
		args []string
	}{
		{"request without a verb", []string{hostproto.CommandRequest}},
		{"request with an unknown verb", []string{hostproto.CommandRequest, "frobnicate"}},
		{"systemone without a verb", []string{"systemone"}},
		{"systemone with an unknown verb", []string{"systemone", "frobnicate"}},
	}
	for _, tt := range refusals {
		t.Run(tt.name+" is a refusal [verifies SP-192~1]", func(t *testing.T) {
			f := newHostFixture(t)
			if code, _, stderr := f.run(nil, tt.args...); code != refusalExit || stderr == "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
		})
	}

	t.Run("an unknown flag is a tool failure [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, _ := f.run(nil, hostproto.CommandRequest, hostproto.SubcommandList, "--bogus"); code != toolFailureExit {
			t.Fatalf("exit %d", code)
		}
	})

	t.Run("a store that cannot be read is a tool failure [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		f.emit(hostprototest.Request(), 0)
		tickets, err := os.ReadDir(filepath.Join(f.store.Dir(), hoststore.TicketsDir))
		if err != nil || len(tickets) != 1 {
			t.Fatalf("tickets %v (%v)", tickets, err)
		}
		if err := os.WriteFile(filepath.Join(f.store.Dir(), hoststore.TicketsDir, tickets[0].Name()), []byte("{"), fileMode); err != nil {
			t.Fatal(err)
		}
		if code, _, stderr := f.run(nil, hostproto.ListArgs()...); code != toolFailureExit || stderr == "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
}

func TestSystemOne(t *testing.T) {
	closed := hostprototest.ClosedRequest()

	t.Run("export lists the closed requests with their nonce and questions and nothing else [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		f.emit(hostprototest.Request(), 0)
		ticket := f.emit(closed, 0)
		f.emit(closed, 1)
		code, stdout, stderr := f.run(nil, "systemone", "export")
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s", code, stderr)
		}
		var exported []struct {
			Ticket string          `json:"ticket"`
			Nonce  string          `json:"nonce"`
			Closed json.RawMessage `json:"closed"`
		}
		if err := json.Unmarshal([]byte(stdout), &exported); err != nil {
			t.Fatal(err)
		}
		if len(exported) != 1 || exported[0].Ticket != ticket.TicketID.String() || exported[0].Nonce != ticket.Nonce || string(exported[0].Closed) != string(closed.Closed) {
			t.Fatalf("exported %s", stdout)
		}
	})

	t.Run("export of an empty store is an empty array [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, stdout, _ := f.run(nil, "systemone", "export"); code != cli.ExitClean || strings.TrimSpace(stdout) != "[]" {
			t.Fatalf("exit %d, stdout %q", code, stdout)
		}
	})

	t.Run("import records the relayed answer under the relay executor and does not mark it exposed [verifies SP-192~1] [verifies SP-408~1]", func(t *testing.T) {
		f := newHostFixture(t)
		ticket := f.emit(closed, 0)
		code, stdout, stderr := f.run([]byte(hostprototest.ClosedSystemOneResponse), "systemone", "import", ticket.TicketID.String(), "--nonce", ticket.Nonce, "--source", relaySource)
		if code != cli.ExitClean || !strings.Contains(stdout, ticket.TicketID.String()) {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		response, err := f.store.GetResponse(ticket.Key())
		if err != nil || response.Executor.Channel != format.ChannelSystemOneRelay || response.Executor.Model.Name != relaySource {
			t.Fatalf("response %+v (%v)", response, err)
		}
		answers, err := hostproto.ParseClosedAnswers(response.Output)
		if err != nil || answers[hostprototest.ClosedQuestionID] != (hostproto.ClosedAnswer{Choice: "VIOLATES", Confidence: 0.9, HasConfidence: true}) {
			t.Fatalf("the stored output %s is not the translation of the endpoint response (%v)", response.Output, err)
		}
		if f.journaled(hostproto.EventExposed) {
			t.Fatal("a relayed answer is journaled as exposed")
		}
	})

	refusals := []struct {
		name  string
		args  func(ticket hostproto.Ticket) []string
		stdin string
	}{
		{"without a source", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", t.TicketID.String(), "--nonce", t.Nonce}
		}, hostprototest.ClosedSystemOneResponse},
		{"without a ticket", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", "--nonce", t.Nonce, "--source", relaySource}
		}, hostprototest.ClosedSystemOneResponse},
		{"with another nonce", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", t.TicketID.String(), "--nonce", staleNonce, "--source", relaySource}
		}, hostprototest.ClosedSystemOneResponse},
		{"with a response that breaks the schema", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", t.TicketID.String(), "--nonce", t.Nonce, "--source", relaySource}
		}, brokenAnswer},
		{"with a response that leaves the question unanswered", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", t.TicketID.String(), "--nonce", t.Nonce, "--source", relaySource}
		}, `{"answers":{}}`},
		{"with a response whose choice is no option", func(t hostproto.Ticket) []string {
			return []string{"systemone", "import", t.TicketID.String(), "--nonce", t.Nonce, "--source", relaySource}
		}, `{"answers":{"q1":{"type":"choice","choice":"MAYBE"}}}`},
	}
	for _, tt := range refusals {
		t.Run("import "+tt.name+" is a refusal [verifies SP-192~1]", func(t *testing.T) {
			f := newHostFixture(t)
			ticket := f.emit(closed, 0)
			if code, _, stderr := f.run([]byte(tt.stdin), tt.args(ticket)...); code != refusalExit || stderr == "" {
				t.Fatalf("exit %d, stderr %q", code, stderr)
			}
			if _, err := f.store.GetResponse(ticket.Key()); err == nil {
				t.Fatal("a refused import left a response")
			}
		})
	}
}

func TestDoctor(t *testing.T) {
	t.Run("a healthy project exits 0 and reports every check [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		code, stdout, stderr := f.run(nil, "doctor")
		if code != cli.ExitClean {
			t.Fatalf("exit %d: %s%s", code, stdout, stderr)
		}
		for _, line := range []string{"ok shallnot ", "ok ecosystem pytest", "ok journal "} {
			if !strings.Contains(stdout, line) {
				t.Errorf("report lacks %q:\n%s", line, stdout)
			}
		}
	})

	failures := []struct {
		name    string
		prepare func(f *hostFixture) (overrides map[string]string, args []string)
		report  string
	}{
		{"a missing shallnot", func(*hostFixture) (map[string]string, []string) {
			return map[string]string{shallnotVariable: missingShallnot}, []string{"doctor"}
		}, "fail shallnot"},
		{"a directory with no known ecosystem", func(f *hostFixture) (map[string]string, []string) {
			return nil, []string{"doctor", "--root", f.t.TempDir()}
		}, "fail project"},
		{"a journal whose chain is broken", func(f *hostFixture) (map[string]string, []string) {
			journal := filepath.Join(f.store.Dir(), hoststore.JournalFile)
			if err := os.WriteFile(journal, []byte(tamperedRecord), fileMode); err != nil {
				f.t.Fatal(err)
			}
			return nil, []string{"doctor"}
		}, "fail journal"},
	}
	for _, tt := range failures {
		t.Run("doctor with "+tt.name+" exits 2 [verifies SP-192~1]", func(t *testing.T) {
			f := newHostFixture(t)
			overrides, args := tt.prepare(f)
			code, stdout, _ := f.runWithEnv(overrides, args...)
			if code != toolFailureExit || !strings.Contains(stdout, tt.report) {
				t.Fatalf("exit %d, report:\n%s", code, stdout)
			}
		})
	}

	t.Run("doctor with an argument it does not take exits 2 [verifies SP-192~1]", func(t *testing.T) {
		f := newHostFixture(t)
		if code, _, stderr := f.run(nil, "doctor", "extra"); code != toolFailureExit || stderr == "" {
			t.Fatalf("exit %d, stderr %q", code, stderr)
		}
	})
}

func TestProveRefusals(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		code    int
		message string
	}{
		{"--changed with --all", []string{"prove", "--changed", "--all"}, toolFailureExit, "--changed and --all exclude each other"},
		{"--record with --replay", []string{"prove", "--record", "one", "--replay", "two"}, toolFailureExit, "--record and --replay exclude each other"},
		{"an unexpected positional argument", []string{"prove", "extra"}, toolFailureExit, "unexpected arguments"},
		{"a positional argument after the flags", []string{"prove", "--all", "extra"}, toolFailureExit, "unexpected arguments"},
		{"an unknown flag", []string{"prove", "--bogus"}, toolFailureExit, "bogus"},
		{"--resume when no request was ever emitted", []string{"prove", "--resume"}, toolFailureExit, "nothing to resume"},
	}
	for _, tt := range tests {
		t.Run(tt.name+" is refused before anything runs [verifies SP-193~1]", func(t *testing.T) {
			f := newHostFixture(t)
			code, stdout, stderr := f.run(nil, tt.args...)
			if code != tt.code || !strings.Contains(stderr, tt.message) || stdout != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
			}
			items, err := f.store.Items()
			if err != nil || len(items) != 0 || len(f.statements()) != 0 {
				t.Fatalf("a refused run did work: items %v (%v), statements %v", items, err, f.statements())
			}
		})
	}
}
