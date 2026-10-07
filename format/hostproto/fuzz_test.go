package hostproto_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// Each fuzz target has a body that states its invariants, a Fuzz function that
// feeds it generated input, and a regular test that feeds it the seed corpus so
// that the invariants are traced and run on every build.

func hookSeeds() []string {
	seeds := []string{``, `{}`, `[]`, `null`, `{"tool_name":`, `{"agent_id":1}`, `{"tool_input":"x","tool_name":"Agent"}`, `{"toolName":"x","toolArgs":"{"}`}
	for _, payload := range hostprototest.AllVectors().Hooks {
		seeds = append(seeds, string(payload.Payload))
	}
	return seeds
}

func hookBody(t *testing.T, payload []byte) {
	t.Helper()
	for _, harness := range []string{hostproto.HarnessClaudeCode, hostproto.HarnessCodex, hostproto.HarnessCopilot} {
		decoder, err := hostproto.DecoderFor(harness)
		if err != nil {
			t.Fatal(err)
		}
		for _, kind := range hostproto.HookKinds() {
			event, err := decoder.Decode(kind, payload)
			if err != nil {
				continue
			}
			if event.Harness != harness || event.Kind != kind {
				t.Fatalf("event %+v does not match %s %s", event, harness, kind)
			}
			if _, err := json.Marshal(event); err != nil {
				t.Fatalf("an event that decoded cannot be encoded: %v", err)
			}
			if len(event.ToolInput) > 0 && !json.Valid(event.ToolInput) {
				t.Fatalf("tool input %q is not JSON", event.ToolInput)
			}
		}
	}
}

func FuzzHookDecoders(f *testing.F) {
	for _, seed := range hookSeeds() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, payload []byte) { hookBody(t, payload) })
}

func transcriptBody(t *testing.T, transcript []byte) {
	t.Helper()
	calls, err := hostproto.DecodeTranscript(strings.NewReader(string(transcript)))
	if err != nil {
		return
	}
	ticket := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID
	_ = hostproto.CheckExecutorTranscript(calls, ticket)
}

func transcriptSeeds() []string {
	return []string{
		``, "\n\n", `{"type":"assistant"}`, `{"type":"assistant","message":{"content":"text"}}`,
		`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"` + fetchToolName + `","input":{"ticket":"t"}}]}}`,
		"{\"type\":\"user\"}\n{", `[1,2]`,
	}
}

func FuzzTranscriptDecoder(f *testing.F) {
	for _, seed := range transcriptSeeds() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, transcript []byte) { transcriptBody(t, transcript) })
}

func ticketBody(t *testing.T, text string) {
	t.Helper()
	if id, err := hostproto.ParseTicketID(text); err == nil && id.String() != text {
		t.Fatalf("%q parsed as %+v, which renders as %q", text, id, id.String())
	}
	if id, err := hostproto.FindTicketID(text); err == nil {
		if !strings.Contains(text, id.String()) || id.Validate() != nil {
			t.Fatalf("%q yielded the ticket %+v", text, id)
		}
	}
	_ = hostproto.ValidNonce(text)
	_, _, _ = hostproto.ParseRole(text)
	_, _ = hostproto.ParseMCPToolName(text)
	_, _, _ = hostproto.ParsePluginServer(text)
}

func ticketSeeds() []string {
	id := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID.String()
	return []string{``, id, "ticket " + id, id + "." + id, id + ".9999999999999999999999.1", strings.ToUpper(id), "mcp__plugin_a_b__c", "p:judge"}
}

func FuzzTicketText(f *testing.F) {
	for _, seed := range ticketSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, text string) { ticketBody(t, text) })
}

func requestIDBody(t *testing.T, document []byte) {
	t.Helper()
	id, err := hostproto.RequestIDOfJSON(document)
	if err != nil {
		return
	}
	if !format.IsSHA256Hex(string(id)) {
		t.Fatalf("id %q is not a digest", id)
	}
	if again, err := hostproto.RequestIDOfJSON(document); err != nil || again != id {
		t.Fatalf("the id is not deterministic: %q then %q (%v)", id, again, err)
	}
}

func requestIDSeeds() []string {
	seeds := []string{``, `{}`, `{"module":{}}`, `{"module":{"version":"1"},"a":1}`, `{"module":{"name":"x","version":"1"},"n":1e400}`}
	for _, vector := range hostprototest.AllVectors().RequestIDs {
		seeds = append(seeds, string(vector.Request))
	}
	return seeds
}

func FuzzRequestID(f *testing.F) {
	for _, seed := range requestIDSeeds() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, document []byte) { requestIDBody(t, document) })
}

func capBody(t *testing.T, capability string) {
	t.Helper()
	agent, err := hostproto.VerifyCap(hostprototest.Secret(), capability)
	if err != nil {
		return
	}
	rebuilt, err := hostproto.FormatCap(hostprototest.Secret(), agent)
	if err != nil || rebuilt != capability {
		t.Fatalf("%q verified as %q but rebuilds as %q (%v)", capability, agent, rebuilt, err)
	}
}

func capSeeds() []string {
	seeds := []string{``, `.`, `a.`, `.b`, `a.zz`, `a.b.c`}
	for _, vector := range hostprototest.AllVectors().Capabilities {
		seeds = append(seeds, vector.Cap)
	}
	return seeds
}

func FuzzCapability(f *testing.F) {
	for _, seed := range capSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, capability string) { capBody(t, capability) })
}

func agentInputBody(t *testing.T, document []byte) {
	t.Helper()
	input, err := hostproto.DecodeAgentInput(document)
	if err != nil {
		return
	}
	sanitized, err := input.Sanitize(hostprototest.PluginName)
	if err != nil {
		return
	}
	if sanitized.Model != "" || sanitized.RunInBackground {
		t.Fatalf("the sanitized input keeps a model or a background run: %+v", sanitized)
	}
	if _, err := hostproto.FindTicketID(sanitized.Prompt); err != nil {
		t.Fatalf("the sanitized prompt holds no single ticket: %v", err)
	}
}

func agentInputSeeds() []string {
	id := hostprototest.Ticket(hostprototest.Request(), 0, 1).TicketID
	return []string{
		``, `{}`, `{"prompt":1}`,
		`{"subagent_type":"spec-proof-dp:judge","prompt":"` + id.String() + `","model":"x","run_in_background":true}`,
		`{"subagent_type":"other:judge","prompt":"` + id.String() + `"}`,
	}
}

func FuzzAgentInput(f *testing.F) {
	for _, seed := range agentInputSeeds() {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, document []byte) { agentInputBody(t, document) })
}

func TestSeedCorpora(t *testing.T) {
	t.Run("the hook decoders hold their invariants on the seed corpus [verifies SP-127~1]", func(t *testing.T) {
		for _, seed := range hookSeeds() {
			hookBody(t, []byte(seed))
		}
	})
	t.Run("the transcript decoder holds its invariants on the seed corpus [verifies SP-130~1]", func(t *testing.T) {
		for _, seed := range transcriptSeeds() {
			transcriptBody(t, []byte(seed))
		}
	})
	t.Run("the ticket codec holds its invariants on the seed corpus [verifies SP-119~1]", func(t *testing.T) {
		for _, seed := range ticketSeeds() {
			ticketBody(t, seed)
		}
	})
	t.Run("the request id holds its invariants on the seed corpus [verifies SP-118~1]", func(t *testing.T) {
		for _, seed := range requestIDSeeds() {
			requestIDBody(t, []byte(seed))
		}
	})
	t.Run("the capability check holds its invariants on the seed corpus [verifies SP-123~1]", func(t *testing.T) {
		for _, seed := range capSeeds() {
			capBody(t, seed)
		}
	})
	t.Run("the launch sanitizer holds its invariants on the seed corpus [verifies SP-129~1]", func(t *testing.T) {
		for _, seed := range agentInputSeeds() {
			agentInputBody(t, []byte(seed))
		}
	})
}
