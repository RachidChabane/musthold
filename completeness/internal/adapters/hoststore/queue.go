package hoststore

import (
	"cmp"
	"errors"
	"slices"

	"github.com/RachidChabane/spec-proof/completeness/internal/app/ports"
)

// queueState is the content of queue.json.
type queueState struct {
	Wave  int               `json:"wave"`
	Items []ports.QueueItem `json:"items"`
}

var errNoQueue = errors.New("no queue yet")

// loadQueue reads queue.json; a directory without one holds the empty queue.
func (s *Store) loadQueue() (queueState, error) {
	var state queueState
	err := readJSON(s.path(QueueFile), &state, errNoQueue)
	if errors.Is(err, errNoQueue) {
		return queueState{}, nil
	}
	return state, err
}

func (s *Store) saveQueue(state queueState) error {
	return writeJSON(s.path(QueueFile), state)
}

// Record implements ports.Queue: the first record of a request fixes its wave,
// later records only add the criteria it serves.
func (s *Store) Record(item ports.QueueItem) error {
	return inLock(s, func() error {
		state, err := s.loadQueue()
		if err != nil {
			return err
		}
		index := slices.IndexFunc(state.Items, func(stored ports.QueueItem) bool { return stored.RequestID == item.RequestID })
		if index < 0 {
			item.Criteria = mergeCriteria(nil, item.Criteria)
			state.Items = append(state.Items, item)
		} else {
			state.Items[index].Criteria = mergeCriteria(state.Items[index].Criteria, item.Criteria)
		}
		slices.SortFunc(state.Items, func(a, b ports.QueueItem) int { return cmp.Compare(a.RequestID, b.RequestID) })
		return s.saveQueue(state)
	})
}

// mergeCriteria returns the sorted union of both lists without duplicates.
func mergeCriteria(stored, added []string) []string {
	merged := append(slices.Clone(stored), added...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// Items implements ports.Queue.
func (s *Store) Items() ([]ports.QueueItem, error) {
	state, err := withLock(s, s.loadQueue)
	if err != nil {
		return nil, err
	}
	if state.Items == nil {
		return []ports.QueueItem{}, nil
	}
	return state.Items, nil
}

// Wave implements ports.Queue.
func (s *Store) Wave() (int, error) {
	state, err := withLock(s, s.loadQueue)
	return state.Wave, err
}

// AdvanceWave implements ports.Queue.
func (s *Store) AdvanceWave() (int, error) {
	return withLock(s, func() (int, error) {
		state, err := s.loadQueue()
		if err != nil {
			return 0, err
		}
		state.Wave++
		return state.Wave, s.saveQueue(state)
	})
}
