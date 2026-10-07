package hostbridge

import (
	"errors"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
)

func TestReissueReplacesTheAnswer(t *testing.T) {
	t.Run("the answer a contradicted ticket held is replaced by the answer to its reissue [verifies SP-537~1]", func(t *testing.T) {
		r := newRig(t)
		request := variant("contradicted")
		first := r.want(1, request)
		r.answer(first.Votes[0].Ticket, `{"validity":"VALID"}`)
		accepted := r.want(1, request)
		if err := r.bridge.Contradict(accepted.Votes[0], "refuted"); err != nil {
			t.Fatal(err)
		}

		reissued := r.want(1, request)
		if reissued.Standing != Pending || reissued.Votes[0].Ticket.Emission != hostproto.FirstEmission+1 {
			t.Fatalf("standing %s, ticket %s", reissued.Standing, reissued.Votes[0].Ticket)
		}
		if _, err := r.store.GetResponse(reissued.Votes[0].Ticket.Key()); !errors.Is(err, hostproto.ErrNotFound) {
			t.Fatalf("the contradicted answer is still stored: %v", err)
		}

		r.answer(reissued.Votes[0].Ticket, `{"validity":"EQUIVALENT"}`)
		answered := r.want(1, request)
		if answered.Standing != Answered || string(answered.Votes[0].Output) != `{"validity":"EQUIVALENT"}` {
			t.Fatalf("standing %s, output %s", answered.Standing, answered.Votes[0].Output)
		}
	})
}
