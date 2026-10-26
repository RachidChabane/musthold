package hostprototest

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// Vectors are the golden vectors of the host protocol: the results every
// implementation of the pure rules must give.
type Vectors struct {
	RequestIDs   []RequestIDVector  `json:"requestIds"`
	Capabilities []CapVector        `json:"capabilities"`
	Tickets      []TicketVector     `json:"tickets"`
	Transitions  []TransitionVector `json:"transitions"`
	Hooks        []HookVector       `json:"hooks"`
	Decisions    []DecisionVector   `json:"decisions"`
}

// RequestIDVector states the id of a request document.
type RequestIDVector struct {
	Name    string          `json:"name"`
	Request json.RawMessage `json:"request"`
	ID      string          `json:"id"`
}

// CapVector states the capability of an agent under a secret.
type CapVector struct {
	Secret  string `json:"secret"`
	AgentID string `json:"agentId"`
	Cap     string `json:"cap"`
	Digest  string `json:"digest"`
}

// TicketVector states whether a text is a ticket identifier.
type TicketVector struct {
	Text     string `json:"text"`
	Valid    bool   `json:"valid"`
	Vote     int    `json:"vote,omitempty"`
	Emission int    `json:"emission,omitempty"`
}

// TransitionVector states whether the lifecycle allows a move.
type TransitionVector struct {
	From  hostproto.State `json:"from"`
	To    hostproto.State `json:"to"`
	Legal bool            `json:"legal"`
}

// HookVector states what a hook payload decodes to.
type HookVector struct {
	Harness string              `json:"harness"`
	Kind    hostproto.HookKind  `json:"kind"`
	Payload json.RawMessage     `json:"payload"`
	Event   hostproto.HookEvent `json:"event"`
}

// DecisionVector states the output of a hook for a decision.
type DecisionVector struct {
	Harness string          `json:"harness"`
	Kind    string          `json:"kind"`
	Block   bool            `json:"block,omitempty"`
	Reason  string          `json:"reason,omitempty"`
	Updated json.RawMessage `json:"updatedInput,omitempty"`
	Output  json.RawMessage `json:"output"`
}

// hookPayloads are the documented payloads the hook vectors decode.
var hookPayloads = []struct {
	harness string
	kind    hostproto.HookKind
	payload string
}{
	{hostproto.HarnessClaudeCode, hostproto.HookPreTool, `{"session_id":"s1","hook_event_name":"PreToolUse","tool_name":"Agent","tool_input":{"subagent_type":"musthold-dp:judge","description":"judge","prompt":"ticket x","model":"haiku"},"tool_use_id":"toolu_1"}`},
	{hostproto.HarnessClaudeCode, hostproto.HookPreTool, `{"hook_event_name":"PreToolUse","tool_name":"mcp__plugin_musthold-dp_exec__fetch_request","tool_input":{"ticket":"t","cap":""},"agent_id":"agent-0001","agent_type":"musthold-dp:judge"}`},
	{hostproto.HarnessClaudeCode, hostproto.HookSubagentStart, `{"hook_event_name":"SubagentStart","session_id":"s1","agent_id":"agent-0001","agent_type":"musthold-dp:judge"}`},
	{hostproto.HarnessClaudeCode, hostproto.HookSubagentStop, `{"hook_event_name":"SubagentStop","agent_id":"agent-0001","agent_type":"musthold-dp:judge","agent_transcript_path":"/s/subagents/agent-0001.jsonl","last_assistant_message":"receipt"}`},
	{hostproto.HarnessClaudeCode, hostproto.HookPostTool, `{"hook_event_name":"PostToolUse","tool_name":"Agent","tool_response":{"status":"completed","resolvedModel":"claude-sonnet-5-5","totalTokens":10,"usage":{"input_tokens":6,"output_tokens":4}}}`},
	{hostproto.HarnessCodex, hostproto.HookPreTool, `{"session_id":"thr_1","hook_event_name":"PreToolUse","turn_id":"turn_4","tool_name":"mcp__exec__fetch_request","tool_use_id":"call_7","tool_input":{"ticket":"t"}}`},
	{hostproto.HarnessCodex, hostproto.HookSubagentStop, `{"hook_event_name":"SubagentStop","agent_id":"agent_xyz","agent_type":"code_review","agent_transcript_path":"/p/t.jsonl","last_assistant_message":"done"}`},
	{hostproto.HarnessCopilot, hostproto.HookPreTool, `{"sessionId":"s","timestamp":1,"toolName":"musthold-dp-exec/fetch_request","toolArgs":"{\"ticket\":\"t\"}"}`},
	{hostproto.HarnessCopilot, hostproto.HookSubagentStop, `{"sessionId":"s","transcriptPath":"/t.jsonl","agentId":"a1","agentType":"x:judge","response":"receipt"}`},
}

// AllVectors computes the vectors from the implementation. The committed copy is
// compared with it, and any other implementation is held to the committed copy.
func AllVectors() Vectors {
	return Vectors{
		RequestIDs:   requestIDVectors(),
		Capabilities: capVectors(),
		Tickets:      ticketVectors(),
		Transitions:  transitionVectors(),
		Hooks:        hookVectors(),
		Decisions:    decisionVectors(),
	}
}

func requestIDVectors() []RequestIDVector {
	documents := map[string]string{
		"judge validity":         mustMarshal(Request()),
		"generation":             mustMarshal(GenerationRequest()),
		"closed question":        mustMarshal(ClosedRequest()),
		"another module version": mustMarshal(Request(func(r *hostproto.Request) { r.Module.Version = "9.9.9" })),
	}
	names := []string{"judge validity", "another module version", "generation", "closed question"}
	vectors := make([]RequestIDVector, 0, len(names))
	for _, name := range names {
		id, err := hostproto.RequestIDOfJSON([]byte(documents[name]))
		if err != nil {
			panic(err)
		}
		vectors = append(vectors, RequestIDVector{Name: name, Request: json.RawMessage(documents[name]), ID: string(id)})
	}
	return vectors
}

func mustMarshal(value any) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}

func capVectors() []CapVector {
	var vectors []CapVector
	for _, agent := range []string{"agent-0001", "a4d2c8f1e0b3a297"} {
		capability, err := hostproto.FormatCap(Secret(), agent)
		if err != nil {
			panic(err)
		}
		vectors = append(vectors, CapVector{Secret: SecretText, AgentID: agent, Cap: capability, Digest: hostproto.CapDigest(capability)})
	}
	return vectors
}

func ticketVectors() []TicketVector {
	id := string(MustID(Request()))
	return []TicketVector{
		{Text: id + ".0.1", Valid: true, Vote: 0, Emission: 1},
		{Text: id + ".2.3", Valid: true, Vote: 2, Emission: 3},
		{Text: id + ".0.0"},
		{Text: id + ".-1.1"},
		{Text: id + ".01.1"},
		{Text: strings.ToUpper(id) + ".0.1"},
		{Text: id},
	}
}

func transitionVectors() []TransitionVector {
	var vectors []TransitionVector
	for _, from := range hostproto.States() {
		for _, to := range hostproto.States() {
			vectors = append(vectors, TransitionVector{From: from, To: to, Legal: hostproto.Legal(from, to)})
		}
	}
	return vectors
}

func hookVectors() []HookVector {
	vectors := make([]HookVector, 0, len(hookPayloads))
	for _, payload := range hookPayloads {
		decoder, err := hostproto.DecoderFor(payload.harness)
		if err != nil {
			panic(err)
		}
		event, err := decoder.Decode(payload.kind, []byte(payload.payload))
		if err != nil {
			panic(fmt.Sprintf("hook vector %s %s: %v", payload.harness, payload.kind, err))
		}
		vectors = append(vectors, HookVector{Harness: payload.harness, Kind: payload.kind, Payload: json.RawMessage(payload.payload), Event: event})
	}
	return vectors
}

func decisionVectors() []DecisionVector {
	var vectors []DecisionVector
	updated := json.RawMessage(`{"cap":"agent-0001.mac"}`)
	for _, harness := range []string{hostproto.HarnessClaudeCode, hostproto.HarnessCodex, hostproto.HarnessCopilot} {
		for _, permission := range []hostproto.Permission{hostproto.PermissionAllow, hostproto.PermissionDeny} {
			decision := hostproto.PreToolDecision{Permission: permission, Reason: "because"}
			vector := DecisionVector{Harness: harness, Kind: "pre-tool"}
			if permission == hostproto.PermissionAllow {
				decision.UpdatedInput, vector.Updated = updated, updated
			}
			output, err := hostproto.EncodePreToolDecision(harness, decision)
			if err != nil {
				panic(err)
			}
			vector.Reason, vector.Output = decision.Reason, output
			vectors = append(vectors, vector)
		}
		for _, block := range []bool{true, false} {
			output, err := hostproto.EncodeStopDecision(harness, hostproto.StopDecision{Block: block, Reason: "because"})
			if err != nil {
				panic(err)
			}
			vector := DecisionVector{Harness: harness, Kind: "subagent-stop", Block: block, Output: output}
			if block {
				vector.Reason = "because"
			}
			vectors = append(vectors, vector)
		}
	}
	return vectors
}
