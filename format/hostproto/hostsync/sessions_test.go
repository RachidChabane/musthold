package hostsync_test

import (
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"github.com/RachidChabane/musthold/format/hostproto/hostsync"
)

var scanSession = hostproto.ScanSession("3f5a9c")

// sessionStore holds, for prove and for a scan, one answered and one pending
// request of each session alone.
type sessionStore struct {
	store                       *hostprototest.MemoryHostStore
	proveAnswered, scanAnswered hostproto.Ticket
	provePending, scanPending   hostproto.Request
}

func newSessionStore(t *testing.T) sessionStore {
	t.Helper()
	store := newStore(t)
	request := func(label string, session hostproto.Session) hostproto.Request {
		r := hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = label })
		item := queueItem(r, "PAR-1~1", "validity")
		item.Sessions = []hostproto.Session{session}
		if err := store.Record(item); err != nil {
			t.Fatal(err)
		}
		return r
	}
	s := sessionStore{store: store}
	s.proveAnswered = emit(t, store, request("prove answered", hostproto.SessionProve), 0)
	answerAsExecutor(t, store, s.proveAnswered)
	s.scanAnswered = emit(t, store, request("scan answered", scanSession), 0)
	answerAsExecutorAgain(t, store, s.scanAnswered)
	s.provePending = request("prove pending", hostproto.SessionProve)
	emit(t, store, s.provePending, 0)
	s.scanPending = request("scan pending", scanSession)
	emit(t, store, s.scanPending, 0)
	return s
}

func TestSessionViews(t *testing.T) {
	t.Run("a push collects the answers of one session only [verifies SP-715~2] [verifies SP-2151~1]", func(t *testing.T) {
		s := newSessionStore(t)
		for session, want := range map[hostproto.Session]hostproto.Ticket{hostproto.SessionProve: s.proveAnswered, scanSession: s.scanAnswered} {
			bundle, err := hostsync.Collect(s.store, session, module, headSHA)
			if err != nil {
				t.Fatal(err)
			}
			if _, found := bundle.Lookup(want.Key()); len(bundle.Responses) != 1 || !found {
				t.Errorf("%s collected %d responses, its own found %v", session, len(bundle.Responses), found)
			}
		}
	})

	t.Run("the requests a session leaves for the host are exported for that session only [verifies SP-716~2] [verifies SP-2151~1]", func(t *testing.T) {
		s := newSessionStore(t)
		for session, want := range map[hostproto.Session]hostproto.Request{hostproto.SessionProve: s.provePending, scanSession: s.scanPending} {
			issued, err := hostsync.ExportIssued(s.store, session)
			if err != nil {
				t.Fatal(err)
			}
			if len(issued.Requests) != 1 || issued.Entries[0].RequestID != hostprototest.MustID(want) {
				t.Errorf("%s exported %+v", session, issued.Entries)
			}
		}
	})

	t.Run("imported requests belong to the session that imports them [verifies SP-716~2] [verifies SP-2151~1]", func(t *testing.T) {
		issued, err := hostsync.ExportIssued(newSessionStore(t).store, scanSession)
		if err != nil {
			t.Fatal(err)
		}
		developer := newStore(t)
		if _, err := hostsync.ImportIssued(developer, scanSession, issued, clock, &counter{}); err != nil {
			t.Fatal(err)
		}
		items, _ := developer.Items()
		if len(items) != 1 || len(items[0].Sessions) != 1 || items[0].Sessions[0] != scanSession {
			t.Fatalf("items %+v", items)
		}
		if again, _ := hostsync.ExportIssued(developer, hostproto.SessionProve); len(again.Requests) != 0 {
			t.Fatalf("prove sees the scan's imported requests: %+v", again.Entries)
		}
	})
}
