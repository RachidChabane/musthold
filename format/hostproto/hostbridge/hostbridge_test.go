package hostbridge

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	threeVotes       = 3
	oneVote          = 1
	firstWave        = 1
	askPurpose       = "judge"
	justPastFetchTTL = hostproto.FetchTTL + time.Second
	justBeforeFetch  = hostproto.FetchTTL - time.Second
	propertyRequests = 3
	propertyVotes    = 2
	propertyMaxSteps = 60
	byteBits         = 8
	entropyStep      = 7
	entropyModulus   = 251
	waveSeedStride   = 1000
)

// sequenceReader is a deterministic entropy source: every byte differs from the
// previous one, so every nonce it makes is new.
type sequenceReader struct{ next uint64 }

func (r *sequenceReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(r.next >> (byteBits * (i % byteBits)) % entropyModulus)
		r.next += entropyStep
	}
	return len(p), nil
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }

type rig struct {
	t      testing.TB
	store  *hostprototest.MemoryHostStore
	clock  *fakeClock
	bridge *Bridge
	// lastAnswer is the answer the test last read.
	lastAnswer Answer
}

func newRig(t testing.TB) *rig {
	t.Helper()
	store := hostprototest.NewMemoryHostStore()
	clock := &fakeClock{now: hostprototest.Epoch}
	return &rig{t: t, store: store, clock: clock, bridge: New(store, clock.Now, &sequenceReader{}, hostproto.SessionProve, firstWave)}
}

func (r *rig) bridgeOnWave(wave int) *Bridge {
	return New(r.store, r.clock.Now, &sequenceReader{next: uint64(wave) * waveSeedStride}, hostproto.SessionProve, wave)
}

func variant(label string) hostproto.Request {
	return hostprototest.Request(func(request *hostproto.Request) { request.Prompt.User = label })
}

func (r *rig) want(votes int, request hostproto.Request) Answer {
	r.t.Helper()
	answer, err := r.bridge.Want(Ask{Request: request, Votes: votes, Criteria: []string{"SP-1"}, Purpose: askPurpose})
	if err != nil {
		r.t.Fatal(err)
	}
	r.lastAnswer = answer
	return answer
}

func (r *rig) entry(ticket hostproto.TicketID) hostproto.Entry {
	r.t.Helper()
	entry, err := r.store.GetEntry(ticket.Key())
	if err != nil {
		r.t.Fatal(err)
	}
	return entry
}

func (r *rig) fetch(ticket hostproto.TicketID) {
	r.t.Helper()
	if _, err := r.store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Fetch("", r.clock.Now()) }); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) failSchemaTwice(ticket hostproto.TicketID) {
	r.t.Helper()
	r.fetch(ticket)
	for range hostproto.MaxCorrections + 1 {
		if _, err := r.store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
			next, _, err := e.FailSchema()
			return next, err
		}); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *rig) answer(ticket hostproto.TicketID, output string) {
	r.t.Helper()
	r.fetchIfEmitted(ticket)
	digest, err := hostproto.OutputDigest([]byte(output))
	if err != nil {
		r.t.Fatal(err)
	}
	entry, err := r.store.UpdateEntry(ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Accept(digest) })
	if err != nil {
		r.t.Fatal(err)
	}
	response := hostprototest.Response(entry.Ticket)
	response.Output = json.RawMessage(output)
	if err := r.store.PutResponse(response); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) fetchIfEmitted(ticket hostproto.TicketID) {
	if r.entry(ticket).State == hostproto.StateEmitted {
		r.fetch(ticket)
	}
}

func (r *rig) journalKinds(ticket hostproto.TicketID) []hostproto.EventKind {
	r.t.Helper()
	records, err := r.store.Records()
	if err != nil {
		r.t.Fatal(err)
	}
	var kinds []hostproto.EventKind
	for _, record := range records {
		if record.Event.Ticket == ticket.String() {
			kinds = append(kinds, record.Event.Kind)
		}
	}
	return kinds
}

func states(answer Answer) []hostproto.State {
	var out []hostproto.State
	for _, vote := range answer.Votes {
		out = append(out, vote.State)
	}
	return out
}

func TestEmission(t *testing.T) {
	t.Run("one ticket is emitted per vote and the request is recorded in the queue [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		request := variant("emission")
		answer := r.want(threeVotes, request)

		if answer.Standing != Pending || len(answer.Votes) != threeVotes {
			t.Fatalf("standing %s with %d votes, want pending with %d", answer.Standing, len(answer.Votes), threeVotes)
		}
		nonces := map[string]bool{}
		for vote, state := range answer.Votes {
			entry := r.entry(state.Ticket)
			if state.State != hostproto.StateEmitted || state.Ticket.Vote != vote || state.Ticket.Emission != hostproto.FirstEmission || state.Ticket.RequestID != answer.ID {
				t.Fatalf("vote %d: %+v", vote, state)
			}
			nonces[entry.Ticket.Nonce] = true
		}
		if len(nonces) != threeVotes {
			t.Fatalf("%d distinct nonces, want %d", len(nonces), threeVotes)
		}
		items, _ := r.store.Items()
		if len(items) != 1 || items[0].RequestID != answer.ID || items[0].Wave != firstWave || items[0].Purpose != askPurpose {
			t.Fatalf("queue %+v", items)
		}
		if _, err := r.store.GetRequest(answer.ID); err != nil {
			t.Fatalf("the request is not stored: %v", err)
		}
		if !r.bridge.Waiting() {
			t.Fatal("a bridge with pending tickets must report waiting")
		}
	})

	t.Run("asking again emits nothing new [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		first := r.want(threeVotes, variant("again"))
		second := r.want(threeVotes, variant("again"))
		entries, _ := r.store.ListEntries()
		if len(entries) != threeVotes || first.Votes[0].Ticket != second.Votes[0].Ticket {
			t.Fatalf("%d entries, tickets %v then %v", len(entries), first.Votes[0].Ticket, second.Votes[0].Ticket)
		}
	})
}

func TestReissue(t *testing.T) {
	t.Run("a rejected ticket is reissued with a new nonce [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		first := r.want(oneVote, variant("rejected"))
		before := r.entry(first.Votes[0].Ticket)
		r.failSchemaTwice(before.Ticket.TicketID)

		again := r.want(oneVote, variant("rejected"))
		after := r.entry(again.Votes[0].Ticket)
		if after.State != hostproto.StateEmitted || after.Ticket.Emission != hostproto.FirstEmission+1 || after.Ticket.Nonce == before.Ticket.Nonce {
			t.Fatalf("reissue: %+v after %+v", after, before)
		}
		if kinds := r.journalKinds(after.Ticket.TicketID); !slices.Equal(kinds, []hostproto.EventKind{hostproto.EventEmitted}) {
			t.Fatalf("journal of the new issue: %v", kinds)
		}
	})

	t.Run("a fetched ticket stays fetched until the time to live passed, then it is reissued [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		first := r.want(oneVote, variant("expired"))
		ticket := first.Votes[0].Ticket
		r.fetch(ticket)

		r.clock.Advance(justBeforeFetch)
		waiting := r.want(oneVote, variant("expired"))
		if waiting.Votes[0].State != hostproto.StateFetched || waiting.Standing != Pending {
			t.Fatalf("before the time to live: %+v", waiting)
		}

		r.clock.Advance(justPastFetchTTL - justBeforeFetch)
		reissued := r.want(oneVote, variant("expired"))
		if reissued.Votes[0].State != hostproto.StateEmitted || reissued.Votes[0].Ticket.Emission != hostproto.FirstEmission+1 {
			t.Fatalf("after the time to live: %+v", reissued.Votes[0])
		}
		if kinds := r.journalKinds(ticket); !slices.Contains(kinds, hostproto.EventExpired) {
			t.Fatalf("the expiry is not journaled: %v", kinds)
		}
	})

	t.Run("a ticket is abandoned after its last emission and the request is unresponsive [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		request := variant("abandoned")
		answer := r.want(oneVote, request)
		for range hostproto.DefaultMaxEmissions - 1 {
			r.failSchemaTwice(answer.Votes[0].Ticket)
			answer = r.want(oneVote, request)
		}
		last := answer.Votes[0].Ticket
		if last.Emission != hostproto.DefaultMaxEmissions {
			t.Fatalf("last emission %d", last.Emission)
		}
		r.failSchemaTwice(last)
		final := r.want(oneVote, request)

		entry := r.entry(last)
		if final.Standing != Unresponsive || final.Votes[0].State != hostproto.StateAbandoned || entry.Reason != hostproto.ReasonHostUnresponsive {
			t.Fatalf("final %+v entry %+v", final, entry)
		}
		if kinds := r.journalKinds(last); kinds[len(kinds)-1] != hostproto.EventAbandoned {
			t.Fatalf("journal %v", kinds)
		}
	})
}

func TestReadingVotes(t *testing.T) {
	const output = `{"validity":"VALID"}`
	tests := []struct {
		name      string
		advance   func(r *rig, ticket hostproto.TicketID)
		wantState hostproto.State
		wantHolds bool
	}{
		{"accepted", func(*rig, hostproto.TicketID) {}, hostproto.StateAccepted, true},
		{"validated", func(r *rig, _ hostproto.TicketID) { r.mustValidate() }, hostproto.StateValidated, true},
		{"consumed", func(r *rig, _ hostproto.TicketID) { r.mustValidate(); r.mustConsume() }, hostproto.StateConsumed, true},
	}
	for _, tt := range tests {
		t.Run(tt.name+" votes expose their output and executor [verifies SP-714~2]", func(t *testing.T) {
			r := newRig(t)
			request := variant("reading " + tt.name)
			first := r.want(oneVote, request)
			r.answer(first.Votes[0].Ticket, output)
			tt.advance(r, first.Votes[0].Ticket)

			answer := r.want(oneVote, request)
			vote := answer.Votes[0]
			if vote.State != tt.wantState || !vote.Holds() || answer.Standing != Answered {
				t.Fatalf("vote %+v, standing %s", vote, answer.Standing)
			}
			if string(vote.Output) != output || vote.Executor.Harness.Name != hostprototest.HarnessName {
				t.Fatalf("output %s executor %+v", vote.Output, vote.Executor)
			}
		})
	}

	for _, state := range []hostproto.State{hostproto.StateEmitted, hostproto.StateFetched} {
		t.Run("a vote in state "+string(state)+" exposes nothing [verifies SP-714~2]", func(t *testing.T) {
			r := newRig(t)
			request := variant("hidden " + string(state))
			first := r.want(oneVote, request)
			ticket := first.Votes[0].Ticket
			if state == hostproto.StateFetched {
				r.fetch(ticket)
			}
			vote := r.want(oneVote, request).Votes[0]
			if vote.State != state || vote.Holds() || vote.Output != nil {
				t.Fatalf("vote %+v", vote)
			}
		})
	}

	markTests := []struct {
		kind   hostproto.EventKind
		marked bool
	}{
		{hostproto.EventLeaked, true},
		{hostproto.EventExposed, true},
		{hostproto.EventResubmissionConflict, true},
		{hostproto.EventAccepted, false},
		{hostproto.EventSchemaFailed, false},
	}
	for _, tt := range markTests {
		t.Run("the journal event "+string(tt.kind)+" sets the mark as expected [verifies SP-714~2]", func(t *testing.T) {
			r := newRig(t)
			request := variant("marks " + string(tt.kind))
			first := r.want(twoVotes, request)
			marked := first.Votes[0].Ticket
			r.answer(marked, output)
			r.answer(first.Votes[1].Ticket, output)
			if _, err := r.store.Append(hostproto.Event{Kind: tt.kind, Ticket: marked.String()}); err != nil {
				t.Fatal(err)
			}
			answer := r.want(twoVotes, request)
			if answer.Votes[0].Marked != tt.marked || answer.Votes[1].Marked {
				t.Fatalf("marks %v %v, want %v and false", answer.Votes[0].Marked, answer.Votes[1].Marked, tt.marked)
			}
		})
	}
}

const twoVotes = 2

func (r *rig) mustValidate() {
	r.t.Helper()
	if err := r.bridge.Validate(r.lastAnswer); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) mustConsume() {
	r.t.Helper()
	if err := r.bridge.Consume(r.lastAnswer); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) answerThenContradict(ticket hostproto.TicketID, output string) {
	r.t.Helper()
	r.answer(ticket, output)
	if err := r.bridge.Contradict(VoteState{Ticket: ticket}, "contradicted"); err != nil {
		r.t.Fatal(err)
	}
}

func TestTransitions(t *testing.T) {
	t.Run("validate moves accepted votes only, consume moves validated votes only [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		request := variant("transitions")
		answer := r.want(threeVotes, request)
		r.answer(answer.Votes[0].Ticket, `{"validity":"VALID"}`)
		r.answer(answer.Votes[1].Ticket, `{"validity":"VALID"}`)
		r.fetch(answer.Votes[2].Ticket)

		if err := r.bridge.Consume(answer); err != nil {
			t.Fatal(err)
		}
		if got := states(r.want(threeVotes, request)); !slices.Equal(got, []hostproto.State{hostproto.StateAccepted, hostproto.StateAccepted, hostproto.StateFetched}) {
			t.Fatalf("consume moved accepted votes: %v", got)
		}
		if err := r.bridge.Validate(answer); err != nil {
			t.Fatal(err)
		}
		if got := states(r.want(threeVotes, request)); !slices.Equal(got, []hostproto.State{hostproto.StateValidated, hostproto.StateValidated, hostproto.StateFetched}) {
			t.Fatalf("validate: %v", got)
		}
		if err := r.bridge.Consume(answer); err != nil {
			t.Fatal(err)
		}
		if got := states(r.want(threeVotes, request)); !slices.Equal(got, []hostproto.State{hostproto.StateConsumed, hostproto.StateConsumed, hostproto.StateFetched}) {
			t.Fatalf("consume: %v", got)
		}
		kinds := r.journalKinds(answer.Votes[0].Ticket)
		if !slices.Contains(kinds, hostproto.EventValidated) || !slices.Contains(kinds, hostproto.EventConsumed) {
			t.Fatalf("journal %v", kinds)
		}
	})

	t.Run("contradict rejects an accepted vote and the next ask reissues it [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		request := variant("contradict")
		first := r.want(oneVote, request)
		ticket := first.Votes[0].Ticket
		r.answerThenContradict(ticket, `{"validity":"VALID"}`)

		entry := r.entry(ticket)
		if entry.State != hostproto.StateRejected || entry.Reason != "contradicted" {
			t.Fatalf("entry %+v", entry)
		}
		again := r.want(oneVote, request)
		if again.Votes[0].Ticket.Emission != hostproto.FirstEmission+1 || again.Votes[0].State != hostproto.StateEmitted {
			t.Fatalf("after contradict: %+v", again.Votes[0])
		}
	})

	t.Run("contradict of a vote that is not accepted fails [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		first := r.want(oneVote, variant("not accepted"))
		err := r.bridge.Contradict(first.Votes[0], "reason")
		if !errors.Is(err, hostproto.ErrIllegalTransition) {
			t.Fatalf("error %v", err)
		}
	})
}

func TestSupersedeUnwanted(t *testing.T) {
	t.Run("only emitted tickets of requests nobody asked for in the wave are superseded [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		previous := r.bridgeOnWave(firstWave - 1)
		stale := variant("stale")
		fetchedStale := variant("fetched stale")
		answeredStale := variant("answered stale")
		for _, request := range []hostproto.Request{stale, fetchedStale, answeredStale} {
			if _, err := previous.Want(Ask{Request: request, Votes: oneVote}); err != nil {
				t.Fatal(err)
			}
		}
		r.fetch(r.ticketOf(fetchedStale))
		r.answer(r.ticketOf(answeredStale), `{"validity":"VALID"}`)
		wanted := r.want(oneVote, variant("wanted"))

		if err := r.bridge.SupersedeUnwanted(); err != nil {
			t.Fatal(err)
		}
		got := map[hostproto.State]hostproto.State{
			hostproto.StateSuperseded: r.entry(r.ticketOf(stale)).State,
			hostproto.StateFetched:    r.entry(r.ticketOf(fetchedStale)).State,
			hostproto.StateAccepted:   r.entry(r.ticketOf(answeredStale)).State,
			hostproto.StateEmitted:    r.entry(wanted.Votes[0].Ticket).State,
		}
		for want, state := range got {
			if state != want {
				t.Errorf("state %s, want %s", state, want)
			}
		}
		if kinds := r.journalKinds(r.ticketOf(stale)); kinds[len(kinds)-1] != hostproto.EventSuperseded {
			t.Fatalf("journal %v", kinds)
		}
	})
}

func (r *rig) ticketOf(request hostproto.Request) hostproto.TicketID {
	r.t.Helper()
	entry := r.entry(hostproto.TicketID{RequestID: hostprototest.MustID(request), Vote: 0, Emission: hostproto.FirstEmission})
	return entry.Ticket.TicketID
}

func TestPendingTickets(t *testing.T) {
	t.Run("pending tickets are the emitted and the fetched ones in store order [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		request := variant("pending")
		answer := r.want(4, request)
		r.fetch(answer.Votes[1].Ticket)
		r.answer(answer.Votes[2].Ticket, `{"validity":"VALID"}`)
		r.failSchemaTwice(answer.Votes[3].Ticket)

		pending, err := r.bridge.PendingTickets()
		if err != nil {
			t.Fatal(err)
		}
		var votes []int
		for _, ticket := range pending {
			votes = append(votes, ticket.Vote)
		}
		if !slices.Equal(votes, []int{0, 1}) {
			t.Fatalf("pending votes %v", votes)
		}
	})
}

func TestWaiting(t *testing.T) {
	t.Run("the bridge waits exactly while an asked request holds a pending ticket [verifies SP-714~2]", func(t *testing.T) {
		r := newRig(t)
		if r.bridge.Waiting() {
			t.Fatal("a new bridge does not wait")
		}
		request := variant("waiting")
		answer := r.want(oneVote, request)
		if !r.bridge.Waiting() {
			t.Fatal("the bridge must wait for the emitted ticket")
		}
		r.answer(answer.Votes[0].Ticket, `{"validity":"VALID"}`)
		fresh := r.bridgeOnWave(firstWave)
		got, err := fresh.Want(Ask{Request: request, Votes: oneVote})
		if err != nil || got.Standing != Answered || fresh.Waiting() {
			t.Fatalf("answered request: %+v, waiting %v, error %v", got, fresh.Waiting(), err)
		}
	})
}

// bridgeModel drives random sequences of host and bridge actions against two
// bridges that share a store, and checks the invariants after every step.
type bridgeModel struct {
	t          *rapid.T
	rig        *rig
	bridges    []*Bridge
	requests   []hostproto.Request
	superseded map[hostproto.TicketID]bool
}

func (m *bridgeModel) pickRequest() hostproto.Request {
	return m.requests[rapid.IntRange(0, len(m.requests)-1).Draw(m.t, "request")]
}

func (m *bridgeModel) pickTicket() (hostproto.Entry, bool) {
	entries, _ := m.rig.store.ListEntries()
	if len(entries) == 0 {
		return hostproto.Entry{}, false
	}
	return entries[rapid.IntRange(0, len(entries)-1).Draw(m.t, "entry")], true
}

func (m *bridgeModel) host(action func(hostproto.Entry)) {
	if entry, found := m.pickTicket(); found {
		action(entry)
	}
}

func (m *bridgeModel) checkAnswer(answer Answer) {
	for _, vote := range answer.Votes {
		if vote.Holds() != (vote.Output != nil) {
			m.t.Fatalf("vote %+v: holds %v with output %s", vote, vote.Holds(), vote.Output)
		}
		if m.superseded[vote.Ticket] && vote.State != hostproto.StateSuperseded {
			m.t.Fatalf("superseded ticket %s revived as %s", vote.Ticket, vote.State)
		}
	}
}

// checkSuperseded checks that a superseded ticket never comes back under its own
// emission, and that a ticket is superseded only when no live session holds its
// request.
func (m *bridgeModel) checkSuperseded() {
	entries, _ := m.rig.store.ListEntries()
	items, _ := m.rig.store.Items()
	states, _ := m.rig.store.Sessions()
	sessions, liveness := hostproto.IndexSessions(items), hostproto.LivenessOf(states)
	for _, entry := range entries {
		ticket := entry.Ticket.TicketID
		switch {
		case entry.State == hostproto.StateSuperseded && !m.superseded[ticket]:
			if liveness.AnyLive(sessions.Of(ticket.RequestID)) {
				m.t.Fatalf("ticket %s was superseded while %v hold its request", ticket, sessions.Of(ticket.RequestID))
			}
			m.superseded[ticket] = true
		case entry.State != hostproto.StateSuperseded && m.superseded[ticket]:
			m.t.Fatalf("superseded ticket %s is now %s", ticket, entry.State)
		}
	}
}

func TestBridgeProperty(t *testing.T) {
	t.Run("the bridges of two sessions expose only accepted, validated or consumed answers and supersede only what no live session holds [verifies SP-714~2] [verifies SP-2148~1]", func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			r := newRig(t)
			m := &bridgeModel{t: rt, rig: r, bridges: []*Bridge{r.on(hostproto.SessionProve), r.on(scanSession)}, superseded: map[hostproto.TicketID]bool{}}
			for index := range propertyRequests {
				m.requests = append(m.requests, variant("property "+string(rune('a'+index))))
			}
			steps := map[string]func(){
				"want": func() {
					bridge := m.bridges[rapid.IntRange(0, 1).Draw(rt, "bridge")]
					answer, err := bridge.Want(Ask{Request: m.pickRequest(), Votes: propertyVotes})
					if err != nil {
						rt.Fatal(err)
					}
					m.checkAnswer(answer)
				},
				"fetch": func() {
					m.host(func(e hostproto.Entry) {
						_, _ = r.store.UpdateEntry(e.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Fetch("", r.clock.Now()) })
					})
				},
				"answer": func() {
					m.host(func(e hostproto.Entry) { m.answerIfFetched(e) })
				},
				"fail": func() {
					m.host(func(e hostproto.Entry) {
						_, _ = r.store.UpdateEntry(e.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
							next, _, err := e.FailSchema()
							return next, err
						})
					})
				},
				"time": func() {
					r.clock.Advance(rapid.SampledFrom([]time.Duration{time.Minute, justPastFetchTTL}).Draw(rt, "delta"))
				},
				"validate": func() {
					m.bridgeMove(func(b *Bridge, a Answer) error { return b.Validate(a) })
				},
				"consume": func() {
					m.bridgeMove(func(b *Bridge, a Answer) error { return b.Consume(a) })
				},
				"supersede": func() {
					if err := m.bridges[rapid.IntRange(0, 1).Draw(rt, "bridge")].SupersedeUnwanted(); err != nil {
						rt.Fatal(err)
					}
				},
				"end": func() {
					if err := m.bridges[rapid.IntRange(0, 1).Draw(rt, "bridge")].End(); err != nil {
						rt.Fatal(err)
					}
				},
			}
			names := make([]string, 0, len(steps))
			for name := range steps {
				names = append(names, name)
			}
			slices.Sort(names)
			for range rapid.IntRange(1, propertyMaxSteps).Draw(rt, "steps") {
				steps[rapid.SampledFrom(names).Draw(rt, "step")]()
				m.checkSuperseded()
			}
		})
	})
}

func (m *bridgeModel) answerIfFetched(entry hostproto.Entry) {
	if entry.State != hostproto.StateFetched {
		return
	}
	if _, err := m.rig.store.GetResponse(entry.Ticket.Key()); err == nil {
		return
	}
	m.rig.answer(entry.Ticket.TicketID, hostprototest.HonestAnswer)
}

func (m *bridgeModel) bridgeMove(move func(*Bridge, Answer) error) {
	request := m.pickRequest()
	bridge := m.bridges[rapid.IntRange(0, 1).Draw(m.t, "bridge")]
	answer, err := bridge.Want(Ask{Request: request, Votes: propertyVotes})
	if err != nil {
		m.t.Fatal(err)
	}
	m.checkAnswer(answer)
	if err := move(bridge, answer); err != nil {
		m.t.Fatal(err)
	}
}
