package executorrun

import (
	"slices"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// JournalIndex groups the events of a journal by the response they bear on.
type JournalIndex struct {
	byTicket map[string][]hostproto.Event
	// blindnessByAgent holds the blindness events journaled against an agent
	// whose run named no ticket the stop hook could read.
	blindnessByAgent map[string][]hostproto.Event
}

// IndexJournal indexes the records of a journal.
func IndexJournal(records []hostproto.Record) JournalIndex {
	index := JournalIndex{byTicket: map[string][]hostproto.Event{}, blindnessByAgent: map[string][]hostproto.Event{}}
	for _, record := range records {
		event := record.Event
		switch {
		case event.Ticket != "":
			index.byTicket[event.Ticket] = append(index.byTicket[event.Ticket], event)
		case event.AgentID != "" && slices.Contains(blindnessKinds, event.Kind):
			index.blindnessByAgent[event.AgentID] = append(index.blindnessByAgent[event.AgentID], event)
		}
	}
	return index
}

// Ticket returns the events journaled against a ticket, in journal order.
func (j JournalIndex) Ticket(ticket string) []hostproto.Event { return j.byTicket[ticket] }

// Response returns the events that bear on the response a ticket holds: those of
// its ticket, then the blindness events journaled without a ticket against the
// agent its stored profile names as the one that answered.
func (j JournalIndex) Response(ticket string, stored format.ExecutorProfile) []hostproto.Event {
	events := slices.Clone(j.byTicket[ticket])
	if agent := stored.Attestation.AgentID; agent != "" {
		events = append(events, j.blindnessByAgent[agent]...)
	}
	return events
}
