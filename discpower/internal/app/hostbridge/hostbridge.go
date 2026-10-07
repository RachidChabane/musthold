// Package hostbridge is the module's side of the host protocol: it turns what a
// proof wants from a model into tickets, reads the answers the lifecycle lets it
// consume, and moves tickets through validation, rejection, reissue and
// consumption. It never talks to a model; a host answers through the store.
package hostbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Standing is where a request stands as a whole.
type Standing string

// Standings of a request.
const (
	// Pending: at least one ticket still waits for its answer.
	Pending Standing = "pending"
	// Answered: every ticket holds an answer a proof may look at.
	Answered Standing = "answered"
	// Unresponsive: a ticket was abandoned after its emissions ran out.
	Unresponsive Standing = "unresponsive"
)

// markEvents are the journal events that cost a response its judged pass credit.
var markEvents = map[hostproto.EventKind]bool{
	hostproto.EventLeaked:               true,
	hostproto.EventExposed:              true,
	hostproto.EventResubmissionConflict: true,
}

// Ask is a request a proof needs answered, with the number of votes it needs and
// the queue metadata that goes with it.
type Ask struct {
	Request  hostproto.Request
	Votes    int
	Criteria []string
	Purpose  string
}

// VoteState is one vote of an answer.
type VoteState struct {
	Ticket   hostproto.TicketID
	State    hostproto.State
	Output   json.RawMessage
	Executor format.ExecutorProfile
	// Marked is set when the journal records the response as leaked, exposed or
	// conflicting.
	Marked bool
}

// Holds reports whether the vote holds an answer a proof may look at.
func (v VoteState) Holds() bool {
	return v.State == hostproto.StateAccepted || v.State == hostproto.StateValidated || v.State == hostproto.StateConsumed
}

// Answer is the state of a request.
type Answer struct {
	ID       hostproto.RequestID
	Standing Standing
	Votes    []VoteState
}

// Bridge moves tickets through the store.
type Bridge struct {
	Store        ports.HostStore
	Clock        func() time.Time
	Entropy      io.Reader
	MaxEmissions int
	Wave         int

	wanted  map[hostproto.RequestID]bool
	waiting bool
}

// New builds a bridge for a wave.
func New(store ports.HostStore, clock func() time.Time, entropy io.Reader, wave int) *Bridge {
	return &Bridge{Store: store, Clock: clock, Entropy: entropy, MaxEmissions: hostproto.DefaultMaxEmissions, Wave: wave, wanted: map[hostproto.RequestID]bool{}}
}

// Want makes sure the request has its tickets and reports how they stand: it
// emits the missing ones, retires fetched tickets that waited past their time to
// live, reissues rejected and expired ones, and reads the answers.
func (b *Bridge) Want(ask Ask) (Answer, error) {
	id, err := b.Store.PutRequest(ask.Request)
	if err != nil {
		return Answer{}, fmt.Errorf("store the request: %w", err)
	}
	b.wanted[id] = true
	if err := b.Store.Record(ports.QueueItem{RequestID: id, Criteria: ask.Criteria, Wave: b.Wave, Purpose: ask.Purpose}); err != nil {
		return Answer{}, fmt.Errorf("record the request in the queue: %w", err)
	}
	marks, err := b.marks()
	if err != nil {
		return Answer{}, err
	}
	answer := Answer{ID: id, Standing: Answered}
	for vote := range ask.Votes {
		state, err := b.settle(ask.Request, id, vote, marks)
		if err != nil {
			return Answer{}, err
		}
		answer.Votes = append(answer.Votes, state)
		switch {
		case state.State == hostproto.StateAbandoned:
			answer.Standing = Unresponsive
		case !state.Holds() && answer.Standing != Unresponsive:
			answer.Standing = Pending
		}
	}
	b.waiting = b.waiting || answer.Standing == Pending
	return answer, nil
}

// Waiting reports whether any request asked for in this wave still waits for an
// answer.
func (b *Bridge) Waiting() bool { return b.waiting }

func (b *Bridge) marks() (map[string]bool, error) {
	records, err := b.Store.Records()
	if err != nil {
		return nil, fmt.Errorf("read the journal: %w", err)
	}
	marks := map[string]bool{}
	for _, record := range records {
		if markEvents[record.Event.Kind] {
			marks[record.Event.Ticket] = true
		}
	}
	return marks, nil
}

func (b *Bridge) settle(request hostproto.Request, id hostproto.RequestID, vote int, marks map[string]bool) (VoteState, error) {
	key := hostproto.Key{RequestID: id, Vote: vote}
	entry, err := b.Store.GetEntry(key)
	if errors.Is(err, hostproto.ErrNotFound) {
		entry, err = b.emit(request, id, vote)
	}
	if err != nil {
		return VoteState{}, err
	}
	entry, err = b.revive(key, entry)
	if err != nil {
		return VoteState{}, err
	}
	state := VoteState{Ticket: entry.Ticket.TicketID, State: entry.State, Marked: marks[entry.Ticket.TicketID.String()]}
	if !state.Holds() {
		return state, nil
	}
	response, err := b.Store.GetResponse(key)
	if err != nil {
		return VoteState{}, fmt.Errorf("read the response of %s: %w", entry.Ticket.TicketID, err)
	}
	state.Output, state.Executor = response.Output, response.Executor
	return state, nil
}

func (b *Bridge) emit(request hostproto.Request, id hostproto.RequestID, vote int) (hostproto.Entry, error) {
	ticket, err := hostproto.NewTicket(hostproto.TicketID{RequestID: id, Vote: vote, Emission: hostproto.FirstEmission}, b.Entropy)
	if err != nil {
		return hostproto.Entry{}, err
	}
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		return hostproto.Entry{}, err
	}
	entry := hostproto.NewEntry(ticket, request.Role, schemaDigest, b.MaxEmissions)
	if err := b.Store.PutEntry(entry); err != nil {
		return hostproto.Entry{}, fmt.Errorf("store the ticket: %w", err)
	}
	return entry, b.journal(hostproto.EventEmitted, ticket.TicketID)
}

// revive retires a fetched ticket that waited past its time to live and reissues
// a rejected or expired one with a new nonce, abandoning it when its emissions
// ran out.
func (b *Bridge) revive(key hostproto.Key, entry hostproto.Entry) (hostproto.Entry, error) {
	if entry.State == hostproto.StateFetched {
		expired, err := b.update(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Expire(b.Clock()) }, hostproto.EventExpired)
		switch {
		case errors.Is(err, hostproto.ErrNotExpired):
			return entry, nil
		case err != nil:
			return hostproto.Entry{}, err
		}
		entry = expired
	}
	if entry.State != hostproto.StateRejected && entry.State != hostproto.StateExpired {
		return entry, nil
	}
	nonce, err := hostproto.NewNonce(b.Entropy)
	if err != nil {
		return hostproto.Entry{}, err
	}
	reissued, err := b.Store.UpdateEntry(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reissue(nonce) })
	if err != nil {
		return hostproto.Entry{}, err
	}
	kind := hostproto.EventEmitted
	if reissued.State == hostproto.StateAbandoned {
		kind = hostproto.EventAbandoned
	} else if err := b.Store.DiscardResponse(key); err != nil {
		return hostproto.Entry{}, fmt.Errorf("discard the answer the reissued ticket replaces: %w", err)
	}
	return reissued, b.journal(kind, reissued.Ticket.TicketID)
}

func (b *Bridge) update(key hostproto.Key, change func(hostproto.Entry) (hostproto.Entry, error), kind hostproto.EventKind) (hostproto.Entry, error) {
	entry, err := b.Store.UpdateEntry(key, change)
	if err != nil {
		return hostproto.Entry{}, err
	}
	return entry, b.journal(kind, entry.Ticket.TicketID)
}

func (b *Bridge) journal(kind hostproto.EventKind, ticket hostproto.TicketID) error {
	if _, err := b.Store.Append(hostproto.Event{Kind: kind, Ticket: ticket.String()}); err != nil {
		return fmt.Errorf("journal %s: %w", kind, err)
	}
	return nil
}

// Validate marks the accepted votes of an answer validated: execution found
// nothing against them. A vote in another state is left alone.
func (b *Bridge) Validate(answer Answer) error {
	return b.moveVotes(answer, hostproto.StateAccepted, hostproto.Entry.MarkValidated, hostproto.EventValidated)
}

// moveVotes applies a transition to the votes of an answer whose ticket is in
// the given state now, whatever state the answer was read in.
func (b *Bridge) moveVotes(answer Answer, from hostproto.State, change func(hostproto.Entry) (hostproto.Entry, error), kind hostproto.EventKind) error {
	for _, vote := range answer.Votes {
		entry, err := b.Store.GetEntry(vote.Ticket.Key())
		if err != nil {
			return fmt.Errorf("read the ticket %s: %w", vote.Ticket, err)
		}
		if entry.State != from {
			continue
		}
		if _, err := b.update(vote.Ticket.Key(), change, kind); err != nil {
			return fmt.Errorf("%s %s: %w", kind, vote.Ticket, err)
		}
	}
	return nil
}

// Contradict rejects an accepted vote that execution contradicts. The next Want
// reissues the ticket with a new nonce while emissions remain.
func (b *Bridge) Contradict(vote VoteState, reason string) error {
	entry, err := b.Store.UpdateEntry(vote.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		next, err := e.Contradict()
		next.Reason = reason
		return next, err
	})
	if err != nil {
		return fmt.Errorf("reject %s: %w", vote.Ticket, err)
	}
	return b.journal(hostproto.EventRejected, entry.Ticket.TicketID)
}

// Consume records that a proof used the validated votes of an answer.
func (b *Bridge) Consume(answer Answer) error {
	return b.moveVotes(answer, hostproto.StateValidated, hostproto.Entry.Consume, hostproto.EventConsumed)
}

// SupersedeUnwanted retires the emitted tickets of requests no proof of this
// wave asked for: an input changed and gave them a new id, so nobody answers
// them any more.
func (b *Bridge) SupersedeUnwanted() error {
	entries, err := b.Store.ListEntries()
	if err != nil {
		return fmt.Errorf("list the tickets: %w", err)
	}
	for _, entry := range entries {
		if entry.State != hostproto.StateEmitted || b.wanted[entry.Ticket.RequestID] {
			continue
		}
		if _, err := b.update(entry.Ticket.Key(), hostproto.Entry.Supersede, hostproto.EventSuperseded); err != nil {
			return fmt.Errorf("supersede %s: %w", entry.Ticket.TicketID, err)
		}
	}
	return nil
}

// PendingTickets lists the tickets still waiting for an answer, in store order.
func (b *Bridge) PendingTickets() ([]hostproto.Ticket, error) {
	entries, err := b.Store.ListEntries()
	if err != nil {
		return nil, fmt.Errorf("list the tickets: %w", err)
	}
	var pending []hostproto.Ticket
	for _, entry := range entries {
		if entry.State == hostproto.StateEmitted || entry.State == hostproto.StateFetched {
			pending = append(pending, entry.Ticket)
		}
	}
	return pending, nil
}
