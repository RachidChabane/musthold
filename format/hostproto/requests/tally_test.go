package requests

import (
	"errors"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
)

const verifiesTally = " [verifies SP-2668~1]"

// recordingTally keeps every total it was given, and fails when told to.
type recordingTally struct {
	totals []int
	fail   error
}

func (r *recordingTally) Accepted(total int) error {
	if r.fail != nil {
		return r.fail
	}
	r.totals = append(r.totals, total)
	return nil
}

var errTallyFull = errors.New("the tally cannot be written")

func TestAnswerTally(t *testing.T) {
	t.Run("every accepted answer, and the replay of one, gives the tally the journal's count of accepted answers"+verifiesTally, func(t *testing.T) {
		f := newFixture(t)
		tally := &recordingTally{}
		f.service.Tally = tally
		_, second := f.emit(f.request, 1)
		f.show()
		if _, err := f.service.Show(second.TicketID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
			t.Fatal(err)
		}
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil {
			t.Fatalf("the replay: %v", err)
		}
		if _, err := f.service.Submit(Submission{Ticket: second.TicketID, Nonce: second.Nonce, Output: []byte(validAnswer), Origin: OriginCLI}); err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(tally.totals, []int{1, 1, 2}) {
			t.Fatalf("totals %v", tally.totals)
		}
	})

	t.Run("a refused submission tallies nothing, a failing tally fails the submission"+verifiesTally, func(t *testing.T) {
		f := newFixture(t)
		tally := &recordingTally{}
		f.service.Tally = tally
		f.show()
		if _, err := f.service.Submit(f.submission(brokenAnswer)); err == nil || len(tally.totals) != 0 {
			t.Fatalf("a schema failure: %v, totals %v", err, tally.totals)
		}
		tally.fail = errTallyFull
		if _, err := f.service.Submit(f.submission(validAnswer)); !errors.Is(err, errTallyFull) {
			t.Fatalf("a failing tally: %v", err)
		}
		if f.state() != hostproto.StateAccepted {
			t.Fatalf("the answer is %s", f.state())
		}
		tally.fail = nil
		if _, err := f.service.Submit(f.submission(validAnswer)); err != nil || !slices.Equal(tally.totals, []int{1}) {
			t.Fatalf("the replay after the failed tally: %v, totals %v", err, tally.totals)
		}
	})
}
