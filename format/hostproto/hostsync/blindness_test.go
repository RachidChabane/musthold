package hostsync_test

import (
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
	"github.com/RachidChabane/musthold/format/hostproto/hostsync"
)

func TestCollectBlindness(t *testing.T) {
	for name, event := range map[string]hostproto.Event{
		"against its ticket":                    {Kind: hostproto.EventHistoryInherited, AgentID: agent, Detail: "line 6: a user message precedes the task message"},
		"against the agent that answered alone": {Kind: hostproto.EventBlindnessUnproven, AgentID: agent, Detail: "transcript unreadable"},
	} {
		t.Run("an answer whose run was not shown blind "+name+" is pushed at inline and marked [verifies SP-1936~1]", func(t *testing.T) {
			store := newStore(t)
			answered := emit(t, store, hostprototest.Request(), 0)
			answerAsExecutor(t, store, answered)
			if event.Kind == hostproto.EventHistoryInherited {
				event.Ticket = answered.TicketID.String()
			}
			if _, err := store.Append(event); err != nil {
				t.Fatal(err)
			}
			bundle, err := hostsync.Collect(store, hostproto.SessionProve, module, headSHA)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := bundle.Lookup(answered.Key())
			if got.Executor.Isolation != format.IsolationInline || !bundle.Marked(answered.Key()) || got.Validate() != nil {
				t.Fatalf("executor %+v, marked %v", got.Executor, bundle.Marked(answered.Key()))
			}
		})
	}

	t.Run("a ticketless blindness event of another agent leaves the answer as it was [verifies SP-1936~1]", func(t *testing.T) {
		store := newStore(t)
		answered := emit(t, store, hostprototest.Request(), 0)
		answerAsExecutor(t, store, answered)
		if _, err := store.Append(hostproto.Event{Kind: hostproto.EventBlindnessUnproven, AgentID: "agent-0009"}); err != nil {
			t.Fatal(err)
		}
		bundle, err := hostsync.Collect(store, hostproto.SessionProve, module, headSHA)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := bundle.Lookup(answered.Key()); got.Executor.Isolation != format.IsolationRestricted || bundle.Marked(answered.Key()) {
			t.Fatalf("executor %+v", got.Executor)
		}
	})
}
