package hostsync_test

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/executorrun"
	"github.com/RachidChabane/musthold/format/hostproto/hostbundle"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"github.com/RachidChabane/musthold/format/hostproto/hostsync"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

const (
	headSHA   = "0123456789abcdef0123456789abcdef01234567"
	judgeRole = "musthold-dp:judge"
	agent     = "agent-0001"
	answer    = `{"validity":"VALID"}`
)

var module = format.Module{Name: "musthold-dp", Version: "0.1.0"}

func clock() time.Time { return hostprototest.Epoch }

func newStore(t *testing.T) *hostprototest.MemoryHostStore {
	t.Helper()
	return hostprototest.NewMemoryHostStore()
}

func emit(t *testing.T, store *hostprototest.MemoryHostStore, request hostproto.Request, vote int) hostproto.Ticket {
	t.Helper()
	if _, err := store.PutRequest(request); err != nil {
		t.Fatal(err)
	}
	digest, err := request.SchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	ticket := hostprototest.Ticket(request, vote, hostproto.FirstEmission)
	if err := store.PutEntry(hostproto.NewEntry(ticket, request.Role, digest, hostproto.DefaultMaxEmissions)); err != nil {
		t.Fatal(err)
	}
	return ticket
}

func service(store *hostprototest.MemoryHostStore, requireAttestation bool) requests.Service {
	return requests.Service{
		Plugin: hostprototest.PluginName, Store: store, Validator: hostprototest.MemberValidator{}, Clock: clock, RequireAttestation: requireAttestation,
		Executor: requests.ExecutorContext{Harness: format.Harness{Name: "claude-code", Version: "2.1.292"}, Isolation: format.IsolationRestricted},
	}
}

// answerAsExecutor answers a ticket the way an executor under Claude Code does.
func answerAsExecutor(t *testing.T, store *hostprototest.MemoryHostStore, ticket hostproto.Ticket) {
	t.Helper()
	if _, err := store.Append(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: agent, Role: judgeRole}); err != nil {
		t.Fatal(err)
	}
	secret, _ := store.Secret()
	capability, err := hostproto.FormatCap(secret, agent)
	if err != nil {
		t.Fatal(err)
	}
	s := service(store, true)
	if _, err := s.Show(ticket.TicketID, capability); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(requests.Submission{Ticket: ticket.TicketID, Nonce: ticket.Nonce, Output: []byte(answer), Cap: capability, Origin: requests.OriginExecutor}); err != nil {
		t.Fatal(err)
	}
}

func TestCollect(t *testing.T) {
	t.Run("the bundle holds the answered tickets with the profile they are credited with and the marks against them [verifies SP-715~2]", func(t *testing.T) {
		store := newStore(t)
		judge := hostprototest.Request()
		answered := emit(t, store, judge, 0)
		answerAsExecutor(t, store, answered)
		digest := strings.Repeat("a", format.Sha256HexLength)
		for _, event := range []hostproto.Event{
			{Kind: hostproto.EventAttested, Ticket: answered.TicketID.String(), AgentID: agent, Detail: executorrun.AttestationDetail(digest)},
			{Kind: hostproto.EventRunReported, Ticket: answered.TicketID.String(), Detail: executorrun.RunReportDetail(executorrun.RunReport{Model: "claude-sonnet-5-5"})},
		} {
			if _, err := store.Append(event); err != nil {
				t.Fatal(err)
			}
		}
		leaky := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "another mutant" })
		leaked := emit(t, store, leaky, 0)
		answerAsExecutorAgain(t, store, leaked)
		if _, err := store.Append(hostproto.Event{Kind: hostproto.EventLeaked, Ticket: leaked.TicketID.String()}); err != nil {
			t.Fatal(err)
		}
		emit(t, store, hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "never answered" }), 0)

		bundle, err := hostsync.Collect(store, hostproto.SessionProve, module, headSHA)
		if err != nil {
			t.Fatal(err)
		}
		if len(bundle.Responses) != 2 || bundle.Manifest.HeadSHA != headSHA || bundle.Manifest.Module != module {
			t.Fatalf("manifest = %+v", bundle.Manifest)
		}
		got, _ := bundle.Lookup(answered.Key())
		if got.Executor.Isolation != format.IsolationRestrictedAttested || got.Executor.Attestation.TranscriptDigest != digest ||
			got.Executor.Model.Name != "claude-sonnet-5-5" || got.Validate() != nil {
			t.Fatalf("executor = %+v", got.Executor)
		}
		if bundle.Marked(answered.Key()) || !bundle.Marked(leaked.Key()) {
			t.Fatalf("marks = %+v", bundle.Manifest.Responses)
		}
		records, _ := store.Records()
		if bundle.Manifest.JournalDigest != records[len(records)-1].Digest {
			t.Fatal("the manifest does not carry the head of the journal")
		}
	})

	t.Run("an empty store gives an empty bundle at the genesis digest [verifies SP-715~2]", func(t *testing.T) {
		bundle, err := hostsync.Collect(newStore(t), hostproto.SessionProve, module, headSHA)
		if err != nil || len(bundle.Responses) != 0 || bundle.Manifest.JournalDigest != hostproto.GenesisDigest {
			t.Fatalf("bundle = %+v, %v", bundle.Manifest, err)
		}
	})

	t.Run("a store whose journal chain is broken is not pushed [verifies SP-715~2]", func(t *testing.T) {
		store := newStore(t)
		emit(t, store, hostprototest.Request(), 0)
		if _, err := store.Append(hostproto.Event{Kind: hostproto.EventEmitted}); err != nil {
			t.Fatal(err)
		}
		if _, err := hostsync.Collect(tamperedJournal{store}, hostproto.SessionProve, module, headSHA); !errors.Is(err, hostproto.ErrChainBroken) {
			t.Fatalf("error = %v", err)
		}
	})
}

// answerAsExecutorAgain answers a ticket as a second executor agent.
func answerAsExecutorAgain(t *testing.T, store *hostprototest.MemoryHostStore, ticket hostproto.Ticket) {
	t.Helper()
	const second = "agent-0002"
	if _, err := store.Append(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: second, Role: judgeRole}); err != nil {
		t.Fatal(err)
	}
	secret, _ := store.Secret()
	capability, _ := hostproto.FormatCap(secret, second)
	s := service(store, true)
	if _, err := s.Show(ticket.TicketID, capability); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(requests.Submission{Ticket: ticket.TicketID, Nonce: ticket.Nonce, Output: []byte(answer), Cap: capability, Origin: requests.OriginExecutor}); err != nil {
		t.Fatal(err)
	}
}

func TestImporter(t *testing.T) {
	// the laptop answers two requests; CI holds three tickets, one of them unknown to the bundle.
	laptop := newStore(t)
	first, second := hostprototest.Request(), hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "another mutant" })
	answerAsExecutor(t, laptop, emit(t, laptop, first, 0))
	answerAsExecutorAgain(t, laptop, emit(t, laptop, second, 0))
	if _, err := laptop.Append(hostproto.Event{Kind: hostproto.EventLeaked, Ticket: hostprototest.Ticket(second, 0, 1).TicketID.String()}); err != nil {
		t.Fatal(err)
	}
	bundle, err := hostsync.Collect(laptop, hostproto.SessionProve, module, headSHA)
	if err != nil {
		t.Fatal(err)
	}

	ci := newStore(t)
	importer := &hostsync.Importer{Service: service(ci, false), Store: ci, Bundle: bundle}
	unknown := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "a mutant only CI drew" })
	tickets := []requests.PendingTicket{}
	for _, request := range []hostproto.Request{first, second, unknown} {
		ticket := emit(t, ci, request, 0)
		tickets = append(tickets, requests.PendingTicket{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce})
	}

	t.Run("a ticket the bundle answers is answered with the executor it had, and the others stay pending [verifies SP-715~2]", func(t *testing.T) {
		if err := importer.Serve(tickets); err != nil {
			t.Fatal(err)
		}
		if importer.Unanswered != 1 {
			t.Fatalf("unanswered = %d", importer.Unanswered)
		}
		got, err := ci.GetResponse(hostproto.Key{RequestID: hostprototest.MustID(first)})
		if err != nil || got.Executor.Channel != format.ChannelHostSubagent || got.Executor.Attestation.AgentID != agent {
			t.Fatalf("response = %+v, %v", got, err)
		}
		entry, _ := ci.GetEntry(hostproto.Key{RequestID: hostprototest.MustID(unknown)})
		if entry.State != hostproto.StateEmitted {
			t.Fatalf("the unknown ticket is %s", entry.State)
		}
	})

	t.Run("a response marked on the host is marked again in CI's journal [verifies SP-715~2]", func(t *testing.T) {
		marked := ticketText(second)
		found := slices.ContainsFunc(journal(t, ci), func(e hostproto.Event) bool { return e.Kind == hostproto.EventExposed && e.Ticket == marked })
		unmarked := slices.ContainsFunc(journal(t, ci), func(e hostproto.Event) bool { return e.Kind == hostproto.EventExposed && e.Ticket == ticketText(first) })
		if !found || unmarked {
			t.Fatalf("journal = %v", journal(t, ci))
		}
	})

	t.Run("a bundle that answers nothing ends the replay instead of asking again [verifies SP-715~2]", func(t *testing.T) {
		empty := &hostsync.Importer{Service: service(ci, false), Store: ci}
		err := empty.Serve([]requests.PendingTicket{tickets[2]})
		if !errors.Is(err, requests.ErrNothingServed) || empty.Unanswered != 1 {
			t.Fatalf("error = %v, unanswered = %d", err, empty.Unanswered)
		}
	})

	t.Run("an answer that breaks the response schema is refused on import [verifies SP-715~2]", func(t *testing.T) {
		broken := bundle
		broken.Responses = slices.Clone(bundle.Responses)
		broken.Responses[0].Output = []byte(`{"verdict":"VALID"}`)
		target := newStore(t)
		ticket := emit(t, target, hostprototest.Request(), 0)
		forged := &hostsync.Importer{Service: service(target, false), Store: target, Bundle: broken}
		err := forged.Serve([]requests.PendingTicket{{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce}})
		if err == nil {
			t.Fatal("a response that breaks the schema was imported")
		}
		if _, err := target.GetResponse(ticket.Key()); err == nil {
			t.Fatal("it left a response")
		}
	})

	t.Run("a ticket that is not an identifier is an error [verifies SP-715~2]", func(t *testing.T) {
		if err := importer.Serve([]requests.PendingTicket{{Ticket: "latest"}}); err == nil {
			t.Fatal("a bad ticket was served")
		}
	})
}

func ticketText(request hostproto.Request) string {
	return hostprototest.Ticket(request, 0, 1).TicketID.String()
}

func journal(t *testing.T, store *hostprototest.MemoryHostStore) []hostproto.Event {
	t.Helper()
	records, err := store.Records()
	if err != nil {
		t.Fatal(err)
	}
	events := make([]hostproto.Event, len(records))
	for index, record := range records {
		events[index] = record.Event
	}
	return events
}

func TestIssuedRequests(t *testing.T) {
	t.Run("the requests that wait are exported in full and imported as tickets on another machine [verifies SP-716~2]", func(t *testing.T) {
		ci := newStore(t)
		pending := hostprototest.Request()
		answered := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "already answered" })
		if err := ci.Record(queueItem(pending, "PAR-1~1", "validity")); err != nil {
			t.Fatal(err)
		}
		emit(t, ci, pending, 0)
		emit(t, ci, pending, 1)
		answerAsExecutor(t, ci, emit(t, ci, answered, 0))

		issued, err := hostsync.ExportIssued(ci, hostproto.SessionProve)
		if err != nil {
			t.Fatal(err)
		}
		if len(issued.Requests) != 1 || issued.Entries[0].Votes != 2 || issued.Entries[0].Purpose != "validity" || issued.Entries[0].Criteria[0] != "PAR-1~1" {
			t.Fatalf("issued = %+v", issued.Entries)
		}

		developer := newStore(t)
		imported, err := hostsync.ImportIssued(developer, hostproto.SessionProve, issued, clock, &counter{})
		if err != nil || imported != 1 {
			t.Fatalf("imported = %d, %v", imported, err)
		}
		entries, _ := developer.ListEntries()
		if len(entries) != 2 || entries[0].State != hostproto.StateEmitted || entries[0].Ticket.RequestID != hostprototest.MustID(pending) {
			t.Fatalf("entries = %+v", entries)
		}
		if again, err := hostsync.ImportIssued(developer, hostproto.SessionProve, issued, clock, &counter{}); err != nil || again != 1 {
			t.Fatalf("a second import: %d, %v", again, err)
		}
		if after, _ := developer.ListEntries(); len(after) != 2 {
			t.Fatalf("a second import added tickets: %d", len(after))
		}
	})

	t.Run("a store with nothing pending issues nothing [verifies SP-716~2]", func(t *testing.T) {
		issued, err := hostsync.ExportIssued(newStore(t), hostproto.SessionProve)
		if err != nil || len(issued.Requests) != 0 {
			t.Fatalf("issued = %+v, %v", issued, err)
		}
	})

	t.Run("a request the manifest does not list is refused on import [verifies SP-716~2]", func(t *testing.T) {
		issued := hostbundle.IssuedBundle{Requests: []hostproto.Request{hostprototest.Request()}}
		if _, err := hostsync.ImportIssued(newStore(t), hostproto.SessionProve, issued, clock, &counter{}); !errors.Is(err, hostbundle.ErrBadBundle) {
			t.Fatalf("error = %v", err)
		}
	})
}

func queueItem(request hostproto.Request, criterion, purpose string) hostproto.QueueItem {
	return hostproto.QueueItem{RequestID: hostprototest.MustID(request), Criteria: []string{criterion}, Wave: 1, Purpose: purpose}
}

// tamperedJournal is a host store whose journal holds a record with wrong digests.
type tamperedJournal struct{ hostproto.HostStore }

func (tamperedJournal) Records() ([]hostproto.Record, error) {
	return []hostproto.Record{{Sequence: 0, Prev: "x", Event: hostproto.Event{Kind: hostproto.EventEmitted}, Digest: "y"}}, nil
}

type counter struct{ next byte }

func (c *counter) Read(p []byte) (int, error) {
	for index := range p {
		c.next++
		p[index] = c.next
	}
	return len(p), nil
}
