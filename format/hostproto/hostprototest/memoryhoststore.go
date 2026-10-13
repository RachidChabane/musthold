package hostprototest

import (
	"sync"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// MemoryHostStore completes the MemoryStore with an in-memory queue, so that it
// satisfies hostproto.HostStore: the reference implementation of the host
// directory a module's adapter is held to.
type MemoryHostStore struct {
	*MemoryStore
	mutex sync.Mutex
	queue hostproto.QueueState
}

var _ hostproto.HostStore = (*MemoryHostStore)(nil)

// NewMemoryHostStore creates an empty host store.
func NewMemoryHostStore() *MemoryHostStore {
	return &MemoryHostStore{MemoryStore: NewMemoryStore()}
}

// inQueue runs fn on the queue while holding the queue's lock.
func inQueue[T any](s *MemoryHostStore, fn func(queue *hostproto.QueueState) (T, error)) (T, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	return fn(&s.queue)
}

// Record implements hostproto.Queue.
func (s *MemoryHostStore) Record(item hostproto.QueueItem) error {
	_, err := inQueue(s, func(queue *hostproto.QueueState) (struct{}, error) {
		queue.Record(item)
		return struct{}{}, nil
	})
	return err
}

// Items implements hostproto.Queue.
func (s *MemoryHostStore) Items() ([]hostproto.QueueItem, error) {
	return inQueue(s, func(queue *hostproto.QueueState) ([]hostproto.QueueItem, error) { return queue.Items(), nil })
}

// Wave implements hostproto.Queue.
func (s *MemoryHostStore) Wave(session hostproto.Session) (int, error) {
	return inQueue(s, func(queue *hostproto.QueueState) (int, error) { return queue.Wave(session), nil })
}

// AdvanceWave implements hostproto.Queue.
func (s *MemoryHostStore) AdvanceWave(session hostproto.Session) (int, error) {
	return inQueue(s, func(queue *hostproto.QueueState) (int, error) { return queue.AdvanceWave(session), nil })
}

// Sessions implements hostproto.Queue.
func (s *MemoryHostStore) Sessions() ([]hostproto.SessionState, error) {
	return inQueue(s, func(queue *hostproto.QueueState) ([]hostproto.SessionState, error) { return queue.Sessions(), nil })
}

// EndSession implements hostproto.Queue.
func (s *MemoryHostStore) EndSession(session hostproto.Session) error {
	_, err := inQueue(s, func(queue *hostproto.QueueState) (struct{}, error) {
		queue.EndSession(session)
		return struct{}{}, nil
	})
	return err
}

// Withdraw implements hostproto.Queue.
func (s *MemoryHostStore) Withdraw(id hostproto.RequestID, session hostproto.Session) (hostproto.QueueItem, error) {
	return inQueue(s, func(queue *hostproto.QueueState) (hostproto.QueueItem, error) { return queue.Withdraw(id, session) })
}
