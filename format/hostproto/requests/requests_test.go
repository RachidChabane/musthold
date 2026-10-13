package requests

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	validAnswer      = `{"validity":"VALID"}`
	reformattedValid = `{ "validity" : "VALID" }`
	otherAnswer      = `{"validity":"EQUIVALENT"}`
	brokenAnswer     = `{"verdict":"VALID"}`
	relaySource      = "system-one:relay-model@0123abcd"
	agentIdentifier  = "agent-1"
	wrongNonce       = "ffffffffffffffffffffffffffffffff"
	secondEmission   = 2
	samplePlugin     = "spec-proof-sample"
)

// fixture is a host store holding one emitted ticket of the fixture request.
type fixture struct {
	t       testing.TB
	store   *hostprototest.MemoryHostStore
	service Service
	request hostproto.Request
	ticket  hostproto.Ticket
}

func newFixture(t testing.TB) *fixture {
	t.Helper()
	store := hostprototest.NewMemoryHostStore()
	f := &fixture{t: t, store: store, service: Service{Plugin: samplePlugin, Store: store, Validator: hostprototest.MemberValidator{}, Clock: func() time.Time { return hostprototest.Epoch }}}
	f.request, f.ticket = f.emit(hostprototest.Request(), 0)
	return f
}

func (f *fixture) emit(request hostproto.Request, vote int) (hostproto.Request, hostproto.Ticket) {
	f.t.Helper()
	id, err := f.store.PutRequest(request)
	if err != nil {
		f.t.Fatal(err)
	}
	digest, err := request.SchemaDigest()
	if err != nil {
		f.t.Fatal(err)
	}
	ticket := hostprototest.Ticket(request, vote, hostproto.FirstEmission)
	if ticket.RequestID != id {
		f.t.Fatalf("request id %s, ticket %s", id, ticket.RequestID)
	}
	if err := f.store.PutEntry(hostproto.NewEntry(ticket, request.Role, digest, hostproto.DefaultMaxEmissions)); err != nil {
		f.t.Fatal(err)
	}
	return request, ticket
}

func (f *fixture) show() hostproto.ExecutorView {
	f.t.Helper()
	view, err := f.service.Show(f.ticket.TicketID, "")
	if err != nil {
		f.t.Fatal(err)
	}
	return view
}

func (f *fixture) submission(output string) Submission {
	return Submission{Ticket: f.ticket.TicketID, Nonce: f.ticket.Nonce, Output: []byte(output), Origin: OriginCLI}
}

func (f *fixture) state() hostproto.State {
	f.t.Helper()
	entry, err := f.store.GetEntry(f.ticket.Key())
	if err != nil {
		f.t.Fatal(err)
	}
	return entry.State
}

func (f *fixture) journalKinds() []hostproto.EventKind {
	f.t.Helper()
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	var kinds []hostproto.EventKind
	for _, record := range records {
		if record.Event.Ticket == f.ticket.TicketID.String() {
			kinds = append(kinds, record.Event.Kind)
		}
	}
	return kinds
}

func (f *fixture) response() hostproto.Response {
	f.t.Helper()
	response, err := f.store.GetResponse(f.ticket.Key())
	if err != nil {
		f.t.Fatal(err)
	}
	return response
}

func TestList(t *testing.T) {
	t.Run("only emitted tickets are listed, with their role and executor view [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		_, fetchedTicket := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "fetched" }), 0)
		if _, err := f.service.Show(fetchedTicket.TicketID, ""); err != nil {
			t.Fatal(err)
		}
		_, supersededTicket := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "superseded" }), 0)
		if _, err := f.store.UpdateEntry(supersededTicket.Key(), hostproto.Entry.Supersede); err != nil {
			t.Fatal(err)
		}
		_, secondVote := f.emit(f.request, 1)

		listing, err := f.service.List(hostproto.EverySession)
		if err != nil {
			t.Fatal(err)
		}
		var listed []hostproto.TicketID
		for _, item := range listing.Items {
			listed = append(listed, item.Ticket.TicketID)
			if item.Role != f.request.Role || !reflect.DeepEqual(item.View, f.request.ViewFor(item.Ticket)) {
				t.Fatalf("item %+v", item)
			}
		}
		want := []hostproto.TicketID{f.ticket.TicketID, secondVote.TicketID}
		if !slices.Equal(listed, want) {
			t.Fatalf("listed %v, want %v", listed, want)
		}
	})

	t.Run("a session lists the tickets of the requests it holds, each with its sessions, and no filter lists them all [verifies SP-2151~1]", func(t *testing.T) {
		f := newFixture(t)
		scan := hostproto.ScanSession("3f5a9c")
		scanRequest, scanTicket := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "scan" }), 0)
		sharedRequest, sharedTicket := f.emit(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "shared" }), 0)
		for request, sessions := range map[*hostproto.Request][]hostproto.Session{&scanRequest: {scan}, &sharedRequest: {hostproto.SessionProve, scan}} {
			if err := f.store.Record(hostproto.QueueItem{RequestID: hostprototest.MustID(*request), Sessions: sessions}); err != nil {
				t.Fatal(err)
			}
		}
		listedFor := func(filter hostproto.SessionFilter) map[hostproto.TicketID][]hostproto.Session {
			listing, err := f.service.List(filter)
			if err != nil {
				t.Fatal(err)
			}
			listed := map[hostproto.TicketID][]hostproto.Session{}
			for _, item := range listing.Items {
				listed[item.Ticket.TicketID] = item.Sessions
			}
			return listed
		}
		prove := listedFor(hostproto.OnlySession(hostproto.SessionProve))
		if len(prove) != 2 || prove[f.ticket.TicketID] == nil || prove[sharedTicket.TicketID] == nil {
			t.Fatalf("prove lists %v", prove)
		}
		scanned := listedFor(hostproto.OnlySession(scan))
		if len(scanned) != 2 || !slices.Equal(scanned[scanTicket.TicketID], []hostproto.Session{scan}) ||
			!slices.Equal(scanned[sharedTicket.TicketID], []hostproto.Session{hostproto.SessionProve, scan}) {
			t.Fatalf("the scan lists %v", scanned)
		}
		if every := listedFor(hostproto.EverySession); len(every) != 3 || !slices.Equal(every[f.ticket.TicketID], []hostproto.Session{hostproto.SessionProve}) {
			t.Fatalf("every session lists %v", every)
		}
	})

	t.Run("an empty store lists no items, not null [verifies SP-702~1]", func(t *testing.T) {
		store := hostprototest.NewMemoryHostStore()
		listing, err := Service{Store: store}.List(hostproto.EverySession)
		encoded, _ := json.Marshal(listing)
		if err != nil || string(encoded) != `{"items":[]}` {
			t.Fatalf("listing %s, error %v", encoded, err)
		}
	})
}

func TestShow(t *testing.T) {
	t.Run("showing a ticket fetches it and returns what an executor sees [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		view := f.show()
		if f.state() != hostproto.StateFetched {
			t.Fatalf("state %s", f.state())
		}
		if view.Ticket != f.ticket.TicketID.String() || view.Nonce != f.ticket.Nonce || view.System != f.request.Prompt.System || view.User != f.request.Prompt.User {
			t.Fatalf("view %+v", view)
		}
		if kinds := f.journalKinds(); !slices.Equal(kinds, []hostproto.EventKind{hostproto.EventFetched}) {
			t.Fatalf("journal %v", kinds)
		}
		listing, _ := f.service.List(hostproto.EverySession)
		if len(listing.Items) != 0 {
			t.Fatalf("a fetched ticket is still listed: %+v", listing)
		}
	})

	refusals := []struct {
		name   string
		ticket func(f *fixture) hostproto.TicketID
		prep   func(f *fixture)
		want   error
	}{
		{"another emission of the ticket", func(f *fixture) hostproto.TicketID {
			id := f.ticket.TicketID
			id.Emission = secondEmission
			return id
		}, func(*fixture) {}, hostproto.ErrTicketMismatch},
		{"a ticket that already holds an answer", func(f *fixture) hostproto.TicketID { return f.ticket.TicketID }, func(f *fixture) {
			f.show()
			if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
				f.t.Fatal(err)
			}
		}, hostproto.ErrNotSubmittable},
	}
	for _, tt := range refusals {
		t.Run("showing "+tt.name+" is refused and journaled as an exposure attempt [verifies SP-702~1]", func(t *testing.T) {
			f := newFixture(t)
			tt.prep(f)
			_, err := f.service.Show(tt.ticket(f), "")
			if !errors.Is(err, tt.want) {
				t.Fatalf("error %v, want %v", err, tt.want)
			}
			if kinds := f.journalKinds(); !slices.Contains(kinds, hostproto.EventExposureAttempt) && tt.want != hostproto.ErrTicketMismatch {
				t.Fatalf("journal %v", kinds)
			}
		})
	}

	t.Run("showing an unknown ticket fails with not found [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		unknown := hostprototest.Ticket(hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = "unknown" }), 0, hostproto.FirstEmission)
		if _, err := f.service.Show(unknown.TicketID, ""); !errors.Is(err, hostproto.ErrNotFound) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestSubmit(t *testing.T) {
	t.Run("a valid answer is accepted and recorded with an inline profile [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		receipt, err := f.service.Submit(f.submission(validAnswer))
		if err != nil || receipt.Text == "" {
			t.Fatalf("receipt %+v, error %v", receipt, err)
		}
		response := f.response()
		if f.state() != hostproto.StateAccepted || string(response.Output) != validAnswer || response.Nonce != f.ticket.Nonce {
			t.Fatalf("state %s response %+v", f.state(), response)
		}
	})

	t.Run("a submission without a capability is journaled as exposed and its profile claims no more than inline [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
			t.Fatal(err)
		}
		executor := f.response().Executor
		if executor.Channel != format.ChannelHostInline || executor.Isolation != format.IsolationInline || executor.Attestation != (format.Attestation{}) {
			t.Fatalf("executor %+v", executor)
		}
		if kinds := f.journalKinds(); !slices.Contains(kinds, hostproto.EventExposed) || !slices.Contains(kinds, hostproto.EventAccepted) {
			t.Fatalf("journal %v", kinds)
		}
	})

	t.Run("the executor profile of a command-line submission ignores the one it names [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		claimed := hostprototest.Profile()
		submission := f.submission(validAnswer)
		submission.Executor = &claimed
		if _, err := f.service.Submit(submission); err != nil {
			t.Fatal(err)
		}
		if f.response().Executor.Channel != format.ChannelHostInline {
			t.Fatalf("executor %+v", f.response().Executor)
		}
	})

	origins := []struct {
		name     string
		origin   Origin
		executor format.ExecutorProfile
	}{
		{"replayed", OriginReplay, hostprototest.Profile()},
		{"relayed", OriginRelay, RelayProfile(relaySource)},
	}
	for _, tt := range origins {
		t.Run("a "+tt.name+" answer keeps the executor it is given and is not journaled as exposed [verifies SP-702~1]", func(t *testing.T) {
			f := newFixture(t)
			f.show()
			submission := f.submission(validAnswer)
			submission.Origin, submission.Executor = tt.origin, &tt.executor
			if _, err := f.service.Submit(submission); err != nil {
				t.Fatal(err)
			}
			if got := f.response().Executor; got != tt.executor {
				t.Fatalf("executor %+v, want %+v", got, tt.executor)
			}
			if kinds := f.journalKinds(); slices.Contains(kinds, hostproto.EventExposed) {
				t.Fatalf("journal %v", kinds)
			}
		})
	}

	t.Run("a schema failure allows one correction and the second rejects the ticket [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		first, err := f.service.Submit(f.submission(brokenAnswer))
		if !errors.Is(err, hostproto.ErrSchemaViolation) || !first.CorrectionAllowed || f.state() != hostproto.StateFetched {
			t.Fatalf("first failure: %+v, %v, state %s", first, err, f.state())
		}
		second, err := f.service.Submit(f.submission(brokenAnswer))
		if !errors.Is(err, hostproto.ErrSchemaViolation) || second.CorrectionAllowed || f.state() != hostproto.StateRejected {
			t.Fatalf("second failure: %+v, %v, state %s", second, err, f.state())
		}
		kinds := f.journalKinds()
		if count(kinds, hostproto.EventSchemaFailed) != hostproto.MaxCorrections+1 || !slices.Contains(kinds, hostproto.EventRejected) {
			t.Fatalf("journal %v", kinds)
		}
	})

	t.Run("a corrected answer is accepted after one schema failure [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		if _, err := f.service.Submit(f.submission(brokenAnswer)); err == nil {
			t.Fatal("the broken answer was accepted")
		}
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil || f.state() != hostproto.StateAccepted {
			t.Fatalf("state %s, error %v", f.state(), err)
		}
	})

	t.Run("a nonce of another issue is refused and journaled as rejected [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		submission := f.submission(validAnswer)
		submission.Nonce = wrongNonce
		_, err := f.service.Submit(submission)
		if !errors.Is(err, hostproto.ErrNonceMismatch) || f.state() != hostproto.StateFetched || !slices.Contains(f.journalKinds(), hostproto.EventRejected) {
			t.Fatalf("error %v, state %s, journal %v", err, f.state(), f.journalKinds())
		}
	})

	t.Run("an answer for another issue of the ticket is refused [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		submission := f.submission(validAnswer)
		submission.Ticket.Emission = secondEmission
		if _, err := f.service.Submit(submission); !errors.Is(err, hostproto.ErrTicketMismatch) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an answer for a ticket that was never shown is refused [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.service.Submit(f.submission(validAnswer)); !errors.Is(err, hostproto.ErrNotSubmittable) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("a different second answer is a journaled resubmission conflict and changes nothing [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
			t.Fatal(err)
		}
		_, err := f.service.Submit(f.submission(otherAnswer))
		if !errors.Is(err, hostproto.ErrResubmissionConflict) || string(f.response().Output) != validAnswer {
			t.Fatalf("error %v, output %s", err, f.response().Output)
		}
		if !slices.Contains(f.journalKinds(), hostproto.EventResubmissionConflict) {
			t.Fatalf("journal %v", f.journalKinds())
		}
	})

	t.Run("an identical replay of the answer is idempotent [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		f.show()
		first, err := f.service.Submit(f.submission(validAnswer))
		if err != nil {
			t.Fatal(err)
		}
		again, err := f.service.Submit(f.submission(reformattedValid))
		if err != nil || again != first {
			t.Fatalf("replay %+v then %+v, error %v", first, again, err)
		}
		if count(f.journalKinds(), hostproto.EventAccepted) != 1 || count(f.journalKinds(), hostproto.EventExposed) != 1 {
			t.Fatalf("journal %v", f.journalKinds())
		}
	})

	t.Run("an answer presented with a capability of a started agent is attested and not exposed [verifies SP-702~1]", func(t *testing.T) {
		f := newFixture(t)
		secret, err := f.store.Secret()
		if err != nil {
			t.Fatal(err)
		}
		capability, err := hostproto.FormatCap(secret, agentIdentifier)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.store.Append(hostproto.Event{Kind: hostproto.EventAgentStarted, AgentID: agentIdentifier, Role: f.request.Role}); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Show(f.ticket.TicketID, capability); err != nil {
			t.Fatal(err)
		}
		submission := f.submission(validAnswer)
		submission.Cap = capability
		if _, err := f.service.Submit(submission); err != nil {
			t.Fatal(err)
		}
		if got := f.response().Executor.Attestation.AgentID; got != agentIdentifier {
			t.Fatalf("agent %q", got)
		}
		if slices.Contains(f.journalKinds(), hostproto.EventExposed) {
			t.Fatalf("journal %v", f.journalKinds())
		}
	})
}

func count(kinds []hostproto.EventKind, kind hostproto.EventKind) int {
	return len(slices.DeleteFunc(slices.Clone(kinds), func(k hostproto.EventKind) bool { return k != kind }))
}

// fakeRecordings serves recordings from a map and fails for an absent request.
type fakeRecordings map[string]Recording

func (r fakeRecordings) Load(requestID string) (Recording, error) {
	recording, found := r[requestID]
	if !found {
		return Recording{}, fmt.Errorf("no file for %s", requestID)
	}
	return recording, nil
}

func TestReplayer(t *testing.T) {
	const pendingRequests = 3
	setup := func(t *testing.T) (*fixture, []PendingTicket, fakeRecordings) {
		f := newFixture(t)
		pending := []PendingTicket{{Ticket: f.ticket.TicketID.String(), Nonce: f.ticket.Nonce}}
		recordings := fakeRecordings{string(f.ticket.RequestID): {Output: []byte(validAnswer)}}
		for index := 1; index < pendingRequests; index++ {
			request := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = fmt.Sprintf("pending %d", index) })
			_, ticket := f.emit(request, 0)
			pending = append(pending, PendingTicket{Ticket: ticket.TicketID.String(), Nonce: ticket.Nonce})
			recordings[string(ticket.RequestID)] = Recording{Output: []byte(validAnswer), Executor: ptr(hostprototest.Profile())}
		}
		return f, pending, recordings
	}

	t.Run("every pending ticket is answered from the recording of its request [verifies SP-720~1]", func(t *testing.T) {
		f, pending, recordings := setup(t)
		replayer := RecordingReplayer{Service: f.service, Recordings: recordings}
		if err := replayer.Serve(pending); err != nil {
			t.Fatal(err)
		}
		entries, _ := f.store.ListEntries()
		if len(entries) != pendingRequests {
			t.Fatalf("%d entries", len(entries))
		}
		for _, entry := range entries {
			if entry.State != hostproto.StateAccepted {
				t.Fatalf("ticket %s is %s", entry.Ticket.TicketID, entry.State)
			}
		}
		if slices.Contains(f.journalKinds(), hostproto.EventExposed) {
			t.Fatalf("a replayed answer is journaled as exposed: %v", f.journalKinds())
		}
	})

	t.Run("the recorded executor is kept and a missing one gives the inline profile [verifies SP-720~1]", func(t *testing.T) {
		f, pending, recordings := setup(t)
		if err := (RecordingReplayer{Service: f.service, Recordings: recordings}).Serve(pending); err != nil {
			t.Fatal(err)
		}
		recorded, _ := hostproto.ParseTicketID(pending[1].Ticket)
		response, err := f.store.GetResponse(recorded.Key())
		if err != nil || response.Executor != hostprototest.Profile() {
			t.Fatalf("recorded executor: %+v, %v", response.Executor, err)
		}
		if f.response().Executor.Channel != format.ChannelHostInline {
			t.Fatalf("an absent executor must give the inline profile, got %+v", f.response().Executor)
		}
	})

	t.Run("a request without a recording is a replay miss [verifies SP-720~1]", func(t *testing.T) {
		f, pending, recordings := setup(t)
		delete(recordings, string(f.ticket.RequestID))
		err := RecordingReplayer{Service: f.service, Recordings: recordings}.Serve(pending)
		if !errors.Is(err, ErrReplayMiss) {
			t.Fatalf("error %v", err)
		}
		if f.state() != hostproto.StateEmitted {
			t.Fatalf("a ticket without a recording moved to %s", f.state())
		}
	})

	t.Run("a pending ticket that is not a ticket identifier is refused [verifies SP-720~1]", func(t *testing.T) {
		f, _, recordings := setup(t)
		err := RecordingReplayer{Service: f.service, Recordings: recordings}.Serve([]PendingTicket{{Ticket: "not-a-ticket"}})
		if !errors.Is(err, hostproto.ErrBadTicket) {
			t.Fatalf("error %v", err)
		}
	})

	t.Run("an answer that breaks the schema fails the replay [verifies SP-720~1]", func(t *testing.T) {
		f, pending, recordings := setup(t)
		recordings[string(f.ticket.RequestID)] = Recording{Output: []byte(brokenAnswer)}
		err := RecordingReplayer{Service: f.service, Recordings: recordings}.Serve(pending)
		if !errors.Is(err, hostproto.ErrSchemaViolation) {
			t.Fatalf("error %v", err)
		}
	})
}

func ptr[T any](value T) *T { return &value }
