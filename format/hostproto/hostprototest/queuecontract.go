package hostprototest

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// Sessions the queue contract records requests for.
var (
	contractScan      = hostproto.ScanSession("contract")
	contractOtherScan = hostproto.ScanSession("contract-other")
)

const (
	contractProveWaves = 3
	contractPurpose    = "validity"
)

// RunQueueContract runs the contract of hostproto.Queue, sessions and waves per
// session included, against the adapter made by newStore. Every call of newStore
// must return an empty, independent store.
func RunQueueContract(t *testing.T, newStore func(t *testing.T) hostproto.HostStore) {
	t.Helper()
	contracts := []struct {
		name string
		run  func(t *testing.T, store hostproto.HostStore)
	}{
		{"an empty queue has no item, no session and wave zero", queueEmpty},
		{"the first record fixes the wave and later records merge criteria and sessions", queueMerge},
		{"a record with no session belongs to the prove session", queueDefaultSession},
		{"waves are counted per session", queueWavesPerSession},
		{"a session ends until it advances a wave again", queueEndSession},
		{"a withdrawn session leaves the others on the request", queueWithdraw},
	}
	for _, contract := range contracts {
		t.Run(contract.name, func(t *testing.T) { contract.run(t, newStore(t)) })
	}
}

func contractItem(id string, wave int, criteria []string, sessions ...hostproto.Session) hostproto.QueueItem {
	return hostproto.QueueItem{RequestID: hostproto.RequestID(id), Criteria: criteria, Sessions: sessions, Wave: wave, Purpose: contractPurpose}
}

func mustQueue(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func queueEmpty(t *testing.T, store hostproto.HostStore) {
	items, err := store.Items()
	mustQueue(t, err)
	sessions, err := store.Sessions()
	mustQueue(t, err)
	wave, err := store.Wave(hostproto.SessionProve)
	mustQueue(t, err)
	if items == nil || len(items) != 0 || len(sessions) != 0 || wave != 0 {
		t.Fatalf("items %v, sessions %v, wave %d", items, sessions, wave)
	}
}

func queueMerge(t *testing.T, store hostproto.HostStore) {
	mustQueue(t, store.Record(contractItem("b", 1, []string{"PAR-2~1"}, contractScan)))
	mustQueue(t, store.Record(contractItem("b", 2, []string{"PAR-1~1", "PAR-2~1"}, hostproto.SessionProve, contractScan)))
	mustQueue(t, store.Record(contractItem("a", 1, []string{"PAR-1~1"}, hostproto.SessionProve)))
	items, err := store.Items()
	mustQueue(t, err)
	if len(items) != 2 || items[0].RequestID != "a" || items[1].RequestID != "b" {
		t.Fatalf("items %+v", items)
	}
	merged := items[1]
	if merged.Wave != 1 || !slices.Equal(merged.Criteria, []string{"PAR-1~1", "PAR-2~1"}) ||
		!slices.Equal(merged.Sessions, []hostproto.Session{hostproto.SessionProve, contractScan}) {
		t.Fatalf("merged item %+v", merged)
	}
}

func queueDefaultSession(t *testing.T, store hostproto.HostStore) {
	mustQueue(t, store.Record(contractItem("a", 1, nil)))
	items, err := store.Items()
	mustQueue(t, err)
	if !slices.Equal(items[0].Sessions, []hostproto.Session{hostproto.SessionProve}) {
		t.Fatalf("sessions %v", items[0].Sessions)
	}
}

func queueWavesPerSession(t *testing.T, store hostproto.HostStore) {
	for want := 1; want <= contractProveWaves; want++ {
		got, err := store.AdvanceWave(hostproto.SessionProve)
		if err != nil || got != want {
			t.Fatalf("prove wave %d, %v; want %d", got, err, want)
		}
	}
	scanWave, err := store.AdvanceWave(contractScan)
	if err != nil || scanWave != 1 {
		t.Fatalf("first scan wave %d, %v", scanWave, err)
	}
	mustQueue(t, store.Record(contractItem("a", 1, nil, contractScan)))
	for session, want := range map[hostproto.Session]int{hostproto.SessionProve: contractProveWaves, contractScan: 1, contractOtherScan: 0} {
		if got, err := store.Wave(session); err != nil || got != want {
			t.Errorf("wave of %s = %d, %v; want %d", session, got, err, want)
		}
	}
}

func queueEndSession(t *testing.T, store hostproto.HostStore) {
	_, err := store.AdvanceWave(contractScan)
	mustQueue(t, err)
	mustQueue(t, store.Record(contractItem("a", 1, nil, contractOtherScan)))
	mustQueue(t, store.EndSession(contractScan))
	states, err := store.Sessions()
	mustQueue(t, err)
	liveness := hostproto.LivenessOf(states)
	if liveness.Live(contractScan) || !liveness.Live(contractOtherScan) || !liveness.Live(hostproto.SessionProve) {
		t.Fatalf("sessions %+v", states)
	}
	_, err = store.AdvanceWave(contractScan)
	mustQueue(t, err)
	states, err = store.Sessions()
	mustQueue(t, err)
	if !hostproto.LivenessOf(states).Live(contractScan) {
		t.Fatalf("a session that advanced a wave is still ended: %+v", states)
	}
}

func queueWithdraw(t *testing.T, store hostproto.HostStore) {
	mustQueue(t, store.Record(contractItem("a", 1, nil, hostproto.SessionProve, contractScan)))
	item, err := store.Withdraw("a", contractScan)
	if err != nil || !slices.Equal(item.Sessions, []hostproto.Session{hostproto.SessionProve}) {
		t.Fatalf("after the scan withdrew: %+v, %v", item, err)
	}
	if _, err := store.Withdraw("a", hostproto.SessionProve); err != nil {
		t.Fatal(err)
	}
	items, err := store.Items()
	mustQueue(t, err)
	if len(items[0].Sessions) != 0 {
		t.Fatalf("a request both sessions withdrew from is held by %v", items[0].Sessions)
	}
	if _, err := store.Withdraw("unknown", hostproto.SessionProve); !errors.Is(err, hostproto.ErrNotFound) {
		t.Fatalf("an unknown request: %v", err)
	}
}
