package hostproto

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// Session names one run that asks a host store for answers: `prove`, or one scan
// with its session key. Every queue item carries the sessions that want its
// request, so that two runs sharing a store neither see nor retire each other's
// tickets, and a request two sessions want is answered once for both.
type Session string

// Sessions and their shape.
const (
	// SessionProve is the session of `prove` in every module. It ends only when
	// prove does; no other session ends it.
	SessionProve Session = "prove"
	// scanSessionPrefix starts the session of a scan, followed by its session key.
	scanSessionPrefix = "scan:"
	// scanKeyPattern is the shape of a scan's session key.
	scanKeyPattern = `^[0-9A-Za-z._-]+$`
)

var scanKeyRegexp = regexp.MustCompile(scanKeyPattern)

// ErrBadSession is returned for a text that names no session.
var ErrBadSession = errors.New("not a session: want prove or scan:<key>")

// ScanSession returns the session of the scan with the session key.
func ScanSession(key string) Session { return Session(scanSessionPrefix + key) }

// IsScan reports whether the session is the session of a scan.
func (s Session) IsScan() bool {
	key, found := strings.CutPrefix(string(s), scanSessionPrefix)
	return found && scanKeyRegexp.MatchString(key)
}

// Validate checks that the session is prove or a scan with a session key.
func (s Session) Validate() error {
	if s == SessionProve || s.IsScan() {
		return nil
	}
	return fmt.Errorf("%w: %q", ErrBadSession, string(s))
}

// HoldsAny reports whether the session holds one of the queue items.
func (s Session) HoldsAny(items []QueueItem) bool {
	return slices.ContainsFunc(items, func(item QueueItem) bool { return slices.Contains(item.Sessions, s) })
}

// ParseSession reads a session as `prove` or `scan:<key>`.
func ParseSession(text string) (Session, error) {
	session := Session(text)
	return session, session.Validate()
}

// mergeSessions returns the sorted union of both lists without duplicates; it is
// never nil, so that an item no session holds stays distinct from an item of a
// queue written before sessions existed.
func mergeSessions(stored, added []Session) []Session {
	merged := append(append([]Session{}, stored...), added...)
	slices.Sort(merged)
	return slices.Compact(merged)
}

// SessionFilter selects what one session sees of a host store, or everything.
type SessionFilter struct {
	session Session
}

// EverySession is the filter that selects the items of every session.
var EverySession = SessionFilter{}

// OnlySession is the filter that selects the items the session holds.
func OnlySession(session Session) SessionFilter { return SessionFilter{session: session} }

// ParseSessionFilter reads the filter a command line names: the empty text
// selects every session, any other text one session.
func ParseSessionFilter(text string) (SessionFilter, error) {
	if text == "" {
		return EverySession, nil
	}
	session, err := ParseSession(text)
	return OnlySession(session), err
}

// Args returns the arguments that select the filter on the command line of a
// module, `--session <session>`, and none for every session.
func (f SessionFilter) Args() []string {
	if f == EverySession {
		return nil
	}
	return []string{FlagSession, string(f.session)}
}

// Selects reports whether an item held by the sessions passes the filter.
func (f SessionFilter) Selects(sessions []Session) bool {
	return f == EverySession || slices.Contains(sessions, f.session)
}

// SessionIndex tells which sessions hold each request of a queue.
type SessionIndex map[RequestID][]Session

// IndexSessions indexes the sessions of the queue items by request id.
func IndexSessions(items []QueueItem) SessionIndex {
	index := SessionIndex{}
	for _, item := range items {
		index[item.RequestID] = item.Sessions
	}
	return index
}

// Of returns the sessions that hold the request. A request the queue has no item
// for belongs to the prove session.
func (i SessionIndex) Of(id RequestID) []Session {
	sessions, found := i[id]
	if !found {
		return []Session{SessionProve}
	}
	return sessions
}

// Selects reports whether the filter selects the request.
func (i SessionIndex) Selects(filter SessionFilter, id RequestID) bool {
	return filter.Selects(i.Of(id))
}

// SessionState is what a queue keeps of one session: the number of its current
// wave and whether it ended. A session the queue has no state for is live.
type SessionState struct {
	Session Session `json:"session"`
	Wave    int     `json:"wave"`
	Ended   bool    `json:"ended,omitempty"`
}

// Liveness tells which sessions are live.
type Liveness map[Session]bool

// LivenessOf reads the liveness of the sessions a queue lists.
func LivenessOf(states []SessionState) Liveness {
	liveness := Liveness{}
	for _, state := range states {
		liveness[state.Session] = !state.Ended
	}
	return liveness
}

// Live reports whether the session is live: a session no state names is.
func (l Liveness) Live(session Session) bool {
	live, found := l[session]
	return !found || live
}

// AnyLive reports whether one of the sessions is live.
func (l Liveness) AnyLive(sessions []Session) bool {
	return slices.ContainsFunc(sessions, l.Live)
}
