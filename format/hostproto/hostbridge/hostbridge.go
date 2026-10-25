// Package hostbridge is a module's side of the host protocol: it turns what a
// proof wants from a model into tickets, reads the answers the lifecycle lets it
// consume, and moves tickets through validation, rejection, reissue and
// consumption. It never talks to a model; a host answers through the store.
//
// A bridge works for one session of the store. It tags every request it wants
// with its session, retires only what no live session wants any more, and sees
// only the tickets of its session.
package hostbridge

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

// Standing is where a request stands as a whole.
type Standing string

// Standings of a request, from the strongest: a request takes the strongest
// standing one of its votes calls for.
const (
	// Unresponsive: a ticket was abandoned after its emissions ran out.
	Unresponsive Standing = "unresponsive"
	// Unasked: the emission cap of the session kept a ticket from being issued,
	// so the request stays unanswered in this session.
	Unasked Standing = "unasked"
	// Pending: at least one ticket still waits for its answer.
	Pending Standing = "pending"
	// Answered: every ticket holds an answer a proof may look at.
	Answered Standing = "answered"
)

// standingOrder ranks the standings from the strongest.
var standingOrder = []Standing{Unresponsive, Unasked, Pending, Answered}

// stronger returns the stronger of two standings.
func stronger(a, b Standing) Standing {
	if slices.Index(standingOrder, b) < slices.Index(standingOrder, a) {
		return b
	}
	return a
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
	// Ticket is the ticket of the vote; a vote the cap kept from its first issue
	// has emission zero.
	Ticket   hostproto.TicketID
	State    hostproto.State
	Output   json.RawMessage
	Executor format.ExecutorProfile
	// Marked is set when the journal records the response as leaked, exposed or
	// conflicting.
	Marked bool
	// Classify is the classification the vote was read from, set exactly for a
	// vote of the channel host-classify.
	Classify *format.ClassifyRecord
	// Unasked is set when the emission cap of the session kept the vote's ticket
	// from being issued or issued again.
	Unasked bool
}

// Holds reports whether the vote holds an answer a proof may look at.
func (v VoteState) Holds() bool {
	return v.State == hostproto.StateAccepted || v.State == hostproto.StateValidated || v.State == hostproto.StateConsumed
}

// standing is the standing the vote calls for.
func (v VoteState) standing() Standing {
	switch {
	case v.State == hostproto.StateAbandoned:
		return Unresponsive
	case v.Unasked:
		return Unasked
	case !v.Holds():
		return Pending
	}
	return Answered
}

// Answer is the state of a request.
type Answer struct {
	ID       hostproto.RequestID
	Standing Standing
	Votes    []VoteState
}

// Bridge moves the tickets of one session through the store.
type Bridge struct {
	Store        hostproto.HostStore
	Clock        func() time.Time
	Entropy      io.Reader
	MaxEmissions int
	// Session is the session the bridge works for.
	Session hostproto.Session
	Wave    int
	// EmissionCap is the most emissions the session makes, counted over the
	// journal across invocations: first issues, reissues and re-emissions alike.
	// Zero means no cap.
	EmissionCap int

	wanted  map[hostproto.RequestID]bool
	unasked map[hostproto.RequestID]bool
	waiting bool
}

// New builds the bridge of a session for one of its waves.
func New(store hostproto.HostStore, clock func() time.Time, entropy io.Reader, session hostproto.Session, wave int) *Bridge {
	return &Bridge{
		Store: store, Clock: clock, Entropy: entropy, MaxEmissions: hostproto.DefaultMaxEmissions, Session: session, Wave: wave,
		wanted: map[hostproto.RequestID]bool{}, unasked: map[hostproto.RequestID]bool{},
	}
}

// Want makes sure the request has its tickets and reports how they stand: it
// tags the request with the bridge's session, emits the missing tickets, retires
// fetched tickets that waited past their time to live, reissues rejected and
// expired ones, re-emits superseded ones, and reads the answers. An emission the
// cap refuses leaves the vote unasked.
func (b *Bridge) Want(ask Ask) (Answer, error) {
	id, err := b.Store.PutRequest(ask.Request)
	if err != nil {
		return Answer{}, fmt.Errorf("store the request: %w", err)
	}
	b.wanted[id] = true
	item := hostproto.QueueItem{RequestID: id, Criteria: ask.Criteria, Sessions: []hostproto.Session{b.Session}, Wave: b.Wave, Purpose: ask.Purpose}
	if err := b.Store.Record(item); err != nil {
		return Answer{}, fmt.Errorf("record the request in the queue: %w", err)
	}
	records, err := b.Store.Records()
	if err != nil {
		return Answer{}, fmt.Errorf("read the journal: %w", err)
	}
	journaled := executorrun.IndexJournal(records)
	budget := newBudget(b.EmissionCap, hostproto.Emissions(records, b.Session))
	answer := Answer{ID: id, Standing: Answered}
	for vote := range ask.Votes {
		state, err := b.settle(ask.Request, id, vote, journaled, budget)
		if err != nil {
			return Answer{}, err
		}
		answer.Votes = append(answer.Votes, state)
		answer.Standing = stronger(answer.Standing, state.standing())
	}
	b.waiting = b.waiting || answer.Standing == Pending
	if answer.Standing == Unasked {
		b.unasked[id] = true
	}
	return answer, nil
}

// Waiting reports whether any request asked for in this wave still waits for an
// answer.
func (b *Bridge) Waiting() bool { return b.waiting }

// Unasked lists, ordered, the requests of this wave the emission cap left
// unasked, so that the caller can name what went unchecked.
func (b *Bridge) Unasked() []hostproto.RequestID {
	ids := make([]hostproto.RequestID, 0, len(b.unasked))
	for id := range b.unasked {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

func (b *Bridge) settle(request hostproto.Request, id hostproto.RequestID, vote int, journaled executorrun.JournalIndex, budget *budget) (VoteState, error) {
	key := hostproto.Key{RequestID: id, Vote: vote}
	entry, err := b.Store.GetEntry(key)
	if errors.Is(err, hostproto.ErrNotFound) {
		if !budget.allows() {
			return VoteState{Ticket: hostproto.TicketID{RequestID: id, Vote: vote}, Unasked: true}, nil
		}
		entry, err = b.emit(request, id, vote, budget)
		if errors.Is(err, hostproto.ErrExists) {
			entry, err = b.Store.GetEntry(key)
		}
	}
	if err != nil {
		return VoteState{}, err
	}
	entry, err = b.revive(key, entry, budget)
	if err != nil {
		return VoteState{}, err
	}
	ticket := entry.Ticket.TicketID.String()
	state := VoteState{Ticket: entry.Ticket.TicketID, State: entry.State, Marked: executorrun.Marked(journaled.Ticket(ticket)), Unasked: needsEmission(entry.State)}
	if !state.Holds() {
		return state, nil
	}
	response, err := b.Store.GetResponse(key)
	if err != nil {
		return VoteState{}, fmt.Errorf("read the response of %s: %w", entry.Ticket.TicketID, err)
	}
	events := journaled.Response(ticket, response.Executor)
	state.Marked = executorrun.Marked(events)
	state.Output, state.Executor = response.Output, executorrun.Effective(response.Executor, events)
	state.Classify = response.Classify
	return state, nil
}

// needsEmission reports whether a ticket left in the state after revive waits
// for an emission it was refused.
func needsEmission(state hostproto.State) bool {
	return state == hostproto.StateRejected || state == hostproto.StateExpired || state == hostproto.StateSuperseded
}

func (b *Bridge) emit(request hostproto.Request, id hostproto.RequestID, vote int, budget *budget) (hostproto.Entry, error) {
	ticket, err := hostproto.NewTicket(hostproto.TicketID{RequestID: id, Vote: vote, Emission: hostproto.FirstEmission}, b.Entropy)
	if err != nil {
		return hostproto.Entry{}, err
	}
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		return hostproto.Entry{}, err
	}
	entry := hostproto.NewEntry(ticket, request.Role, schemaDigest, b.MaxEmissions)
	entry.EmittedAt = b.Clock()
	if err := b.Store.PutEntry(entry); err != nil {
		return hostproto.Entry{}, fmt.Errorf("store the ticket: %w", err)
	}
	budget.spend()
	return entry, b.journal(hostproto.EventEmitted, ticket.TicketID)
}

// revive retires a fetched ticket that waited past its time to live, reissues a
// rejected or expired one with a new nonce, abandoning it when its emissions ran
// out, and re-emits a superseded one, whose request is wanted again. A new
// emission the cap refuses leaves the ticket as it is.
func (b *Bridge) revive(key hostproto.Key, entry hostproto.Entry, budget *budget) (hostproto.Entry, error) {
	if entry.State == hostproto.StateFetched {
		expired, err := b.update(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Expire(b.Clock()) }, hostproto.EventExpired)
		switch {
		case errors.Is(err, hostproto.ErrNotExpired):
			return entry, nil
		case err != nil:
			return b.raced(key, err)
		}
		entry = expired
	}
	switch entry.State {
	case hostproto.StateSuperseded:
		return b.reemit(key, entry, budget)
	case hostproto.StateRejected, hostproto.StateExpired:
		return b.reissue(key, entry, budget)
	}
	return entry, nil
}

// reissue issues a rejected or expired ticket again with a new nonce, once the
// answer it held is discarded, or abandons it when its emissions ran out. An
// abandonment is no emission, so the cap never holds it back.
func (b *Bridge) reissue(key hostproto.Key, entry hostproto.Entry, budget *budget) (hostproto.Entry, error) {
	if !entry.Exhausted() && !budget.allows() {
		return entry, nil
	}
	nonce, err := hostproto.NewNonce(b.Entropy)
	if err != nil {
		return hostproto.Entry{}, err
	}
	reissued, err := b.Store.UpdateEntry(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reissue(nonce) })
	if err != nil {
		return b.raced(key, err)
	}
	if reissued.State == hostproto.StateAbandoned {
		return reissued, b.journal(hostproto.EventAbandoned, reissued.Ticket.TicketID)
	}
	if err := b.Store.DiscardResponse(key); err != nil {
		return hostproto.Entry{}, fmt.Errorf("discard the answer the reissued ticket replaces: %w", err)
	}
	budget.spend()
	return reissued, b.journal(hostproto.EventEmitted, reissued.Ticket.TicketID)
}

// reemit issues a superseded ticket again with a new nonce.
func (b *Bridge) reemit(key hostproto.Key, entry hostproto.Entry, budget *budget) (hostproto.Entry, error) {
	if !budget.allows() {
		return entry, nil
	}
	nonce, err := hostproto.NewNonce(b.Entropy)
	if err != nil {
		return hostproto.Entry{}, err
	}
	reemitted, err := b.update(key, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reemit(nonce) }, hostproto.EventEmitted)
	if err != nil {
		return b.raced(key, err)
	}
	budget.spend()
	return reemitted, nil
}

// raced reads the entry again when the transition this run attempted turned
// illegal because another run sharing the store moved the ticket first.
func (b *Bridge) raced(key hostproto.Key, err error) (hostproto.Entry, error) {
	if !errors.Is(err, hostproto.ErrIllegalTransition) {
		return hostproto.Entry{}, err
	}
	return b.Store.GetEntry(key)
}

func (b *Bridge) update(key hostproto.Key, change func(hostproto.Entry) (hostproto.Entry, error), kind hostproto.EventKind) (hostproto.Entry, error) {
	entry, err := b.Store.UpdateEntry(key, change)
	if err != nil {
		return hostproto.Entry{}, err
	}
	return entry, b.journal(kind, entry.Ticket.TicketID)
}

func (b *Bridge) journal(kind hostproto.EventKind, ticket hostproto.TicketID) error {
	if _, err := b.Store.Append(hostproto.Event{Kind: kind, Ticket: ticket.String(), Session: b.Session}); err != nil {
		return fmt.Errorf("journal %s: %w", kind, err)
	}
	return nil
}

// Commit journals an event about no ticket once: an event of the same kind and
// detail already in the journal is not appended again. It reports whether the
// event was journaled before this call.
func (b *Bridge) Commit(kind hostproto.EventKind, detail string) (alreadyJournaled bool, err error) {
	records, err := b.Store.Records()
	if err != nil {
		return false, fmt.Errorf("read the journal: %w", err)
	}
	for _, record := range records {
		if record.Event.Kind == kind && record.Event.Detail == detail {
			return true, nil
		}
	}
	if _, err := b.Store.Append(hostproto.Event{Kind: kind, Detail: detail}); err != nil {
		return false, fmt.Errorf("journal %s: %w", kind, err)
	}
	return false, nil
}

// Validate marks the accepted votes of an answer validated: execution found
// nothing against them. A vote in another state is left alone.
func (b *Bridge) Validate(answer Answer) error {
	return b.moveVotes(answer, hostproto.StateAccepted, hostproto.Entry.MarkValidated, hostproto.EventValidated)
}

// moveVotes applies a transition to the votes of an answer whose ticket is in
// the given state now, whatever state the answer was read in. A vote the cap left
// without a ticket is skipped.
func (b *Bridge) moveVotes(answer Answer, from hostproto.State, change func(hostproto.Entry) (hostproto.Entry, error), kind hostproto.EventKind) error {
	for _, vote := range answer.Votes {
		entry, err := b.Store.GetEntry(vote.Ticket.Key())
		if errors.Is(err, hostproto.ErrNotFound) && vote.Unasked {
			continue
		}
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
