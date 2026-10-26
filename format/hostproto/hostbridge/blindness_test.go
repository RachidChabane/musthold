package hostbridge

import (
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
)

const (
	codexAgent  = "01a12287-50ec-7773-98c2-aeb643ce6d1e"
	codexOutput = `{"validity":"VALID"}`
)

// answerFromCodex answers a ticket with the profile a Codex executor's answer is
// stored with: unrestricted, backed by the capability of its agent.
func (r *rig) answerFromCodex(ticket hostproto.TicketID) {
	r.t.Helper()
	r.answer(ticket, codexOutput)
	response, err := r.store.GetResponse(ticket.Key())
	if err != nil {
		r.t.Fatal(err)
	}
	if err := r.store.DiscardResponse(ticket.Key()); err != nil {
		r.t.Fatal(err)
	}
	response.Executor.Harness = format.Harness{Name: hostproto.HarnessCodex, Version: "0.155.1"}
	response.Executor.Isolation = format.IsolationUnrestricted
	response.Executor.Attestation.AgentID = codexAgent
	if err := r.store.PutResponse(response); err != nil {
		r.t.Fatal(err)
	}
}

func TestBlindnessOfTheRun(t *testing.T) {
	for name, event := range map[string]func(ticket hostproto.TicketID) hostproto.Event{
		"inherited, against the ticket": func(ticket hostproto.TicketID) hostproto.Event {
			return hostproto.Event{Kind: hostproto.EventHistoryInherited, Ticket: ticket.String(), AgentID: codexAgent}
		},
		"unproven, against the agent alone": func(hostproto.TicketID) hostproto.Event {
			return hostproto.Event{Kind: hostproto.EventBlindnessUnproven, AgentID: codexAgent}
		},
	} {
		t.Run("a vote whose run was "+name+" is read at inline and marked, and the other vote is not [verifies SP-1936~1]", func(t *testing.T) {
			r := newRig(t)
			request := variant("blindness " + name)
			first := r.want(twoVotes, request)
			r.answerFromCodex(first.Votes[0].Ticket)
			r.answer(first.Votes[1].Ticket, codexOutput)
			if _, err := r.store.Append(event(first.Votes[0].Ticket)); err != nil {
				t.Fatal(err)
			}
			answer := r.want(twoVotes, request)
			lowered, other := answer.Votes[0], answer.Votes[1]
			if lowered.Executor.Isolation != format.IsolationInline || !lowered.Marked {
				t.Fatalf("vote %+v", lowered)
			}
			if other.Executor.Isolation != format.IsolationRestricted || other.Marked {
				t.Fatalf("vote %+v", other)
			}
		})
	}

	t.Run("a vote of a Codex run with no blindness event keeps unrestricted [verifies SP-1936~1]", func(t *testing.T) {
		r := newRig(t)
		request := variant("blind run")
		first := r.want(oneVote, request)
		r.answerFromCodex(first.Votes[0].Ticket)
		if vote := r.want(oneVote, request).Votes[0]; vote.Executor.Isolation != format.IsolationUnrestricted || vote.Marked {
			t.Fatalf("vote %+v", vote)
		}
	})
}
