package hoststore

import (
	"errors"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

var errNoQueue = errors.New("no queue yet")

// loadQueue reads queue.json; a directory without one holds the empty queue.
func (s *Store) loadQueue() (hostproto.QueueState, error) {
	var state hostproto.QueueState
	err := readJSON(s.path(QueueFile), &state, errNoQueue)
	if errors.Is(err, errNoQueue) {
		return hostproto.QueueState{}, nil
	}
	return state, err
}

// readQueue runs read on the queue under the lock.
func readQueue[T any](s *Store, read func(queue *hostproto.QueueState) T) (T, error) {
	return withLock(s, func() (T, error) {
		state, err := s.loadQueue()
		if err != nil {
			var zero T
			return zero, err
		}
		return read(&state), nil
	})
}

// changeQueue runs change on the queue under the lock and saves what it leaves
// when it succeeds.
func changeQueue[T any](s *Store, change func(queue *hostproto.QueueState) (T, error)) (T, error) {
	return withLock(s, func() (T, error) {
		var zero T
		state, err := s.loadQueue()
		if err != nil {
			return zero, err
		}
		result, err := change(&state)
		if err != nil {
			return zero, err
		}
		return result, writeJSON(s.path(QueueFile), state)
	})
}

// Record implements hostproto.Queue.
func (s *Store) Record(item hostproto.QueueItem) error {
	_, err := changeQueue(s, func(queue *hostproto.QueueState) (struct{}, error) {
		queue.Record(item)
		return struct{}{}, nil
	})
	return err
}

// Items implements hostproto.Queue.
func (s *Store) Items() ([]hostproto.QueueItem, error) {
	return readQueue(s, (*hostproto.QueueState).Items)
}

// Wave implements hostproto.Queue.
func (s *Store) Wave(session hostproto.Session) (int, error) {
	return readQueue(s, func(queue *hostproto.QueueState) int { return queue.Wave(session) })
}

// AdvanceWave implements hostproto.Queue.
func (s *Store) AdvanceWave(session hostproto.Session) (int, error) {
	return changeQueue(s, func(queue *hostproto.QueueState) (int, error) { return queue.AdvanceWave(session), nil })
}

// Sessions implements hostproto.Queue.
func (s *Store) Sessions() ([]hostproto.SessionState, error) {
	return readQueue(s, (*hostproto.QueueState).Sessions)
}

// EndSession implements hostproto.Queue.
func (s *Store) EndSession(session hostproto.Session) error {
	_, err := changeQueue(s, func(queue *hostproto.QueueState) (struct{}, error) {
		queue.EndSession(session)
		return struct{}{}, nil
	})
	return err
}

// Withdraw implements hostproto.Queue.
func (s *Store) Withdraw(id hostproto.RequestID, session hostproto.Session) (hostproto.QueueItem, error) {
	return changeQueue(s, func(queue *hostproto.QueueState) (hostproto.QueueItem, error) { return queue.Withdraw(id, session) })
}
