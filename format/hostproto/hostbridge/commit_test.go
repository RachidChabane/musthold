package hostbridge

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesSP1982 = " [verifies SP-1982~1]"

	committedDetail = "SP-7~1 1 3f5a"
	otherDetail     = "SP-7~1 2 9c1d"
)

func (r *rig) records() []hostproto.Record {
	r.t.Helper()
	records, err := r.store.Records()
	if err != nil {
		r.t.Fatal(err)
	}
	return records
}

func (r *rig) commit(kind hostproto.EventKind, detail string) bool {
	r.t.Helper()
	already, err := r.bridge.Commit(kind, detail)
	if err != nil {
		r.t.Fatal(err)
	}
	return already
}

func TestCommit(t *testing.T) {
	t.Run("an event about no ticket is journaled with its kind and detail"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		if subject.commit(hostproto.EventWitnessSetCommitted, committedDetail) {
			t.Fatal("the first commit reports the event as journaled before")
		}
		records := subject.records()
		if len(records) != 1 {
			t.Fatalf("%d records", len(records))
		}
		event := records[0].Event
		if event.Kind != hostproto.EventWitnessSetCommitted || event.Detail != committedDetail || event.Ticket != "" {
			t.Errorf("event = %+v", event)
		}
	})
	t.Run("the same event again appends nothing and reports it was journaled"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		subject.commit(hostproto.EventWitnessSetCommitted, committedDetail)
		if !subject.commit(hostproto.EventWitnessSetCommitted, committedDetail) {
			t.Error("the second commit does not report the event as journaled before")
		}
		if got := len(subject.records()); got != 1 {
			t.Errorf("%d records", got)
		}
	})
	t.Run("a different detail appends another event"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		subject.commit(hostproto.EventWitnessSetCommitted, committedDetail)
		if subject.commit(hostproto.EventWitnessSetCommitted, otherDetail) {
			t.Error("a new detail reports as journaled before")
		}
		if got := len(subject.records()); got != 2 {
			t.Errorf("%d records", got)
		}
	})
	t.Run("a different kind with the same detail appends another event"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		subject.commit(hostproto.EventWitnessSetCommitted, committedDetail)
		if subject.commit(hostproto.EventWitnessConsumed, committedDetail) {
			t.Error("a new kind reports as journaled before")
		}
		if got := len(subject.records()); got != 2 {
			t.Errorf("%d records", got)
		}
	})
	t.Run("an event journaled by another bridge of the same journal is found"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		subject.commit(hostproto.EventWitnessSetCommitted, committedDetail)
		next := subject.bridgeOnWave(2)
		already, err := next.Commit(hostproto.EventWitnessSetCommitted, committedDetail)
		if err != nil || !already {
			t.Errorf("already = %v, err = %v", already, err)
		}
		if got := len(subject.records()); got != 1 {
			t.Errorf("%d records", got)
		}
	})
	t.Run("a committed event leaves the journal a valid chain"+verifiesSP1982, func(t *testing.T) {
		subject := newRig(t)
		subject.commit(hostproto.EventWitnessSetCommitted, committedDetail)
		subject.commit(hostproto.EventWitnessSetCommitted, otherDetail)
		if err := hostproto.VerifyChain(subject.records()); err != nil {
			t.Error(err)
		}
	})
	t.Run("any run of commits journals each distinct event exactly once"+verifiesSP1982, func(t *testing.T) {
		outer := t
		rapid.Check(t, func(t *rapid.T) {
			subject := newRig(outer)
			details := rapid.SliceOfN(rapid.StringMatching(`[a-f0-9]{1,4}`), 0, 20).Draw(t, "details")
			distinct := map[string]bool{}
			for _, detail := range details {
				already := subject.commit(hostproto.EventWitnessSetCommitted, detail)
				if already != distinct[detail] {
					t.Fatalf("detail %q: already = %v", detail, already)
				}
				distinct[detail] = true
			}
			if got := len(subject.records()); got != len(distinct) {
				t.Fatalf("%d records for %d distinct events", got, len(distinct))
			}
		})
	})
}
