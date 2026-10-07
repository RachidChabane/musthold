package hooks_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/app/hooks"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/executorrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

const (
	transcriptPath = "/runs/agent-0001.jsonl"
	agentID        = "agent-0001"
	otherAgentID   = "agent-0002"
	judgeRole      = "spec-proof-dp:judge"
	deciderRole    = "spec-proof-dp:decider"
)

// memoryTranscripts serves transcripts from memory.
type memoryTranscripts map[string][]byte

func (m memoryTranscripts) Read(path string) ([]byte, error) {
	content, found := m[path]
	if !found {
		return nil, errors.New("no such transcript")
	}
	return content, nil
}

// failingStore fails every journal append and secret read.
type failingStore struct{ *hostprototest.MemoryStore }

func (failingStore) Secret() ([]byte, error) { return nil, errors.New("secret unreadable") }
func (failingStore) Append(hostproto.Event) (hostproto.Record, error) {
	return hostproto.Record{}, errors.New("journal unwritable")
}

type fixture struct {
	t           *testing.T
	store       *hostprototest.MemoryStore
	transcripts memoryTranscripts
	ticket      hostproto.Ticket
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	request := hostprototest.Request()
	store := hostprototest.NewMemoryStore()
	ticket := hostprototest.Ticket(request, 0, 1)
	schemaDigest, err := request.SchemaDigest()
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutEntry(hostproto.NewEntry(ticket, request.Role, schemaDigest, hostproto.DefaultMaxEmissions)); err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, store: store, transcripts: memoryTranscripts{}, ticket: ticket}
}

func (f *fixture) handler(harness hostharness.Harness) hooks.Handler {
	return hooks.Handler{Harness: harness, Open: f.open, Transcripts: f.transcripts}
}

func (f *fixture) open() (hooks.Store, error) { return f.store, nil }

func (f *fixture) events() []hostproto.Event {
	records, err := f.store.Records()
	if err != nil {
		f.t.Fatal(err)
	}
	events := make([]hostproto.Event, len(records))
	for index, record := range records {
		events[index] = record.Event
	}
	return events
}

func (f *fixture) kinds() []hostproto.EventKind {
	var kinds []hostproto.EventKind
	for _, event := range f.events() {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func (f *fixture) lastEvent(kind hostproto.EventKind) hostproto.Event {
	f.t.Helper()
	events := f.events()
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Kind == kind {
			return events[index]
		}
	}
	f.t.Fatalf("no %s event in %v", kind, events)
	return hostproto.Event{}
}

func (f *fixture) ticketText() string { return f.ticket.TicketID.String() }

// decision is the part of a PreToolUse answer the tests look at.
type decision struct {
	Permission string
	Reason     string
	Updated    map[string]any
}

func claudeDecision(t *testing.T, output []byte) decision {
	t.Helper()
	var parsed struct {
		Output struct {
			Permission string         `json:"permissionDecision"`
			Reason     string         `json:"permissionDecisionReason"`
			Updated    map[string]any `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal(output, &parsed); err != nil {
		t.Fatalf("output %q: %v", output, err)
	}
	return decision{Permission: parsed.Output.Permission, Reason: parsed.Output.Reason, Updated: parsed.Output.Updated}
}

func marshal(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func agentLaunch(t *testing.T, subagentType, prompt string, extra map[string]any) string {
	t.Helper()
	input := map[string]any{"subagent_type": subagentType, "description": "judge a mutant", "prompt": prompt}
	for key, value := range extra {
		input[key] = value
	}
	return marshal(t, map[string]any{"hook_event_name": "PreToolUse", "tool_name": "Agent", "tool_input": input})
}

func TestLaunchRewrite(t *testing.T) {
	claude := hostharness.Claude()

	t.Run("a launch of an executor is rebuilt to the bare ticket, without the model and in the foreground [verifies SP-233~1]", func(t *testing.T) {
		f := newFixture(t)
		payload := agentLaunch(t, judgeRole, "Please be quick. "+executorrun.Receipt(f.ticket.TicketID)+" thanks", map[string]any{"model": "haiku", "run_in_background": true})
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(payload))
		if err != nil {
			t.Fatal(err)
		}
		got := claudeDecision(t, output)
		want := map[string]any{
			"subagent_type": judgeRole, "description": "judge a mutant",
			"prompt": hostproto.ExecutorPrompt(f.ticket.TicketID), "run_in_background": false,
		}
		if got.Permission != "allow" || fmt.Sprint(got.Updated) != fmt.Sprint(want) {
			t.Fatalf("decision = %+v, want updated %v", got, want)
		}
	})

	t.Run("a prompt without exactly one ticket is denied [verifies SP-233~1]", func(t *testing.T) {
		f := newFixture(t)
		other := hostproto.ExecutorPrompt(hostproto.TicketID{RequestID: f.ticket.RequestID, Vote: 1, Emission: 1})
		for name, prompt := range map[string]string{
			"none": "judge the mutant", "two": hostproto.ExecutorPrompt(f.ticket.TicketID) + " " + other,
		} {
			output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(agentLaunch(t, judgeRole, prompt, nil)))
			if err != nil {
				t.Fatal(err)
			}
			if got := claudeDecision(t, output); got.Permission != "deny" || !strings.Contains(got.Reason, hostproto.ErrNoSingleTicket.Error()) {
				t.Errorf("%s: decision = %+v", name, got)
			}
		}
	})

	t.Run("a ticket that is unknown, or for another role, is denied [verifies SP-233~1]", func(t *testing.T) {
		f := newFixture(t)
		unknown := hostproto.TicketID{RequestID: hostprototest.MustID(hostprototest.ClosedRequest()), Emission: 1}
		for name, launch := range map[string]string{
			"unknown ticket": agentLaunch(t, judgeRole, hostproto.ExecutorPrompt(unknown), nil),
			"another role":   agentLaunch(t, deciderRole, hostproto.ExecutorPrompt(f.ticket.TicketID), nil),
		} {
			output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(launch))
			if err != nil {
				t.Fatal(err)
			}
			if got := claudeDecision(t, output); got.Permission != "deny" {
				t.Errorf("%s: decision = %+v", name, got)
			}
		}
	})

	t.Run("a launch of any other subagent, and any other tool, is left alone [verifies SP-233~1]", func(t *testing.T) {
		f := newFixture(t)
		for name, payload := range map[string]string{
			"general purpose":    agentLaunch(t, "general-purpose", "do things", nil),
			"another plugin":     agentLaunch(t, "other-plugin:judge", "ticket x", nil),
			"a shell command":    marshal(t, map[string]any{"tool_name": "Bash", "tool_input": map[string]string{"command": "ls"}}),
			"a malformed launch": marshal(t, map[string]any{"tool_name": "Agent", "tool_input": "text"}),
		} {
			output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(payload))
			if err != nil || len(output) != 0 {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
		if len(f.events()) != 0 {
			t.Fatalf("events = %v", f.events())
		}
	})

	t.Run("the former name of the launch tool is rewritten as well [verifies SP-233~1]", func(t *testing.T) {
		f := newFixture(t)
		payload := strings.Replace(agentLaunch(t, judgeRole, hostproto.ExecutorPrompt(f.ticket.TicketID), nil), `"Agent"`, `"Task"`, 1)
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(payload))
		if err != nil || claudeDecision(t, output).Permission != "allow" {
			t.Fatalf("output %q, error %v", output, err)
		}
	})
}

func executorCall(t *testing.T, harness hostharness.Harness, ticket, agent, agentType string) string {
	t.Helper()
	payload := map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": harness.ToolName(hostproto.ToolFetchRequest),
		"tool_input": map[string]string{"ticket": ticket, "cap": ""},
	}
	if agent != "" {
		payload["agent_id"] = agent
	}
	if agentType != "" {
		payload["agent_type"] = agentType
	}
	return marshal(t, payload)
}

func TestExecutorToolGuard(t *testing.T) {
	claude := hostharness.Claude()

	t.Run("the executor of the ticket's role gets its capability added to the call [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(executorCall(t, claude, f.ticketText(), agentID, judgeRole)))
		if err != nil {
			t.Fatal(err)
		}
		got := claudeDecision(t, output)
		capability, _ := got.Updated["cap"].(string)
		verified, err := hostproto.VerifyCap(hostprototest.Secret(), capability)
		if got.Permission != "allow" || err != nil || verified != agentID || got.Updated["ticket"] != f.ticketText() {
			t.Fatalf("decision = %+v (cap error %v)", got, err)
		}
		if len(f.events()) != 0 {
			t.Fatalf("an allowed call journaled %v", f.kinds())
		}
	})

	t.Run("a call of the main agent is denied and journaled as an exposure attempt [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(executorCall(t, claude, f.ticketText(), "", "")))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
		if event := f.lastEvent(hostproto.EventExposureAttempt); event.Ticket != f.ticketText() {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a call of an executor of another role is denied and journaled with its agent [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(executorCall(t, claude, f.ticketText(), otherAgentID, deciderRole)))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
		if event := f.lastEvent(hostproto.EventExposureAttempt); event.AgentID != otherAgentID || event.Role != deciderRole {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a call naming no ticket of the module is denied [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		unknown := hostproto.TicketID{RequestID: hostprototest.MustID(hostprototest.ClosedRequest()), Emission: 1}
		for name, ticket := range map[string]string{"malformed": "not-a-ticket", "unknown": unknown.String()} {
			output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(executorCall(t, claude, ticket, agentID, judgeRole)))
			if err != nil || claudeDecision(t, output).Permission != "deny" {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
	})

	t.Run("a call that cannot be checked is denied, never allowed [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		handler := hooks.Handler{Harness: claude, Open: func() (hooks.Store, error) { return failingStore{f.store}, nil }, Transcripts: f.transcripts}
		output, err := handler.Handle(hostproto.HookPreTool, []byte(executorCall(t, claude, f.ticketText(), agentID, judgeRole)))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
	})

	t.Run("the drain tool is not a ticket tool and is left to the main agent [verifies SP-234~1]", func(t *testing.T) {
		f := newFixture(t)
		payload := marshal(t, map[string]any{"tool_name": claude.ToolName(hostproto.ToolDrain), "tool_input": map[string]string{"module": "dp"}})
		output, err := f.handler(claude).Handle(hostproto.HookPreTool, []byte(payload))
		if err != nil || len(output) != 0 {
			t.Fatalf("output %q, error %v", output, err)
		}
	})
}

func TestSubagentStart(t *testing.T) {
	t.Run("the start of an executor is journaled with its agent and role [verifies SP-235~1]", func(t *testing.T) {
		f := newFixture(t)
		payload := marshal(t, map[string]any{"hook_event_name": "SubagentStart", "agent_id": agentID, "agent_type": judgeRole})
		if _, err := f.handler(hostharness.Claude()).Handle(hostproto.HookSubagentStart, []byte(payload)); err != nil {
			t.Fatal(err)
		}
		records, _ := f.store.Records()
		if !hostproto.JournalRoster(records).Started(agentID, judgeRole) {
			t.Fatalf("the roster does not show the agent: %v", f.events())
		}
	})

	t.Run("the start of any other agent is not journaled [verifies SP-235~1]", func(t *testing.T) {
		f := newFixture(t)
		for _, agentType := range []string{"general-purpose", "other-plugin:judge", "spec-proof-dp:boss", ""} {
			payload := marshal(t, map[string]any{"hook_event_name": "SubagentStart", "agent_id": agentID, "agent_type": agentType})
			if _, err := f.handler(hostharness.Claude()).Handle(hostproto.HookSubagentStart, []byte(payload)); err != nil {
				t.Fatal(err)
			}
		}
		if len(f.events()) != 0 {
			t.Fatalf("events = %v", f.events())
		}
	})

	t.Run("a payload without an agent identifier is refused [verifies SP-235~1]", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.handler(hostharness.Claude()).Handle(hostproto.HookSubagentStart, []byte(`{"agent_type":"spec-proof-dp:judge"}`))
		if !errors.Is(err, hostproto.ErrBadHookPayload) {
			t.Fatalf("error = %v", err)
		}
	})
}

// executorTranscript is the JSONL transcript of a run that made the given calls.
func executorTranscript(t *testing.T, harness hostharness.Harness, calls ...hostproto.ToolCall) []byte {
	t.Helper()
	var lines []string
	for _, call := range calls {
		lines = append(lines, marshal(t, map[string]any{
			"type":    "assistant",
			"message": map[string]any{"content": []map[string]any{{"type": "tool_use", "name": call.Name, "input": call.Input}}},
		}))
	}
	return []byte(strings.Join(lines, "\n") + "\n")
}

func call(t *testing.T, harness hostharness.Harness, tool, ticket string) hostproto.ToolCall {
	t.Helper()
	return hostproto.ToolCall{Name: harness.ToolName(tool), Input: json.RawMessage(marshal(t, map[string]string{"ticket": ticket}))}
}

func stopPayload(t *testing.T, reply string) []byte { return stopPayloadOf(t, judgeRole, reply) }

func stopPayloadOf(t *testing.T, agentType, reply string) []byte {
	t.Helper()
	return []byte(marshal(t, map[string]any{
		"hook_event_name": "SubagentStop", "agent_id": agentID, "agent_type": agentType,
		"agent_transcript_path": transcriptPath, "last_assistant_message": reply,
	}))
}

func TestSubagentStop(t *testing.T) {
	claude := hostharness.Claude()

	t.Run("a run of fetch and submit that replies with the receipt is attested with its transcript digest [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		transcript := executorTranscript(t, claude,
			call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), call(t, claude, hostproto.ToolSubmitResponse, f.ticketText()))
		f.transcripts[transcriptPath] = transcript
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		event := f.lastEvent(hostproto.EventAttested)
		stored := fmt.Sprintf(`{"transcriptDigest":%q}`, hostproto.TranscriptDigest(transcript))
		if event.Ticket != f.ticketText() || event.AgentID != agentID || event.Detail != stored {
			t.Fatalf("event = %+v, want detail %s", event, stored)
		}
	})

	t.Run("one corrective submit is still attested [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, claude,
			call(t, claude, hostproto.ToolFetchRequest, f.ticketText()),
			call(t, claude, hostproto.ToolSubmitResponse, f.ticketText()), call(t, claude, hostproto.ToolSubmitResponse, f.ticketText()))
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(f.kinds(), hostproto.EventAttested) {
			t.Fatalf("events = %v", f.kinds())
		}
	})

	t.Run("a run that did anything else is journaled as an exposure and not attested [verifies SP-236~1]", func(t *testing.T) {
		other := hostproto.TicketID{RequestID: f0(t).ticket.RequestID, Vote: 1, Emission: 1}.String()
		for name, calls := range map[string]func(*fixture) []hostproto.ToolCall{
			"a shell command": func(f *fixture) []hostproto.ToolCall {
				return []hostproto.ToolCall{call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), {Name: "Bash", Input: json.RawMessage(`{"command":"ls"}`)}, call(t, claude, hostproto.ToolSubmitResponse, f.ticketText())}
			},
			"only a fetch": func(f *fixture) []hostproto.ToolCall {
				return []hostproto.ToolCall{call(t, claude, hostproto.ToolFetchRequest, f.ticketText())}
			},
			"another ticket": func(f *fixture) []hostproto.ToolCall {
				return []hostproto.ToolCall{call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), call(t, claude, hostproto.ToolSubmitResponse, other)}
			},
			"two corrections": func(f *fixture) []hostproto.ToolCall {
				submit := call(t, claude, hostproto.ToolSubmitResponse, f.ticketText())
				return []hostproto.ToolCall{call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), submit, submit, submit}
			},
		} {
			f := newFixture(t)
			f.transcripts[transcriptPath] = executorTranscript(t, claude, calls(f)...)
			if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, executorrun.Receipt(f.ticket.TicketID))); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			if kinds := f.kinds(); !slices.Contains(kinds, hostproto.EventExposed) || slices.Contains(kinds, hostproto.EventAttested) {
				t.Errorf("%s: events = %v", name, kinds)
			}
		}
	})

	t.Run("a run that never fetched a ticket is journaled as an exposure without a ticket [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, claude, hostproto.ToolCall{Name: "Read", Input: json.RawMessage(`{}`)})
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, "done")); err != nil {
			t.Fatal(err)
		}
		if event := f.lastEvent(hostproto.EventExposed); event.Ticket != "" || event.AgentID != agentID {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a reply that is more than the receipt is journaled as a leak and not attested [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, claude,
			call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), call(t, claude, hostproto.ToolSubmitResponse, f.ticketText()))
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, executorrun.Receipt(f.ticket.TicketID)+" The mutant flips a boundary.")); err != nil {
			t.Fatal(err)
		}
		if kinds := f.kinds(); !slices.Contains(kinds, hostproto.EventLeaked) || slices.Contains(kinds, hostproto.EventAttested) {
			t.Fatalf("events = %v", kinds)
		}
	})

	t.Run("a transcript that cannot be read leaves the run unattested with a trace [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, "x")); err != nil {
			t.Fatal(err)
		}
		f.transcripts[transcriptPath] = []byte("{not json\n")
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, "x")); err != nil {
			t.Fatal(err)
		}
		if kinds := f.kinds(); len(kinds) != 2 || kinds[0] != hostproto.EventRestricted || kinds[1] != hostproto.EventRestricted {
			t.Fatalf("events = %v", kinds)
		}
	})

	t.Run("the stop of any other agent is not checked [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		payload := marshal(t, map[string]any{"hook_event_name": "SubagentStop", "agent_id": agentID, "agent_type": "general-purpose", "agent_transcript_path": transcriptPath})
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, []byte(payload)); err != nil || len(f.events()) != 0 {
			t.Fatalf("events = %v, error %v", f.events(), err)
		}
	})

	t.Run("a stop hook never blocks the subagent [verifies SP-236~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, claude, hostproto.ToolCall{Name: "Read", Input: json.RawMessage(`{}`)})
		output, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, "x"))
		if err != nil || len(output) != 0 {
			t.Fatalf("output %q, error %v", output, err)
		}
	})
}

// f0 builds a fixture just for its ticket.
func f0(t *testing.T) *fixture { return newFixture(t) }

func postPayload(t *testing.T, subagentType, prompt string, response map[string]any) []byte {
	t.Helper()
	return []byte(marshal(t, map[string]any{
		"hook_event_name": "PostToolUse", "tool_name": "Agent",
		"tool_input":    map[string]any{"subagent_type": subagentType, "description": "judge", "prompt": prompt},
		"tool_response": response,
	}))
}

func TestPostToolUse(t *testing.T) {
	claude := hostharness.Claude()
	response := func(reply string) map[string]any {
		return map[string]any{
			"status": "completed", "content": []map[string]string{{"type": "text", "text": reply}},
			"resolvedModel": "claude-sonnet-5-5", "totalTokens": 12450,
			"usage": map[string]int{"input_tokens": 8320, "output_tokens": 4130},
		}
	}

	t.Run("the resolved model and the usage of an executor run are recorded against its ticket [verifies SP-237~1]", func(t *testing.T) {
		f := newFixture(t)
		prompt := hostproto.ExecutorPrompt(f.ticket.TicketID)
		if _, err := f.handler(claude).Handle(hostproto.HookPostTool, postPayload(t, judgeRole, prompt, response(executorrun.Receipt(f.ticket.TicketID)))); err != nil {
			t.Fatal(err)
		}
		event := f.lastEvent(hostproto.EventRunReported)
		var report executorrun.RunReport
		if err := json.Unmarshal([]byte(event.Detail), &report); err != nil {
			t.Fatal(err)
		}
		want := hostproto.TokenUsage{InputTokens: 8320, OutputTokens: 4130, TotalTokens: 12450}
		if event.Ticket != f.ticketText() || report.Model != "claude-sonnet-5-5" || report.Usage != want {
			t.Fatalf("event = %+v, report = %+v", event, report)
		}
		if slices.Contains(f.kinds(), hostproto.EventLeaked) {
			t.Fatalf("events = %v", f.kinds())
		}
	})

	t.Run("a reply other than the bare receipt marks the response as leaked [verifies SP-237~1]", func(t *testing.T) {
		f := newFixture(t)
		prompt := hostproto.ExecutorPrompt(f.ticket.TicketID)
		if _, err := f.handler(claude).Handle(hostproto.HookPostTool, postPayload(t, judgeRole, prompt, response("The tests are in tests/test_x.py"))); err != nil {
			t.Fatal(err)
		}
		if event := f.lastEvent(hostproto.EventLeaked); event.Ticket != f.ticketText() {
			t.Fatalf("event = %+v", event)
		}
	})

	t.Run("a response with no readable reply is recorded without a leak [verifies SP-237~1]", func(t *testing.T) {
		f := newFixture(t)
		prompt := hostproto.ExecutorPrompt(f.ticket.TicketID)
		if _, err := f.handler(claude).Handle(hostproto.HookPostTool, postPayload(t, judgeRole, prompt, map[string]any{"resolvedModel": "m"})); err != nil {
			t.Fatal(err)
		}
		if kinds := f.kinds(); len(kinds) != 1 || kinds[0] != hostproto.EventRunReported {
			t.Fatalf("events = %v", kinds)
		}
	})

	t.Run("a launch of another agent, or without a ticket, is not recorded [verifies SP-237~1]", func(t *testing.T) {
		f := newFixture(t)
		for name, payload := range map[string][]byte{
			"another agent": postPayload(t, "general-purpose", hostproto.ExecutorPrompt(f.ticket.TicketID), response("x")),
			"no ticket":     postPayload(t, judgeRole, "judge it", response("x")),
			"a shell":       []byte(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`),
		} {
			if _, err := f.handler(claude).Handle(hostproto.HookPostTool, payload); err != nil {
				t.Fatalf("%s: %v", name, err)
			}
		}
		if len(f.events()) != 0 {
			t.Fatalf("events = %v", f.events())
		}
	})
}

func TestHarnessVariants(t *testing.T) {
	codex, copilot := hostharness.Codex(), hostharness.Copilot()

	t.Run("on Codex the executor of the role gets a capability, the main agent is denied and a launch is left alone [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(codex).Handle(hostproto.HookPreTool, []byte(executorCall(t, codex, f.ticketText(), agentID, codex.AgentType(judgeRole))))
		if err != nil || claudeDecision(t, output).Updated["cap"] == nil {
			t.Fatalf("output %q, error %v", output, err)
		}
		output, err = f.handler(codex).Handle(hostproto.HookPreTool, []byte(executorCall(t, codex, f.ticketText(), "", "")))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
		output, err = f.handler(codex).Handle(hostproto.HookPreTool, []byte(agentLaunch(t, judgeRole, "judge it", map[string]any{"model": "x"})))
		if err != nil || len(output) != 0 {
			t.Fatalf("a launch was rewritten on Codex: %q, %v", output, err)
		}
	})

	t.Run("on Codex the role of an agent may come from the journal when the call does not carry its type [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		start := marshal(t, map[string]any{"hook_event_name": "SubagentStart", "agent_id": agentID, "agent_type": codex.AgentType(judgeRole)})
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStart, []byte(start)); err != nil {
			t.Fatal(err)
		}
		output, err := f.handler(codex).Handle(hostproto.HookPreTool, []byte(executorCall(t, codex, f.ticketText(), agentID, "")))
		if err != nil || claudeDecision(t, output).Updated["cap"] == nil {
			t.Fatalf("output %q, error %v", output, err)
		}
		output, err = f.handler(codex).Handle(hostproto.HookPreTool, []byte(executorCall(t, codex, f.ticketText(), otherAgentID, "")))
		if err != nil || claudeDecision(t, output).Permission != "deny" {
			t.Fatalf("an agent the journal does not know was served: %q, %v", output, err)
		}
	})

	t.Run("on Codex the stop of an executor is attested like on Claude Code [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, codex,
			call(t, codex, hostproto.ToolFetchRequest, f.ticketText()), call(t, codex, hostproto.ToolSubmitResponse, f.ticketText()))
		if _, err := f.handler(codex).Handle(hostproto.HookSubagentStop, stopPayloadOf(t, codex.AgentType(judgeRole), executorrun.Receipt(f.ticket.TicketID))); err != nil {
			t.Fatal(err)
		}
		if !slices.Contains(f.kinds(), hostproto.EventAttested) {
			t.Fatalf("events = %v", f.kinds())
		}
	})

	t.Run("on Copilot a call of the wrong role is denied in its own format and a right one gets no capability [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		call := func(agentType string) []byte {
			args := marshal(t, map[string]string{"ticket": f.ticketText(), "cap": ""})
			return []byte(marshal(t, map[string]any{
				"sessionId": "s", "toolName": copilot.ToolName(hostproto.ToolFetchRequest), "toolArgs": args, "agentType": agentType, "agentId": agentID,
			}))
		}
		output, err := f.handler(copilot).Handle(hostproto.HookPreTool, call(copilot.AgentType(deciderRole)))
		var parsed struct {
			Permission string `json:"permissionDecision"`
		}
		if err != nil || json.Unmarshal(output, &parsed) != nil || parsed.Permission != "deny" {
			t.Fatalf("output %q, error %v", output, err)
		}
		output, err = f.handler(copilot).Handle(hostproto.HookPreTool, call(copilot.AgentType(judgeRole)))
		if err != nil || len(output) != 0 {
			t.Fatalf("a capability was issued on Copilot: %q, %v", output, err)
		}
	})

	t.Run("on Copilot a call that carries no agent identity is not concluded about [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		args := marshal(t, map[string]string{"ticket": f.ticketText(), "cap": ""})
		payload := marshal(t, map[string]any{"sessionId": "s", "toolName": copilot.ToolName(hostproto.ToolFetchRequest), "toolArgs": args})
		output, err := f.handler(copilot).Handle(hostproto.HookPreTool, []byte(payload))
		if err != nil || len(output) != 0 || len(f.events()) != 0 {
			t.Fatalf("output %q, error %v, events %v", output, err, f.events())
		}
	})

	t.Run("Copilot has no start hook and no run report to serve [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		start := marshal(t, map[string]any{"agentId": agentID, "agentType": copilot.AgentType(judgeRole)})
		if _, err := f.handler(copilot).Handle(hostproto.HookSubagentStart, []byte(start)); err != nil || len(f.events()) != 0 {
			t.Fatalf("events = %v, error %v", f.events(), err)
		}
		post := marshal(t, map[string]any{"toolName": "Agent", "toolArgs": map[string]any{"subagent_type": judgeRole, "prompt": hostproto.ExecutorPrompt(f.ticket.TicketID)}})
		if _, err := f.handler(copilot).Handle(hostproto.HookPostTool, []byte(post)); err != nil || len(f.events()) != 0 {
			t.Fatalf("events = %v, error %v", f.events(), err)
		}
	})

	t.Run("a hook the module does not serve is refused [verifies SP-238~1]", func(t *testing.T) {
		f := newFixture(t)
		if _, err := f.handler(hostharness.Claude()).Handle(hostproto.HookKind("stop"), []byte(`{}`)); err == nil {
			t.Fatal("an unknown hook was served")
		}
	})
}

func TestHooksOfOtherProjectsAndAgents(t *testing.T) {
	claude := hostharness.Claude()
	unusable := func() (hooks.Store, error) { return nil, errors.New("the state directory cannot be created") }
	handler := hooks.Handler{Harness: claude, Open: unusable, Transcripts: memoryTranscripts{}}

	t.Run("a call that is not about the module's executors never opens the store and is left alone [verifies SP-239~1]", func(t *testing.T) {
		for name, test := range map[string]struct {
			kind    hostproto.HookKind
			payload string
		}{
			"a launch of another subagent": {hostproto.HookPreTool, agentLaunch(t, "general-purpose", "do things", nil)},
			"a shell command":              {hostproto.HookPreTool, `{"tool_name":"Bash","tool_input":{"command":"ls"}}`},
			"a tool of another server":     {hostproto.HookPreTool, `{"tool_name":"mcp__plugin_other_exec__fetch_request","tool_input":{}}`},
			"the drain tool":               {hostproto.HookPreTool, `{"tool_name":"mcp__plugin_spec-proof-dp_exec__drain","tool_input":{"module":"dp"}}`},
			"the start of another agent":   {hostproto.HookSubagentStart, `{"agent_id":"a","agent_type":"general-purpose"}`},
			"the stop of another agent":    {hostproto.HookSubagentStop, `{"agent_id":"a","agent_type":"other:judge","agent_transcript_path":"/x"}`},
			"the end of another launch":    {hostproto.HookPostTool, `{"tool_name":"Agent","tool_input":{"subagent_type":"general-purpose","prompt":"x"}}`},
		} {
			output, err := handler.Handle(test.kind, []byte(test.payload))
			if err != nil || len(output) != 0 {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
	})

	t.Run("a call of the module's own tools that cannot be checked is denied and the others report an error [verifies SP-239~1]", func(t *testing.T) {
		f := newFixture(t)
		for name, payload := range map[string]string{
			"an executor launch": agentLaunch(t, judgeRole, hostproto.ExecutorPrompt(f.ticket.TicketID), nil),
			"an executor tool":   executorCall(t, claude, f.ticketText(), agentID, judgeRole),
		} {
			output, err := handler.Handle(hostproto.HookPreTool, []byte(payload))
			if err != nil || claudeDecision(t, output).Permission != "deny" {
				t.Errorf("%s: output %q, error %v", name, output, err)
			}
		}
		start := marshal(t, map[string]any{"agent_id": agentID, "agent_type": judgeRole})
		if _, err := handler.Handle(hostproto.HookSubagentStart, []byte(start)); err == nil {
			t.Error("a start that could not be journaled reported no error")
		}
	})

	t.Run("a receipt with whitespace around it is still the bare receipt [verifies SP-239~1]", func(t *testing.T) {
		f := newFixture(t)
		f.transcripts[transcriptPath] = executorTranscript(t, claude,
			call(t, claude, hostproto.ToolFetchRequest, f.ticketText()), call(t, claude, hostproto.ToolSubmitResponse, f.ticketText()))
		if _, err := f.handler(claude).Handle(hostproto.HookSubagentStop, stopPayload(t, "\n"+executorrun.Receipt(f.ticket.TicketID)+"\n")); err != nil {
			t.Fatal(err)
		}
		if kinds := f.kinds(); !slices.Contains(kinds, hostproto.EventAttested) || slices.Contains(kinds, hostproto.EventLeaked) {
			t.Fatalf("events = %v", kinds)
		}
	})
}
