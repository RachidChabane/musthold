package prove

import (
	"crypto/rand"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostbridge"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	verifiesSP1963 = " [verifies SP-1963~1]"

	sharedRequest  = "request"
	setWitnesses   = 4
	readingPasses  = 2
	firstDrawToken = " 1 "
)

// writerOutput is a complete set of witnesses as a writer returns it.
const writerOutput = `{"imports":"from parcel.rates import shipping_fee","witnesses":[` +
	`{"name":"free_from_fifty","clause":"reaches 50","kind":"example","code":"def free_from_fifty():\n    assert shipping_fee(50) == 0"},` +
	`{"name":"charged_below","clause":"reaches 50","kind":"example","code":"def charged_below():\n    assert shipping_fee(49.99) == 4.5"},` +
	`{"name":"free_above","clause":"waive","kind":"example","code":"def free_above():\n    assert shipping_fee(100) == 0"},` +
	`{"name":"waived","clause":"waive","kind":"property","code":"def waived():\n    assert True"}]}`

func TestReadSets(t *testing.T) {
	t.Run("each set's digest is journaled once and two writers' identical sets count once"+verifiesSP1963, func(t *testing.T) {
		f := newFixture()
		store := hostprototest.NewMemoryHostStore()
		f.run.bridge = hostbridge.New(store, time.Now, rand.Reader, hostproto.SessionProve, 1)
		answer := hostbridge.Answer{ID: sharedRequest, Votes: []hostbridge.VoteState{
			{Ticket: hostproto.TicketID{RequestID: sharedRequest, Vote: 0}, Output: []byte(writerOutput)},
			{Ticket: hostproto.TicketID{RequestID: sharedRequest, Vote: 1}, Output: []byte(writerOutput)},
		}}
		for range readingPasses {
			draw := newWitnessDraw(firstDraw)
			contradicted, err := f.run.readSets(draw, answer)
			if err != nil || contradicted {
				t.Fatalf("contradicted %t, error %v", contradicted, err)
			}
			if len(draw.sets) != 1 || len(draw.records) != setWitnesses {
				t.Fatalf("%d sets, %d witnesses", len(draw.sets), len(draw.records))
			}
		}
		records, err := store.Records()
		if err != nil {
			t.Fatal(err)
		}
		committed := 0
		for _, record := range records {
			if record.Event.Kind == hostproto.EventWitnessSetCommitted {
				committed++
				if !strings.HasPrefix(record.Event.Detail, f.run.c.Ref.String()+firstDrawToken) {
					t.Errorf("detail %q", record.Event.Detail)
				}
			}
		}
		if committed != 1 {
			t.Errorf("%d commits, want 1", committed)
		}
	})
}
