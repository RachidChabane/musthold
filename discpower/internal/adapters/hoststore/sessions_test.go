package hoststore

import (
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	verifiesConcurrentSessions = " [verifies SP-2152~1]"

	ownRequests    = 3
	sharedRequests = 2
	oneVote        = 1
	sessionPurpose = "judge"
)

var concurrentScan = hostproto.ScanSession("3f5a9c")

// counterEntropy is an entropy source whose bytes never repeat within a run.
type counterEntropy struct {
	mutex sync.Mutex
	next  byte
	seed  byte
}

func (c *counterEntropy) Read(p []byte) (int, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	for index := range p {
		c.next++
		p[index] = c.next ^ c.seed
	}
	return len(p), nil
}

func sessionRequest(label string) hostproto.Request {
	return hostprototest.Request(func(r *hostproto.Request) { r.Prompt.User = label })
}

// sessionRun is one run of a session over its own store of the shared directory,
// as a separate process holds it.
type sessionRun struct {
	t       *testing.T
	session hostproto.Session
	bridge  *hostbridge.Bridge
}

func startSession(t *testing.T, dir string, session hostproto.Session, seed byte) sessionRun {
	t.Helper()
	store := mustOpen(t, dir)
	wave, err := store.AdvanceWave(session)
	if err != nil {
		t.Fatal(err)
	}
	clock := func() time.Time { return hostprototest.Epoch }
	return sessionRun{t: t, session: session, bridge: hostbridge.New(store, clock, &counterEntropy{seed: seed}, session, wave)}
}

func (r sessionRun) want(labels ...string) error {
	for _, label := range labels {
		if _, err := r.bridge.Want(hostbridge.Ask{Request: sessionRequest(label), Votes: oneVote, Purpose: sessionPurpose}); err != nil {
			return fmt.Errorf("%s wants %s: %w", r.session, label, err)
		}
	}
	return nil
}

func (r sessionRun) pending() []hostproto.RequestID {
	r.t.Helper()
	tickets, err := r.bridge.PendingTickets()
	if err != nil {
		r.t.Fatal(err)
	}
	ids := make([]hostproto.RequestID, len(tickets))
	for index, ticket := range tickets {
		ids[index] = ticket.RequestID
	}
	return ids
}

func idsOf(labels ...string) []hostproto.RequestID {
	ids := make([]hostproto.RequestID, len(labels))
	for index, label := range labels {
		ids[index] = hostprototest.MustID(sessionRequest(label))
	}
	slices.Sort(ids)
	return ids
}

func labels(prefix string, count int) []string {
	out := make([]string, count)
	for index := range out {
		out[index] = fmt.Sprintf("%s %d", prefix, index)
	}
	return out
}

func TestConcurrentSessions(t *testing.T) {
	t.Run("prove and a scan share the filesystem store without retiring each other's tickets"+verifiesConcurrentSessions, func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "host")
		proveOwn, scanOwn, shared := labels("prove", ownRequests), labels("scan", ownRequests), labels("shared", sharedRequests)
		prove := startSession(t, dir, hostproto.SessionProve, 0)
		scan := startSession(t, dir, concurrentScan, 0x80)

		var group sync.WaitGroup
		errs := make([]error, 2)
		for index, run := range []struct {
			session sessionRun
			wanted  []string
		}{{prove, append(slices.Clone(proveOwn), shared...)}, {scan, append(slices.Clone(scanOwn), shared...)}} {
			group.Add(1)
			go func() {
				defer group.Done()
				if errs[index] = run.session.want(run.wanted...); errs[index] == nil {
					errs[index] = run.session.bridge.SupersedeUnwanted()
				}
			}()
		}
		group.Wait()
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}

		entries, err := mustOpen(t, dir).ListEntries()
		mustNoError(t, err)
		if len(entries) != 2*ownRequests+sharedRequests {
			t.Fatalf("%d tickets for %d requests", len(entries), 2*ownRequests+sharedRequests)
		}
		if got, want := prove.pending(), idsOf(append(slices.Clone(proveOwn), shared...)...); !slices.Equal(got, want) {
			t.Fatalf("prove pending %v, want %v", got, want)
		}
		if got, want := scan.pending(), idsOf(append(slices.Clone(scanOwn), shared...)...); !slices.Equal(got, want) {
			t.Fatalf("scan pending %v, want %v", got, want)
		}

		nextScan := startSession(t, dir, concurrentScan, 0x40)
		mustNoError(t, nextScan.bridge.SupersedeUnwanted())
		if got := nextScan.pending(); len(got) != 0 {
			t.Fatalf("the scan that wants nothing still has %v", got)
		}
		if got, want := prove.pending(), idsOf(append(slices.Clone(proveOwn), shared...)...); !slices.Equal(got, want) {
			t.Fatalf("after the scan's wave prove has %v, want %v", got, want)
		}

		mustNoError(t, nextScan.bridge.End())
		reopened := startSession(t, dir, hostproto.SessionProve, 0x20)
		mustNoError(t, reopened.want(append(slices.Clone(proveOwn), shared...)...))
		if got, want := reopened.pending(), idsOf(append(slices.Clone(proveOwn), shared...)...); !slices.Equal(got, want) {
			t.Fatalf("prove after the scan ended has %v, want %v", got, want)
		}
		if reopened.bridge.Wave != 2 {
			t.Fatalf("prove's second wave is %d", reopened.bridge.Wave)
		}
	})
}
