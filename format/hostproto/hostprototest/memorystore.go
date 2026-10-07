package hostprototest

import (
	"cmp"
	"slices"
	"sync"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// MemoryStore is the reference implementation of hostproto.Store: everything in
// memory, safe for concurrent use. Module adapters are held to the same contract
// by RunStoreContract.
type MemoryStore struct {
	mutex     sync.Mutex
	requests  map[hostproto.RequestID]hostproto.Request
	entries   map[hostproto.Key]hostproto.Entry
	responses map[hostproto.Key]hostproto.Response
	records   []hostproto.Record
	secret    []byte
}

// NewMemoryStore creates an empty store whose secret is the fixture secret.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		requests:  map[hostproto.RequestID]hostproto.Request{},
		entries:   map[hostproto.Key]hostproto.Entry{},
		responses: map[hostproto.Key]hostproto.Response{},
		secret:    Secret(),
	}
}

// PutRequest implements hostproto.Requests.
func (s *MemoryStore) PutRequest(request hostproto.Request) (hostproto.RequestID, error) {
	id, err := request.ID()
	if err != nil {
		return "", err
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.requests[id] = request
	return id, nil
}

// GetRequest implements hostproto.Requests.
func (s *MemoryStore) GetRequest(id hostproto.RequestID) (hostproto.Request, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	request, found := s.requests[id]
	if !found {
		return hostproto.Request{}, hostproto.ErrNotFound
	}
	return request, nil
}

// PutEntry implements hostproto.Tickets.
func (s *MemoryStore) PutEntry(entry hostproto.Entry) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	key := entry.Ticket.Key()
	if _, found := s.entries[key]; found {
		return hostproto.ErrExists
	}
	s.entries[key] = entry
	return nil
}

// GetEntry implements hostproto.Tickets.
func (s *MemoryStore) GetEntry(key hostproto.Key) (hostproto.Entry, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	entry, found := s.entries[key]
	if !found {
		return hostproto.Entry{}, hostproto.ErrNotFound
	}
	return entry, nil
}

// ListEntries implements hostproto.Tickets.
func (s *MemoryStore) ListEntries() ([]hostproto.Entry, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	listed := make([]hostproto.Entry, 0, len(s.entries))
	for _, entry := range s.entries {
		listed = append(listed, entry)
	}
	slices.SortFunc(listed, func(a, b hostproto.Entry) int {
		return cmp.Or(cmp.Compare(a.Ticket.RequestID, b.Ticket.RequestID), cmp.Compare(a.Ticket.Vote, b.Ticket.Vote))
	})
	return listed, nil
}

// UpdateEntry implements hostproto.Tickets.
func (s *MemoryStore) UpdateEntry(key hostproto.Key, update func(hostproto.Entry) (hostproto.Entry, error)) (hostproto.Entry, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	entry, found := s.entries[key]
	if !found {
		return hostproto.Entry{}, hostproto.ErrNotFound
	}
	next, err := update(entry)
	if err != nil {
		return entry, err
	}
	s.entries[key] = next
	return next, nil
}

// PutResponse implements hostproto.Responses.
func (s *MemoryStore) PutResponse(response hostproto.Response) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	key := response.Key()
	if _, found := s.responses[key]; found {
		return hostproto.ErrExists
	}
	s.responses[key] = response
	return nil
}

// DiscardResponse implements hostproto.ResponseDiscarder.
func (s *MemoryStore) DiscardResponse(key hostproto.Key) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.responses, key)
	return nil
}

// GetResponse implements hostproto.Responses.
func (s *MemoryStore) GetResponse(key hostproto.Key) (hostproto.Response, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	response, found := s.responses[key]
	if !found {
		return hostproto.Response{}, hostproto.ErrNotFound
	}
	return response, nil
}

// Append implements hostproto.Journal.
func (s *MemoryStore) Append(event hostproto.Event) (hostproto.Record, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	var head *hostproto.Record
	if len(s.records) > 0 {
		head = &s.records[len(s.records)-1]
	}
	record, err := hostproto.Chain(head, event)
	if err != nil {
		return hostproto.Record{}, err
	}
	s.records = append(s.records, record)
	return record, nil
}

// Records implements hostproto.Journal.
func (s *MemoryStore) Records() ([]hostproto.Record, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.Clone(s.records), nil
}

// Secret implements hostproto.Secrets.
func (s *MemoryStore) Secret() ([]byte, error) { return slices.Clone(s.secret), nil }
