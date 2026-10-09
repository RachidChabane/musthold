package evidence_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

const (
	decider = "spec-proof-dp-decider"
	mainID  = "01a11334-f5df-7a33-ba1e-3dd65b19d059"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assembleRollouts(t *testing.T) evidence.Rollouts {
	t.Helper()
	found, err := evidence.Assemble(evidence.Files{
		RolloutMain: fixture(t, "rollout-main.jsonl"),
		RolloutChildren: map[string][]byte{
			"rollout-child.jsonl":         fixture(t, "rollout-child.jsonl"),
			"rollout-generic-child.jsonl": fixture(t, "rollout-generic-child.jsonl"),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return found.Rollouts
}

func TestRolloutDecoder(t *testing.T) {
	rollouts := assembleRollouts(t)

	t.Run("each spawn call is read with its agent type, the history members it wrote, whether they ask for a blind child and the thread it started [verifies SP-1510~1] [verifies SP-1511~3]", func(t *testing.T) {
		if !rollouts.Recorded || len(rollouts.Spawns) != 4 {
			t.Fatalf("rollouts = %+v", rollouts)
		}
		for index, want := range []evidence.RolloutSpawn{
			{Line: 3, CallID: "call_a1", TaskName: "decide_a", AgentType: decider, History: map[string]string{"fork_turns": `"none"`}, Blind: true, ChildPath: "/root/decide_a"},
			{Line: 5, CallID: "call_b2", TaskName: "decide_b", AgentType: decider, History: map[string]string{}},
			{Line: 6, CallID: "call_c3", TaskName: "decide_c", AgentType: decider, History: map[string]string{"fork_turns": `"all"`}},
			{Line: 7, CallID: "call_d4", TaskName: "decide_d", AgentType: "spec-proof-dp-judge", History: map[string]string{"fork_turns": "3"}},
		} {
			if got := rollouts.Spawns[index]; !reflect.DeepEqual(got, want) {
				t.Errorf("spawn %d = %+v, want %+v", index, got, want)
			}
		}
		if got := rollouts.SpawnsOf(decider); len(got) != 3 {
			t.Errorf("spawns of %s = %+v", decider, got)
		}
	})

	t.Run("a spawn that returned the id of its thread is tied to the rollout of that thread [verifies SP-1511~3]", func(t *testing.T) {
		found, err := evidence.Assemble(evidence.Files{
			RolloutMain:     fixture(t, "rollout-main-agent-id.jsonl"),
			RolloutChildren: map[string][]byte{"rollout-child-agent-id.jsonl": fixture(t, "rollout-child-agent-id.jsonl")},
		})
		if err != nil || len(found.Rollouts.Spawns) != 1 || len(found.Rollouts.Children) != 1 {
			t.Fatalf("rollouts = %+v, %v", found.Rollouts, err)
		}
		spawn, child := found.Rollouts.Spawns[0], found.Rollouts.Children[0]
		if !spawn.Blind || spawn.History["fork_context"] != "false" || spawn.ChildThreadID != child.ThreadID {
			t.Fatalf("spawn = %+v, child = %+v", spawn, child)
		}
		if tied, ok := found.Rollouts.SpawnOf(child); !ok || tied.CallID != spawn.CallID {
			t.Fatalf("the child is tied to %+v, %v", tied, ok)
		}
		if child.History != hostproto.ChildBlind {
			t.Fatalf("child history %s: %s", child.History, child.HistoryReason)
		}
	})

	t.Run("a spawned thread names its parent, its path, its depth and its role, and a thread with no role has none [verifies SP-1510~1]", func(t *testing.T) {
		if len(rollouts.Children) != 2 {
			t.Fatalf("children = %+v", rollouts.Children)
		}
		child := rollouts.Children[0]
		if child.File != "rollout-child.jsonl" || child.ParentThreadID != mainID || child.AgentPath != "/root/decide_a" || child.Depth != 1 || child.AgentRole != decider {
			t.Errorf("child = %+v", child)
		}
		if got := rollouts.ChildrenOf(decider); len(got) != 1 || got[0].File != "rollout-child.jsonl" {
			t.Errorf("children of %s = %+v", decider, got)
		}
		if generic := rollouts.Children[1]; generic.AgentRole != "" || generic.ParentThreadID != mainID {
			t.Errorf("generic child = %+v", generic)
		}
	})

	t.Run("each spawned thread is judged by the rule of the host protocol: a task after the harness context is blind, a rollout with no task is unproven [verifies SP-1511~3]", func(t *testing.T) {
		if child := rollouts.Children[0]; child.History != hostproto.ChildBlind || child.HistoryReason != "" {
			t.Errorf("child history %s: %s", child.History, child.HistoryReason)
		}
		if generic := rollouts.Children[1]; generic.History != hostproto.ChildUnproven || generic.HistoryReason == "" {
			t.Errorf("generic child history %s: %s", generic.History, generic.HistoryReason)
		}
		if spawn, ok := rollouts.SpawnOf(rollouts.Children[0]); !ok || spawn.CallID != "call_a1" {
			t.Errorf("the child is tied to %+v, %v", spawn, ok)
		}
		if _, ok := rollouts.SpawnOf(rollouts.Children[1]); ok {
			t.Error("a thread no recorded spawn returned is tied to one")
		}
	})

	t.Run("a session that left no rollout has none recorded [verifies SP-1510~1]", func(t *testing.T) {
		found, err := evidence.Assemble(evidence.Files{})
		if err != nil || found.Rollouts.Recorded || len(found.Rollouts.Spawns) != 0 {
			t.Fatalf("rollouts = %+v, %v", found.Rollouts, err)
		}
	})

	t.Run("a rollout line that is not JSON is an error that names the file and the line [verifies SP-1510~1]", func(t *testing.T) {
		_, err := evidence.Assemble(evidence.Files{RolloutMain: []byte("{\"type\":\"session_meta\"}\nnot json\n")})
		if err == nil || !strings.Contains(err.Error(), "rollout-main.jsonl:2") {
			t.Fatalf("error = %v", err)
		}
		_, err = evidence.Assemble(evidence.Files{RolloutChildren: map[string][]byte{"c.jsonl": []byte("{\n")}})
		if err == nil || !strings.Contains(err.Error(), "rollout-children/c.jsonl:1") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a last line left incomplete by an interrupted write is ignored and the lines before it are read [verifies SP-1510~1]", func(t *testing.T) {
		whole := string(fixture(t, "rollout-main.jsonl"))
		found, err := evidence.Assemble(evidence.Files{
			RolloutMain:     []byte(whole + `{"type":"response_item","payload":{"type":"function_ca`),
			RolloutChildren: map[string][]byte{"c.jsonl": []byte(`{"type":"session_meta","payl`)},
		})
		if err != nil || len(found.Rollouts.Spawns) != 4 || len(found.Rollouts.Children) != 1 || found.Rollouts.Children[0].ThreadID != "" {
			t.Fatalf("rollouts = %+v, %v", found.Rollouts, err)
		}
		_, err = evidence.Assemble(evidence.Files{RolloutMain: []byte("not json\n" + whole)})
		if err == nil || !strings.Contains(err.Error(), "rollout-main.jsonl:1") {
			t.Fatalf("a bad line before the end was accepted: %v", err)
		}
	})

	t.Run("a spawn whose arguments are not JSON is kept with nothing read from them [verifies SP-1510~1]", func(t *testing.T) {
		line := `{"type":"response_item","payload":{"type":"function_call","name":"spawn_agent","arguments":"not json","call_id":"c"}}`
		found, err := evidence.Assemble(evidence.Files{RolloutMain: []byte(line + "\n")})
		if err != nil || len(found.Rollouts.Spawns) != 1 || found.Rollouts.Spawns[0].AgentType != "" || found.Rollouts.Spawns[0].Blind || len(found.Rollouts.Spawns[0].History) != 0 {
			t.Fatalf("spawns = %+v, %v", found.Rollouts.Spawns, err)
		}
	})
}

func TestRolloutParent(t *testing.T) {
	t.Run("the first line of a spawned thread's rollout names the thread it came from, and no other line does [verifies SP-1510~1]", func(t *testing.T) {
		first := strings.SplitN(string(fixture(t, "rollout-child.jsonl")), "\n", 2)[0]
		if parent, ok := evidence.RolloutParent([]byte(first)); !ok || parent != mainID {
			t.Errorf("parent = %q, %v", parent, ok)
		}
		mainFirst := strings.SplitN(string(fixture(t, "rollout-main.jsonl")), "\n", 2)[0]
		for _, line := range []string{mainFirst, `{"type":"response_item","payload":{}}`, "not json", ""} {
			if parent, ok := evidence.RolloutParent([]byte(line)); ok {
				t.Errorf("%q has parent %q", line, parent)
			}
		}
	})
}

func TestCodexThreadID(t *testing.T) {
	t.Run("the main thread is the one the stream starts, whichever event follows [verifies SP-1510~1]", func(t *testing.T) {
		stream := evidence.DecodeCodex([]evidence.Line{
			{Number: 1, Raw: json.RawMessage(`{"type":"thread.started","thread_id":"` + mainID + `"}`)},
			{Number: 2, Raw: json.RawMessage(`{"type":"thread.started","thread_id":"other"}`)},
		})
		if stream.ThreadID != mainID {
			t.Errorf("thread id = %q", stream.ThreadID)
		}
		if evidence.DecodeCodex(nil).ThreadID != "" {
			t.Error("an empty stream has a thread")
		}
	})
}

func TestHookPayloadKeys(t *testing.T) {
	t.Run("the members of a payload are listed sorted, and a payload that is not an object has none [verifies SP-1511~3]", func(t *testing.T) {
		call := evidence.HookCall{Stdin: json.RawMessage(`{"session_id":"s","agent_type":"a","agent_id":"i"}`)}
		if got := strings.Join(call.PayloadKeys(), ","); got != "agent_id,agent_type,session_id" {
			t.Errorf("keys = %s", got)
		}
		for _, stdin := range []string{`[1]`, `"x"`, ``} {
			if keys := (evidence.HookCall{Stdin: json.RawMessage(stdin)}).PayloadKeys(); len(keys) != 0 {
				t.Errorf("%q has keys %v", stdin, keys)
			}
		}
	})
}

func lines(raw ...string) []evidence.Line {
	var found []evidence.Line
	for index, line := range raw {
		found = append(found, evidence.Line{Number: index + 1, Raw: json.RawMessage(line)})
	}
	return found
}

func TestStartedATurn(t *testing.T) {
	t.Run("a stream shows a turn when the model produced a step of the harness's kind [verifies SP-1513~1]", func(t *testing.T) {
		for _, tc := range []struct {
			flag   string
			stream []evidence.Line
			want   bool
		}{
			{harness.FlagClaude, lines(`{"type":"system","subtype":"init"}`, `{"type":"assistant","message":{}}`), true},
			{harness.FlagClaude, lines(`{"type":"system","subtype":"init"}`, `{"type":"result","is_error":true}`), false},
			{harness.FlagCodex, lines(`{"type":"thread.started","thread_id":"t"}`, `{"type":"turn.started"}`, `{"type":"item.started","item":{"type":"reasoning"}}`), true},
			{harness.FlagCodex, lines(`{"type":"thread.started","thread_id":"t"}`, `{"type":"turn.started"}`, `{"type":"error","message":"model not available"}`, `{"type":"turn.failed"}`), false},
			{harness.FlagCodex, lines(`{"type":"thread.started","thread_id":"t"}`, `{"type":"turn.started"}`, `{"type":"item.completed","item":{"type":"error","message":"model not available"}}`, `{"type":"turn.failed"}`), false},
			{harness.FlagCopilot, lines(`{"type":"session.mcp_servers_loaded","data":{}}`, `{"type":"assistant.turn_start","data":{}}`), true},
			{harness.FlagCopilot, lines(`{"type":"session.mcp_servers_loaded","data":{}}`, `{"type":"model.call_start","data":{"model":"gpt-5.6-luna"}}`), true},
			{harness.FlagCopilot, lines(`{"type":"session.mcp_servers_loaded","data":{}}`, `{"type":"session.error","data":{"errorType":"model"}}`), false},
			{harness.FlagCopilot, nil, false},
			{"gemini", lines(`{"type":"assistant"}`), false},
		} {
			if got := evidence.StartedATurn(tc.flag, tc.stream); got != tc.want {
				t.Errorf("%s %v: started %v, want %v", tc.flag, tc.stream, got, tc.want)
			}
		}
	})

	t.Run("a line that is not JSON shows nothing [verifies SP-1513~1]", func(t *testing.T) {
		if evidence.StartedATurn(harness.FlagClaude, []evidence.Line{{Number: 1, Raw: json.RawMessage("nope")}}) {
			t.Error("garbage began a turn")
		}
	})
}

func TestErrorLine(t *testing.T) {
	t.Run("the line that says why the CLI failed is the last one naming an error, else the last one printed [verifies SP-1513~1]", func(t *testing.T) {
		for _, tc := range []struct{ stderr, want string }{
			{"Error: Model \"x\" from --model flag is not available.\n", "Error: Model \"x\" from --model flag is not available."},
			{"loading\nERROR: unauthorized\ntry again later\n", "ERROR: unauthorized"},
			{"starting\n\n  could not start  \n", "could not start"},
			{"Error: first\nError: second\n", "Error: second"},
			{"\n  \n", ""},
			{"", ""},
		} {
			if got := evidence.ErrorLine(tc.stderr); got != tc.want {
				t.Errorf("ErrorLine(%q) = %q, want %q", tc.stderr, got, tc.want)
			}
		}
	})
}

func TestCopilotModelAndTaskCall(t *testing.T) {
	t.Run("the model a session ran on is read from the first model.call_start that names one [verifies SP-1514~2]", func(t *testing.T) {
		stream := decodeCopilot(
			`{"type":"model.call_start","data":{"callId":"s"}}`,
			`{"type":"model.call_start","data":{"model":"gpt-5.6-luna"}}`)
		event, ok := stream.SelectedModel()
		if !ok || event.Model != "gpt-5.6-luna" || event.Line != 2 {
			t.Fatalf("event = %+v, %v", event, ok)
		}
		if _, ok := decodeCopilot(`{"type":"model.call_start","data":{}}`, `{"type":"assistant.message","data":{"model":"x"}}`).SelectedModel(); ok {
			t.Error("a model was read from an event that is not a call start with a model")
		}
	})

	t.Run("the agent type of a task call is read from its arguments, and only a task call has one [verifies SP-1515~2]", func(t *testing.T) {
		stream := decodeCopilot(
			`{"type":"tool.execution_start","data":{"toolCallId":"c","toolName":"task","arguments":{"agent_type":"spec-proof-dp:spec-proof-dp-decider","prompt":"p"}}}`,
			`{"type":"tool.execution_start","data":{"toolCallId":"d","toolName":"bash","arguments":{"agent_type":"x"}}}`,
			`{"type":"tool.execution_start","data":{"toolCallId":"e","toolName":"task","arguments":"not an object"}}`)
		events := stream.Of(evidence.CopilotEventToolStart)
		if got, isTask := events[0].TaskAgentType(); !isTask || got != "spec-proof-dp:spec-proof-dp-decider" {
			t.Errorf("task call = %q, %v", got, isTask)
		}
		if _, isTask := events[1].TaskAgentType(); isTask {
			t.Error("a shell call is a task call")
		}
		if got, isTask := events[2].TaskAgentType(); !isTask || got != "" {
			t.Errorf("a task call with odd arguments = %q, %v", got, isTask)
		}
	})
}
