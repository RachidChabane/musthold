package scan_test

import (
	"context"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/scan"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const verifiesScanSteps = " [verifies SP-2676~1]"

// spentScanContext is the context of a step whose budget already ran out.
func spentScanContext() context.Context {
	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(hostproto.ErrStepSpent)
	return ctx
}

func TestScanSteps(t *testing.T) {
	t.Run("a scan step whose budget ran out ends unfinished, keeps the tickets and the next step continues"+verifiesScanSteps, func(t *testing.T) {
		f := newHostFixture()
		first := f.invoke(t, hostRequest())
		resumed := hostRequest()
		resumed.Resume = true
		cut, err := f.hostService(t).RunHost(spentScanContext(), resumed)
		if err != nil {
			t.Fatal(err)
		}
		if cut.Finished || cut.Waiting.Step != hostproto.StepBudgetSpent || cut.ExitCode() != scan.ExitAwaitingHost {
			t.Fatalf("cut step %+v", cut)
		}
		again := f.invoke(t, resumed)
		if again.Waiting.Step.Unfinished() || len(again.Waiting.Launchable) != len(first.Waiting.Launchable) || again.Wave != first.Wave {
			t.Fatalf("first %+v, again %+v", first, again)
		}
	})

	t.Run("a scan step whose session another run holds changes nothing and ends unfinished"+verifiesScanSteps, func(t *testing.T) {
		f := newHostFixture()
		first := f.invoke(t, hostRequest())
		release, err := f.store.LockRun(first.Session)
		if err != nil {
			t.Fatal(err)
		}
		resumed := hostRequest()
		resumed.Resume = true
		busy, err := f.hostService(t).RunHost(context.Background(), resumed)
		release()
		if err != nil || busy.Waiting.Step != hostproto.StepSessionBusy || busy.Finished {
			t.Fatalf("busy step %+v, %v", busy, err)
		}
	})

	t.Run("a scan step without an answer since its wave opened continues that wave"+verifiesScanSteps, func(t *testing.T) {
		f := newHostFixture()
		first := f.invoke(t, hostRequest())
		resumed := hostRequest()
		resumed.Resume = true
		if again := f.invoke(t, resumed); again.Wave != first.Wave {
			t.Fatalf("wave %d after %d with no answer", again.Wave, first.Wave)
		}
		f.host.answer(t, first.Session)
		if next := f.invoke(t, resumed); next.Wave != first.Wave+1 {
			t.Fatalf("wave %d after an answer, want %d", next.Wave, first.Wave+1)
		}
	})
}
