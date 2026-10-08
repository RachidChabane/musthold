package evidence_test

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

func decodeCopilot(lines ...string) evidence.CopilotStream {
	var raw []evidence.Line
	for index, line := range lines {
		raw = append(raw, evidence.Line{Number: index + 1, Raw: json.RawMessage(line)})
	}
	return evidence.DecodeCopilot(raw)
}

func TestCopilotStreamDecoder(t *testing.T) {
	t.Run("a tool start carries its server, its tool and the sub-agent that originated it [verifies SP-1501~1]", func(t *testing.T) {
		stream := decodeCopilot(
			`{"type":"tool.execution_start","agentId":"a1","data":{"toolCallId":"c","toolName":"s/t","mcpServerName":"s","mcpToolName":"t","arguments":{"x":1}}}`,
			`{"type":"tool.execution_complete","data":{"toolCallId":"c","success":false}}`)
		start := stream.Of(evidence.CopilotEventToolStart)[0]
		if start.AgentID != "a1" || start.ToolName != "s/t" || start.MCPServer != "s" || start.MCPTool != "t" || start.ToolCallID != "c" {
			t.Fatalf("start = %+v", start)
		}
		done, found := stream.CompletionOf("c")
		if !found || done.Success == nil || *done.Success {
			t.Fatalf("completion = %+v, %v", done, found)
		}
		if _, found := stream.CompletionOf(""); found {
			t.Error("a completion was found for no call")
		}
	})

	t.Run("a record keeps every member it has, known or not, and one of an unexpected shape decodes as far as it can [verifies SP-1501~1]", func(t *testing.T) {
		stream := decodeCopilot(`{"type":"future.event","data":{"toolName":7,"success":"yes","mcpToolName":"kept"},"novel":[1]}`, `[1,2]`, `{"type":3}`)
		if len(stream.Events) != 3 {
			t.Fatalf("%d events", len(stream.Events))
		}
		first := stream.Events[0]
		if first.Type != "future.event" || first.ToolName != "" || first.Success != nil || first.MCPTool != "kept" || !first.Mentions("novel") {
			t.Fatalf("first = %+v", first)
		}
		if stream.Events[1].Type != "" || stream.Events[2].Type != "" {
			t.Errorf("events = %+v", stream.Events[1:])
		}
	})

	t.Run("the closing event is the result record with exit code 0 and it reports the premium requests [verifies SP-1518~1]", func(t *testing.T) {
		if _, closed := decodeCopilot(`{"type":"result","exitCode":1,"usage":{"premiumRequests":1}}`).Closing(); closed {
			t.Error("a result with a failing exit code closed the session")
		}
		if _, closed := decodeCopilot(`{"type":"assistant.idle","data":{}}`, `{"type":"result","usage":{}}`).Closing(); closed {
			t.Error("a result with no exit code, or an idle event, closed the session")
		}
		event, closed := decodeCopilot(`{"type":"assistant.idle","data":{}}`, `{"type":"result","exitCode":0,"usage":{"premiumRequests":3}}`).Closing()
		if !closed || event.Line != 2 || event.PremiumRequests == nil || *event.PremiumRequests != 3 {
			t.Errorf("result: %+v, %v", event, closed)
		}
	})

	t.Run("a refusal is an authentication, quota or rate limit error and no other [verifies SP-1501~1]", func(t *testing.T) {
		if _, refused := decodeCopilot(`{"type":"session.error","data":{"errorType":"tool","message":"m"}}`).Refusal(); refused {
			t.Error("a tool error is a refusal")
		}
		event, refused := decodeCopilot(`{"type":"session.error","data":{"errorType":"quota","message":"out of credits"}}`).Refusal()
		if !refused || event.Message != "out of credits" {
			t.Errorf("refusal = %+v, %v", event, refused)
		}
	})
}

func TestCopilotListingDecoder(t *testing.T) {
	t.Run("the plugin and skill listings are read with the members the CLI prints and the others ignored [verifies SP-1501~1]", func(t *testing.T) {
		listing := evidence.CopilotListing{
			PluginsOutput: `[{"name":"p","marketplace":"","version":"0.1.0","enabled":true,"source":"external"}]`,
			SkillsOutput:  `[{"name":"s","description":"d","source":"plugin","path":"/x","enabled":false}]`,
		}
		plugins, pluginsOK := listing.Plugins()
		skills, skillsOK := listing.Skills()
		if !pluginsOK || !skillsOK || plugins[0].Name != "p" || plugins[0].Source != "external" || plugins[0].Enabled == nil || !*plugins[0].Enabled {
			t.Fatalf("plugins %+v %v", plugins, pluginsOK)
		}
		if skills[0].Name != "s" || skills[0].Path != "/x" || *skills[0].Enabled {
			t.Fatalf("skills %+v", skills)
		}
	})

	t.Run("an output that is not a JSON list is reported as such [verifies SP-1501~1]", func(t *testing.T) {
		if _, ok := (evidence.CopilotListing{PluginsOutput: "No plugins"}).Plugins(); ok {
			t.Error("plain text was read as a listing")
		}
		if _, ok := (evidence.CopilotListing{}).Skills(); ok {
			t.Error("an empty output was read as a listing")
		}
	})
}

func TestInspectPluginTree(t *testing.T) {
	skill := []byte("---\nname: \"spec-proof-dp-fulfil\"\ndescription: d\n---\nbody\n")
	files := evidence.TreeFiles{
		Manifest:  []byte(`{"name":"spec-proof-dp","version":"0.1.0"}`),
		MCP:       []byte(`{"mcpServers":{"spec-proof-dp":{"type":"stdio"}}}`),
		Skills:    map[string][]byte{"spec-proof-dp-fulfil": skill},
		Namespace: []string{"com.github.copilot/hooks/hooks.json"},
	}

	t.Run("a tree of the expected shape has no problem [verifies SP-1707~2]", func(t *testing.T) {
		tree := evidence.InspectPluginTree("/t", files)
		if len(tree.Problems) != 0 || tree.Name != "spec-proof-dp" || !reflect.DeepEqual(tree.Servers, []string{"spec-proof-dp"}) {
			t.Fatalf("tree = %+v", tree)
		}
		if len(tree.Skills) != 1 || tree.Skills[0].Name != "spec-proof-dp-fulfil" || tree.Skills[0].Dir != "spec-proof-dp-fulfil" {
			t.Fatalf("skills = %+v", tree.Skills)
		}
	})

	t.Run("each shape the standard does not allow is a problem [verifies SP-1707~2]", func(t *testing.T) {
		for name, edit := range map[string]func(*evidence.TreeFiles){
			"a missing manifest":             func(f *evidence.TreeFiles) { f.Manifest = nil },
			"a manifest that is a list":      func(f *evidence.TreeFiles) { f.Manifest = []byte(`[1]`) },
			"a manifest that is a string":    func(f *evidence.TreeFiles) { f.Manifest = []byte(`"text"`) },
			"a manifest with no name":        func(f *evidence.TreeFiles) { f.Manifest = []byte(`{"version":"1"}`) },
			"a missing mcp.json":             func(f *evidence.TreeFiles) { f.MCP = nil },
			"an mcp.json that is not JSON":   func(f *evidence.TreeFiles) { f.MCP = []byte(`not json`) },
			"an mcp.json with no mcpServers": func(f *evidence.TreeFiles) { f.MCP = []byte(`{}`) },
			"a skill in another directory":   func(f *evidence.TreeFiles) { f.Skills = map[string][]byte{"other": skill} },
			"a skill with no frontmatter":    func(f *evidence.TreeFiles) { f.Skills = map[string][]byte{"x": []byte("no frontmatter")} },
		} {
			edited := files
			edit(&edited)
			tree := evidence.InspectPluginTree("/t", edited)
			if len(tree.Problems) == 0 {
				t.Errorf("%s: no problem reported", name)
			}
		}
	})
}

func TestCopilotHookPayload(t *testing.T) {
	t.Run("the camelCase payload is read by the module's own decoder, tool arguments as a string included [verifies SP-1501~1]", func(t *testing.T) {
		call := evidence.HookCall{Module: "spec-proof-dp", Args: []string{"hook", "pre-tool", "--harness", "copilot"},
			Stdin: json.RawMessage(`{"sessionId":"s","toolName":"spec-proof-dp-exec/fetch_request","toolArgs":"{\"ticket\":\"t\"}","agentId":"a","agentType":"spec-proof-dp-decider"}`)}
		event := call.CopilotHook()
		if event.ToolName != "spec-proof-dp-exec/fetch_request" || event.AgentType != "spec-proof-dp-decider" || event.AgentID != "a" || string(event.ToolInput) != `{"ticket":"t"}` {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a payload that is not JSON decodes to nothing and never panics [verifies SP-1501~1]", func(t *testing.T) {
		call := evidence.HookCall{Args: []string{"hook", "pre-tool"}, Stdin: json.RawMessage(`"plain text"`)}
		if event := call.CopilotHook(); event.ToolName != "" {
			t.Fatalf("event = %+v", event)
		}
	})
}
