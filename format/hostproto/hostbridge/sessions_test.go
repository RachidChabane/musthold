package hostbridge

import (
	"cmp"
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	verifiesSessionSupersession = " [verifies SP-2148~1]"
	verifiesReemission          = " [verifies SP-2149~1]"
	verifiesEmissionCap         = " [verifies SP-2150~1]"
	verifiesSessionViews        = " [verifies SP-2151~1]"

	scanSeedOffset = 500
	testCap        = 3
)

var (
	scanSession  = hostproto.ScanSession("3f5a9c")
	olderScan    = hostproto.ScanSession("0b1e22")
	sessionSeeds = map[hostproto.Session]uint64{hostproto.SessionProve: 0, scanSession: scanSeedOffset, olderScan: 2 * scanSeedOffset}
)

// on returns the bridge of the session for its next wave, as a run of that
// session starts it.
func (r *rig) on(session hostproto.Session) *Bridge {
	r.t.Helper()
	wave, err := r.store.AdvanceWave(session)
	if err != nil {
		r.t.Fatal(err)
	}
	return New(r.store, r.clock.Now, &sequenceReader{next: sessionSeeds[session] + uint64(wave)*waveSeedStride}, session, wave)
}

func (r *rig) wantOn(bridge *Bridge, votes int, request hostproto.Request) Answer {
	r.t.Helper()
	answer, err := bridge.Want(Ask{Request: request, Votes: votes, Criteria: []string{"SP-1"}, Purpose: askPurpose})
	if err != nil {
		r.t.Fatal(err)
	}
	return answer
}

func (r *rig) supersedeUnwanted(bridge *Bridge) {
	r.t.Helper()
	if err := bridge.SupersedeUnwanted(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) pendingOf(bridge *Bridge) []hostproto.TicketID {
	r.t.Helper()
	pending, err := bridge.PendingTickets()
	if err != nil {
		r.t.Fatal(err)
	}
	ids := make([]hostproto.TicketID, len(pending))
	for index, ticket := range pending {
		ids[index] = ticket.TicketID
	}
	return ids
}

func (r *rig) stateOf(answer Answer) hostproto.State {
	r.t.Helper()
	return r.entry(answer.Votes[0].Ticket).State
}

func TestSupersededRequestWantedAgain(t *testing.T) {
	t.Run("a superseded request that is wanted again is reissued, not left pending with no ticket"+verifiesReemission, func(t *testing.T) {
		r := newRig(t)
		request := variant("wanted again")
		first := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)

		r.supersedeUnwanted(r.on(hostproto.SessionProve))
		if got := r.stateOf(first); got != hostproto.StateSuperseded {
			t.Fatalf("state after the unwanted wave %s", got)
		}

		third := r.on(hostproto.SessionProve)
		again := r.wantOn(third, oneVote, request)
		pending := r.pendingOf(third)
		vote := again.Votes[0]
		if vote.State != hostproto.StateEmitted || again.Standing != Pending || vote.Ticket.Emission != first.Votes[0].Ticket.Emission+1 {
			t.Fatalf("vote %+v, standing %s", vote, again.Standing)
		}
		if !slices.Equal(pending, []hostproto.TicketID{vote.Ticket}) {
			t.Fatalf("pending %v, want the re-emitted ticket %s", pending, vote.Ticket)
		}
		if r.entry(vote.Ticket).Reemissions != 1 {
			t.Fatalf("entry %+v", r.entry(vote.Ticket))
		}
		if kinds := r.journalKinds(vote.Ticket); !slices.Equal(kinds, []hostproto.EventKind{hostproto.EventEmitted}) {
			t.Fatalf("journal of the re-emission %v", kinds)
		}
	})

	t.Run("supersessions never make a ticket abandoned, host failures still do"+verifiesReemission, func(t *testing.T) {
		r := newRig(t)
		request := variant("superseded often")
		for range hostproto.DefaultMaxEmissions + 1 {
			r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
			r.supersedeUnwanted(r.on(hostproto.SessionProve))
		}
		answer := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		for range hostproto.DefaultMaxEmissions - 1 {
			if answer.Standing != Pending {
				t.Fatalf("standing %s before the host failed every emission", answer.Standing)
			}
			r.failSchemaTwice(answer.Votes[0].Ticket)
			answer = r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		}
		r.failSchemaTwice(answer.Votes[0].Ticket)
		final := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		if final.Standing != Unresponsive || r.entry(final.Votes[0].Ticket).Reason != hostproto.ReasonHostUnresponsive {
			t.Fatalf("final %+v", final)
		}
	})
}

func TestSessionBoundSupersession(t *testing.T) {
	t.Run("a scan wave leaves the emitted tickets of prove emitted"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		proved := r.wantOn(r.on(hostproto.SessionProve), oneVote, variant("prove only"))
		scan := r.on(scanSession)
		r.wantOn(scan, oneVote, variant("scan only"))
		r.supersedeUnwanted(scan)
		if got := r.stateOf(proved); got != hostproto.StateEmitted {
			t.Fatalf("the prove ticket is %s after a scan wave", got)
		}
	})

	t.Run("a prove wave leaves the emitted tickets of a scan emitted"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		scanned := r.wantOn(r.on(scanSession), oneVote, variant("scan only"))
		prove := r.on(hostproto.SessionProve)
		r.wantOn(prove, oneVote, variant("prove only"))
		r.supersedeUnwanted(prove)
		if got := r.stateOf(scanned); got != hostproto.StateEmitted {
			t.Fatalf("the scan ticket is %s after a prove wave", got)
		}
	})

	t.Run("a request two live sessions want is never superseded while the other one still holds it"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		shared := variant("shared")
		fromProve := r.wantOn(r.on(hostproto.SessionProve), oneVote, shared)
		fromScan := r.wantOn(r.on(scanSession), oneVote, shared)
		if fromScan.Votes[0].Ticket != fromProve.Votes[0].Ticket {
			t.Fatalf("the sessions got two tickets for one request: %s and %s", fromProve.Votes[0].Ticket, fromScan.Votes[0].Ticket)
		}

		r.supersedeUnwanted(r.on(scanSession))
		if got := r.stateOf(fromProve); got != hostproto.StateEmitted {
			t.Fatalf("the shared ticket is %s while prove still wants it", got)
		}
		items, _ := r.store.Items()
		if !slices.Equal(items[0].Sessions, []hostproto.Session{hostproto.SessionProve}) {
			t.Fatalf("the scan did not withdraw: %v", items[0].Sessions)
		}

		r.supersedeUnwanted(r.on(hostproto.SessionProve))
		if got := r.stateOf(fromProve); got != hostproto.StateSuperseded {
			t.Fatalf("the ticket no session wants is %s", got)
		}
	})

	t.Run("a session that withdrew wants the request again and it comes back for both"+verifiesSessionSupersession+verifiesReemission, func(t *testing.T) {
		r := newRig(t)
		shared := variant("shared again")
		r.wantOn(r.on(hostproto.SessionProve), oneVote, shared)
		r.supersedeUnwanted(r.on(hostproto.SessionProve))
		answer := r.wantOn(r.on(scanSession), oneVote, shared)
		if answer.Standing != Pending || answer.Votes[0].State != hostproto.StateEmitted {
			t.Fatalf("the scan's ask %+v", answer)
		}
		if pending := r.pendingOf(r.on(hostproto.SessionProve)); len(pending) != 0 {
			t.Fatalf("prove sees the scan's ticket %v", pending)
		}
	})
}

func TestEndingSessions(t *testing.T) {
	t.Run("ending a session supersedes only what no live session holds"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		shared, scanOnly, proveOnly := variant("shared"), variant("scan only"), variant("prove only")
		sharedAnswer := r.wantOn(r.on(hostproto.SessionProve), oneVote, shared)
		proveAnswer := r.wantOn(r.on(hostproto.SessionProve), oneVote, proveOnly)
		scan := r.on(scanSession)
		r.wantOn(scan, oneVote, shared)
		scanAnswer := r.wantOn(scan, oneVote, scanOnly)

		if err := scan.End(); err != nil {
			t.Fatal(err)
		}
		got := map[string]hostproto.State{"shared": r.stateOf(sharedAnswer), "scan only": r.stateOf(scanAnswer), "prove only": r.stateOf(proveAnswer)}
		want := map[string]hostproto.State{"shared": hostproto.StateEmitted, "scan only": hostproto.StateSuperseded, "prove only": hostproto.StateEmitted}
		for name, state := range want {
			if got[name] != state {
				t.Errorf("%s is %s, want %s", name, got[name], state)
			}
		}
		if kinds := r.journalKinds(scanAnswer.Votes[0].Ticket); kinds[len(kinds)-1] != hostproto.EventSuperseded {
			t.Fatalf("journal %v", kinds)
		}
	})

	t.Run("a new scan ends the older scans and never the prove session"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		proveAnswer := r.wantOn(r.on(hostproto.SessionProve), oneVote, variant("prove only"))
		older := r.wantOn(r.on(olderScan), oneVote, variant("older scan only"))
		newer := r.on(scanSession)
		if err := newer.EndOlderScans(); err != nil {
			t.Fatal(err)
		}
		states, _ := r.store.Sessions()
		liveness := hostproto.LivenessOf(states)
		if liveness.Live(olderScan) || !liveness.Live(scanSession) || !liveness.Live(hostproto.SessionProve) {
			t.Fatalf("sessions %+v", states)
		}
		if r.stateOf(older) != hostproto.StateSuperseded || r.stateOf(proveAnswer) != hostproto.StateEmitted {
			t.Fatalf("older scan %s, prove %s", r.stateOf(older), r.stateOf(proveAnswer))
		}
	})

	t.Run("only the bridge of a scan ends the older scans"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		older := r.wantOn(r.on(olderScan), oneVote, variant("older scan only"))
		if err := r.on(hostproto.SessionProve).EndOlderScans(); !errors.Is(err, ErrNotScan) {
			t.Fatalf("error %v", err)
		}
		if r.stateOf(older) != hostproto.StateEmitted {
			t.Fatal("prove ended a scan")
		}
	})

	t.Run("finishing a wave returns the session's waiting tickets, and ends the session once none waits"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		request := variant("finished")
		first := r.on(hostproto.SessionProve)
		answer := r.wantOn(first, oneVote, request)
		pending, err := first.FinishWave()
		states, _ := r.store.Sessions()
		if err != nil || len(pending) != 1 || !hostproto.LivenessOf(states).Live(hostproto.SessionProve) {
			t.Fatalf("pending %v, %v, sessions %+v", pending, err, states)
		}
		r.answer(answer.Votes[0].Ticket, `{"validity":"VALID"}`)
		last := r.on(hostproto.SessionProve)
		r.wantOn(last, oneVote, request)
		pending, err = last.FinishWave()
		states, _ = r.store.Sessions()
		if err != nil || len(pending) != 0 || hostproto.LivenessOf(states).Live(hostproto.SessionProve) {
			t.Fatalf("pending %v, %v, sessions %+v", pending, err, states)
		}
	})

	t.Run("an ended session comes back live with its next wave"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		if err := r.on(hostproto.SessionProve).End(); err != nil {
			t.Fatal(err)
		}
		r.on(hostproto.SessionProve)
		states, _ := r.store.Sessions()
		if !hostproto.LivenessOf(states).Live(hostproto.SessionProve) {
			t.Fatalf("sessions %+v", states)
		}
	})
}

// lateReader is a host store that misses the entries another run emits until
// they are read a second time, as a run racing another one over the store does.
type lateReader struct {
	*hostprototest.MemoryHostStore
	missed map[hostproto.Key]bool
}

func (s *lateReader) GetEntry(key hostproto.Key) (hostproto.Entry, error) {
	if !s.missed[key] {
		s.missed[key] = true
		return hostproto.Entry{}, hostproto.ErrNotFound
	}
	return s.MemoryHostStore.GetEntry(key)
}

func TestRacingSessions(t *testing.T) {
	t.Run("a ticket another session's run emitted first is read, not emitted again"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		request := variant("raced")
		first := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		late := &lateReader{MemoryHostStore: r.store, missed: map[hostproto.Key]bool{}}
		wave, _ := r.store.AdvanceWave(scanSession)
		raced, err := New(late, r.clock.Now, &sequenceReader{next: scanSeedOffset}, scanSession, wave).Want(Ask{Request: request, Votes: oneVote})
		if err != nil {
			t.Fatal(err)
		}
		if raced.Votes[0].Ticket != first.Votes[0].Ticket || raced.Standing != Pending {
			t.Fatalf("the late run got %+v, want the ticket %s", raced, first.Votes[0].Ticket)
		}
	})

	t.Run("a ticket issued again since the view was read is not superseded"+verifiesSessionSupersession, func(t *testing.T) {
		r := newRig(t)
		request := variant("issued again")
		first := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		view, err := r.on(hostproto.SessionProve).view()
		if err != nil {
			t.Fatal(err)
		}
		r.supersedeUnwanted(r.on(hostproto.SessionProve))
		again := r.wantOn(r.on(hostproto.SessionProve), oneVote, request)
		if err := r.on(hostproto.SessionProve).supersede(view, first.ID); err != nil {
			t.Fatal(err)
		}
		if got := r.stateOf(again); got != hostproto.StateEmitted {
			t.Fatalf("the ticket issued again is %s", got)
		}
	})
}

func TestSessionViews(t *testing.T) {
	t.Run("each session's pending tickets are its own, a shared one in both"+verifiesSessionViews, func(t *testing.T) {
		r := newRig(t)
		prove, scan := r.on(hostproto.SessionProve), r.on(scanSession)
		proveOnly := r.wantOn(prove, oneVote, variant("prove only"))
		scanOnly := r.wantOn(scan, oneVote, variant("scan only"))
		shared := r.wantOn(prove, oneVote, variant("shared"))
		r.wantOn(scan, oneVote, variant("shared"))

		wantProve := []hostproto.TicketID{proveOnly.Votes[0].Ticket, shared.Votes[0].Ticket}
		wantScan := []hostproto.TicketID{scanOnly.Votes[0].Ticket, shared.Votes[0].Ticket}
		for _, want := range [][]hostproto.TicketID{wantProve, wantScan} {
			slices.SortFunc(want, func(a, b hostproto.TicketID) int { return cmp.Compare(a.RequestID, b.RequestID) })
		}
		if got := r.pendingOf(prove); !slices.Equal(got, wantProve) {
			t.Errorf("prove pending %v, want %v", got, wantProve)
		}
		if got := r.pendingOf(scan); !slices.Equal(got, wantScan) {
			t.Errorf("scan pending %v, want %v", got, wantScan)
		}
	})

	t.Run("a ticket with no queue item is pending for prove only"+verifiesSessionViews, func(t *testing.T) {
		r := newRig(t)
		request := variant("no item")
		if _, err := r.store.PutRequest(request); err != nil {
			t.Fatal(err)
		}
		entry := hostproto.NewEntry(hostprototest.Ticket(request, 0, hostproto.FirstEmission), request.Role, hostprototest.Digest("schema"), hostproto.DefaultMaxEmissions)
		if err := r.store.PutEntry(entry); err != nil {
			t.Fatal(err)
		}
		if len(r.pendingOf(r.on(hostproto.SessionProve))) != 1 || len(r.pendingOf(r.on(scanSession))) != 0 {
			t.Fatal("a ticket with no item is not prove's alone")
		}
	})

	t.Run("each session's waves are its own"+verifiesSessionViews, func(t *testing.T) {
		r := newRig(t)
		r.on(hostproto.SessionProve)
		r.on(hostproto.SessionProve)
		if scan := r.on(scanSession); scan.Wave != 1 {
			t.Fatalf("the first scan wave is %d", scan.Wave)
		}
		if prove := r.on(hostproto.SessionProve); prove.Wave != 3 {
			t.Fatalf("the third prove wave is %d", prove.Wave)
		}
	})
}

func TestEmissionCap(t *testing.T) {
	t.Run("a session with a cap emits up to it, leaves the rest unasked and names it"+verifiesEmissionCap, func(t *testing.T) {
		r := newRig(t)
		scan := r.on(scanSession)
		scan.EmissionCap = testCap
		within := r.wantOn(scan, testCap-1, variant("within"))
		straddling := r.wantOn(scan, twoVotes, variant("straddling"))
		beyond := r.wantOn(scan, oneVote, variant("beyond"))

		if within.Standing != Pending || straddling.Standing != Unasked || beyond.Standing != Unasked {
			t.Fatalf("standings %s %s %s", within.Standing, straddling.Standing, beyond.Standing)
		}
		if straddling.Votes[0].Unasked || !straddling.Votes[1].Unasked || straddling.Votes[1].Ticket.Emission != 0 {
			t.Fatalf("straddling votes %+v", straddling.Votes)
		}
		if got := len(r.pendingOf(scan)); got != testCap {
			t.Fatalf("%d tickets emitted, want %d", got, testCap)
		}
		want := []hostproto.RequestID{straddling.ID, beyond.ID}
		slices.Sort(want)
		if got := scan.Unasked(); !slices.Equal(got, want) {
			t.Fatalf("unasked %v, want %v", got, want)
		}
		if scan.Waiting() != true {
			t.Fatal("the emitted tickets are waited for")
		}
	})

	t.Run("the cap counts the session's reissues and re-emissions across invocations"+verifiesEmissionCap, func(t *testing.T) {
		r := newRig(t)
		request := variant("reissued")
		first := r.on(scanSession)
		first.EmissionCap = testCap
		answer := r.wantOn(first, oneVote, request)
		r.failSchemaTwice(answer.Votes[0].Ticket)

		second := r.on(scanSession)
		second.EmissionCap = testCap
		if reissued := r.wantOn(second, oneVote, request); reissued.Votes[0].Ticket.Emission != hostproto.FirstEmission+1 {
			t.Fatalf("reissue %+v", reissued.Votes[0])
		}
		r.supersedeUnwanted(r.on(scanSession))

		third := r.on(scanSession)
		third.EmissionCap = testCap
		if reemitted := r.wantOn(third, oneVote, request); reemitted.Standing != Pending {
			t.Fatalf("the third emission is refused: %+v", reemitted)
		}
		r.supersedeUnwanted(r.on(scanSession))

		fourth := r.on(scanSession)
		fourth.EmissionCap = testCap
		refused := r.wantOn(fourth, oneVote, request)
		if refused.Standing != Unasked || refused.Votes[0].State != hostproto.StateSuperseded || !refused.Votes[0].Unasked {
			t.Fatalf("past the cap %+v", refused)
		}
		records, _ := r.store.Records()
		if got := hostproto.Emissions(records, scanSession); got != testCap {
			t.Fatalf("%d emissions journaled for the scan, want %d", got, testCap)
		}
	})

	t.Run("the cap of one session ignores the emissions of another, and zero means no cap"+verifiesEmissionCap, func(t *testing.T) {
		r := newRig(t)
		prove := r.on(hostproto.SessionProve)
		r.wantOn(prove, threeVotes, variant("prove's"))
		scan := r.on(scanSession)
		scan.EmissionCap = testCap
		if answer := r.wantOn(scan, threeVotes, variant("scan's")); answer.Standing != Pending {
			t.Fatalf("the scan was charged prove's emissions: %+v", answer)
		}
		if answer := r.wantOn(prove, threeVotes, variant("more of prove's")); answer.Standing != Pending {
			t.Fatalf("prove has no cap: %+v", answer)
		}
	})

	t.Run("an exhausted ticket is abandoned even when the cap is reached"+verifiesEmissionCap, func(t *testing.T) {
		r := newRig(t)
		request := variant("exhausted")
		answer := r.wantOn(r.on(scanSession), oneVote, request)
		for range hostproto.DefaultMaxEmissions - 1 {
			r.failSchemaTwice(answer.Votes[0].Ticket)
			answer = r.wantOn(r.on(scanSession), oneVote, request)
		}
		r.failSchemaTwice(answer.Votes[0].Ticket)
		capped := r.on(scanSession)
		capped.EmissionCap = hostproto.DefaultMaxEmissions
		if final := r.wantOn(capped, oneVote, request); final.Standing != Unresponsive {
			t.Fatalf("final %+v", final)
		}
	})
}
