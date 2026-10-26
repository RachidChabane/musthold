package hostprototest_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

// fakeModule is a module's command line over the reference host on the inline
// channel: `request show` fetches and `request submit` answers.
type fakeModule struct {
	host    *hostprototest.ReferenceHost
	request hostproto.Request
	ticket  hostproto.Ticket
	runs    [][]string
}

func newFakeModule(t *testing.T) *fakeModule {
	t.Helper()
	host := hostprototest.NewReferenceHost(hostproto.Observed{}, hostproto.Policy{})
	request := hostprototest.Request()
	ticket, err := host.Emit(request)
	if err != nil {
		t.Fatal(err)
	}
	return &fakeModule{host: host, request: request, ticket: ticket}
}

func (m *fakeModule) current(t testing.TB) hostproto.Entry {
	entry, err := m.host.Entry(m.ticket.Key())
	if err != nil {
		t.Fatal(err)
	}
	return entry
}

func (m *fakeModule) Run(args []string, stdin []byte) ([]byte, error) {
	m.runs = append(m.runs, args)
	entry, err := m.host.Entry(m.ticket.Key())
	if err != nil {
		return nil, err
	}
	switch args[1] {
	case hostproto.SubcommandList:
		listing := hostproto.Listing{}
		if entry.State == hostproto.StateEmitted || entry.State == hostproto.StateFetched {
			listing.Items = append(listing.Items, hostproto.ListingItem{Ticket: entry.Ticket, Role: entry.Role, View: m.request.ViewFor(entry.Ticket)})
		}
		return json.Marshal(listing)
	case hostproto.SubcommandShow:
		_, err := m.host.Fetch(entry.Ticket.TicketID, "")
		return nil, err
	default:
		_, err := m.host.Submit(hostprototest.Submit{Ticket: entry.Ticket.TicketID, Nonce: args[len(args)-1], Output: stdin})
		return nil, err
	}
}

func TestCommandLineHost(t *testing.T) {
	answers := hostprototest.SchemaAnswers{}
	noSleep := func(time.Duration) {}

	t.Run("an honest host lists, shows and submits through the command line [verifies SP-142~2]", func(t *testing.T) {
		module := newFakeModule(t)
		results, err := hostprototest.PlayCLI(module, hostprototest.PersonaHonest, answers, noSleep)
		if err != nil || len(results) != 1 || !results[0].Submitted {
			t.Fatalf("results = %+v, %v", results, err)
		}
		if module.current(t).State != hostproto.StateAccepted {
			t.Fatalf("state = %s", module.current(t).State)
		}
		wantCommands := []string{hostproto.SubcommandList, hostproto.SubcommandShow, hostproto.SubcommandSubmit}
		var got []string
		for _, run := range module.runs {
			got = append(got, run[1])
		}
		if !slices.Equal(got, wantCommands) {
			t.Fatalf("commands = %v, want %v", got, wantCommands)
		}
	})

	t.Run("a forger submits without showing the ticket and is refused [verifies SP-142~2]", func(t *testing.T) {
		module := newFakeModule(t)
		results, err := hostprototest.PlayCLI(module, hostprototest.PersonaForger, answers, noSleep)
		if err != nil || len(results) != 1 || results[0].Submitted || results[0].Error == "" {
			t.Fatalf("results = %+v, %v", results, err)
		}
		if module.current(t).State != hostproto.StateEmitted {
			t.Fatalf("state = %s", module.current(t).State)
		}
	})

	t.Run("an omitter submits nothing [verifies SP-142~2]", func(t *testing.T) {
		module := newFakeModule(t)
		results, err := hostprototest.PlayCLI(module, hostprototest.PersonaOmitter, answers, noSleep)
		if err != nil || len(results) != 0 || module.current(t).State != hostproto.StateEmitted {
			t.Fatalf("results = %+v, %v, state %s", results, err, module.current(t).State)
		}
	})

	t.Run("a stale host is refused for its nonce and a breaker is rejected after its correction [verifies SP-142~2]", func(t *testing.T) {
		stale := newFakeModule(t)
		results, err := hostprototest.PlayCLI(stale, hostprototest.PersonaStale, answers, noSleep)
		if err != nil || len(results) != 1 || results[0].Submitted {
			t.Fatalf("stale: %+v, %v", results, err)
		}
		breaker := newFakeModule(t)
		results, err = hostprototest.PlayCLI(breaker, hostprototest.PersonaBreaker, answers, noSleep)
		if err != nil || len(results) != 2 || results[0].Submitted || results[1].Submitted || breaker.current(t).State != hostproto.StateRejected {
			t.Fatalf("breaker: %+v, %v, state %s", results, err, breaker.current(t).State)
		}
	})

	t.Run("a slow host waits past the time to live before it submits [verifies SP-142~2]", func(t *testing.T) {
		var waited []time.Duration
		_, err := hostprototest.PlayCLI(newFakeModule(t), hostprototest.PersonaSlow, answers, func(d time.Duration) { waited = append(waited, d) })
		if err != nil || len(waited) != 1 || waited[0] <= hostproto.FetchTTL {
			t.Fatalf("waited %v, %v", waited, err)
		}
	})

	t.Run("a churner keeps submitting content that breaks the schema, in bounded rounds [verifies SP-142~2]", func(t *testing.T) {
		module := newFakeModule(t)
		results, err := hostprototest.PlayCLI(module, hostprototest.PersonaChurner, answers, noSleep)
		if err != nil || len(results) == 0 || len(results) > hostproto.DefaultMaxEmissions*(hostproto.MaxCorrections+1) {
			t.Fatalf("results = %+v, %v", results, err)
		}
	})

	t.Run("a persona that needs hooks has no command-line behaviour [verifies SP-142~2]", func(t *testing.T) {
		for _, name := range []string{hostprototest.PersonaLeaker, hostprototest.PersonaPadder, hostprototest.PersonaModelSwapper, hostprototest.PersonaHookless, hostprototest.PersonaUnrestricted} {
			if _, err := hostprototest.PlayCLI(newFakeModule(t), name, answers, noSleep); !errors.Is(err, hostprototest.ErrUnsupported) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
		for _, name := range hostprototest.CLIPersonaNames() {
			if !slices.Contains(hostprototest.PersonaNames(), name) {
				t.Errorf("%s is not a persona", name)
			}
		}
	})

	t.Run("a module that prints something else than a listing is an error [verifies SP-142~2]", func(t *testing.T) {
		broken := runnerFunc(func([]string, []byte) ([]byte, error) { return []byte("not json"), nil })
		if _, err := hostprototest.PlayCLI(broken, hostprototest.PersonaHonest, answers, noSleep); err == nil {
			t.Fatal("an unreadable listing was accepted")
		}
		failing := runnerFunc(func([]string, []byte) ([]byte, error) { return nil, errors.New("boom") })
		if _, err := hostprototest.PlayCLI(failing, hostprototest.PersonaHonest, answers, noSleep); err == nil {
			t.Fatal("a failing module was accepted")
		}
	})

	t.Run("recorded answers are found by request id and a missing one is reported [verifies SP-142~2]", func(t *testing.T) {
		request := hostprototest.Request()
		ticket := hostprototest.Ticket(request, 0, 1)
		view := request.ViewFor(ticket)
		recordings := hostprototest.Recordings{ticket.RequestID: " " + hostprototest.HonestAnswer + "\n"}
		got, err := recordings.Answer(hostprototest.PersonaHonest, view)
		if err != nil || got != hostprototest.HonestAnswer {
			t.Fatalf("answer = %q, %v", got, err)
		}
		if _, err := (hostprototest.Recordings{}).Answer(hostprototest.PersonaHonest, view); !errors.Is(err, hostprototest.ErrNoRecording) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a forged answer satisfies the schema and takes VALID wherever it is offered [verifies SP-142~2]", func(t *testing.T) {
		if got := hostprototest.ForgedAnswer([]byte(hostprototest.ResponseSchema)); got != `{"validity":"VALID"}` {
			t.Fatalf("forged = %s", got)
		}
		schema := `{"type":"object","required":["verdict","note"],"properties":{"verdict":{"enum":["A","B"]},"note":{"type":"string"}}}`
		if got := hostprototest.ForgedAnswer([]byte(schema)); got != `{"note":"x","verdict":"A"}` {
			t.Fatalf("forged = %s", got)
		}
		if got := hostprototest.ForgedAnswer([]byte(`[`)); got != `{}` {
			t.Fatalf("forged = %s", got)
		}
	})
}

type runnerFunc func(args []string, stdin []byte) ([]byte, error)

func (f runnerFunc) Run(args []string, stdin []byte) ([]byte, error) { return f(args, stdin) }
