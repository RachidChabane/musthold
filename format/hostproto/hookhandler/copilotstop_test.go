package hookhandler_test

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	copilotLogPath     = "session-state/events.jsonl"
	copilotLogFixture  = "../testdata/copilot/session-events.jsonl"
	copilotAgentID     = "a7e0364c-b934-43d8-9bbb-9744dbf2af35"
	copilotTicket      = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"
	copilotAnswer      = `{"answers":[{"id":"q1","choice":"VIOLATES","confidence":1,"probabilities":{"VIOLATES":1,"CONSISTENT":0}}]}`
	copilotDeciderRole = "spec-proof-dp:decider"
	// copilotReportedType is the type the payload of subagentStop reports.
	copilotReportedType = "spec-proof-dp:spec-proof-dp-decider"
)

// copilotStop is a Copilot CLI executor that stopped, with its session log served
// from the fixture and its answer stored.
type copilotStop struct {
	*fixture
	ticket hostproto.TicketID
}

func newCopilotStop(t *testing.T) copilotStop {
	t.Helper()
	f := newFixture(t)
	ticket, err := hostproto.ParseTicketID(copilotTicket)
	if err != nil {
		t.Fatal(err)
	}
	log, err := os.ReadFile(copilotLogFixture)
	if err != nil {
		t.Fatal(err)
	}
	f.transcripts[copilotLogPath] = log
	response := hostprototest.Response(hostprototest.Ticket(hostprototest.Request(), 0, 1))
	response.RequestID, response.Vote, response.Output = ticket.RequestID, ticket.Vote, json.RawMessage(copilotAnswer)
	if err := f.store.PutResponse(response); err != nil {
		t.Fatal(err)
	}
	return copilotStop{fixture: f, ticket: ticket}
}

func (s copilotStop) stop(agentType, response string) error {
	payload := marshal(s.t, map[string]any{
		"sessionId": "9b3aa43b-1935-4c09-a07b-bd74be7c7956", "transcriptPath": copilotLogPath,
		"agentId": copilotAgentID, "agentType": agentType, "agentName": agentType, "response": response, "stopReason": "end_turn",
	})
	_, err := s.handler(harness.Copilot(identity)).Handle(hostproto.HookSubagentStop, []byte(payload))
	return err
}

func (s copilotStop) receipt() string { return executorrun.Receipt(s.ticket) }

func TestCopilotSubagentStop(t *testing.T) {
	t.Run("the stop of the executor reported with the plugin-prefixed type journals both events for its ticket and agent [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		if err := s.stop(copilotReportedType, s.receipt()); err != nil {
			t.Fatal(err)
		}
		if got := s.kinds(); !slices.Equal(got, []hostproto.EventKind{hostproto.EventAllowlistObserved, hostproto.EventLaunchBare}) {
			t.Fatalf("events = %v", got)
		}
		for _, event := range s.events() {
			if event.Ticket != copilotTicket || event.AgentID != copilotAgentID || event.Role != copilotDeciderRole {
				t.Errorf("event = %+v", event)
			}
		}
		profile := hostprototest.Profile()
		profile.Isolation = format.IsolationRestrictedUnverified
		if got := executorrun.Effective(profile, s.events()).Isolation; got != format.IsolationRestricted {
			t.Errorf("isolation = %s", got)
		}
	})

	t.Run("the stop of the executor reported with its plain file name is checked as well [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		if err := s.stop("spec-proof-dp-decider", s.receipt()); err != nil {
			t.Fatal(err)
		}
		if got := s.kinds(); len(got) != 2 {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("a type of another plugin is no executor of the module and journals nothing [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		if err := s.stop("other-plugin:spec-proof-dp-decider", s.receipt()); err != nil || len(s.events()) != 0 {
			t.Fatalf("events %v, error %v", s.events(), err)
		}
	})

	t.Run("a reply that is not the receipt is a leak and no event of the observation is journaled [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		if err := s.stop(copilotReportedType, s.receipt()+" and the file says x"); err != nil {
			t.Fatal(err)
		}
		if got := s.kinds(); !slices.Equal(got, []hostproto.EventKind{hostproto.EventLeaked}) {
			t.Fatalf("events = %v", got)
		}
	})

	t.Run("a log the hook cannot read or decode journals a restricted run [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		delete(s.transcripts, copilotLogPath)
		if err := s.stop(copilotReportedType, s.receipt()); err != nil {
			t.Fatal(err)
		}
		s.transcripts[copilotLogPath] = []byte("{\"type\":\"session.start\"}\nnot json\n{\"type\":\"session.start\"}\n")
		if err := s.stop(copilotReportedType, s.receipt()); err != nil {
			t.Fatal(err)
		}
		details := []string{}
		for _, event := range s.events() {
			if event.Kind != hostproto.EventRestricted {
				t.Errorf("event = %+v", event)
			}
			details = append(details, event.Detail)
		}
		if len(details) != 2 || !strings.Contains(details[0], "unreadable") || !strings.Contains(details[1], "undecodable") {
			t.Errorf("details = %v", details)
		}
	})

	t.Run("an answer the module did not store keeps the allowlist unobserved [verifies SP-1871~1]", func(t *testing.T) {
		s := newCopilotStop(t)
		if err := s.store.DiscardResponse(s.ticket.Key()); err != nil {
			t.Fatal(err)
		}
		if err := s.stop(copilotReportedType, s.receipt()); err != nil {
			t.Fatal(err)
		}
		if got := s.kinds(); !slices.Equal(got, []hostproto.EventKind{hostproto.EventRestricted, hostproto.EventLaunchBare}) {
			t.Fatalf("events = %v", got)
		}
	})
}
