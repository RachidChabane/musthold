package hostbridge

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
)

const (
	verifiesRenewal     = " [verifies SP-2660~1]"
	verifiesOrphans     = " [verifies SP-2662~1]"
	verifiesWaves       = " [verifies SP-2663~1]"
	verifiesOneQuery    = " [verifies SP-2661~1]"
	stoppedAgent        = "agent-stopped"
	validOutput         = `{"validity":"VALID"}`
	maxDrawnWaves       = 8
	drawnRequestsPerRun = 4
)

// stop journals what the stop hook of a harness journals when the executor that
// fetched the ticket ends its run without answering.
func (r *rig) stop(ticket hostproto.TicketID) {
	r.t.Helper()
	if _, err := r.store.Append(hostproto.Event{Kind: hostproto.EventLeaked, Ticket: ticket.String(), AgentID: stoppedAgent}); err != nil {
		r.t.Fatal(err)
	}
}

// journalAccepted journals the acceptance of an answer, as the command line and
// the executor server do once they stored it.
func (r *rig) journalAccepted(ticket hostproto.TicketID) {
	r.t.Helper()
	if _, err := r.store.Append(hostproto.Event{Kind: hostproto.EventAccepted, Ticket: ticket.String()}); err != nil {
		r.t.Fatal(err)
	}
}

// opened opens the next pass of the prove session as a run does.
func (r *rig) opened() *Bridge {
	r.t.Helper()
	bridge, err := Open(r.store, r.clock.Now, &sequenceReader{next: uint64(len(r.records())) * waveSeedStride}, hostproto.SessionProve)
	if err != nil {
		r.t.Fatal(err)
	}
	return bridge
}

func (r *rig) abandon(request hostproto.Request) Answer {
	r.t.Helper()
	answer := r.want(oneVote, request)
	for range hostproto.DefaultMaxEmissions {
		r.failSchemaTwice(answer.Votes[0].Ticket)
		answer = r.want(oneVote, request)
	}
	if answer.Standing != Unresponsive {
		r.t.Fatalf("not abandoned: %+v", answer)
	}
	return answer
}

func TestOrphans(t *testing.T) {
	t.Run("a fetched ticket whose executor's run ended is reissued at once as a host failure, long before its time to live"+verifiesOrphans, func(t *testing.T) {
		r := newRig(t)
		request := variant("orphan")
		ticket := r.want(oneVote, request).Votes[0].Ticket
		r.fetch(ticket)
		if again := r.want(oneVote, request); again.Votes[0].State != hostproto.StateFetched {
			t.Fatalf("a live fetch was retired: %+v", again.Votes[0])
		}
		r.stop(ticket)
		reissued := r.want(oneVote, request)
		if reissued.Votes[0].State != hostproto.StateEmitted || reissued.Votes[0].Ticket.Emission != ticket.Emission+1 || reissued.Standing != Pending {
			t.Fatalf("the orphan was not reissued: %+v", reissued.Votes[0])
		}
	})

	t.Run("closing a wave retires the orphaned tickets of the requests the wave did not ask for"+verifiesOrphans, func(t *testing.T) {
		r := newRig(t)
		ticket := r.want(oneVote, variant("left behind")).Votes[0].Ticket
		r.fetch(ticket)
		r.stop(ticket)
		next := r.bridgeOnWave(firstWave + 1)
		outstanding, err := next.FinishWave()
		if err != nil {
			t.Fatal(err)
		}
		if outstanding.Waiting() != 0 || r.entry(ticket).State != hostproto.StateExpired {
			t.Fatalf("outstanding %+v, entry %s", outstanding, r.entry(ticket).State)
		}
	})

	t.Run("a ticket fetched from the command line, which no stop event follows, waits for its time to live"+verifiesOrphans, func(t *testing.T) {
		r := newRig(t)
		request := variant("command line")
		ticket := r.want(oneVote, request).Votes[0].Ticket
		r.fetch(ticket)
		if _, err := r.store.Append(hostproto.Event{Kind: hostproto.EventRunReported, Ticket: ticket.String()}); err != nil {
			t.Fatal(err)
		}
		r.clock.Advance(justBeforeFetch)
		if waiting := r.want(oneVote, request); waiting.Votes[0].State != hostproto.StateFetched {
			t.Fatalf("retired before its time to live: %+v", waiting.Votes[0])
		}
	})
}

func TestRenewal(t *testing.T) {
	t.Run("a new run renews an abandoned ticket with a fresh count, a resumed one leaves it abandoned"+verifiesRenewal, func(t *testing.T) {
		r := newRig(t)
		request := variant("renewed")
		abandoned := r.abandon(request).Votes[0].Ticket
		if resumed := r.want(oneVote, request); resumed.Standing != Unresponsive {
			t.Fatalf("a resumed run renewed the ticket: %+v", resumed)
		}
		r.bridge.Renew = true
		renewed := r.want(oneVote, request)
		vote := renewed.Votes[0]
		if renewed.Standing != Pending || vote.State != hostproto.StateEmitted || vote.Ticket.Emission != abandoned.Emission+1 {
			t.Fatalf("renewed as %+v", renewed)
		}
		if entry := r.entry(vote.Ticket); entry.Exhausted() || entry.Reason != "" {
			t.Fatalf("the renewed entry %+v", entry)
		}
		r.answer(vote.Ticket, validOutput)
		if answered := r.want(oneVote, request); answered.Standing != Answered {
			t.Fatalf("the renewed ticket's answer: %+v", answered)
		}
	})
}

func TestWavesAdvanceOnAnswers(t *testing.T) {
	t.Run("passes without an answer continue the open wave, an answer opens the next one"+verifiesWaves, func(t *testing.T) {
		r := newRig(t)
		request := variant("waves")
		first := r.opened()
		ticket := r.wantOn(first, oneVote, request).Votes[0].Ticket
		if _, err := first.FinishWave(); err != nil {
			t.Fatal(err)
		}
		for range 3 {
			if again := r.opened(); again.Wave != first.Wave {
				t.Fatalf("a pass without an answer opened wave %d after %d", again.Wave, first.Wave)
			}
		}
		r.answer(ticket, validOutput)
		r.journalAccepted(ticket)
		if r.opened().Wave != first.Wave+1 {
			t.Fatal("an answer did not open the next wave")
		}
	})

	t.Run("over any interleaving of passes and answers, the wave number counts the passes that saw a new answer or followed the session's end"+verifiesWaves, func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			r := newRig(t)
			wave, answeredSinceOpen := 0, true
			for step := range rapid.IntRange(1, maxDrawnWaves).Draw(rt, "steps") {
				bridge := r.opened()
				if answeredSinceOpen {
					wave++
					answeredSinceOpen = false
				}
				if bridge.Wave != wave {
					rt.Fatalf("step %d: wave %d, want %d", step, bridge.Wave, wave)
				}
				answer := r.wantOn(bridge, oneVote, variant(string(rune('a'+rapid.IntRange(0, drawnRequestsPerRun).Draw(rt, "request")))))
				outstanding, err := bridge.FinishWave()
				if err != nil {
					rt.Fatal(err)
				}
				// A wave that leaves nothing waiting ends the session, whose next pass opens a wave.
				answeredSinceOpen = outstanding.Waiting() == 0
				if rapid.Bool().Draw(rt, "answer") && answer.Votes[0].State == hostproto.StateEmitted {
					r.answer(answer.Votes[0].Ticket, validOutput)
					r.journalAccepted(answer.Votes[0].Ticket)
					answeredSinceOpen = true
				}
			}
		})
	})
}

func TestOneQuery(t *testing.T) {
	t.Run("the launchable count a wave closes on equals the items `request list` shows, in any store"+verifiesOneQuery, func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			r := newRig(t)
			service := requests.Service{Store: r.store}
			for index := range rapid.IntRange(1, drawnRequestsPerRun).Draw(rt, "requests") {
				answer := r.want(rapid.IntRange(1, threeVotes).Draw(rt, "votes"), variant(string(rune('a'+index))))
				for _, vote := range answer.Votes {
					switch rapid.IntRange(0, 3).Draw(rt, "fate") {
					case 1:
						r.fetch(vote.Ticket)
					case 2:
						r.fetch(vote.Ticket)
						r.stop(vote.Ticket)
					case 3:
						r.answer(vote.Ticket, validOutput)
					}
				}
			}
			outstanding, err := r.bridge.Outstanding()
			if err != nil {
				rt.Fatal(err)
			}
			listing, err := service.List(hostproto.OnlySession(hostproto.SessionProve))
			if err != nil {
				rt.Fatal(err)
			}
			progress, err := service.Progress(hostproto.OnlySession(hostproto.SessionProve), 0)
			if err != nil {
				rt.Fatal(err)
			}
			if len(listing.Items) != len(outstanding.Launchable) || listing.InFlight != len(outstanding.InFlight) || listing.Orphaned != len(outstanding.Orphaned) ||
				progress.Launchable != len(listing.Items) || progress.InFlight != listing.InFlight || progress.Orphaned != listing.Orphaned {
				rt.Fatalf("outstanding %+v, listing %d items %d in flight %d orphaned, progress %+v", outstanding, len(listing.Items), listing.InFlight, listing.Orphaned, progress)
			}
		})
	})

	t.Run("a session that had an orphaned ticket lists it once it is reissued, never counts it unlisted"+verifiesOneQuery+verifiesOrphans, func(t *testing.T) {
		r := newRig(t)
		request := variant("lf1")
		ticket := r.want(oneVote, request).Votes[0].Ticket
		r.fetch(ticket)
		r.stop(ticket)
		r.want(oneVote, request)
		outstanding, err := r.bridge.FinishWave()
		if err != nil {
			t.Fatal(err)
		}
		listing, err := requests.Service{Store: r.store}.List(hostproto.OnlySession(hostproto.SessionProve))
		if err != nil {
			t.Fatal(err)
		}
		if len(outstanding.Launchable) != 1 || len(listing.Items) != 1 || listing.Items[0].Ticket.Emission != ticket.Emission+1 || outstanding.Waiting() != 1 {
			t.Fatalf("outstanding %+v, listing %+v", outstanding, listing)
		}
	})
}
