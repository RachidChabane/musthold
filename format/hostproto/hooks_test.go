package hostproto_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	fetchToolName  = "mcp__plugin_spec-proof-dp_exec__fetch_request"
	submitToolName = "mcp__plugin_spec-proof-dp_exec__submit_response"
)

func decode(t *testing.T, harness string, kind hostproto.HookKind, payload string) hostproto.HookEvent {
	t.Helper()
	decoder, err := hostproto.DecoderFor(harness)
	if err != nil {
		t.Fatal(err)
	}
	event, err := decoder.Decode(kind, []byte(payload))
	if err != nil {
		t.Fatalf("%s %s: %v", harness, kind, err)
	}
	return event
}

func TestClaudeCodeHookDecoder(t *testing.T) {
	ticket := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID

	t.Run("PreToolUse on the Agent tool carries the launch input [verifies SP-127~1]", func(t *testing.T) {
		payload := `{"session_id":"s1","transcript_path":"/t.jsonl","cwd":"/p","permission_mode":"default","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"subagent_type":"spec-proof-dp:judge","description":"judge","prompt":"ticket ` + ticket.String() + `","model":"haiku","run_in_background":true},"tool_use_id":"toolu_1"}`
		event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPreTool, payload)
		if event.ToolName != "Agent" || event.SessionID != "s1" || event.ToolUseID != "toolu_1" || !hostproto.IsAgentTool(event.ToolName) {
			t.Fatalf("event = %+v", event)
		}
		input, err := hostproto.DecodeAgentInput(event.ToolInput)
		if err != nil || input.SubagentType != "spec-proof-dp:judge" || input.Model != "haiku" || !input.RunInBackground {
			t.Fatalf("input = %+v, %v", input, err)
		}
	})

	t.Run("PreToolUse on an MCP tool inside a subagent carries the agent and the tool [verifies SP-127~1]", func(t *testing.T) {
		payload := `{"hook_event_name":"PreToolUse","tool_name":"` + fetchToolName + `","tool_input":{"ticket":"` + ticket.String() + `","cap":""},"agent_id":"agent-0001","agent_type":"spec-proof-dp:judge"}`
		event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPreTool, payload)
		if event.AgentID != "agent-0001" || event.AgentType != "spec-proof-dp:judge" {
			t.Fatalf("event = %+v", event)
		}
		tool, ok := hostproto.ParseMCPToolName(event.ToolName)
		if !ok || tool.Tool != hostproto.ToolFetchRequest {
			t.Fatalf("tool = %+v", tool)
		}
		plugin, server, ok := hostproto.ParsePluginServer(tool.Server)
		if !ok || plugin != "spec-proof-dp" || server != "exec" {
			t.Fatalf("server = %q %q %v", plugin, server, ok)
		}
		if !hostproto.IsExecutorToolOf(event.ToolName, "spec-proof-dp") || hostproto.IsExecutorToolOf(event.ToolName, "spec-proof-alignment") {
			t.Fatal("the tool is not attributed to its plugin")
		}
	})

	t.Run("SubagentStart carries the agent identifier and type [verifies SP-127~1]", func(t *testing.T) {
		event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookSubagentStart, `{"hook_event_name":"SubagentStart","session_id":"s1","agent_id":"agent_123","agent_type":"spec-proof-dp:judge"}`)
		if event.AgentID != "agent_123" || event.AgentType != "spec-proof-dp:judge" || event.Kind != hostproto.HookSubagentStart {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("SubagentStop carries the transcript path and the last message [verifies SP-127~1]", func(t *testing.T) {
		event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookSubagentStop, `{"hook_event_name":"SubagentStop","agent_id":"agent_123","agent_type":"x:judge","agent_transcript_path":"/s/subagents/agent-123.jsonl","stop_hook_active":false,"last_assistant_message":"receipt"}`)
		if event.TranscriptPath != "/s/subagents/agent-123.jsonl" || event.LastMessage != "receipt" {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("PostToolUse on the Agent tool carries the resolved model and the usage [verifies SP-127~1]", func(t *testing.T) {
		payload := `{"hook_event_name":"PostToolUse","tool_name":"Agent","tool_input":{},"tool_response":{"status":"completed","agentId":"a1","content":[{"type":"text","text":"receipt"}],"resolvedModel":"claude-sonnet-5-5","totalTokens":12450,"usage":{"input_tokens":8320,"output_tokens":4130}}}`
		event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPostTool, payload)
		want := hostproto.TokenUsage{InputTokens: 8320, OutputTokens: 4130, TotalTokens: 12450}
		if event.ResolvedModel != "claude-sonnet-5-5" || event.Usage != want {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("PostToolUse on the Agent tool carries the reply text, from text blocks or a string [verifies SP-127~1]", func(t *testing.T) {
		blocks := `{"tool_name":"Agent","tool_input":{},"tool_response":{"content":[{"type":"text","text":"receipt "},{"type":"image"},{"type":"text","text":"x"}]}}`
		if event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPostTool, blocks); event.Reply != "receipt x" {
			t.Fatalf("reply = %q", event.Reply)
		}
		plain := `{"tool_name":"Agent","tool_input":{},"tool_response":{"content":"receipt y"}}`
		if event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPostTool, plain); event.Reply != "receipt y" {
			t.Fatalf("reply = %q", event.Reply)
		}
		none := `{"tool_name":"Agent","tool_input":{},"tool_response":{"content":7}}`
		if event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPostTool, none); event.Reply != "" {
			t.Fatalf("reply = %q", event.Reply)
		}
	})

	t.Run("a tool response of another shape yields no model and no usage [verifies SP-127~1]", func(t *testing.T) {
		for _, response := range []string{`"text"`, `null`, `[1]`, `{"other":true}`, ``} {
			payload := `{"tool_name":"Bash","tool_input":{}` + map[bool]string{true: `,"tool_response":` + response, false: ``}[response != ""] + `}`
			event := decode(t, hostproto.HarnessClaudeCode, hostproto.HookPostTool, payload)
			if event.ResolvedModel != "" || event.Usage != (hostproto.TokenUsage{}) {
				t.Errorf("response %q: event = %+v", response, event)
			}
		}
	})

	t.Run("a payload for another hook, without its required field, or malformed is refused [verifies SP-127~1]", func(t *testing.T) {
		decoder, _ := hostproto.DecoderFor(hostproto.HarnessClaudeCode)
		bad := []struct {
			kind    hostproto.HookKind
			payload string
		}{
			{hostproto.HookPreTool, `{"hook_event_name":"SubagentStart","tool_name":"x"}`},
			{hostproto.HookPreTool, `{"hook_event_name":"PreToolUse"}`},
			{hostproto.HookPostTool, `{}`},
			{hostproto.HookSubagentStart, `{"agent_type":"x"}`},
			{hostproto.HookSubagentStop, `{}`},
			{hostproto.HookPreTool, `[]`},
			{hostproto.HookPreTool, `{`},
			{hostproto.HookKind("other"), `{"tool_name":"x"}`},
		}
		for _, c := range bad {
			if _, err := decoder.Decode(c.kind, []byte(c.payload)); !errors.Is(err, hostproto.ErrBadHookPayload) {
				t.Errorf("%s %q: error = %v", c.kind, c.payload, err)
			}
		}
	})

	t.Run("an unknown harness has no decoder [verifies SP-127~1]", func(t *testing.T) {
		if _, err := hostproto.DecoderFor("emacs"); !errors.Is(err, hostproto.ErrUnknownHarness) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("the Agent tool was formerly named Task and MCP names split on their separators [verifies SP-127~1]", func(t *testing.T) {
		if !hostproto.IsAgentTool("Agent") || !hostproto.IsAgentTool("Task") || hostproto.IsAgentTool("Bash") {
			t.Error("agent tool names")
		}
		for _, bad := range []string{"", "Bash", "mcp__", "mcp__server", "mcp____tool", "mcp__server__"} {
			if _, ok := hostproto.ParseMCPToolName(bad); ok {
				t.Errorf("%q parsed", bad)
			}
		}
		for _, bad := range []string{"", "exec", "plugin_", "plugin_x", "plugin__exec", "plugin_x_"} {
			if _, _, ok := hostproto.ParsePluginServer(bad); ok {
				t.Errorf("%q parsed as a plugin server", bad)
			}
		}
	})
}

func TestCodexHookDecoder(t *testing.T) {
	t.Run("PreToolUse, SubagentStart and SubagentStop use the documented fields [verifies SP-128~1]", func(t *testing.T) {
		pre := decode(t, hostproto.HarnessCodex, hostproto.HookPreTool, `{"session_id":"thr_1","cwd":"/w","hook_event_name":"PreToolUse","turn_id":"turn_4","tool_name":"mcp__exec__fetch_request","tool_use_id":"call_7","tool_input":{"ticket":"t"},"permission_mode":"default"}`)
		if pre.Harness != hostproto.HarnessCodex || pre.ToolUseID != "call_7" || pre.AgentID != "" {
			t.Fatalf("pre = %+v", pre)
		}
		start := decode(t, hostproto.HarnessCodex, hostproto.HookSubagentStart, `{"hook_event_name":"SubagentStart","turn_id":"turn_4","agent_id":"agent_xyz","agent_type":"code_review"}`)
		if start.AgentID != "agent_xyz" || start.AgentType != "code_review" {
			t.Fatalf("start = %+v", start)
		}
		stop := decode(t, hostproto.HarnessCodex, hostproto.HookSubagentStop, `{"hook_event_name":"SubagentStop","agent_id":"agent_xyz","agent_type":"code_review","agent_transcript_path":"/p/t.jsonl","stop_hook_active":false,"last_assistant_message":"done"}`)
		if stop.TranscriptPath != "/p/t.jsonl" || stop.LastMessage != "done" {
			t.Fatalf("stop = %+v", stop)
		}
	})

	t.Run("PostToolUse carries the tool and its response [verifies SP-128~1]", func(t *testing.T) {
		post := decode(t, hostproto.HarnessCodex, hostproto.HookPostTool, `{"hook_event_name":"PostToolUse","tool_name":"apply_patch","tool_use_id":"c","tool_input":{},"tool_response":{"output":"ok"}}`)
		if post.ToolName != "apply_patch" {
			t.Fatalf("post = %+v", post)
		}
	})

	t.Run("a payload of another hook is refused [verifies SP-128~1]", func(t *testing.T) {
		decoder, _ := hostproto.DecoderFor(hostproto.HarnessCodex)
		if _, err := decoder.Decode(hostproto.HookSubagentStop, []byte(`{"hook_event_name":"PreToolUse","agent_id":"a"}`)); !errors.Is(err, hostproto.ErrBadHookPayload) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestCopilotHookDecoder(t *testing.T) {
	t.Run("preToolUse carries the camel-case fields and its arguments as a JSON string [verifies SP-128~1]", func(t *testing.T) {
		event := decode(t, hostproto.HarnessCopilot, hostproto.HookPreTool, `{"sessionId":"s","timestamp":1234,"cwd":"/p","toolName":"spec-proof-dp-exec/fetch_request","toolArgs":"{\"ticket\":\"t\"}","agentType":"spec-proof-dp:judge"}`)
		if event.Harness != hostproto.HarnessCopilot || event.ToolName != "spec-proof-dp-exec/fetch_request" || event.AgentType != "spec-proof-dp:judge" {
			t.Fatalf("event = %+v", event)
		}
		var args map[string]string
		if err := json.Unmarshal(event.ToolInput, &args); err != nil || args["ticket"] != "t" {
			t.Fatalf("args = %s, %v", event.ToolInput, err)
		}
	})

	t.Run("arguments given as an object are kept as they are [verifies SP-128~1]", func(t *testing.T) {
		event := decode(t, hostproto.HarnessCopilot, hostproto.HookPreTool, `{"toolName":"bash","toolArgs":{"command":"ls"}}`)
		if string(event.ToolInput) != `{"command":"ls"}` {
			t.Fatalf("args = %s", event.ToolInput)
		}
	})

	t.Run("subagentStop carries the response and the transcript path [verifies SP-128~1]", func(t *testing.T) {
		event := decode(t, hostproto.HarnessCopilot, hostproto.HookSubagentStop, `{"sessionId":"s","timestamp":1,"cwd":"/p","transcriptPath":"/t.jsonl","agentId":"a1","agentType":"x:judge","agentName":"judge","response":"receipt","stopReason":"end_turn"}`)
		if event.AgentID != "a1" || event.TranscriptPath != "/t.jsonl" || event.LastMessage != "receipt" {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a payload without its required field is refused [verifies SP-128~1]", func(t *testing.T) {
		decoder, _ := hostproto.DecoderFor(hostproto.HarnessCopilot)
		for _, c := range []struct {
			kind    hostproto.HookKind
			payload string
		}{
			{hostproto.HookPreTool, `{}`}, {hostproto.HookSubagentStop, `{"response":"x"}`}, {hostproto.HookPreTool, `[`},
		} {
			if _, err := decoder.Decode(c.kind, []byte(c.payload)); !errors.Is(err, hostproto.ErrBadHookPayload) {
				t.Errorf("%s %q: error = %v", c.kind, c.payload, err)
			}
		}
	})
}

func TestHookDecisions(t *testing.T) {
	ticket := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID
	launch := func(prompt, subagent string) hostproto.AgentInput {
		return hostproto.AgentInput{SubagentType: subagent, Description: "judge", Prompt: prompt, Model: "haiku", RunInBackground: true}
	}

	t.Run("an executor launch is rebuilt from its role and its single ticket [verifies SP-129~1]", func(t *testing.T) {
		got, err := launch("Please note the tests are in tests/. "+ticket.String(), "spec-proof-dp:judge").Sanitize("spec-proof-dp")
		if err != nil {
			t.Fatal(err)
		}
		want := hostproto.AgentInput{SubagentType: "spec-proof-dp:judge", Description: "judge", Prompt: "ticket " + ticket.String()}
		if got != want {
			t.Fatalf("got %+v, want %+v", got, want)
		}
	})

	t.Run("a launch without a single ticket or of another plugin's role is refused [verifies SP-129~1]", func(t *testing.T) {
		if _, err := launch("no ticket here", "spec-proof-dp:judge").Sanitize("spec-proof-dp"); !errors.Is(err, hostproto.ErrNoSingleTicket) {
			t.Errorf("no ticket: %v", err)
		}
		if _, err := launch(ticket.String(), "spec-proof-alignment:judge").Sanitize("spec-proof-dp"); !errors.Is(err, hostproto.ErrNotExecutorRole) {
			t.Errorf("another plugin: %v", err)
		}
		if _, err := launch(ticket.String(), "general-purpose").Sanitize("spec-proof-dp"); !errors.Is(err, hostproto.ErrNotExecutorRole) {
			t.Errorf("not a role: %v", err)
		}
	})

	t.Run("the capability is injected into the tool input without losing its members [verifies SP-129~1]", func(t *testing.T) {
		injected, err := hostproto.InjectCap(json.RawMessage(`{"ticket":"t","cap":""}`), "agent.mac")
		if err != nil {
			t.Fatal(err)
		}
		var members map[string]string
		if err := json.Unmarshal(injected, &members); err != nil || members["cap"] != "agent.mac" || members["ticket"] != "t" || len(members) != 2 {
			t.Fatalf("injected = %s, %v", injected, err)
		}
		if empty, err := hostproto.InjectCap(nil, "c"); err != nil || string(empty) != `{"cap":"c"}` {
			t.Fatalf("empty input: %s, %v", empty, err)
		}
		if _, err := hostproto.InjectCap(json.RawMessage(`[1]`), "c"); !errors.Is(err, hostproto.ErrBadHookPayload) {
			t.Fatalf("a non-object input: %v", err)
		}
	})

	t.Run("a pre-tool decision is rendered in the output format of each harness [verifies SP-129~1]", func(t *testing.T) {
		decision := hostproto.PreToolDecision{Permission: hostproto.PermissionAllow, Reason: "ok", UpdatedInput: json.RawMessage(`{"cap":"c"}`)}
		for _, harness := range []string{hostproto.HarnessClaudeCode, hostproto.HarnessCodex} {
			encoded, err := hostproto.EncodePreToolDecision(harness, decision)
			if err != nil {
				t.Fatal(err)
			}
			const want = `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"allow","permissionDecisionReason":"ok","updatedInput":{"cap":"c"}}}`
			if string(encoded) != want {
				t.Errorf("%s: %s", harness, encoded)
			}
		}
		encoded, err := hostproto.EncodePreToolDecision(hostproto.HarnessCopilot, decision)
		if err != nil || string(encoded) != `{"permissionDecision":"allow","permissionDecisionReason":"ok","modifiedArgs":{"cap":"c"}}` {
			t.Errorf("copilot: %s, %v", encoded, err)
		}
		if _, err := hostproto.EncodePreToolDecision("emacs", decision); !errors.Is(err, hostproto.ErrUnknownHarness) {
			t.Errorf("unknown harness: %v", err)
		}
	})

	t.Run("a stop decision blocks with a reason or says nothing [verifies SP-129~1]", func(t *testing.T) {
		for _, harness := range []string{hostproto.HarnessClaudeCode, hostproto.HarnessCodex, hostproto.HarnessCopilot} {
			blocked, err := hostproto.EncodeStopDecision(harness, hostproto.StopDecision{Block: true, Reason: "why"})
			if err != nil || string(blocked) != `{"decision":"block","reason":"why"}` {
				t.Errorf("%s: %s, %v", harness, blocked, err)
			}
			allowed, err := hostproto.EncodeStopDecision(harness, hostproto.StopDecision{Reason: "ignored"})
			if err != nil || string(allowed) != `{}` {
				t.Errorf("%s: %s, %v", harness, allowed, err)
			}
		}
		if _, err := hostproto.EncodeStopDecision("emacs", hostproto.StopDecision{}); !errors.Is(err, hostproto.ErrUnknownHarness) {
			t.Errorf("unknown harness: %v", err)
		}
	})
}

func TestTranscripts(t *testing.T) {
	ticket := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID
	call := func(name string, ticketText string) string {
		return `{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"x"},{"type":"tool_use","id":"toolu","name":"` + name + `","input":{"ticket":"` + ticketText + `"}}]}}`
	}
	user := `{"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu","content":"ok"}]}}`
	transcript := func(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

	t.Run("the tool calls are read from the assistant messages in order [verifies SP-130~1]", func(t *testing.T) {
		calls, err := hostproto.DecodeTranscript(strings.NewReader(transcript(call(fetchToolName, ticket.String()), user, "", call(submitToolName, ticket.String()))))
		if err != nil || len(calls) != 2 || calls[0].Name != fetchToolName || calls[1].Name != submitToolName {
			t.Fatalf("calls = %+v, %v", calls, err)
		}
	})

	t.Run("a line that is not JSON is refused and an oversized one is bounded [verifies SP-130~1]", func(t *testing.T) {
		if _, err := hostproto.DecodeTranscript(strings.NewReader("{\n")); !errors.Is(err, hostproto.ErrBadTranscript) {
			t.Errorf("not JSON: %v", err)
		}
		huge := strings.Repeat("a", hostproto.MaxTranscriptLineBytes+1)
		if _, err := hostproto.DecodeTranscript(strings.NewReader(huge)); !errors.Is(err, hostproto.ErrBadTranscript) {
			t.Errorf("oversized: %v", err)
		}
	})

	checked := func(lines ...string) error {
		calls, err := hostproto.DecodeTranscript(strings.NewReader(transcript(lines...)))
		if err != nil {
			t.Fatal(err)
		}
		return hostproto.CheckExecutorTranscript(calls, ticket)
	}

	t.Run("fetch then submit, with at most one corrective submit, is the only attested transcript [verifies SP-130~1]", func(t *testing.T) {
		fetch, submit := call(fetchToolName, ticket.String()), call(submitToolName, ticket.String())
		if err := checked(fetch, user, submit); err != nil {
			t.Errorf("fetch, submit: %v", err)
		}
		if err := checked(fetch, submit, submit); err != nil {
			t.Errorf("fetch, submit, correction: %v", err)
		}
		deviations := map[string][]string{
			"nothing":                         {},
			"only a fetch":                    {fetch},
			"two corrections":                 {fetch, submit, submit, submit},
			"submit before fetch":             {submit, fetch},
			"a shell call":                    {fetch, call("Bash", ticket.String()), submit},
			"another ticket":                  {fetch, call(submitToolName, strings.Replace(ticket.String(), ".1", ".2", 1))},
			"a second fetch":                  {fetch, fetch},
			"a read of the tests":             {fetch, call("Read", ticket.String())},
			"a fetch of a ticket not its own": {call(fetchToolName, "other"), submit},
		}
		for name, lines := range deviations {
			if err := checked(lines...); !errors.Is(err, hostproto.ErrTranscriptDeviation) {
				t.Errorf("%s: error = %v", name, err)
			}
		}
	})

	t.Run("the transcript digest is the SHA-256 of its bytes [verifies SP-130~1]", func(t *testing.T) {
		if hostproto.TranscriptDigest([]byte("a")) == hostproto.TranscriptDigest([]byte("b")) || len(hostproto.TranscriptDigest(nil)) != 64 {
			t.Fatal("digests")
		}
	})
}
