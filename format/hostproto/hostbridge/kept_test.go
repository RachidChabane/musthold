package hostbridge

import (
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	verifiesKept     = " [verifies SP-2668~1]"
	unusableWitness  = "the witnesses fail on the reference"
	maxKeptDrawSteps = 12
)

// eventsOf returns the journaled events of a kind about the ticket's emissions.
func (r *rig) eventsOf(kind hostproto.EventKind) []hostproto.Event {
	r.t.Helper()
	var events []hostproto.Event
	for _, record := range r.records() {
		if record.Event.Kind == kind {
			events = append(events, record.Event)
		}
	}
	return events
}

func TestKeptAnswers(t *testing.T) {
	t.Run("a rejection journals its reason and the abandonment journals every earlier one"+verifiesKept, func(t *testing.T) {
		r := newRig(t)
		request := variant("rejected thrice")
		answer := r.want(oneVote, request)
		for range hostproto.DefaultMaxEmissions {
			r.answer(answer.Votes[0].Ticket, validOutput)
			answered := r.want(oneVote, request)
			if err := r.bridge.Contradict(answered.Votes[0], unusableWitness); err != nil {
				t.Fatal(err)
			}
			answer = r.want(oneVote, request)
		}
		rejections := r.eventsOf(hostproto.EventRejected)
		abandonments := r.eventsOf(hostproto.EventAbandoned)
		if len(rejections) != hostproto.DefaultMaxEmissions || rejections[0].Detail != unusableWitness || len(abandonments) != 1 {
			t.Fatalf("rejections %+v, abandonments %+v", rejections, abandonments)
		}
		if want := strings.Repeat(unusableWitness+hostproto.FailureSeparator, hostproto.DefaultMaxEmissions-1) + unusableWitness; abandonments[0].Detail != want {
			t.Fatalf("the abandonment records %q", abandonments[0].Detail)
		}
		if kept := hostproto.KeptAnswers(r.records()); kept != 0 {
			t.Fatalf("%d rejected answers were kept", kept)
		}
	})

	t.Run("whatever answers execution keeps or rejects, the count of kept answers counts exactly the kept ones and never decreases"+verifiesKept, func(t *testing.T) {
		rapid.Check(t, func(rt *rapid.T) {
			r := newRig(t)
			kept, previous := 0, 0
			for step := range rapid.IntRange(1, maxKeptDrawSteps).Draw(rt, "steps") {
				request := variant(string(rune('a' + step)))
				answer := r.want(oneVote, request)
				r.answer(answer.Votes[0].Ticket, validOutput)
				answered := r.want(oneVote, request)
				if rapid.Bool().Draw(rt, "kept") {
					if err := r.bridge.Validate(answered); err != nil {
						rt.Fatal(err)
					}
					kept++
				} else if err := r.bridge.Contradict(answered.Votes[0], unusableWitness); err != nil {
					rt.Fatal(err)
				}
				count := hostproto.KeptAnswers(r.records())
				if count != kept || count < previous {
					rt.Fatalf("step %d: %d kept answers counted, %d kept, %d before", step, count, kept, previous)
				}
				previous = count
			}
		})
	})
}
