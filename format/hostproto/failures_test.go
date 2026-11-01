package hostproto_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const verifiesKeptAnswers = " [verifies SP-2668~1]"

func TestFailureReasons(t *testing.T) {
	t.Run("every failed emission records its reason, and abandonment and renewal keep them in order"+verifiesKeptAnswers, func(t *testing.T) {
		nonce := strings.Repeat("5", hostproto.NonceHexLength)
		entry := freshEntry()
		contradicted, err := contradictStep(mustStep(t, mustStep(t, entry, fetchStep), acceptStep))
		if err != nil || contradicted.Reason != contradictionReason {
			t.Fatalf("contradicted %+v, %v", contradicted, err)
		}
		entry = mustStep(t, contradicted, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reissue(nonce) })
		expired := mustStep(t, mustStep(t, entry, fetchStep), expireStep)
		entry = mustStep(t, expired, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reissue(nonce) })
		broken := mustStep(t, entry, fetchStep)
		for range hostproto.MaxCorrections + 1 {
			broken = mustStep(t, broken, func(e hostproto.Entry) (hostproto.Entry, error) {
				next, _, err := e.FailSchema()
				return next, err
			})
		}
		abandoned := mustStep(t, broken, func(e hostproto.Entry) (hostproto.Entry, error) { return e.Reissue(nonce) })
		want := []string{contradictionReason, hostproto.FailureExpired, hostproto.FailureSchema}
		if abandoned.State != hostproto.StateAbandoned || abandoned.Reason != hostproto.ReasonHostUnresponsive || !slices.Equal(abandoned.FailureReasons(), want) {
			t.Fatalf("abandoned %+v, failures %v", abandoned, abandoned.FailureReasons())
		}
		renewed := mustStep(t, abandoned, renewStep)
		released := mustStep(t, mustStep(t, renewed, fetchStep), hostproto.Entry.Release)
		if got := released.FailureReasons(); !slices.Equal(got, append(want, hostproto.FailureReleased)) {
			t.Fatalf("after the renewal and a release: %v", got)
		}
	})

	t.Run("only answers the journal records as validated are kept, never an accepted or rejected one"+verifiesKeptAnswers, func(t *testing.T) {
		ticket := hostprototest.Ticket(hostprototest.Request(), 0, hostproto.FirstEmission).TicketID.String()
		records := chain(t, []hostproto.Event{
			{Kind: hostproto.EventAccepted, Ticket: ticket}, {Kind: hostproto.EventRejected, Ticket: ticket, Detail: contradictionReason},
			{Kind: hostproto.EventAccepted, Ticket: ticket}, {Kind: hostproto.EventValidated, Ticket: ticket}, {Kind: hostproto.EventConsumed, Ticket: ticket},
		})
		if kept := hostproto.KeptAnswers(records); kept != 1 {
			t.Fatalf("kept %d", kept)
		}
	})
}

func mustStep(t *testing.T, entry hostproto.Entry, step func(hostproto.Entry) (hostproto.Entry, error)) hostproto.Entry {
	t.Helper()
	next, err := step(entry)
	if err != nil {
		t.Fatal(err)
	}
	return next
}
