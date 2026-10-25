package hostproto

import (
	"cmp"
	"encoding/json"
	"slices"
)

// QueueItem is the queue-only metadata of a request: which criteria it serves,
// which sessions want it, in which wave it was first emitted and what for. An
// executor never sees it.
type QueueItem struct {
	RequestID RequestID `json:"requestId"`
	Criteria  []string  `json:"criteria"`
	// Sessions are the sessions that want the request. An empty list means that
	// no session holds it any more.
	Sessions []Session `json:"sessions"`
	Wave     int       `json:"wave"`
	Purpose  string    `json:"purpose"`
}

// Queue is the module-owned queue state next to the host store.
type Queue interface {
	// Record adds the item, or adds its criteria and its sessions to the item
	// already stored for the request. An item recorded with no session belongs to
	// the prove session.
	Record(item QueueItem) error
	// Items lists every item ordered by request id.
	Items() ([]QueueItem, error)
	// Wave returns the number of the session's current wave, zero before its first.
	Wave(session Session) (int, error)
	// AdvanceWave starts the next wave of the session, marks the session live and
	// returns the wave's number.
	AdvanceWave(session Session) (int, error)
	// Sessions lists the state of every session the queue knows, ordered by
	// session: those that advanced a wave or ended, and those an item names.
	Sessions() ([]SessionState, error)
	// EndSession marks the session ended until it advances a wave again.
	EndSession(session Session) error
	// Withdraw removes the session from the sessions that want the request and
	// returns the item as it is stored then, or ErrNotFound.
	Withdraw(id RequestID, session Session) (QueueItem, error)
}

// HostStore is the host directory of a module: the shared protocol store, the
// discard of a response a reissued ticket replaces, and the queue.
type HostStore interface {
	Store
	ResponseDiscarder
	Queue
}

// QueueVersion is the version of the queue document that holds sessions. A
// document without a version holds one wave counter, which is the prove
// session's, and items that all belong to the prove session.
const QueueVersion = 2

// QueueState is the content of a queue, and the one place its rules live: every
// store keeps it, and only persists what its methods return.
type QueueState struct {
	items    []QueueItem
	sessions []SessionState
}

// Record implements Queue.Record: the first record of a request fixes its wave
// and its purpose, later records only add the criteria and the sessions.
func (q *QueueState) Record(item QueueItem) {
	sessions := item.Sessions
	if len(sessions) == 0 {
		sessions = []Session{SessionProve}
	}
	index := q.indexOf(item.RequestID)
	if index < 0 {
		item.Criteria = mergeCriteria(nil, item.Criteria)
		item.Sessions = mergeSessions(nil, sessions)
		q.items = append(q.items, item)
		slices.SortFunc(q.items, func(a, b QueueItem) int { return cmp.Compare(a.RequestID, b.RequestID) })
		return
	}
	stored := &q.items[index]
	stored.Criteria = mergeCriteria(stored.Criteria, item.Criteria)
	stored.Sessions = mergeSessions(stored.Sessions, sessions)
}

func (q *QueueState) indexOf(id RequestID) int {
	return slices.IndexFunc(q.items, func(stored QueueItem) bool { return stored.RequestID == id })
}

// mergeCriteria returns the sorted union of both lists without duplicates.
func mergeCriteria(stored, added []string) []string {
	merged := append(append([]string{}, stored...), added...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// Items implements Queue.Items: a copy no caller can change the state through.
func (q *QueueState) Items() []QueueItem {
	items := make([]QueueItem, len(q.items))
	for index, item := range q.items {
		item.Criteria = slices.Clone(item.Criteria)
		item.Sessions = slices.Clone(item.Sessions)
		items[index] = item
	}
	return items
}

// Wave implements Queue.Wave.
func (q *QueueState) Wave(session Session) int {
	if index := q.sessionIndex(session); index >= 0 {
		return q.sessions[index].Wave
	}
	return 0
}

// AdvanceWave implements Queue.AdvanceWave.
func (q *QueueState) AdvanceWave(session Session) int {
	state := q.stateOf(session)
	state.Wave++
	state.Ended = false
	return state.Wave
}

// EndSession implements Queue.EndSession.
func (q *QueueState) EndSession(session Session) {
	q.stateOf(session).Ended = true
}

// Withdraw implements Queue.Withdraw.
func (q *QueueState) Withdraw(id RequestID, session Session) (QueueItem, error) {
	index := q.indexOf(id)
	if index < 0 {
		return QueueItem{}, ErrNotFound
	}
	stored := &q.items[index]
	stored.Sessions = slices.DeleteFunc(mergeSessions(nil, stored.Sessions), func(held Session) bool { return held == session })
	return q.Items()[index], nil
}

// Sessions implements Queue.Sessions.
func (q *QueueState) Sessions() []SessionState {
	states := slices.Clone(q.sessions)
	for _, item := range q.items {
		for _, session := range item.Sessions {
			if !slices.ContainsFunc(states, func(state SessionState) bool { return state.Session == session }) {
				states = append(states, SessionState{Session: session})
			}
		}
	}
	slices.SortFunc(states, func(a, b SessionState) int { return cmp.Compare(a.Session, b.Session) })
	return states
}

func (q *QueueState) sessionIndex(session Session) int {
	return slices.IndexFunc(q.sessions, func(state SessionState) bool { return state.Session == session })
}

// stateOf returns the state of the session, adding it when the queue has none.
func (q *QueueState) stateOf(session Session) *SessionState {
	index := q.sessionIndex(session)
	if index < 0 {
		q.sessions = append(q.sessions, SessionState{Session: session})
		slices.SortFunc(q.sessions, func(a, b SessionState) int { return cmp.Compare(a.Session, b.Session) })
		index = q.sessionIndex(session)
	}
	return &q.sessions[index]
}

// queueDocument is the JSON form of a queue. A document without a version holds
// the single wave counter of the prove session in Wave.
type queueDocument struct {
	Version  int            `json:"version,omitempty"`
	Wave     int            `json:"wave,omitempty"`
	Items    []QueueItem    `json:"items"`
	Sessions []SessionState `json:"sessions,omitempty"`
}

// MarshalJSON writes the queue as a document of QueueVersion.
func (q QueueState) MarshalJSON() ([]byte, error) {
	document := queueDocument{Version: QueueVersion, Items: q.Items(), Sessions: q.sessions}
	return json.Marshal(document)
}

// UnmarshalJSON reads a queue document. A document without a version is read as
// the prove session's: its wave counter becomes the prove session's wave and
// every item it holds belongs to the prove session.
func (q *QueueState) UnmarshalJSON(content []byte) error {
	var document queueDocument
	if err := json.Unmarshal(content, &document); err != nil {
		return err
	}
	*q = QueueState{sessions: slices.Clone(document.Sessions)}
	for _, item := range document.Items {
		item.Sessions = mergeSessions(nil, item.Sessions)
		if document.Version < QueueVersion {
			item.Sessions = []Session{SessionProve}
		}
		item.Criteria = mergeCriteria(nil, item.Criteria)
		q.items = append(q.items, item)
	}
	slices.SortFunc(q.items, func(a, b QueueItem) int { return cmp.Compare(a.RequestID, b.RequestID) })
	if document.Version < QueueVersion && document.Wave > 0 {
		q.stateOf(SessionProve).Wave = document.Wave
	}
	return nil
}
