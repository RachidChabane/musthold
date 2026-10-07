package hostprototest

import (
	"cmp"
	"slices"
	"sync"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// MemoryHostStore completes the MemoryStore with an in-memory queue, so that it
// satisfies hostproto.HostStore: the reference implementation of the host
// directory a module's adapter is held to.
type MemoryHostStore struct {
	*MemoryStore
	mutex sync.Mutex
	items []hostproto.QueueItem
	wave  int
}

var _ hostproto.HostStore = (*MemoryHostStore)(nil)

// NewMemoryHostStore creates an empty host store.
func NewMemoryHostStore() *MemoryHostStore {
	return &MemoryHostStore{MemoryStore: NewMemoryStore()}
}

// Record implements hostproto.Queue.
func (s *MemoryHostStore) Record(item hostproto.QueueItem) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for index := range s.items {
		if s.items[index].RequestID == item.RequestID {
			s.items[index].Criteria = append(s.items[index].Criteria, item.Criteria...)
			return nil
		}
	}
	s.items = append(s.items, item)
	return nil
}

// Items implements hostproto.Queue.
func (s *MemoryHostStore) Items() ([]hostproto.QueueItem, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return slices.SortedFunc(slices.Values(s.items), func(a, b hostproto.QueueItem) int { return cmp.Compare(a.RequestID, b.RequestID) }), nil
}

// Wave implements hostproto.Queue.
func (s *MemoryHostStore) Wave() (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return s.wave, nil
}

// AdvanceWave implements hostproto.Queue.
func (s *MemoryHostStore) AdvanceWave() (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.wave++
	return s.wave, nil
}
