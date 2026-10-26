// Package hostsync moves host answers and requests between the developer's
// machine and CI: it collects the responses a push publishes, serves CI's
// pending tickets from a pushed bundle, exports the requests CI leaves for the
// host and imports them on the developer's machine.
package hostsync

import (
	"fmt"
	"io"
	"slices"
	"time"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/executorrun"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/hostbundle"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

// Details journaled for what a bundle brings in.
const (
	markedOnHost = "marked on the host that pushed the response"
)

func holdsAnswer(state hostproto.State) bool {
	return state == hostproto.StateAccepted || state == hostproto.StateValidated || state == hostproto.StateConsumed
}

// sessionEntries lists the entries of the requests the session holds, in store
// order.
func sessionEntries(store hostproto.HostStore, session hostproto.Session) ([]hostproto.Entry, []hostproto.QueueItem, error) {
	entries, err := store.ListEntries()
	if err != nil {
		return nil, nil, fmt.Errorf("list the tickets: %w", err)
	}
	queue, err := store.Items()
	if err != nil {
		return nil, nil, fmt.Errorf("read the queue: %w", err)
	}
	index, filter := hostproto.IndexSessions(queue), hostproto.OnlySession(session)
	return slices.DeleteFunc(entries, func(entry hostproto.Entry) bool { return !index.Selects(filter, entry.Ticket.RequestID) }), queue, nil
}

// Collect builds the bundle of a push from the store: every response of the
// session's requests the lifecycle keeps, with the profile it is credited with
// and the marks the journal holds against it, and the digest at the head of the
// journal.
func Collect(store hostproto.HostStore, session hostproto.Session, module format.Module, headSHA string) (hostbundle.Bundle, error) {
	entries, _, err := sessionEntries(store, session)
	if err != nil {
		return hostbundle.Bundle{}, err
	}
	records, err := store.Records()
	if err != nil {
		return hostbundle.Bundle{}, fmt.Errorf("read the journal: %w", err)
	}
	if err := hostproto.VerifyChain(records); err != nil {
		return hostbundle.Bundle{}, err
	}
	journaled := executorrun.IndexJournal(records)
	var responses []hostproto.Response
	marked := map[hostproto.Key]bool{}
	for _, entry := range entries {
		if !holdsAnswer(entry.State) {
			continue
		}
		response, err := store.GetResponse(entry.Ticket.Key())
		if err != nil {
			return hostbundle.Bundle{}, fmt.Errorf("read the response of %s: %w", entry.Ticket.TicketID, err)
		}
		events := journaled.Response(entry.Ticket.TicketID.String(), response.Executor)
		response.Executor = executorrun.Effective(response.Executor, events)
		marked[response.Key()] = executorrun.Marked(events)
		responses = append(responses, response)
	}
	digest := hostproto.GenesisDigest
	if len(records) > 0 {
		digest = records[len(records)-1].Digest
	}
	return hostbundle.NewBundle(module, headSHA, digest, responses, marked), nil
}

// ErrNothingServed is what an Importer returns when the bundle answers none of the
// pending tickets, so that the run stops waiting instead of asking again.
var ErrNothingServed = requests.ErrNothingServed

// Importer serves pending tickets from a pushed bundle. Ids the bundle does not
// hold stay pending and ids the run does not ask for are never read.
type Importer struct {
	Service requests.Service
	Store   hostproto.HostStore
	Bundle  hostbundle.Bundle
	// Unanswered is set by Serve to the number of tickets the bundle did not answer.
	Unanswered int
}

var _ requests.Replayer = (*Importer)(nil)

// Serve implements requests.Replayer.
func (i *Importer) Serve(tickets []requests.PendingTicket) error {
	served := 0
	i.Unanswered = 0
	for _, pending := range tickets {
		ticket, err := hostproto.ParseTicketID(pending.Ticket)
		if err != nil {
			return err
		}
		response, found := i.Bundle.Lookup(ticket.Key())
		if !found {
			i.Unanswered++
			continue
		}
		if err := i.serve(ticket, pending.Nonce, response); err != nil {
			return err
		}
		served++
	}
	if served == 0 {
		return ErrNothingServed
	}
	return nil
}

func (i *Importer) serve(ticket hostproto.TicketID, nonce string, response hostproto.Response) error {
	if _, err := i.Service.Show(ticket, ""); err != nil {
		return fmt.Errorf("fetch %s for the import: %w", ticket, err)
	}
	profile := response.Executor
	_, err := i.Service.Submit(requests.Submission{Ticket: ticket, Nonce: nonce, Output: response.Output, Origin: requests.OriginReplay, Executor: &profile})
	if err != nil {
		return fmt.Errorf("import %s: %w", ticket, err)
	}
	if i.Bundle.Marked(response.Key()) {
		if _, err := i.Store.Append(hostproto.Event{Kind: hostproto.EventExposed, Ticket: ticket.String(), Detail: markedOnHost}); err != nil {
			return fmt.Errorf("journal the mark of %s: %w", ticket, err)
		}
	}
	return nil
}

// ExportIssued gathers the requests of the session whose tickets still wait for
// an answer: the requests in full, with their votes, criteria and purpose.
func ExportIssued(store hostproto.HostStore, session hostproto.Session) (hostbundle.IssuedBundle, error) {
	entries, queue, err := sessionEntries(store, session)
	if err != nil {
		return hostbundle.IssuedBundle{}, err
	}
	votes := map[hostproto.RequestID]int{}
	for _, entry := range entries {
		if hostproto.Waits(entry.State) {
			votes[entry.Ticket.RequestID] = max(votes[entry.Ticket.RequestID], entry.Ticket.Vote+1)
		}
	}
	var issued hostbundle.IssuedBundle
	for id, count := range votes {
		request, err := store.GetRequest(id)
		if err != nil {
			return hostbundle.IssuedBundle{}, fmt.Errorf("read the request %s: %w", id, err)
		}
		entry := hostbundle.IssuedEntry{RequestID: id, Votes: count, Criteria: []string{}}
		for _, item := range queue {
			if item.RequestID == id {
				entry.Criteria, entry.Purpose = slices.Clone(item.Criteria), item.Purpose
			}
		}
		issued.Entries = append(issued.Entries, entry)
		issued.Requests = append(issued.Requests, request)
	}
	return issued, nil
}

// ImportIssued puts the requests CI issued into the store as tickets of the
// session for the host to answer. A request already there keeps the tickets it
// has.
func ImportIssued(store hostproto.HostStore, session hostproto.Session, issued hostbundle.IssuedBundle, clock func() time.Time, entropy io.Reader) (int, error) {
	wave, err := store.Wave(session)
	if err != nil {
		return 0, fmt.Errorf("read the wave: %w", err)
	}
	bridge := hostbridge.New(store, clock, entropy, session, wave)
	imported := 0
	for _, request := range issued.Requests {
		id, err := request.ID()
		if err != nil {
			return imported, err
		}
		entry, found := issued.EntryOf(id)
		if !found {
			return imported, fmt.Errorf("%w: %s", hostbundle.ErrBadBundle, id)
		}
		if _, err := bridge.Want(hostbridge.Ask{Request: request, Votes: entry.Votes, Criteria: entry.Criteria, Purpose: entry.Purpose}); err != nil {
			return imported, err
		}
		imported++
	}
	return imported, nil
}
