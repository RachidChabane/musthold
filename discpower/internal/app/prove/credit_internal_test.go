package prove

import (
	"context"
	"crypto/rand"
	"io/fs"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/witness"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	verifiesSP1987 = " [verifies SP-1987~1]"

	localWitnessTickets = firstDrawVotes
	ciWitnessTickets    = firstDrawVotes + creditDrawVotes
	creditRequestID     = "credit"
)

// layoutOnly is a witness host that lays out nothing: the draws under test never
// reach an execution.
type layoutOnly struct{}

func (layoutOnly) Brief(fs.FS, []mutation.SourceFile, string) (ports.WitnessBrief, error) {
	return ports.WitnessBrief{}, nil
}

func (layoutOnly) Layout(fs.FS, witness.Set) (ports.WitnessFile, error) {
	return ports.WitnessFile{}, nil
}

// producing is a briefed fixture whose run produces its Statements as named,
// with a witness host and a bridge to a host that has answered nothing.
func producing(producedBy format.ProducedBy) fixture {
	f := briefed(newFixture())
	f.project.settings.ProducedBy = producedBy
	f.project.runner.Witnesses = execution.Witnesses{Host: layoutOnly{}}
	f.run.bridge = hostbridge.New(hostprototest.NewMemoryHostStore(), time.Now, rand.Reader, 1)
	return f
}

func pendingTickets(t *testing.T, f fixture) int {
	t.Helper()
	pending, err := f.run.bridge.PendingTickets()
	if err != nil {
		t.Fatal(err)
	}
	return len(pending)
}

func TestWitnessSeed(t *testing.T) {
	t.Run("the seed of a draw depends on the statement and the draw, never on the run seed"+verifiesSP1969, func(t *testing.T) {
		f := newFixture()
		first, further := f.run.witnessSeed(firstDraw), f.run.witnessSeed(furtherDraw)
		if first == further || first != f.run.witnessSeed(firstDraw) {
			t.Fatalf("seeds %q and %q", first, further)
		}
		other := newFixture()
		other.project.settings.Seed = otherRunSeed
		if other.run.witnessSeed(firstDraw) != first || other.run.witnessSeed(furtherDraw) != further {
			t.Error("another run seed changed the seed")
		}
		statement := newFixture()
		statement.run.c.StatementDigest = format.SHA256Hex([]byte("another statement"))
		if statement.run.witnessSeed(firstDraw) == first {
			t.Error("another statement gave the same seed")
		}
	})

	t.Run("a CI seed leaves the witness request unchanged, so the pushed answers serve it"+verifiesSP1969, func(t *testing.T) {
		local := briefed(newFixture())
		ci := briefed(newFixture())
		ci.project.settings.Seed = otherRunSeed
		if witnessRequestID(t, local, firstDraw) != witnessRequestID(t, ci, firstDraw) {
			t.Error("the CI seed changed the witness request")
		}
	})

	t.Run("the credit draw is seeded by the run seed, so each CI run asks fresh writers"+verifiesSP1987, func(t *testing.T) {
		one := briefed(newFixture())
		other := briefed(newFixture())
		other.project.settings.Seed = otherRunSeed
		if witnessRequestID(t, one, creditDraw) == witnessRequestID(t, other, creditDraw) {
			t.Error("another run seed left the credit request unchanged")
		}
	})
}

func TestCreditDraw(t *testing.T) {
	t.Run("a CI run asks a credit draw of two writers beside the first draw; a local run asks none"+verifiesSP1987, func(t *testing.T) {
		for _, tc := range []struct {
			producedBy format.ProducedBy
			credit     int
			tickets    int
		}{
			{format.ProducedByLocal, 0, localWitnessTickets},
			{format.ProducedByCI, 1, ciWitnessTickets},
		} {
			f := producing(tc.producedBy)
			if err := f.run.firstDraw(context.Background()); err != nil {
				t.Fatal(err)
			}
			if len(f.run.draws) != 1 || len(f.run.creditDraws) != tc.credit {
				t.Fatalf("%s: %d draws, %d credit draws", tc.producedBy, len(f.run.draws), len(f.run.creditDraws))
			}
			if tc.credit > 0 && f.run.creditDraws[0].number != creditDraw {
				t.Errorf("credit draw numbered %d", f.run.creditDraws[0].number)
			}
			if got := pendingTickets(t, f); got != tc.tickets {
				t.Errorf("%s: %d pending tickets, want %d", tc.producedBy, got, tc.tickets)
			}
		}
	})

	t.Run("the search waits for its own draws only, the Statement for the credit draw too"+verifiesSP1987, func(t *testing.T) {
		f := producing(format.ProducedByCI)
		f.run.draws = []*witnessDraw{{number: firstDraw, settled: true}}
		f.run.creditDraws = []*witnessDraw{{number: creditDraw}}
		if !f.run.witnessesSettled() {
			t.Error("the pending credit draw held back the search")
		}
		if f.run.creditSettled() {
			t.Error("the pending credit draw reads as settled")
		}
		if len(f.run.searchDraws()) != 1 || len(f.run.allDraws()) != 2 {
			t.Errorf("%d search draws, %d draws", len(f.run.searchDraws()), len(f.run.allDraws()))
		}
	})

	t.Run("in CI only the credit draw's writers keep their credit"+verifiesSP1987, func(t *testing.T) {
		for _, tc := range []struct {
			producedBy format.ProducedBy
			draw       int
			withheld   bool
		}{
			{format.ProducedByLocal, firstDraw, false},
			{format.ProducedByCI, firstDraw, true},
			{format.ProducedByCI, furtherDraw, true},
			{format.ProducedByCI, creditDraw, false},
		} {
			f := producing(tc.producedBy)
			answer := hostbridge.Answer{ID: creditRequestID, Votes: []hostbridge.VoteState{
				{Ticket: hostproto.TicketID{RequestID: creditRequestID}, Output: []byte(writerOutput), Executor: format.ExecutorProfile{Isolation: format.IsolationRestricted}},
			}}
			draw := newWitnessDraw(tc.draw)
			if contradicted, err := f.run.readSets(draw, answer); err != nil || contradicted {
				t.Fatalf("contradicted %t, error %v", contradicted, err)
			}
			writer := draw.sets[0].Writer
			if writer.CreditWithheld != tc.withheld {
				t.Errorf("%s draw %d: credit withheld %t", tc.producedBy, tc.draw, writer.CreditWithheld)
			}
			if writer.AtFloor(format.IsolationRestricted) == tc.withheld {
				t.Errorf("%s draw %d: at floor %t", tc.producedBy, tc.draw, writer.AtFloor(format.IsolationRestricted))
			}
		}
	})
}
