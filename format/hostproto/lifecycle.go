package hostproto

import (
	"errors"
	"fmt"
	"slices"
	"time"
)

// Lifecycle limits.
const (
	// FetchTTL is how long a fetched ticket stays answerable.
	FetchTTL = 20 * time.Minute
	// DefaultMaxEmissions is how many times a ticket is issued before it is abandoned.
	DefaultMaxEmissions = 3
	// MaxCorrections is how many schema corrections an executor is allowed.
	MaxCorrections = 1
)

// ReasonHostUnresponsive is the reason of an abandoned ticket: the claim it was
// meant to settle is UNDETERMINED.
const ReasonHostUnresponsive = "host_unresponsive"

// State is the lifecycle state of a ticket.
type State string

// Lifecycle states.
const (
	StateEmitted    State = "emitted"
	StateFetched    State = "fetched"
	StateAccepted   State = "accepted"
	StateRejected   State = "rejected"
	StateExpired    State = "expired"
	StateValidated  State = "validated"
	StateConsumed   State = "consumed"
	StateSuperseded State = "superseded"
	StateAbandoned  State = "abandoned"
)

// states lists the states in lifecycle order.
var states = []State{
	StateEmitted, StateFetched, StateAccepted, StateRejected, StateExpired,
	StateValidated, StateConsumed, StateSuperseded, StateAbandoned,
}

// transitions is the one authoritative table of legal transitions.
var transitions = map[State][]State{
	StateEmitted:    {StateFetched, StateSuperseded},
	StateFetched:    {StateAccepted, StateExpired, StateRejected},
	StateAccepted:   {StateValidated, StateRejected},
	StateRejected:   {StateEmitted, StateAbandoned},
	StateExpired:    {StateEmitted, StateAbandoned},
	StateValidated:  {StateConsumed},
	StateConsumed:   nil,
	StateSuperseded: nil,
	StateAbandoned:  nil,
}

// States lists every lifecycle state.
func States() []State { return slices.Clone(states) }

// Legal reports whether the lifecycle allows moving from one state to another.
func Legal(from, to State) bool { return slices.Contains(transitions[from], to) }

// Terminal reports whether no transition leaves the state.
func Terminal(state State) bool { return len(transitions[state]) == 0 }

// Errors of the lifecycle.
var (
	// ErrIllegalTransition is returned for a move the table does not allow.
	ErrIllegalTransition = errors.New("illegal lifecycle transition")
	// ErrNotExpired is returned when a fetched ticket is still answerable.
	ErrNotExpired = errors.New("the ticket has not expired")
	// ErrAlreadyAccepted is returned for the replay of an accepted answer; it is idempotent.
	ErrAlreadyAccepted = errors.New("the answer was already accepted")
	// ErrResubmissionConflict is returned for a second, different answer.
	ErrResubmissionConflict = errors.New("resubmission_conflict")
	// ErrCapBoundElsewhere is returned when a ticket is fetched with another cap than the one it is bound to.
	ErrCapBoundElsewhere = errors.New("the ticket is bound to another capability")
)

// Entry is the stored record of one issued ticket and the unit the lifecycle
// moves. Its methods return the next entry and never change the receiver, so an
// illegal call leaves the stored record untouched.
type Entry struct {
	Ticket         Ticket    `json:"ticket"`
	Role           string    `json:"role"`
	SchemaDigest   string    `json:"schemaDigest"`
	State          State     `json:"state"`
	MaxEmissions   int       `json:"maxEmissions"`
	Corrections    int       `json:"corrections"`
	FetchedAt      time.Time `json:"fetchedAt"`
	CapDigest      string    `json:"capDigest,omitempty"`
	AcceptedOutput string    `json:"acceptedOutput,omitempty"`
	Reason         string    `json:"reason,omitempty"`
}

// NewEntry creates the entry of a freshly emitted ticket.
func NewEntry(ticket Ticket, role, schemaDigest string, maxEmissions int) Entry {
	return Entry{Ticket: ticket, Role: role, SchemaDigest: schemaDigest, State: StateEmitted, MaxEmissions: maxEmissions}
}

func (e Entry) move(to State) (Entry, error) {
	if !Legal(e.State, to) {
		return e, fmt.Errorf("%w: %s to %s", ErrIllegalTransition, e.State, to)
	}
	e.State = to
	return e, nil
}

// Fetch records that an executor fetched the ticket with the capability whose
// digest is given. Fetching again with the same capability changes nothing.
func (e Entry) Fetch(capDigest string, at time.Time) (Entry, error) {
	if e.State == StateFetched {
		if e.CapDigest != capDigest {
			return e, ErrCapBoundElsewhere
		}
		return e, nil
	}
	next, err := e.move(StateFetched)
	if err != nil {
		return e, err
	}
	next.FetchedAt = at
	next.CapDigest = capDigest
	return next, nil
}

// Accept records an answer whose schema and binding checks passed. Replaying the
// same output returns ErrAlreadyAccepted; a different one returns
// ErrResubmissionConflict. Both leave the entry unchanged.
func (e Entry) Accept(outputDigest string) (Entry, error) {
	if holdsAnswer(e.State) {
		switch ClassifyResubmission(e.AcceptedOutput, outputDigest) {
		case ResubmissionIdentical:
			return e, ErrAlreadyAccepted
		case ResubmissionConflict:
			return e, ErrResubmissionConflict
		}
	}
	next, err := e.move(StateAccepted)
	if err != nil {
		return e, err
	}
	next.AcceptedOutput = outputDigest
	return next, nil
}

// holdsAnswer reports whether the state keeps the accepted answer.
func holdsAnswer(state State) bool {
	return state == StateAccepted || state == StateValidated || state == StateConsumed
}

// FailSchema records an answer that does not satisfy the response schema. The
// first failure leaves the entry fetched and reports that a correction is
// allowed; the next one rejects it.
func (e Entry) FailSchema() (next Entry, correctionAllowed bool, err error) {
	if e.State != StateFetched {
		return e, false, fmt.Errorf("%w: a schema failure in state %s", ErrIllegalTransition, e.State)
	}
	if e.Corrections < MaxCorrections {
		e.Corrections++
		return e, true, nil
	}
	next, err = e.move(StateRejected)
	return next, false, err
}

// Contradict rejects an accepted answer that execution contradicts.
func (e Entry) Contradict() (Entry, error) { return e.move(StateRejected) }

// MarkValidated records that the answer passed every execution check.
func (e Entry) MarkValidated() (Entry, error) { return e.move(StateValidated) }

// Consume records that a proof used the validated answer.
func (e Entry) Consume() (Entry, error) { return e.move(StateConsumed) }

// Supersede retires an emitted ticket whose inputs changed.
func (e Entry) Supersede() (Entry, error) { return e.move(StateSuperseded) }

// Expire retires a fetched ticket that was not answered within FetchTTL.
func (e Entry) Expire(at time.Time) (Entry, error) {
	if e.State == StateFetched && at.Sub(e.FetchedAt) < FetchTTL {
		return e, ErrNotExpired
	}
	return e.move(StateExpired)
}

// Reissue turns a rejected or expired ticket into a new emission with the given
// nonce while emissions remain, and abandons it otherwise.
func (e Entry) Reissue(nonce string) (Entry, error) {
	if e.State != StateRejected && e.State != StateExpired {
		return e, fmt.Errorf("%w: reissue in state %s", ErrIllegalTransition, e.State)
	}
	if e.Ticket.Emission >= e.MaxEmissions {
		next, err := e.move(StateAbandoned)
		next.Reason = ReasonHostUnresponsive
		return next, err
	}
	next, err := e.move(StateEmitted)
	if err != nil {
		return e, err
	}
	next.Ticket = Ticket{TicketID: TicketID{RequestID: e.Ticket.RequestID, Vote: e.Ticket.Vote, Emission: e.Ticket.Emission + 1}, Nonce: nonce}
	next.Corrections = 0
	next.FetchedAt = time.Time{}
	next.CapDigest = ""
	next.AcceptedOutput = ""
	return next, nil
}

// Consumable reports whether a proof may use the entry's answer.
func (e Entry) Consumable() bool { return e.State == StateValidated }

// ResubmissionVerdict classifies a submission against the answer already accepted.
type ResubmissionVerdict string

// Resubmission verdicts.
const (
	// ResubmissionFirst: nothing was accepted yet.
	ResubmissionFirst ResubmissionVerdict = "first"
	// ResubmissionIdentical: the same answer arrived again.
	ResubmissionIdentical ResubmissionVerdict = "identical"
	// ResubmissionConflict: a different answer arrived for an accepted ticket.
	ResubmissionConflict ResubmissionVerdict = "conflict"
)

// ClassifyResubmission applies the resubmission rule: a second, different output
// for an accepted (requestId, vote, nonce) is a conflict.
func ClassifyResubmission(acceptedOutputDigest, outputDigest string) ResubmissionVerdict {
	switch {
	case acceptedOutputDigest == "":
		return ResubmissionFirst
	case sameSecret(acceptedOutputDigest, outputDigest):
		return ResubmissionIdentical
	default:
		return ResubmissionConflict
	}
}
