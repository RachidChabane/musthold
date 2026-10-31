package hostproto

import (
	"fmt"
	"slices"
)

// stopEventKinds are the events the stop hook of a harness journals when an
// executor run ends: each names the ticket the run fetched and the agent that
// ran it.
var stopEventKinds = []EventKind{
	EventAttested, EventLeaked, EventExposed, EventRestricted, EventAllowlistObserved,
	EventLaunchBare, EventHistoryInherited, EventBlindnessUnproven,
}

// RunEnded reports whether the events of one ticket show that the run of an
// executor ended: a stop event bound to an agent. A ticket fetched outside an
// executor, from the command line or a relay, has no such event.
func RunEnded(events []Event) bool {
	return slices.ContainsFunc(events, func(event Event) bool {
		return event.AgentID != "" && slices.Contains(stopEventKinds, event.Kind)
	})
}

// Outstanding is what a session waits for, sorted by what can be done about each
// ticket. One read of the store serves every report of it, so that the count a
// run prints and the tickets `request list` shows never disagree.
type Outstanding struct {
	// Launchable tickets are emitted: an executor may be started for each.
	Launchable []Entry
	// InFlight tickets were fetched by an executor whose run has not ended.
	InFlight []Entry
	// Orphaned tickets were fetched by an executor whose run ended without an
	// answer: nothing can answer them until they are reissued.
	Orphaned []Entry
}

// Waiting counts every ticket the session waits for.
func (o Outstanding) Waiting() int { return len(o.Launchable) + len(o.InFlight) + len(o.Orphaned) }

// Tickets lists the launchable tickets.
func (o Outstanding) Tickets() []Ticket {
	tickets := make([]Ticket, len(o.Launchable))
	for index, entry := range o.Launchable {
		tickets[index] = entry.Ticket
	}
	return tickets
}

// TicketEvents indexes the journaled events by the ticket they name.
type TicketEvents map[string][]Event

// IndexTicketEvents indexes the events of the records by ticket.
func IndexTicketEvents(records []Record) TicketEvents {
	index := TicketEvents{}
	for _, record := range records {
		if ticket := record.Event.Ticket; ticket != "" {
			index[ticket] = append(index[ticket], record.Event)
		}
	}
	return index
}

// Orphaned reports whether the entry is fetched and the run of the executor that
// fetched this emission ended.
func (t TicketEvents) Orphaned(entry Entry) bool {
	return entry.State == StateFetched && RunEnded(t[entry.Ticket.TicketID.String()])
}

// ReadOutstanding sorts the tickets of the sessions the filter selects that wait
// for an answer, in store order.
func ReadOutstanding(entries []Entry, items []QueueItem, records []Record, filter SessionFilter) Outstanding {
	sessions := IndexSessions(items)
	events := IndexTicketEvents(records)
	var outstanding Outstanding
	for _, entry := range entries {
		if !Waits(entry.State) || !sessions.Selects(filter, entry.Ticket.RequestID) {
			continue
		}
		switch {
		case entry.State == StateEmitted:
			outstanding.Launchable = append(outstanding.Launchable, entry)
		case events.Orphaned(entry):
			outstanding.Orphaned = append(outstanding.Orphaned, entry)
		default:
			outstanding.InFlight = append(outstanding.InFlight, entry)
		}
	}
	return outstanding
}

// OutstandingSource is what the outstanding tickets are read from.
type OutstandingSource interface {
	Tickets
	Journal
	Items() ([]QueueItem, error)
}

// QueryOutstanding reads the outstanding tickets of the sessions the filter
// selects from the store.
func QueryOutstanding(store OutstandingSource, filter SessionFilter) (Outstanding, error) {
	view, err := ReadStoreView(store)
	if err != nil {
		return Outstanding{}, err
	}
	return view.Outstanding(filter), nil
}

// StoreView is one read of the tickets, the queue and the journal of a store.
type StoreView struct {
	Entries []Entry
	Items   []QueueItem
	Records []Record
}

// ReadStoreView reads the tickets, the queue and the journal of the store.
func ReadStoreView(store OutstandingSource) (StoreView, error) {
	entries, err := store.ListEntries()
	if err != nil {
		return StoreView{}, fmt.Errorf("list the tickets: %w", err)
	}
	items, err := store.Items()
	if err != nil {
		return StoreView{}, fmt.Errorf("read the queue: %w", err)
	}
	records, err := store.Records()
	if err != nil {
		return StoreView{}, fmt.Errorf("read the journal: %w", err)
	}
	return StoreView{Entries: entries, Items: items, Records: records}, nil
}

// Outstanding sorts the waiting tickets of the sessions the filter selects.
func (v StoreView) Outstanding(filter SessionFilter) Outstanding {
	return ReadOutstanding(v.Entries, v.Items, v.Records, filter)
}

// Progress reads what the journal shows of the sessions the filter selects since
// the cursor, with their outstanding tickets.
func (v StoreView) Progress(filter SessionFilter, since int) Progress {
	outstanding := v.Outstanding(filter)
	return Progress{
		Cursor: JournalCursor(v.Records), Since: since,
		Answered:   AnsweredSince(v.Records, since, IndexSessions(v.Items), filter),
		Launchable: len(outstanding.Launchable), InFlight: len(outstanding.InFlight), Orphaned: len(outstanding.Orphaned),
	}
}

// JournalCursor is the sequence number the next journal record will carry: a
// point in the journal that a later read can ask "since" about.
func JournalCursor(records []Record) int { return len(records) }

// AnsweredSince counts the answers the journal accepted, from the record whose
// sequence is the cursor on, for tickets of the sessions the filter selects.
func AnsweredSince(records []Record, cursor int, sessions SessionIndex, filter SessionFilter) int {
	count := 0
	for _, record := range records {
		if record.Sequence < cursor || record.Event.Kind != EventAccepted {
			continue
		}
		ticket, err := ParseTicketID(record.Event.Ticket)
		if err == nil && sessions.Selects(filter, ticket.RequestID) {
			count++
		}
	}
	return count
}

// AcceptedAnswers counts every answer the journal accepted, of any session: the
// count a module's pending file keeps. The journal is append-only, so the count
// never decreases.
func AcceptedAnswers(records []Record) int {
	count := 0
	for _, record := range records {
		if record.Event.Kind == EventAccepted {
			count++
		}
	}
	return count
}
