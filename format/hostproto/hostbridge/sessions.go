package hostbridge

import (
	"errors"
	"fmt"
	"slices"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// ErrNotScan is returned when a bridge that does not work for a scan tries to end
// the older scans.
var ErrNotScan = errors.New("only the bridge of a scan ends the older scans")

// storeView is what the bridge reads of the store to decide what a session sees
// and what it may retire: the tickets, the sessions that hold each request and
// which sessions are live.
type storeView struct {
	entries  []hostproto.Entry
	sessions hostproto.SessionIndex
	liveness hostproto.Liveness
}

func (b *Bridge) view() (storeView, error) {
	entries, err := b.Store.ListEntries()
	if err != nil {
		return storeView{}, fmt.Errorf("list the tickets: %w", err)
	}
	items, err := b.Store.Items()
	if err != nil {
		return storeView{}, fmt.Errorf("read the queue: %w", err)
	}
	states, err := b.Store.Sessions()
	if err != nil {
		return storeView{}, fmt.Errorf("read the sessions: %w", err)
	}
	return storeView{entries: entries, sessions: hostproto.IndexSessions(items), liveness: hostproto.LivenessOf(states)}, nil
}

// emittedRequests lists, in store order and once each, the requests that hold an
// emitted ticket.
func (v storeView) emittedRequests() []hostproto.RequestID {
	var ids []hostproto.RequestID
	seen := map[hostproto.RequestID]bool{}
	for _, entry := range v.entries {
		id := entry.Ticket.RequestID
		if entry.State == hostproto.StateEmitted && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}

// SupersedeUnwanted withdraws the bridge's session from the requests with an
// emitted ticket that it did not ask for in this wave: an input changed and gave
// them a new id. A request no live session holds any more has its emitted
// tickets superseded; one another live session still holds keeps them.
func (b *Bridge) SupersedeUnwanted() error {
	view, err := b.view()
	if err != nil {
		return err
	}
	for _, id := range view.emittedRequests() {
		if b.wanted[id] || !hostproto.OnlySession(b.Session).Selects(view.sessions.Of(id)) {
			continue
		}
		remaining, err := b.withdraw(id)
		if err != nil {
			return err
		}
		if !view.liveness.AnyLive(remaining) {
			if err := b.supersede(view, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// withdraw removes the bridge's session from the request and returns the
// sessions that still hold it. A request the queue has no item for belonged to
// the prove session alone.
func (b *Bridge) withdraw(id hostproto.RequestID) ([]hostproto.Session, error) {
	item, err := b.Store.Withdraw(id, b.Session)
	switch {
	case errors.Is(err, hostproto.ErrNotFound):
		return nil, nil
	case err != nil:
		return nil, fmt.Errorf("withdraw %s from %s: %w", b.Session, id, err)
	}
	return item.Sessions, nil
}

// supersede retires the emitted tickets of the request. A ticket another run
// moved since the view was read is left as that run left it.
func (b *Bridge) supersede(view storeView, id hostproto.RequestID) error {
	for _, entry := range view.entries {
		if entry.Ticket.RequestID != id || entry.State != hostproto.StateEmitted {
			continue
		}
		viewed := entry.Ticket.TicketID
		_, err := b.update(viewed.Key(), func(stored hostproto.Entry) (hostproto.Entry, error) {
			if stored.Ticket.TicketID != viewed {
				return stored, fmt.Errorf("%w: %s was issued again", hostproto.ErrIllegalTransition, viewed)
			}
			return stored.Supersede()
		}, hostproto.EventSuperseded)
		if err != nil && !errors.Is(err, hostproto.ErrIllegalTransition) {
			return fmt.Errorf("supersede %s: %w", entry.Ticket.TicketID, err)
		}
	}
	return nil
}

// End ends the bridge's session, as it does once it wrote its statement, and
// supersedes the emitted tickets of the requests it held that no live session
// holds any more.
func (b *Bridge) End() error {
	return b.end([]hostproto.Session{b.Session})
}

// EndOlderScans ends every other scan session of the store, as a new scan does
// when it starts, and supersedes the emitted tickets of the requests they held
// that no live session holds any more. The prove session never ends by it.
func (b *Bridge) EndOlderScans() error {
	if !b.Session.IsScan() {
		return fmt.Errorf("%w: %s", ErrNotScan, b.Session)
	}
	states, err := b.Store.Sessions()
	if err != nil {
		return fmt.Errorf("read the sessions: %w", err)
	}
	var older []hostproto.Session
	for _, state := range states {
		if state.Session.IsScan() && state.Session != b.Session && !state.Ended {
			older = append(older, state.Session)
		}
	}
	return b.end(older)
}

// end marks the sessions ended, then supersedes the emitted tickets of the
// requests one of them held and no live session holds. A request none of them
// held is left alone.
func (b *Bridge) end(sessions []hostproto.Session) error {
	for _, session := range sessions {
		if err := b.Store.EndSession(session); err != nil {
			return fmt.Errorf("end the session %s: %w", session, err)
		}
	}
	view, err := b.view()
	if err != nil {
		return err
	}
	for _, id := range view.emittedRequests() {
		holders := view.sessions.Of(id)
		heldByEnded := slices.ContainsFunc(sessions, func(session hostproto.Session) bool { return slices.Contains(holders, session) })
		if !heldByEnded || view.liveness.AnyLive(holders) {
			continue
		}
		if err := b.supersede(view, id); err != nil {
			return err
		}
	}
	return nil
}

// FinishWave closes the wave of the bridge's session: it supersedes what the
// session no longer wants and returns the session's tickets that still wait for
// an answer. A wave that leaves none waiting is the session's last, in which the
// run writes its statement, so the session ends.
func (b *Bridge) FinishWave() ([]hostproto.Ticket, error) {
	if err := b.SupersedeUnwanted(); err != nil {
		return nil, err
	}
	pending, err := b.PendingTickets()
	if err != nil || len(pending) > 0 {
		return pending, err
	}
	return nil, b.End()
}

// PendingTickets lists the tickets of the bridge's session still waiting for an
// answer, in store order.
func (b *Bridge) PendingTickets() ([]hostproto.Ticket, error) {
	view, err := b.view()
	if err != nil {
		return nil, err
	}
	filter := hostproto.OnlySession(b.Session)
	var pending []hostproto.Ticket
	for _, entry := range view.entries {
		if hostproto.Waits(entry.State) && view.sessions.Selects(filter, entry.Ticket.RequestID) {
			pending = append(pending, entry.Ticket)
		}
	}
	return pending, nil
}
