package hostproto_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorrun"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	childThread    = "01a12287-50ec-7773-98c2-aeb643ce6d1e"
	childRole      = "spec-proof-dp-decider"
	recordedTicket = "864f5e696da12d6944d9380733d5c5a9fba0f0140f3b243c3efae9a6cc74cb38.0.1"
	fetchTool      = "mcp__spec_proof_dp_exec__fetch_request"
	submitTool     = "mcp__spec_proof_dp_exec__submit_response"
	parentPrompt   = "Run the proofs of the project and fix what they report."
)

func readRollout(t *testing.T, name string) []byte {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func rolloutBytes(t *testing.T, rollout *hostprototest.CodexRollout) []byte {
	t.Helper()
	content, err := rollout.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	return content
}

func fixtureTicket(t *testing.T) hostproto.TicketID {
	t.Helper()
	return hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID
}

func toolCall(t *testing.T, name string, ticket hostproto.TicketID) hostproto.ToolCall {
	t.Helper()
	input, err := json.Marshal(map[string]string{"ticket": ticket.String(), "cap": ""})
	if err != nil {
		t.Fatal(err)
	}
	return hostproto.ToolCall{Name: name, Input: input}
}

// executorStart is a rollout that has reached the harness context of a spawned
// executor thread.
func executorStart() *hostprototest.CodexRollout {
	return hostprototest.NewCodexRollout(childThread, childRole).Bookkeeping().Developer("Fetch the ticket in your task.").Context()
}

func TestCodexChildRolloutBlind(t *testing.T) {
	t.Run("a recorded executor rollout with a plain task message is blind, with its ticket, its calls and its reply [verifies SP-1934~1]", func(t *testing.T) {
		run := hostproto.ReadCodexChildRollout(readRollout(t, "codex-child-rollout.jsonl"))
		if run.History != hostproto.ChildBlind || run.Reason != "" {
			t.Fatalf("history %s: %s", run.History, run.Reason)
		}
		if run.ThreadID != childThread || run.ParentThreadID != "01a12286-ded0-79f1-a8d0-73748a01598a" || run.AgentRole != childRole {
			t.Errorf("identity = %q %q %q", run.ThreadID, run.ParentThreadID, run.AgentRole)
		}
		if !run.TicketFound || run.Ticket.String() != recordedTicket {
			t.Errorf("ticket = %v %v", run.Ticket, run.TicketFound)
		}
		if len(run.Calls) != 2 || run.Calls[0].Name != fetchTool || run.Calls[1].Name != submitTool {
			t.Errorf("calls = %+v", run.Calls)
		}
		if run.Reply != "receipt "+recordedTicket {
			t.Errorf("reply = %q", run.Reply)
		}
	})

	t.Run("a recorded executor rollout whose task came as an encrypted agent message is blind, with its reply and no ticket [verifies SP-1934~1]", func(t *testing.T) {
		run := hostproto.ReadCodexChildRollout(readRollout(t, "codex-child-rollout-agent-message.jsonl"))
		if run.History != hostproto.ChildBlind || run.TicketFound {
			t.Fatalf("run = %+v", run)
		}
		if ticket, found := executorrun.ReceiptTicket(run.Reply); !found || ticket.String() != recordedTicket {
			t.Errorf("reply = %q", run.Reply)
		}
	})

	t.Run("an agent message task takes the ticket of the first fetch [verifies SP-1934~1]", func(t *testing.T) {
		ticket := fixtureTicket(t)
		run := hostproto.ReadCodexChildRollout(rolloutBytes(t, executorStart().AgentMessage().Call(toolCall(t, fetchTool, ticket)).Call(toolCall(t, submitTool, ticket))))
		if run.History != hostproto.ChildBlind || !run.TicketFound || run.Ticket != ticket {
			t.Fatalf("run = %+v", run)
		}
	})

	t.Run("the rollout written for a blind executor is blind [verifies SP-1934~1]", func(t *testing.T) {
		ticket := fixtureTicket(t)
		content, err := hostprototest.CodexBlindRollout(childThread, childRole, ticket, []hostproto.ToolCall{toolCall(t, fetchTool, ticket)}, executorrun.Receipt(ticket))
		if err != nil {
			t.Fatal(err)
		}
		if run := hostproto.ReadCodexChildRollout(content); run.History != hostproto.ChildBlind || run.Ticket != ticket {
			t.Fatalf("run = %+v", run)
		}
	})

	t.Run("a last line the harness is still writing is ignored [verifies SP-1934~1]", func(t *testing.T) {
		content := append(readRollout(t, "codex-child-rollout.jsonl"), []byte(`{"timestamp":"2026-10-09T21:18:07.6Z","type":"event_msg","payl`)...)
		if run := hostproto.ReadCodexChildRollout(content); run.History != hostproto.ChildBlind {
			t.Fatalf("history %s: %s", run.History, run.Reason)
		}
	})
}

func TestCodexChildRolloutInherited(t *testing.T) {
	ticket := fixtureTicket(t)
	fetch := toolCall(t, fetchTool, ticket)
	for name, rollout := range map[string]*hostprototest.CodexRollout{
		"the parent's prompt before the task":     executorStart().User(parentPrompt).Task(ticket).Call(fetch),
		"a call of the parent before the task":    executorStart().Call(hostproto.ToolCall{Name: "exec_command", Input: json.RawMessage(`{"cmd":"ls"}`)}).Task(ticket),
		"an assistant message before the task":    executorStart().Assistant("I will spawn a decider.").Task(ticket),
		"reasoning before the task":               executorStart().Item("reasoning").Task(ticket),
		"a custom tool call before the task":      executorStart().Item("custom_tool_call").AgentMessage(),
		"a second task message":                   executorStart().Task(ticket).Call(fetch).Task(ticket),
		"a parent prompt that reads as a task":    executorStart().Task(ticket).Call(fetch).AgentMessage(),
		"user text after the harness context":     executorStart().User("<environment_context>\n</environment_context>").User(parentPrompt).Task(ticket),
		"user text that names no ticket":          executorStart().User("ticket please"),
		"a task-like message holding two tickets": executorStart().User("ticket " + ticket.String() + " and " + recordedTicket),
	} {
		t.Run(name+" means the thread inherited history [verifies SP-1934~1]", func(t *testing.T) {
			run := hostproto.ReadCodexChildRollout(rolloutBytes(t, rollout))
			if run.History != hostproto.ChildInherited || !strings.Contains(run.Reason, "line ") {
				t.Fatalf("history %s: %s", run.History, run.Reason)
			}
		})
	}

	t.Run("an inherited thread still yields the ticket of its task [verifies SP-1934~1]", func(t *testing.T) {
		run := hostproto.ReadCodexChildRollout(rolloutBytes(t, executorStart().User(parentPrompt).Task(ticket)))
		if run.History != hostproto.ChildInherited || !run.TicketFound || run.Ticket != ticket {
			t.Fatalf("run = %+v", run)
		}
	})
}

func TestCodexChildRolloutUnproven(t *testing.T) {
	ticket := fixtureTicket(t)
	recorded := readRollout(t, "codex-child-rollout.jsonl")
	lines := strings.SplitAfter(string(recorded), "\n")
	for name, content := range map[string][]byte{
		"an empty rollout":                   nil,
		"no task message":                    rolloutBytes(t, executorStart().Bookkeeping()),
		"an unknown item before the task":    rolloutBytes(t, executorStart().Item("compaction").Task(ticket)),
		"an unknown context tag":             rolloutBytes(t, executorStart().User("<turn_aborted>\n</turn_aborted>").Task(ticket)),
		"a line that is not JSON":            []byte(lines[0] + lines[1] + "not json\n" + strings.Join(lines[2:], "")),
		"the rollout of a main thread":       rolloutBytes(t, hostprototest.NewCodexMainRollout(childThread).Developer("x").Context().Task(ticket)),
		"a first line that is not a session": []byte(strings.Join(lines[1:], "")),
		"an item that is not an object":      []byte(lines[0] + `{"type":"response_item","payload":[1]}` + "\n" + strings.Join(lines[1:], "")),
		"a session line that cannot be read": []byte(`{"type":"session_meta","payload":"x"}` + "\n" + strings.Join(lines[1:], "")),
		"a message of an unknown role":       rolloutBytes(t, executorStart().Item("message").Task(ticket)),
	} {
		t.Run(name+" leaves the blindness unproven [verifies SP-1934~1]", func(t *testing.T) {
			run := hostproto.ReadCodexChildRollout(content)
			if run.History != hostproto.ChildUnproven || run.Reason == "" {
				t.Fatalf("history %s: %s", run.History, run.Reason)
			}
		})
	}

	t.Run("a mid-file line that is not JSON keeps the ticket of the task [verifies SP-1934~1]", func(t *testing.T) {
		content := []byte(strings.Join(lines[:len(lines)-2], "") + "not json\n" + lines[len(lines)-2])
		run := hostproto.ReadCodexChildRollout(content)
		if run.History != hostproto.ChildUnproven || !run.TicketFound || run.Ticket.String() != recordedTicket {
			t.Fatalf("run = %+v", run)
		}
	})
}
