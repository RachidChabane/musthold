package hostproto_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesQueueSessions = " [verifies SP-2147~1]"
	verifiesSessionViews  = " [verifies SP-2151~1]"

	scanKey      = "3f5a9c"
	otherScanKey = "9c1d07"
	legacyWave   = 4
)

var (
	scanSession      = hostproto.ScanSession(scanKey)
	otherScanSession = hostproto.ScanSession(otherScanKey)
)

func queueItem(id string, sessions ...hostproto.Session) hostproto.QueueItem {
	return hostproto.QueueItem{RequestID: hostproto.RequestID(id), Criteria: []string{"SP-1~1"}, Sessions: sessions, Wave: 1, Purpose: "judge"}
}

func TestSessions(t *testing.T) {
	t.Run("a session is prove or scan with a key, and nothing else"+verifiesQueueSessions, func(t *testing.T) {
		for _, text := range []string{"prove", "scan:" + scanKey, "scan:a.b_c-1"} {
			if _, err := hostproto.ParseSession(text); err != nil {
				t.Errorf("%q refused: %v", text, err)
			}
		}
		for _, text := range []string{"", "scan:", "scan", "Prove", "scan:a b", "scan:a/b", "ci"} {
			if _, err := hostproto.ParseSession(text); !errors.Is(err, hostproto.ErrBadSession) {
				t.Errorf("%q accepted: %v", text, err)
			}
		}
		if !scanSession.IsScan() || hostproto.SessionProve.IsScan() {
			t.Fatal("IsScan tells the sessions apart wrongly")
		}
	})

	t.Run("an empty filter text selects every session, any other one session"+verifiesSessionViews, func(t *testing.T) {
		every, err := hostproto.ParseSessionFilter("")
		if err != nil || every != hostproto.EverySession || !every.Selects(nil) {
			t.Fatalf("empty filter %+v, %v", every, err)
		}
		prove, err := hostproto.ParseSessionFilter(string(hostproto.SessionProve))
		if err != nil || !prove.Selects([]hostproto.Session{hostproto.SessionProve, scanSession}) || prove.Selects([]hostproto.Session{scanSession}) || prove.Selects(nil) {
			t.Fatalf("prove filter %+v, %v", prove, err)
		}
		if _, err := hostproto.ParseSessionFilter("scan"); !errors.Is(err, hostproto.ErrBadSession) {
			t.Fatalf("a bad filter: %v", err)
		}
	})

	t.Run("a request the queue has no item for belongs to the prove session"+verifiesSessionViews, func(t *testing.T) {
		index := hostproto.IndexSessions([]hostproto.QueueItem{queueItem("a", scanSession), {RequestID: "b", Sessions: []hostproto.Session{}}})
		if !slices.Equal(index.Of("unknown"), []hostproto.Session{hostproto.SessionProve}) {
			t.Fatalf("unknown request: %v", index.Of("unknown"))
		}
		if index.Selects(hostproto.OnlySession(hostproto.SessionProve), "a") || !index.Selects(hostproto.OnlySession(scanSession), "a") {
			t.Fatal("the scan item is selected wrongly")
		}
		if index.Selects(hostproto.OnlySession(hostproto.SessionProve), "b") || !index.Selects(hostproto.EverySession, "b") {
			t.Fatal("an item no session holds is selected wrongly")
		}
	})
}

func TestQueueState(t *testing.T) {
	t.Run("the sessions of a request merge to their sorted union, and an item recorded with none is the prove session's"+verifiesQueueSessions, func(t *testing.T) {
		var queue hostproto.QueueState
		queue.Record(queueItem("a", scanSession))
		queue.Record(queueItem("a", hostproto.SessionProve, scanSession))
		queue.Record(queueItem("b"))
		items := queue.Items()
		if !slices.Equal(items[0].Sessions, []hostproto.Session{hostproto.SessionProve, scanSession}) {
			t.Fatalf("merged sessions %v", items[0].Sessions)
		}
		if !slices.Equal(items[1].Sessions, []hostproto.Session{hostproto.SessionProve}) {
			t.Fatalf("an item with no session: %v", items[1].Sessions)
		}
	})

	t.Run("waves are counted per session and advancing one leaves the others"+verifiesQueueSessions, func(t *testing.T) {
		var queue hostproto.QueueState
		for want := 1; want <= 3; want++ {
			if got := queue.AdvanceWave(hostproto.SessionProve); got != want {
				t.Fatalf("prove wave %d, want %d", got, want)
			}
		}
		if got := queue.AdvanceWave(scanSession); got != 1 {
			t.Fatalf("first scan wave %d", got)
		}
		if queue.Wave(hostproto.SessionProve) != 3 || queue.Wave(scanSession) != 1 || queue.Wave(otherScanSession) != 0 {
			t.Fatalf("waves %d %d %d", queue.Wave(hostproto.SessionProve), queue.Wave(scanSession), queue.Wave(otherScanSession))
		}
	})

	t.Run("a session ends until it advances a wave again, and a session only an item names is live"+verifiesQueueSessions, func(t *testing.T) {
		var queue hostproto.QueueState
		queue.Record(queueItem("a", otherScanSession))
		queue.AdvanceWave(scanSession)
		queue.EndSession(scanSession)
		liveness := hostproto.LivenessOf(queue.Sessions())
		if liveness.Live(scanSession) || !liveness.Live(otherScanSession) || !liveness.Live(hostproto.SessionProve) {
			t.Fatalf("sessions %+v", queue.Sessions())
		}
		queue.AdvanceWave(scanSession)
		if !hostproto.LivenessOf(queue.Sessions()).Live(scanSession) || queue.Wave(scanSession) != 2 {
			t.Fatalf("after a new wave: %+v", queue.Sessions())
		}
		var listed []hostproto.Session
		for _, state := range queue.Sessions() {
			listed = append(listed, state.Session)
		}
		if !slices.IsSorted(listed) || !slices.Equal(listed, []hostproto.Session{scanSession, otherScanSession}) {
			t.Fatalf("listed sessions %v", listed)
		}
	})

	t.Run("withdrawing a session leaves the others and can leave a request no session holds"+verifiesQueueSessions, func(t *testing.T) {
		var queue hostproto.QueueState
		queue.Record(queueItem("a", hostproto.SessionProve, scanSession))
		item, err := queue.Withdraw("a", scanSession)
		if err != nil || !slices.Equal(item.Sessions, []hostproto.Session{hostproto.SessionProve}) {
			t.Fatalf("after the scan withdrew: %+v, %v", item, err)
		}
		item, err = queue.Withdraw("a", hostproto.SessionProve)
		if err != nil || item.Sessions == nil || len(item.Sessions) != 0 {
			t.Fatalf("after prove withdrew: %+v, %v", item, err)
		}
		if _, err := queue.Withdraw("unknown", hostproto.SessionProve); !errors.Is(err, hostproto.ErrNotFound) {
			t.Fatalf("an unknown request: %v", err)
		}
		queue.Record(queueItem("a", scanSession))
		if got := queue.Items()[0].Sessions; !slices.Equal(got, []hostproto.Session{scanSession}) {
			t.Fatalf("wanted again: %v", got)
		}
	})

	t.Run("a queue survives its JSON form, with a request no session holds kept as such"+verifiesQueueSessions, func(t *testing.T) {
		var queue hostproto.QueueState
		queue.Record(queueItem("a", scanSession))
		queue.Record(queueItem("b", hostproto.SessionProve))
		queue.AdvanceWave(scanSession)
		queue.EndSession(scanSession)
		if _, err := queue.Withdraw("a", scanSession); err != nil {
			t.Fatal(err)
		}
		var reread hostproto.QueueState
		roundTrip(t, queue, &reread)
		if !slices.EqualFunc(reread.Items(), queue.Items(), sameItem) || !slices.Equal(reread.Sessions(), queue.Sessions()) {
			t.Fatalf("reread %+v %+v, want %+v %+v", reread.Items(), reread.Sessions(), queue.Items(), queue.Sessions())
		}
		if len(reread.Items()[0].Sessions) != 0 {
			t.Fatalf("an item no session holds came back as %v", reread.Items()[0].Sessions)
		}
	})

	t.Run("a queue written without sessions is the prove session's: its wave and every item"+verifiesQueueSessions, func(t *testing.T) {
		legacy := `{"wave":4,"items":[{"requestId":"b","criteria":["SP-2~1","SP-1~1"],"wave":2,"purpose":"judge"},{"requestId":"a","criteria":null,"wave":1,"purpose":"validity"}]}`
		var queue hostproto.QueueState
		if err := json.Unmarshal([]byte(legacy), &queue); err != nil {
			t.Fatal(err)
		}
		if queue.Wave(hostproto.SessionProve) != legacyWave || queue.Wave(scanSession) != 0 {
			t.Fatalf("waves %d, %d", queue.Wave(hostproto.SessionProve), queue.Wave(scanSession))
		}
		items := queue.Items()
		if len(items) != 2 || items[0].RequestID != "a" || items[1].Wave != 2 || !slices.Equal(items[1].Criteria, []string{"SP-1~1", "SP-2~1"}) {
			t.Fatalf("items %+v", items)
		}
		for _, item := range items {
			if !slices.Equal(item.Sessions, []hostproto.Session{hostproto.SessionProve}) {
				t.Fatalf("%s belongs to %v", item.RequestID, item.Sessions)
			}
		}
		if got := queue.AdvanceWave(hostproto.SessionProve); got != legacyWave+1 {
			t.Fatalf("the next prove wave is %d", got)
		}
	})

	t.Run("the sessions of any records merge to their sorted union"+verifiesQueueSessions, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			pool := []hostproto.Session{hostproto.SessionProve, scanSession, otherScanSession}
			batches := rapid.SliceOfN(rapid.SliceOfN(rapid.SampledFrom(pool), 1, 3), 1, 4).Draw(t, "batches")
			var queue hostproto.QueueState
			var union []hostproto.Session
			for _, batch := range batches {
				union = append(union, batch...)
				queue.Record(queueItem("a", batch...))
			}
			slices.Sort(union)
			if got := queue.Items()[0].Sessions; !slices.Equal(got, slices.Compact(union)) {
				t.Fatalf("sessions %v, want %v", got, slices.Compact(union))
			}
		})
	})
}

func sameItem(a, b hostproto.QueueItem) bool {
	return a.RequestID == b.RequestID && a.Wave == b.Wave && a.Purpose == b.Purpose &&
		slices.Equal(a.Criteria, b.Criteria) && slices.Equal(a.Sessions, b.Sessions)
}

func roundTrip(t *testing.T, queue hostproto.QueueState, into *hostproto.QueueState) {
	t.Helper()
	encoded, err := json.Marshal(queue)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, into); err != nil {
		t.Fatal(err)
	}
}
