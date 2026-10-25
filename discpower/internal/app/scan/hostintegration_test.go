package scan_test

import (
	"bytes"
	"context"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hoststore"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/localcache"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/promise"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
)

const (
	verifiesMemoLoss   = " [verifies SP-2241~1]"
	verifiesConcurrent = " [verifies SP-2242~1]"
	verifiesReplayHost = " [verifies SP-2256~1]"
	verifiesReplayFull = " [verifies SP-2257~1]"
	verifiesReplayDeps = " [verifies SP-2258~1]"
)

// diskMemo is the module's memo on a directory a test can delete.
type diskMemo struct {
	dir  string
	memo *localcache.Memo
}

func newDiskMemo(t *testing.T) diskMemo {
	t.Helper()
	dir := t.TempDir()
	return diskMemo{dir: dir, memo: localcache.NewMemo(dir)}
}

// withDiskMemo makes the fixture keep its witness facts on disk.
func (f *hostFixture) withDiskMemo(memo diskMemo) *hostFixture {
	f.diskMemo = memo.memo
	return f
}

// toAdmission runs the first three invocations, answering each.
func (f *hostFixture) toAdmission(t *testing.T, req scan.Request) scan.Request {
	t.Helper()
	for range 3 {
		result := f.invoke(t, req)
		if result.Finished {
			t.Fatalf("finished early: %+v", result)
		}
		f.host.answer(t, result.Session)
		req.Resume = true
	}
	return req
}

func TestHostMemoLoss(t *testing.T) {
	t.Run("losing the memo with a stable reference costs nothing: the scan finishes without a new emission"+verifiesMemoLoss, func(t *testing.T) {
		memo := newDiskMemo(t)
		f := newHostFixture().withDiskMemo(memo)
		req := f.toAdmission(t, hostRequest())
		if err := os.RemoveAll(memo.dir); err != nil {
			t.Fatal(err)
		}
		before := len(f.allEmissions(t))
		final := f.invoke(t, req)
		if !final.Finished || len(f.allEmissions(t)) != before {
			t.Fatalf("finished %t, emissions %d → %d", final.Finished, before, len(f.allEmissions(t)))
		}
	})
	t.Run("losing the memo while the reference flakes re-asks what the new admission changes, and the scan still finishes"+verifiesMemoLoss, func(t *testing.T) {
		memo := newDiskMemo(t)
		f := newHostFixture().withDiskMemo(memo)
		req := f.toAdmission(t, hostRequest())
		if err := os.RemoveAll(memo.dir); err != nil {
			t.Fatal(err)
		}
		f.runs.failRuns["test_va_free"] = []int{4, 5, 6}
		before := len(f.allEmissions(t))
		again := f.invoke(t, req)
		if again.Finished || len(again.Pending) == 0 {
			t.Fatalf("result %+v", again)
		}
		for _, kind := range pendingKinds(t, f.store, again) {
			if kind == hostproto.KindGeneratePromises || kind == hostproto.KindGenerateWitness {
				t.Fatalf("losing the memo re-asked a %s", kind)
			}
		}
		f.host.answer(t, again.Session)
		final := f.invoke(t, req)
		if !final.Finished {
			t.Fatalf("result %+v", final)
		}
		if _, err := format.AdmitScan(final.Report.Encoded); err != nil {
			t.Fatal(err)
		}
		if added := len(f.allEmissions(t)) - before; added != len(again.Pending) || final.Report.Promises.Calls > promise.Budget {
			t.Fatalf("%d emissions added for %d re-asks", added, len(again.Pending))
		}
	})
}

// allEmissions lists the emitted tickets the journal records.
func (f *hostFixture) allEmissions(t *testing.T) []string {
	t.Helper()
	records, err := f.store.Records()
	if err != nil {
		t.Fatal(err)
	}
	var tickets []string
	for _, record := range records {
		if record.Event.Kind == hostproto.EventEmitted {
			tickets = append(tickets, record.Event.Ticket)
		}
	}
	return tickets
}

func TestHostSharesTheFileStoreWithProve(t *testing.T) {
	t.Run("a file host store holds a prove session and a scan session side by side: neither retires the other's tickets"+verifiesConcurrent, func(t *testing.T) {
		store, err := hoststore.Open(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		f := newHostFixture()
		f.store = store
		f.host = newFakeHost(store, f.clock)
		prove := hostprototestRequest(t)
		proveID := mustID(t, prove.Request)
		var wait sync.WaitGroup
		failures := make(chan error, 1)
		stop := make(chan struct{})
		wait.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if err := proveWave(store, f.clock.read, prove); err != nil {
					select {
					case failures <- err:
					default:
					}
					return
				}
			}
		})
		results := f.loop(t, hostRequest(), 6)
		close(stop)
		wait.Wait()
		select {
		case err := <-failures:
			t.Fatal(err)
		default:
		}
		if !results[len(results)-1].Finished {
			t.Fatal("the scan did not finish")
		}
		entry, err := store.GetEntry(hostproto.Key{RequestID: proveID, Vote: 0})
		if err != nil || entry.State != hostproto.StateEmitted {
			t.Fatalf("the prove ticket is %+v, %v", entry, err)
		}
		pending := provePending(t, store)
		if !slices.Contains(pending, proveID) || len(pending) != 1 {
			t.Fatalf("prove waits for %v", pending)
		}
	})
}

// proveWave is one wave of a prove session that wants one request.
func proveWave(store hostproto.HostStore, clock func() time.Time, ask hostbridge.Ask) error {
	wave, err := store.AdvanceWave(hostproto.SessionProve)
	if err != nil {
		return err
	}
	bridge := hostbridge.New(store, clock, bytes.NewReader(bytes.Repeat([]byte{9}, 1<<12)), hostproto.SessionProve, wave)
	if _, err := bridge.Want(ask); err != nil {
		return err
	}
	_, err = bridge.FinishWave()
	return err
}

// provePending lists the requests of the prove session's waiting tickets.
func provePending(t *testing.T, store hostproto.HostStore) []hostproto.RequestID {
	t.Helper()
	wave, err := store.Wave(hostproto.SessionProve)
	if err != nil {
		t.Fatal(err)
	}
	bridge := hostbridge.New(store, nil, nil, hostproto.SessionProve, wave)
	tickets, err := bridge.PendingTickets()
	if err != nil {
		t.Fatal(err)
	}
	var ids []hostproto.RequestID
	for _, ticket := range tickets {
		ids = append(ids, ticket.RequestID)
	}
	return ids
}

func TestHostReplay(t *testing.T) {
	f := newHostFixture()
	results := f.loop(t, hostRequest(), 5)
	report := results[len(results)-1].Report
	if report.Promises == nil || len(report.Promises.Broken) == 0 {
		t.Fatalf("no broken promise to replay: %+v", report.Promises)
	}
	broken := report.Promises.Broken[0]
	f.store, f.memo = nil, nil
	replayer := f.replayService(t)
	t.Run("a broken promise replays in full with no host store and no memo"+verifiesReplayFull, func(t *testing.T) {
		replay, err := replayer.Replay(context.Background(), "/work/invoicing", scan.BrokenRecordOf(report, broken))
		if err != nil {
			t.Fatal(err)
		}
		if !replay.Held() || len(replay.Checks) != 4 {
			t.Fatalf("replay %+v", replay)
		}
	})
	t.Run("a witness_failure route replays: the witness passes the reference and fails the mutant three runs out of three"+verifiesReplayHost, func(t *testing.T) {
		record := scan.FindingRecord{
			Commit: report.Commit, Runner: string(report.Runner), Test: vacuousTest, Mutant: broken.Mutant,
			Route: broken.Killer.Route, Witnesses: broken.File, Promises: broken.Spec,
		}
		replay, err := replayer.Replay(context.Background(), "/work/invoicing", record)
		if err != nil {
			t.Fatal(err)
		}
		if !replay.Held() {
			t.Fatalf("replay %+v", replay)
		}
	})
	t.Run("a replay's ports hold no host store and no memo"+verifiesReplayDeps, func(t *testing.T) {
		if deps := f.deps(t, false); deps.Host != nil {
			t.Fatalf("replay deps %+v", deps.Host)
		}
	})
	t.Run("a witness that fails a replayed run on the reference leaves the broken promise not held"+verifiesReplayHost, func(t *testing.T) {
		record := scan.BrokenRecordOf(report, broken)
		f.runs.failRuns[broken.Witness.Name] = []int{f.runs.onSource[broken.Witness.Name] + 1}
		replay, err := replayer.Replay(context.Background(), "/work/invoicing", record)
		if err != nil {
			t.Fatal(err)
		}
		if replay.Held() {
			t.Fatalf("replay held with a witness that failed the reference: %+v", replay)
		}
	})
}
