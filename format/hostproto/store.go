package hostproto

import "errors"

// Errors every Store adapter returns for the same conditions.
var (
	// ErrNotFound is returned when a record does not exist.
	ErrNotFound = errors.New("not found")
	// ErrExists is returned when a create-only record already exists.
	ErrExists = errors.New("already exists")
)

// Key addresses the entry and the response of one vote of a request. The emission
// is not part of it: a reissued ticket replaces its predecessor under the same key.
type Key struct {
	RequestID RequestID
	Vote      int
}

// Requests stores emitted requests by content address.
type Requests interface {
	// PutRequest stores the request and returns its id. Storing the same request
	// again is not an error.
	PutRequest(request Request) (RequestID, error)
	// GetRequest returns the request with the id, or ErrNotFound.
	GetRequest(id RequestID) (Request, error)
}

// Tickets stores the entry of every issued ticket.
type Tickets interface {
	// PutEntry stores a new entry, or returns ErrExists for its key.
	PutEntry(entry Entry) error
	// GetEntry returns the entry at the key, or ErrNotFound.
	GetEntry(key Key) (Entry, error)
	// ListEntries returns every entry ordered by request id, then vote.
	ListEntries() ([]Entry, error)
	// UpdateEntry atomically replaces the entry with what update returns for the
	// stored one. When update fails, the stored entry is unchanged and the error
	// is returned as is. Concurrent updates of one entry are serialized.
	UpdateEntry(key Key, update func(Entry) (Entry, error)) (Entry, error)
}

// Responses stores accepted responses.
type Responses interface {
	// PutResponse stores a response, or returns ErrExists for its key.
	PutResponse(response Response) error
	// GetResponse returns the response at the key, or ErrNotFound.
	GetResponse(key Key) (Response, error)
}

// ResponseDiscarder removes the response stored for a key. A reissued ticket
// takes the place of its predecessor under the same key, so the answer a rejected
// ticket held must go before the new ticket can be answered. Discarding an absent
// response is not an error.
type ResponseDiscarder interface {
	DiscardResponse(key Key) error
}

// Journal is the digest-chained, append-only log of events.
type Journal interface {
	// Append chains the event after the current head and returns the record.
	// Concurrent appends are serialized, so the chain stays valid.
	Append(event Event) (Record, error)
	// Records returns the whole chain in order.
	Records() ([]Record, error)
}

// Secrets holds the per-install secret that capabilities are made with.
type Secrets interface {
	// Secret returns the secret, creating it on first use. It is stable for the
	// life of the store.
	Secret() ([]byte, error)
}

// Store is the port every module implements for its host directory. It is the
// union of the narrow ports, so that code that needs one capability depends on
// that one only.
type Store interface {
	Requests
	Tickets
	Responses
	Journal
	Secrets
}
