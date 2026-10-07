package hostprototest

import (
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	contractSubmitters = 16
	contractAppends    = 32
)

// RunStoreContract runs the Store contract against the adapter made by newStore.
// Every call of newStore must return an empty, independent store.
func RunStoreContract(t *testing.T, newStore func(t *testing.T) hostproto.Store) {
	t.Helper()
	contracts := []struct {
		name string
		run  func(t *testing.T, store hostproto.Store)
	}{
		{"a stored request is returned by its id", requestRoundTrip},
		{"storing a request twice is not an error", requestIdempotent},
		{"an unknown request is not found", requestNotFound},
		{"an entry is created once per key", entryCreateOnce},
		{"an unknown entry is not found", entryNotFound},
		{"entries are listed by request id then vote", entryListOrder},
		{"a failed update leaves the entry unchanged", entryFailedUpdate},
		{"concurrent updates of one entry are serialized", entryConcurrentUpdates},
		{"a response is created once per key", responseCreateOnce},
		{"an unknown response is not found", responseNotFound},
		{"the journal chain verifies and survives concurrent appends", journalChain},
		{"the secret is stable and not empty", secretStable},
	}
	for _, contract := range contracts {
		t.Run(contract.name, func(t *testing.T) { contract.run(t, newStore(t)) })
	}
}

func requestRoundTrip(t *testing.T, store hostproto.Store) {
	request := Request()
	id, err := store.PutRequest(request)
	if err != nil || id != MustID(request) {
		t.Fatalf("PutRequest = %q, %v", id, err)
	}
	got, err := store.GetRequest(id)
	if err != nil {
		t.Fatal(err)
	}
	gotID, err := got.ID()
	if err != nil || gotID != id {
		t.Fatalf("the stored request has id %q, %v", gotID, err)
	}
}

func requestIdempotent(t *testing.T, store hostproto.Store) {
	for range 2 {
		if _, err := store.PutRequest(Request()); err != nil {
			t.Fatal(err)
		}
	}
}

func requestNotFound(t *testing.T, store hostproto.Store) {
	if _, err := store.GetRequest(MustID(Request())); !errors.Is(err, hostproto.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func freshEntry(vote int) hostproto.Entry {
	request := Request()
	return hostproto.NewEntry(Ticket(request, vote, hostproto.FirstEmission), request.Role, Digest("schema"), hostproto.DefaultMaxEmissions)
}

func entryCreateOnce(t *testing.T, store hostproto.Store) {
	entry := freshEntry(0)
	if err := store.PutEntry(entry); err != nil {
		t.Fatal(err)
	}
	if err := store.PutEntry(entry); !errors.Is(err, hostproto.ErrExists) {
		t.Fatalf("second PutEntry error = %v", err)
	}
	got, err := store.GetEntry(entry.Ticket.Key())
	if err != nil || got != entry {
		t.Fatalf("GetEntry = %+v, %v", got, err)
	}
}

func entryNotFound(t *testing.T, store hostproto.Store) {
	if _, err := store.GetEntry(freshEntry(0).Ticket.Key()); !errors.Is(err, hostproto.ErrNotFound) {
		t.Fatalf("GetEntry error = %v", err)
	}
	_, err := store.UpdateEntry(freshEntry(0).Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e, nil })
	if !errors.Is(err, hostproto.ErrNotFound) {
		t.Fatalf("UpdateEntry error = %v", err)
	}
}

func entryListOrder(t *testing.T, store hostproto.Store) {
	for _, vote := range []int{2, 0, 1} {
		if err := store.PutEntry(freshEntry(vote)); err != nil {
			t.Fatal(err)
		}
	}
	listed, err := store.ListEntries()
	if err != nil {
		t.Fatal(err)
	}
	votes := make([]int, len(listed))
	for index, entry := range listed {
		votes[index] = entry.Ticket.Vote
	}
	if !slices.Equal(votes, []int{0, 1, 2}) {
		t.Fatalf("votes = %v", votes)
	}
}

var errRefused = errors.New("update refused")

func entryFailedUpdate(t *testing.T, store hostproto.Store) {
	entry := freshEntry(0)
	if err := store.PutEntry(entry); err != nil {
		t.Fatal(err)
	}
	_, err := store.UpdateEntry(entry.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) {
		e.State = hostproto.StateConsumed
		return e, errRefused
	})
	if !errors.Is(err, errRefused) {
		t.Fatalf("error = %v", err)
	}
	got, err := store.GetEntry(entry.Ticket.Key())
	if err != nil || got != entry {
		t.Fatalf("the entry changed to %+v, %v", got, err)
	}
}

func entryConcurrentUpdates(t *testing.T, store hostproto.Store) {
	entry := freshEntry(0)
	if err := store.PutEntry(entry); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var group sync.WaitGroup
	for range contractSubmitters {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.UpdateEntry(entry.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Fetch(Digest("cap"), epoch) })
			if err == nil {
				accepted.Add(1)
			}
		}()
	}
	group.Wait()
	got, err := store.GetEntry(entry.Ticket.Key())
	if err != nil || got.State != hostproto.StateFetched {
		t.Fatalf("entry = %+v, %v", got, err)
	}
	if accepted.Load() != contractSubmitters {
		t.Fatalf("%d of %d idempotent fetches succeeded", accepted.Load(), contractSubmitters)
	}
	var transitions atomic.Int32
	for range contractSubmitters {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := store.UpdateEntry(entry.Ticket.Key(), func(e hostproto.Entry) (hostproto.Entry, error) { return e.Accept(Digest("answer")) })
			if err == nil {
				transitions.Add(1)
			}
		}()
	}
	group.Wait()
	if transitions.Load() != 1 {
		t.Fatalf("%d concurrent accepts moved the entry, want exactly 1", transitions.Load())
	}
}

func responseCreateOnce(t *testing.T, store hostproto.Store) {
	response := Response(Ticket(Request(), 0, hostproto.FirstEmission))
	if err := store.PutResponse(response); err != nil {
		t.Fatal(err)
	}
	if err := store.PutResponse(response); !errors.Is(err, hostproto.ErrExists) {
		t.Fatalf("second PutResponse error = %v", err)
	}
	got, err := store.GetResponse(response.Key())
	if err != nil || got.Nonce != response.Nonce || string(got.Output) != string(response.Output) {
		t.Fatalf("GetResponse = %+v, %v", got, err)
	}
}

func responseNotFound(t *testing.T, store hostproto.Store) {
	if _, err := store.GetResponse(hostproto.Key{RequestID: MustID(Request())}); !errors.Is(err, hostproto.ErrNotFound) {
		t.Fatalf("error = %v", err)
	}
}

func journalChain(t *testing.T, store hostproto.Store) {
	var group sync.WaitGroup
	for index := range contractAppends {
		group.Add(1)
		go func() {
			defer group.Done()
			if _, err := store.Append(hostproto.Event{Kind: hostproto.EventEmitted, Detail: string(rune('a' + index%26))}); err != nil {
				t.Error(err)
			}
		}()
	}
	group.Wait()
	records, err := store.Records()
	if err != nil || len(records) != contractAppends {
		t.Fatalf("%d records, %v", len(records), err)
	}
	if err := hostproto.VerifyChain(records); err != nil {
		t.Fatal(err)
	}
}

func secretStable(t *testing.T, store hostproto.Store) {
	first, err := store.Secret()
	if err != nil || len(first) == 0 {
		t.Fatalf("Secret = %x, %v", first, err)
	}
	second, err := store.Secret()
	if err != nil || !slices.Equal(first, second) {
		t.Fatalf("the secret changed: %x then %x (%v)", first, second, err)
	}
}
