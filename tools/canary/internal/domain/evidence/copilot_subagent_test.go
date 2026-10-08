package evidence_test

import (
	"reflect"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

const (
	taskCall   = "call_qGur5ZF9cX5qI20DNWOhYok9"
	subagentID = "d7ba8126-5bc3-47cc-9f2a-9dcfe2d9e16e"
	started    = `{"type":"subagent.started","agentId":"` + subagentID + `","data":{"toolCallId":"` + taskCall + `","agentName":"spec-proof-dp:spec-proof-dp-decider","agentDisplayName":"spec-proof-dp-decider","model":"gpt-5.6-luna"}}`
	selected   = `{"type":"subagent.selected","agentId":"` + subagentID + `","data":{"agentName":"spec-proof-dp:spec-proof-dp-decider","agentDisplayName":"spec-proof-dp-decider","tools":["spec-proof-dp-exec/fetch_request","spec-proof-dp-exec/submit_response"]}}`
	completed  = `{"type":"subagent.completed","agentId":"` + subagentID + `","data":{"toolCallId":"` + taskCall + `","agentName":"spec-proof-dp:spec-proof-dp-decider","totalToolCalls":2,"totalTokens":5301}}`
)

func TestCopilotSubagentDecoder(t *testing.T) {
	t.Run("a subagent is assembled from its start, its selection joined by agent id and its end joined by the task call [verifies SP-1521~1]", func(t *testing.T) {
		subagents := decodeCopilot(started, selected, completed).Subagents()
		if len(subagents) != 1 {
			t.Fatalf("%d subagents", len(subagents))
		}
		subagent := subagents[0]
		if subagent.Name != "spec-proof-dp:spec-proof-dp-decider" || subagent.AgentID != subagentID || subagent.ToolCallID != taskCall {
			t.Fatalf("subagent = %+v", subagent)
		}
		wantTools := []string{"spec-proof-dp-exec/fetch_request", "spec-proof-dp-exec/submit_response"}
		if !subagent.HasSelected || !reflect.DeepEqual(subagent.Selected.Tools, wantTools) || subagent.Selected.Line != 2 {
			t.Errorf("selected = %+v", subagent.Selected)
		}
		if !subagent.HasCompleted || subagent.Completed.Line != 3 {
			t.Errorf("completed = %+v", subagent.Completed)
		}
	})

	t.Run("a selection of another subagent and an end of another task call are not joined [verifies SP-1521~1]", func(t *testing.T) {
		other := `{"type":"subagent.selected","agentId":"other","data":{"tools":["bash"]}}`
		elsewhere := `{"type":"subagent.completed","agentId":"other","data":{"toolCallId":"call_other"}}`
		subagent := decodeCopilot(started, other, elsewhere).Subagents()[0]
		if subagent.HasSelected || subagent.HasCompleted {
			t.Errorf("subagent = %+v", subagent)
		}
	})

	t.Run("a stream that started no subagent has none [verifies SP-1521~1]", func(t *testing.T) {
		if got := decodeCopilot(selected, completed).Subagents(); len(got) != 0 {
			t.Errorf("subagents = %+v", got)
		}
	})

	t.Run("a tool list that is absent or not a list of strings reads as no selection while an empty list reads as an empty one [verifies SP-1521~1]", func(t *testing.T) {
		events := decodeCopilot(
			`{"type":"subagent.selected","data":{}}`,
			`{"type":"subagent.selected","data":{"tools":"bash"}}`,
			`{"type":"subagent.selected","data":{"tools":[]}}`).Events
		if events[0].Tools != nil || events[1].Tools != nil {
			t.Errorf("tools = %v, %v", events[0].Tools, events[1].Tools)
		}
		if events[2].Tools == nil || len(events[2].Tools) != 0 {
			t.Errorf("tools = %#v", events[2].Tools)
		}
	})
}

func TestCopilotSubagentAttribution(t *testing.T) {
	subagent := decodeCopilot(started, selected, completed).Subagents()[0]
	event := func(line string) evidence.CopilotEvent { return decodeCopilot(line).Events[0] }

	t.Run("a tool start is the subagent's when it names the task call as its parent, whatever its envelope says [verifies SP-1521~1]", func(t *testing.T) {
		start := event(`{"type":"tool.execution_start","data":{"toolCallId":"c1","toolName":"spec-proof-dp-exec-fetch_request","parentToolCallId":"` + taskCall + `"}}`)
		if start.ParentToolCallID != taskCall || !subagent.Originated(start) {
			t.Errorf("start = %+v", start)
		}
	})

	t.Run("an event is the subagent's when it carries its agent id, and not when it names another task call or agent [verifies SP-1521~1]", func(t *testing.T) {
		if !subagent.Originated(event(`{"type":"assistant.reasoning","agentId":"` + subagentID + `","data":{}}`)) {
			t.Error("an event of the subagent's envelope is not attributed")
		}
		for _, line := range []string{
			`{"type":"tool.execution_start","data":{"toolCallId":"c2","toolName":"apply_patch"}}`,
			`{"type":"tool.execution_start","agentId":"other","data":{"toolCallId":"c3","parentToolCallId":"call_other"}}`,
		} {
			if subagent.Originated(event(line)) {
				t.Errorf("%s attributed to the subagent", line)
			}
		}
	})

	t.Run("the completion of the task call is what the subagent returned and its start is the main agent's call [verifies SP-1521~1]", func(t *testing.T) {
		returned := event(`{"type":"tool.execution_complete","data":{"toolCallId":"` + taskCall + `","success":true}}`)
		launched := event(`{"type":"tool.execution_start","data":{"toolCallId":"` + taskCall + `","toolName":"task"}}`)
		if !subagent.Returned(returned) || subagent.Returned(launched) || !subagent.Launched(launched) || subagent.Launched(returned) {
			t.Error("the task call is not told apart from its completion")
		}
		if subagent.Originated(launched) || subagent.Originated(returned) {
			t.Error("the main agent's task call is attributed to the subagent")
		}
	})
}
