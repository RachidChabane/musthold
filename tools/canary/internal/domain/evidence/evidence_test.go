package evidence_test

import (
	"encoding/json"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

func decode(t *testing.T, lines ...string) evidence.ClaudeStream {
	t.Helper()
	var raw []evidence.Line
	for index, line := range lines {
		raw = append(raw, evidence.Line{Number: index + 1, Raw: json.RawMessage(line)})
	}
	return evidence.DecodeClaude(raw)
}

func TestClaudeDecoder(t *testing.T) {
	t.Run("plugins and servers are read whether init lists them as names or as objects [verifies SP-1501~1]", func(t *testing.T) {
		stream := decode(t, `{"type":"system","subtype":"init","plugins":["a",{"name":"b","path":"/p"}],"mcp_servers":[{"name":"s","status":"connected"}],"plugin_errors":[{"plugin":"c"}]}`)
		init, ok := stream.Init()
		if !ok || len(init.Init.Plugins) != 2 || init.Init.Plugins[0].Name != "a" || init.Init.Plugins[1].Name != "b" {
			t.Fatalf("init = %+v", init.Init)
		}
		if init.Init.Servers[0].Status != "connected" || len(init.Init.PluginErrors) != 1 {
			t.Fatalf("init = %+v", init.Init)
		}
	})

	t.Run("a tool result is read whether its content is a string or a list of text blocks [verifies SP-1501~1]", func(t *testing.T) {
		stream := decode(t,
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"a","content":"plain"}]}}`,
			`{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"b","is_error":true,"content":[{"type":"text","text":"one"},{"type":"text","text":"two"}]}]}}`)
		first, _ := stream.ResultOf("a")
		second, _ := stream.ResultOf("b")
		if first.Text != "plain" || first.IsError || second.Text != "one\ntwo" || !second.IsError {
			t.Fatalf("results %+v %+v", first, second)
		}
		if _, found := stream.ResultOf("c"); found {
			t.Error("a result was found for a call that has none")
		}
	})

	t.Run("a subagent's events are those whose parent is the launching call [verifies SP-1501~1]", func(t *testing.T) {
		stream := decode(t,
			`{"type":"assistant","message":{"content":[{"type":"tool_use","id":"agent","name":"Agent","input":{}}]},"parent_tool_use_id":null}`,
			`{"type":"assistant","message":{"model":"m","content":[{"type":"tool_use","id":"inner","name":"X","input":{}}]},"parent_tool_use_id":"agent"}`)
		if children := stream.Children("agent"); len(children) != 1 || children[0].Model != "m" {
			t.Fatalf("children = %+v", children)
		}
		uses := stream.ToolUses("")
		if len(uses) != 2 || uses[0].Parent != "" || uses[1].Parent != "agent" {
			t.Fatalf("uses = %+v", uses)
		}
	})

	t.Run("the closing event carries turns, cost and models [verifies SP-1501~1]", func(t *testing.T) {
		stream := decode(t, `{"type":"result","subtype":"success","num_turns":4,"total_cost_usd":0.5,"modelUsage":{"m":{}},"permission_denials":[{"tool_name":"X"}]}`)
		final, ok := stream.Final()
		if !ok || final.Result.NumTurns != 4 || final.Result.TotalCostUSD != 0.5 || len(final.Result.Models) != 1 || len(final.Result.PermissionDenials) != 1 {
			t.Fatalf("final = %+v", final.Result)
		}
	})

	t.Run("an event of a shape nobody documented is kept with no type [verifies SP-1501~1]", func(t *testing.T) {
		stream := decode(t, `[1,2,3]`, `"text"`, `{"unexpected":true}`)
		if len(stream.Events) != 3 || stream.Events[0].Type != "" {
			t.Fatalf("events = %+v", stream.Events)
		}
	})
}

func FuzzDecodeClaude(f *testing.F) {
	f.Add(`{"type":"system","subtype":"init","plugins":["a"]}`)
	f.Add(`{"type":"assistant","message":{"content":"text"}}`)
	f.Add(`{"type":"user","message":{"content":[{"type":"tool_result","content":[1]}]}}`)
	f.Fuzz(func(t *testing.T, line string) {
		if !json.Valid([]byte(line)) {
			t.Skip()
		}
		stream := evidence.DecodeClaude([]evidence.Line{{Number: 1, Raw: json.RawMessage(line)}})
		_ = stream.ToolUses("")
		_, _ = stream.Init()
		_, _ = stream.Final()
	})
}

func FuzzDecodeCodex(f *testing.F) {
	f.Add(`{"type":"item.completed","item":{"type":"command_execution","exit_code":1}}`)
	f.Add(`{"type":"item.completed","item":"x"}`)
	f.Fuzz(func(t *testing.T, line string) {
		if !json.Valid([]byte(line)) {
			t.Skip()
		}
		stream := evidence.DecodeCodex([]evidence.Line{{Number: 1, Raw: json.RawMessage(line)}})
		_ = stream.ItemsOf(evidence.CodexItemCommand)
	})
}

func TestCodexDecoder(t *testing.T) {
	t.Run("items keep their type, server, tool, command and exit code, and everything else stays searchable [verifies SP-1501~1]", func(t *testing.T) {
		stream := evidence.DecodeCodex([]evidence.Line{
			{Number: 1, Raw: json.RawMessage(`{"type":"thread.started","thread_id":"t"}`)},
			{Number: 2, Raw: json.RawMessage(`{"type":"item.completed","item":{"id":"i","type":"command_execution","command":"touch x","exit_code":1,"status":"failed","extra":"agent_type-xyz"}}`)},
			{Number: 3, Raw: json.RawMessage(`{"type":"item.completed","item":{"type":"mcp_tool_call","server":"s","tool":"fetch_request"}}`)},
		})
		commands := stream.ItemsOf(evidence.CodexItemCommand)
		if len(commands) != 1 || commands[0].Command != "touch x" || *commands[0].ExitCode != 1 || !commands[0].Mentions("agent_type-xyz") {
			t.Fatalf("commands = %+v", commands)
		}
		calls := stream.ItemsOf(evidence.CodexItemMCPCall)
		if len(calls) != 1 || calls[0].Server != "s" || calls[0].Tool != "fetch_request" || len(stream.Types) != 3 {
			t.Fatalf("calls = %+v, types %v", calls, stream.Types)
		}
	})
}

func TestHookRecords(t *testing.T) {
	files := evidence.Files{Hooks: []byte(
		`{"module":"spec-proof-dp","args":["hook","pre-tool","--harness","claude"],"stdin":{"tool_name":"X","agent_id":"a"},"stdout":"{\"hookSpecificOutput\":{\"permissionDecision\":\"allow\",\"updatedInput\":{\"cap\":\"c\"}}}","exitCode":0}` + "\n" +
			`{"module":"spec-proof-dp","args":["mcp","--role","executor"],"exitCode":0}` + "\n" +
			`{"label":"Stop-probe","stdin":{"hook_event_name":"Stop"},"exitCode":0}` + "\n")}

	t.Run("a record is a module hook, a probe or another command, and its payload and answer decode [verifies SP-1506~1]", func(t *testing.T) {
		found, err := evidence.Assemble(files)
		if err != nil {
			t.Fatal(err)
		}
		hooks := found.HookCalls("spec-proof-dp", "pre-tool")
		if len(hooks) != 1 || hooks[0].Payload().ToolName != "X" || hooks[0].Payload().AgentID != "a" || hooks[0].Decision().Permission != "allow" {
			t.Fatalf("hooks = %+v", hooks)
		}
		if got := string(hooks[0].Decision().UpdatedInput); got != `{"cap":"c"}` {
			t.Errorf("updated input = %s", got)
		}
		if found.Hooks[1].IsModuleHook() || found.Hooks[1].Kind() != "" {
			t.Errorf("an mcp command was taken for a hook: %+v", found.Hooks[1])
		}
		if probes := found.Probes(evidence.LabelStopProbe); len(probes) != 1 || probes[0].Payload().EventName != "Stop" {
			t.Fatalf("probes = %+v", probes)
		}
	})

	t.Run("an answer that is empty or not JSON decodes to no decision [verifies SP-1506~1]", func(t *testing.T) {
		for _, stdout := range []string{"", "{}", "not json"} {
			if got := (evidence.HookCall{Stdout: stdout}).Decision(); got.Permission != "" {
				t.Errorf("%q decoded to %+v", stdout, got)
			}
		}
	})
}

func TestClassifyJournal(t *testing.T) {
	const shadow = `{"event":{"kind":"classify_shadow","ticket":"t.0.1"}}` + "\n"

	t.Run("the journal of the classifying module is decoded apart from the primary journal [verifies SP-1925~1]", func(t *testing.T) {
		found, err := evidence.Assemble(evidence.Files{ClassifyJournal: []byte("\n" + shadow)})
		if err != nil {
			t.Fatal(err)
		}
		events := found.ClassifyEvents(hostproto.EventClassifyShadow)
		if len(events) != 1 || events[0].Line != 2 || len(found.Events(hostproto.EventClassifyShadow)) != 0 {
			t.Fatalf("classify events %+v, primary events %+v", events, found.Events(hostproto.EventClassifyShadow))
		}
		if pointer := evidence.ClassifyJournalAt(2, "note"); pointer.File != evidence.ClassifyJournalFile || pointer.Line != 2 {
			t.Errorf("pointer %+v", pointer)
		}
	})

	t.Run("an absent file reads as an empty journal and a corrupt line names the file [verifies SP-1925~1]", func(t *testing.T) {
		found, err := evidence.Assemble(evidence.Files{})
		if err != nil || len(found.ClassifyJournal) != 0 {
			t.Fatalf("journal %+v, error %v", found.ClassifyJournal, err)
		}
		if _, err := evidence.Assemble(evidence.Files{ClassifyJournal: []byte("{not json\n")}); err == nil || !strings.Contains(err.Error(), evidence.ClassifyJournalFile) {
			t.Errorf("error %v", err)
		}
	})
}
