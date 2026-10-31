package format_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/format"
)

const verifiesAnswered = " [verifies SP-2667~1]"

func TestPendingAnswered(t *testing.T) {
	waiting := []format.PendingEntry{{Criterion: "PARK-1~1", Reason: format.PendingAwaitingHost}}

	t.Run("the count of answers is decoded, always encoded, and a negative one is refused"+verifiesAnswered, func(t *testing.T) {
		pending, err := format.DecodePending([]byte(`{"module":"discpower","pending":[],"answered":7}`))
		if err != nil || pending.Answered != 7 {
			t.Fatalf("decoded %+v, %v", pending, err)
		}
		encoded, err := format.Canonicalize(format.Pending{Module: "discpower", Pending: []format.PendingEntry{}})
		if err != nil || string(encoded) != `{"answered":0,"module":"discpower","pending":[]}` {
			t.Fatalf("encoded %s, %v", encoded, err)
		}
		_, err = format.DecodePending([]byte(`{"module":"m","pending":[],"answered":-1}`))
		var validation *format.ValidationError
		if !errors.As(err, &validation) || !slices.Contains(validation.Codes(), format.CodeRequirement) {
			t.Fatalf("a negative count: %v", err)
		}
	})

	t.Run("a file written over a stored one keeps the larger count, whichever writer comes last"+verifiesAnswered, func(t *testing.T) {
		for _, c := range []struct{ next, stored, want int }{{0, 5, 5}, {6, 5, 6}, {3, 3, 3}, {0, 0, 0}} {
			got := format.Pending{Module: "m", Pending: waiting, Answered: c.next}.Over(format.Pending{Module: "m", Answered: c.stored})
			if got.Answered != c.want || len(got.Pending) != 1 {
				t.Errorf("%d over %d: %+v", c.next, c.stored, got)
			}
		}
	})

	t.Run("a file is kept while a criterion waits or once an answer was accepted"+verifiesAnswered, func(t *testing.T) {
		for _, c := range []struct {
			pending format.Pending
			kept    bool
		}{
			{format.Pending{Module: "m"}, false},
			{format.Pending{Module: "m", Answered: 1}, true},
			{format.Pending{Module: "m", Pending: waiting}, true},
		} {
			if c.pending.Kept() != c.kept {
				t.Errorf("%+v kept %v", c.pending, c.pending.Kept())
			}
		}
	})
}
