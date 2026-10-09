package executorrun_test

import (
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
)

var blindnessKinds = []hostproto.EventKind{hostproto.EventHistoryInherited, hostproto.EventBlindnessUnproven}

func codexStored() format.ExecutorProfile {
	profile := stored(format.IsolationUnrestricted)
	profile.Harness = format.Harness{Name: hostproto.HarnessCodex, Version: "0.155.1"}
	return profile
}

func TestBlindnessLost(t *testing.T) {
	for _, kind := range blindnessKinds {
		t.Run("a "+string(kind)+" event lowers an unrestricted answer to inline and marks it [verifies SP-1936~1]", func(t *testing.T) {
			events := []hostproto.Event{{Kind: kind, Ticket: ticketText, AgentID: agent}}
			got := executorrun.Effective(codexStored(), events)
			if got.Isolation != format.IsolationInline || !executorrun.Marked(events) || !executorrun.BlindnessLost(events) {
				t.Fatalf("profile = %+v", got)
			}
			if err := got.Validate(); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("a "+string(kind)+" event keeps an attested run from being raised and lowers it to inline [verifies SP-1936~1]", func(t *testing.T) {
			events := []hostproto.Event{attested(agent, transcriptDigest), {Kind: kind, Ticket: ticketText}}
			if got := executorrun.Effective(stored(format.IsolationRestricted), events); got.Isolation != format.IsolationInline || got.Attestation.TranscriptDigest != "" {
				t.Fatalf("profile = %+v", got)
			}
		})
	}

	t.Run("a blind run keeps the stored isolation and no mark [verifies SP-1936~1]", func(t *testing.T) {
		events := []hostproto.Event{{Kind: hostproto.EventAccepted, Ticket: ticketText}}
		if got := executorrun.Effective(codexStored(), events); got.Isolation != format.IsolationUnrestricted || executorrun.Marked(events) {
			t.Fatalf("profile = %+v", got)
		}
	})

	t.Run("the lowered answer loses judged pass credit even under the floor unrestricted, where the blind one keeps it [verifies SP-1936~1]", func(t *testing.T) {
		lowered := executorrun.Effective(codexStored(), []hostproto.Event{{Kind: hostproto.EventHistoryInherited, Ticket: ticketText}})
		if lowered.Isolation.AtLeast(format.IsolationUnrestricted) {
			t.Fatalf("%s reaches the floor unrestricted", lowered.Isolation)
		}
		if blind := executorrun.Effective(codexStored(), nil); !blind.Isolation.AtLeast(format.IsolationUnrestricted) {
			t.Fatalf("%s does not reach the floor unrestricted", blind.Isolation)
		}
	})
}

func TestJournalIndex(t *testing.T) {
	const otherAgent = "agent-0002"
	otherTicket := ticketText[:len(ticketText)-1] + "2"
	records := func(events ...hostproto.Event) []hostproto.Record {
		out := make([]hostproto.Record, len(events))
		for index, event := range events {
			out[index] = hostproto.Record{Sequence: index, Event: event}
		}
		return out
	}

	t.Run("a response gets the events of its ticket and the ticketless blindness events of the agent that answered it [verifies SP-1936~1]", func(t *testing.T) {
		index := executorrun.IndexJournal(records(
			hostproto.Event{Kind: hostproto.EventAccepted, Ticket: ticketText},
			hostproto.Event{Kind: hostproto.EventBlindnessUnproven, AgentID: agent, Detail: "transcript unreadable"},
			hostproto.Event{Kind: hostproto.EventHistoryInherited, AgentID: otherAgent},
			hostproto.Event{Kind: hostproto.EventExposed, AgentID: agent},
			hostproto.Event{Kind: hostproto.EventLeaked, Ticket: otherTicket, AgentID: agent},
		))
		events := index.Response(ticketText, codexStored())
		kinds := make([]hostproto.EventKind, len(events))
		for position, event := range events {
			kinds[position] = event.Kind
		}
		if !slices.Equal(kinds, []hostproto.EventKind{hostproto.EventAccepted, hostproto.EventBlindnessUnproven}) {
			t.Fatalf("kinds = %v", kinds)
		}
		if got := executorrun.Effective(codexStored(), events); got.Isolation != format.IsolationInline {
			t.Fatalf("profile = %+v", got)
		}
		if ticketOnly := index.Ticket(ticketText); len(ticketOnly) != 1 {
			t.Fatalf("ticket events = %v", ticketOnly)
		}
	})

	t.Run("a response no agent backs gets the events of its ticket alone [verifies SP-1936~1]", func(t *testing.T) {
		index := executorrun.IndexJournal(records(hostproto.Event{Kind: hostproto.EventHistoryInherited}, hostproto.Event{Kind: hostproto.EventBlindnessUnproven, AgentID: agent}))
		unbacked := codexStored()
		unbacked.Attestation = format.Attestation{}
		if events := index.Response(ticketText, unbacked); len(events) != 0 {
			t.Fatalf("events = %v", events)
		}
	})
}

func TestReceiptTicket(t *testing.T) {
	t.Run("the ticket of a bare receipt is read whatever whitespace surrounds it, and nothing else is [verifies SP-1935~1]", func(t *testing.T) {
		if ticket, found := executorrun.ReceiptTicket("  receipt " + ticketText + "\n"); !found || ticket.String() != ticketText {
			t.Fatalf("ticket %v %v", ticket, found)
		}
		for _, reply := range []string{"", "receipt", "receipt x", "done " + ticketText, "receipt " + ticketText + " and more"} {
			if _, found := executorrun.ReceiptTicket(reply); found {
				t.Errorf("%q yielded a ticket", reply)
			}
		}
	})
}
