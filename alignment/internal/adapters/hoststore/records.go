package hoststore

import (
	"cmp"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/RachidChabane/musthold/alignment/internal/platform/atomicfile"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

func (s *Store) requestPath(id hostproto.RequestID) string {
	return s.path(RequestsDir, string(id)+recordExt)
}

func (s *Store) keyPath(directory string, key hostproto.Key) string {
	return s.path(directory, string(key.RequestID)+keySep+strconv.Itoa(key.Vote)+recordExt)
}

// validKey reports whether the key can name a file of the store: a request id is
// a SHA-256 hex digest and never holds a path separator.
func validKey(key hostproto.Key) bool { return format.IsSHA256Hex(string(key.RequestID)) }

// PutRequest implements hostproto.Requests. A request already stored under its
// content address is kept as it is.
func (s *Store) PutRequest(request hostproto.Request) (hostproto.RequestID, error) {
	id, err := request.ID()
	if err != nil {
		return "", err
	}
	return withLock(s, func() (hostproto.RequestID, error) {
		path := s.requestPath(id)
		if _, err := os.Stat(path); err == nil {
			return id, nil
		}
		return id, writeJSON(path, request)
	})
}

// GetRequest implements hostproto.Requests.
func (s *Store) GetRequest(id hostproto.RequestID) (hostproto.Request, error) {
	var request hostproto.Request
	if !format.IsSHA256Hex(string(id)) {
		return request, hostproto.ErrNotFound
	}
	err := readJSON(s.requestPath(id), &request, hostproto.ErrNotFound)
	return request, err
}

// PutEntry implements hostproto.Tickets.
func (s *Store) PutEntry(entry hostproto.Entry) error {
	key := entry.Ticket.Key()
	if !validKey(key) {
		return fmt.Errorf("entry key %q: %w", key.RequestID, ErrInvalidKey)
	}
	return inLock(s, func() error {
		return createJSON(s.keyPath(TicketsDir, key), entry, hostproto.ErrExists)
	})
}

// GetEntry implements hostproto.Tickets.
func (s *Store) GetEntry(key hostproto.Key) (hostproto.Entry, error) {
	var entry hostproto.Entry
	if !validKey(key) {
		return entry, hostproto.ErrNotFound
	}
	err := readJSON(s.keyPath(TicketsDir, key), &entry, hostproto.ErrNotFound)
	return entry, err
}

// ListEntries implements hostproto.Tickets.
func (s *Store) ListEntries() ([]hostproto.Entry, error) {
	directory := s.path(TicketsDir)
	names, err := recordNames(directory)
	if err != nil {
		return nil, err
	}
	entries := make([]hostproto.Entry, 0, len(names))
	for _, name := range names {
		var entry hostproto.Entry
		if err := readJSON(filepath.Join(directory, name), &entry, hostproto.ErrNotFound); err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}
	slices.SortFunc(entries, func(a, b hostproto.Entry) int {
		return cmp.Or(cmp.Compare(a.Ticket.RequestID, b.Ticket.RequestID), cmp.Compare(a.Ticket.Vote, b.Ticket.Vote))
	})
	return entries, nil
}

// recordNames lists the record files of a directory, ignoring temporary files.
func recordNames(directory string) ([]string, error) {
	listed, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", directory, err)
	}
	var names []string
	for _, item := range listed {
		if !item.IsDir() && strings.HasSuffix(item.Name(), recordExt) && !atomicfile.IsTemp(item.Name()) {
			names = append(names, item.Name())
		}
	}
	return names, nil
}

// UpdateEntry implements hostproto.Tickets.
func (s *Store) UpdateEntry(key hostproto.Key, update func(hostproto.Entry) (hostproto.Entry, error)) (hostproto.Entry, error) {
	if !validKey(key) {
		return hostproto.Entry{}, hostproto.ErrNotFound
	}
	return withLock(s, func() (hostproto.Entry, error) {
		path := s.keyPath(TicketsDir, key)
		var stored hostproto.Entry
		if err := readJSON(path, &stored, hostproto.ErrNotFound); err != nil {
			return hostproto.Entry{}, err
		}
		next, err := update(stored)
		if err != nil {
			return stored, err
		}
		return next, writeJSON(path, next)
	})
}

// PutResponse implements hostproto.Responses.
func (s *Store) PutResponse(response hostproto.Response) error {
	key := response.Key()
	if !validKey(key) {
		return fmt.Errorf("response key %q: %w", key.RequestID, ErrInvalidKey)
	}
	return inLock(s, func() error {
		return createJSON(s.keyPath(ResponsesDir, key), response, hostproto.ErrExists)
	})
}

// DiscardResponse implements hostproto.ResponseDiscarder.
func (s *Store) DiscardResponse(key hostproto.Key) error {
	if !validKey(key) {
		return fmt.Errorf("response key %q: %w", key.RequestID, ErrInvalidKey)
	}
	return inLock(s, func() error {
		if err := os.Remove(s.keyPath(ResponsesDir, key)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("discard the response: %w", err)
		}
		return nil
	})
}

// GetResponse implements hostproto.Responses.
func (s *Store) GetResponse(key hostproto.Key) (hostproto.Response, error) {
	var response hostproto.Response
	if !validKey(key) {
		return response, hostproto.ErrNotFound
	}
	err := readJSON(s.keyPath(ResponsesDir, key), &response, hostproto.ErrNotFound)
	return response, err
}
