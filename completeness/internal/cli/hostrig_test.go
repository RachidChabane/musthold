package cli_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/completeness/internal/adapters/hoststore"
	"github.com/RachidChabane/musthold/completeness/internal/adapters/xdg"
	"github.com/RachidChabane/musthold/completeness/internal/cli"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostbridge"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const (
	protocolVersion    = "2025-06-18"
	moduleConfigYAML   = "proofs:\n  completeness: true\n"
	requireOptOutYAML  = "proofs:\n  completeness: true\nhost:\n  require_attestation: false\n"
	agentIDFormat      = "agent-%04d"
	sessionID          = "session-1"
	usageInputTokens   = 8320
	usageOutputTokens  = 4130
	usageTotalTokens   = 12450
	nonceStep          = 7
	transcriptFileName = "agent-transcript.jsonl"
)

// counterEntropy yields different bytes on every read, so that nonces are
// deterministic.
type counterEntropy struct{ next byte }

func (c *counterEntropy) Read(p []byte) (int, error) {
	for index := range p {
		c.next += nonceStep
		p[index] = c.next
	}
	return len(p), nil
}

// deciderRole is the role of the module's only executor agent.
var deciderRole = hostproto.RoleFor(hostharness.PluginName, hostproto.AgentDecider)

// deciderRequest is a closed request carrying the role of the module's decider agent.
func deciderRequest() hostproto.Request {
	return hostprototest.ClosedRequest(func(r *hostproto.Request) {
		r.Module.Name = hostharness.PluginName
		r.Role = deciderRole
	})
}

// moduleRole turns the role of a generic fixture request into the module's own:
// the personas of the host protocol are written once, for any plugin, and the
// module has the decider as its only executor agent.
func moduleRole(role string) string {
	if _, found := strings.CutPrefix(role, hostprototest.PluginName+hostproto.RoleSeparator); found {
		return deciderRole
	}
	return role
}

// closedForm turns the generic judge request of the personas into the closed
// request the module's decider answers.
func closedForm(request hostproto.Request) hostproto.Request {
	closed := hostprototest.ClosedRequest()
	request.Kind = closed.Kind
	request.Shape = closed.Shape
	request.Closed = closed.Closed
	request.ResponseSchema = closed.ResponseSchema
	return request
}

// closedAnswer turns the honest answer of the personas into the one the closed
// schema accepts; any other output is left as it is.
func closedAnswer(output json.RawMessage) json.RawMessage {
	if string(output) == hostprototest.HonestAnswer {
		return json.RawMessage(hostprototest.ClosedHonestAnswer)
	}
	return output
}

// commandHost implements hostprototest.Host over the real binary wiring: the
// executor tools are served by the `mcp` command, the hooks by the `hook` command
// and the lifecycle by the host store, so that the verdicts of the fake-host
// personas are asserted on the module as it ships.
type commandHost struct {
	t        *testing.T
	root     string
	state    string
	harness  hostharness.Harness
	hooks    bool
	now      time.Time
	entropy  io.Reader
	store    *hoststore.Store
	roles    map[string]string
	current  hostproto.Ticket
	request  hostproto.Request
	counter  int
	scratch  string
	askVotes int
}

func newCommandHost(t *testing.T, observed hostproto.Observed, policy hostproto.Policy) *commandHost {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	h := &commandHost{
		t: t, root: filepath.Join(base, "project"), state: filepath.Join(base, "state"), scratch: filepath.Join(base, "scratch"),
		hooks: observed.HooksPresent, now: hostprototest.Epoch, entropy: &counterEntropy{}, roles: map[string]string{}, askVotes: 1,
	}
	for _, dir := range []string{h.root, h.state, h.scratch, filepath.Join(h.root, ".musthold")} {
		if err := os.MkdirAll(dir, directoryMode); err != nil {
			t.Fatal(err)
		}
	}
	switch observed.Allowlist {
	case hostproto.AllowlistAbsent:
		h.harness = hostharness.Codex()
	case hostproto.AllowlistUnverified:
		h.harness = hostharness.Copilot()
	default:
		h.harness = hostharness.Claude()
	}
	h.writeConfig(moduleConfigYAML)
	if !policy.RequireAttestation {
		h.writeConfig(requireOptOutYAML)
	}
	lookup := func(name string) (string, bool) { return h.state, name == stateVariable }
	if h.store, err = hoststore.Open(xdg.HostDir(lookup, h.state, h.root)); err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *commandHost) writeConfig(content string) {
	h.t.Helper()
	if err := os.WriteFile(filepath.Join(h.root, ".musthold", "config.yaml"), []byte(content), fileMode); err != nil {
		h.t.Fatal(err)
	}
}

// run runs a command of the binary with the given standard input.
func (h *commandHost) run(stdin string, args ...string) (code int, stdout, stderr string) {
	h.t.Helper()
	variables := map[string]string{stateVariable: h.state}
	var out, errOut bytes.Buffer
	code = cli.Run(cli.Env{
		Binary: binaryName, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut,
		WorkDir: h.root, HomeDir: h.state,
		LookupEnv: func(name string) (string, bool) { value, found := variables[name]; return value, found },
		Now:       func() time.Time { return h.now }, Entropy: h.entropy,
	}, args)
	return code, out.String(), errOut.String()
}

// toolResult is what one tools/call returned.
type toolResult struct {
	Text    string
	IsError bool
}

// callTool runs the `mcp` command over one conversation: the handshake and one
// tool call.
func (h *commandHost) callTool(tool string, arguments any) toolResult {
	h.t.Helper()
	encoded, err := json.Marshal(arguments)
	if err != nil {
		h.t.Fatal(err)
	}
	frames := []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`, protocolVersion),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tool, encoded),
	}
	code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor", "--harness", h.harness.Flag)
	if code != cli.ExitClean {
		h.t.Fatalf("mcp exited %d: %s", code, stderr)
	}
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var message struct {
			ID     int `json:"id"`
			Result struct {
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
				IsError bool `json:"isError"`
			} `json:"result"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(line), &message); err != nil || message.ID != 2 {
			continue
		}
		if message.Error != nil {
			return toolResult{Text: message.Error.Message, IsError: true}
		}
		if len(message.Result.Content) == 0 {
			h.t.Fatalf("no content in %s", line)
		}
		return toolResult{Text: message.Result.Content[0].Text, IsError: message.Result.IsError}
	}
	h.t.Fatalf("no response to the tool call in %q", stdout)
	return toolResult{}
}

// refusals are the protocol errors an executor can be told of; the server returns
// their text, and the host maps the text back.
var refusals = []error{
	hostproto.ErrCapMissing, hostproto.ErrBadCap, hostproto.ErrAgentNotStarted, hostproto.ErrTicketMismatch,
	hostproto.ErrNonceMismatch, hostproto.ErrSchemaDigestMismatch, hostproto.ErrNotSubmittable,
	hostproto.ErrResubmissionConflict, hostproto.ErrCapBoundElsewhere, hostproto.ErrNotFound,
}

func refusalOf(text string) error {
	for _, known := range refusals {
		if strings.Contains(text, known.Error()) {
			return fmt.Errorf("%w: %s", known, text)
		}
	}
	return errors.New(text)
}

func (h *commandHost) runHook(kind hostproto.HookKind, payload map[string]any) (stdout string, code int) {
	h.t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatal(err)
	}
	code, stdout, _ = h.run(string(encoded), "hook", string(kind), "--harness", h.harness.Flag)
	return stdout, code
}

// Emit implements hostprototest.Host.
func (h *commandHost) Emit(request hostproto.Request) (hostproto.Ticket, error) {
	request = closedForm(request)
	request.Role = moduleRole(request.Role)
	request.Module.Name = hostharness.PluginName
	h.request = request
	return h.want()
}

func (h *commandHost) bridge() *hostbridge.Bridge {
	return hostbridge.New(h.store, func() time.Time { return h.now }, h.entropy, hostproto.SessionProve, 0)
}

// want asks for the request's ticket the way a proof does, which emits, expires
// and reissues as the lifecycle requires.
func (h *commandHost) want() (hostproto.Ticket, error) {
	answer, err := h.bridge().Want(hostbridge.Ask{Request: h.request, Votes: h.askVotes})
	if err != nil {
		return hostproto.Ticket{}, err
	}
	entry, err := h.store.GetEntry(hostproto.Key{RequestID: answer.ID})
	if err != nil {
		return hostproto.Ticket{}, err
	}
	h.current = entry.Ticket
	return entry.Ticket, nil
}

// Launch implements hostprototest.Host.
func (h *commandHost) Launch(input hostproto.AgentInput) (hostproto.AgentInput, error) {
	if !h.hooks {
		return input, nil
	}
	input.SubagentType = h.harness.AgentType(moduleRole(input.SubagentType))
	stdout, code := h.runHook(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": sessionID, "tool_name": hostproto.ToolAgent, "tool_input": input,
	})
	if code != cli.ExitClean {
		h.t.Fatalf("pre-tool exited %d", code)
	}
	if stdout == "" {
		return input, nil
	}
	var decision struct {
		Output struct {
			Permission string          `json:"permissionDecision"`
			Reason     string          `json:"permissionDecisionReason"`
			Updated    json.RawMessage `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &decision); err != nil {
		h.t.Fatalf("decision %q: %v", stdout, err)
	}
	if decision.Output.Permission == string(hostproto.PermissionDeny) {
		if strings.Contains(decision.Output.Reason, hostproto.ErrNoSingleTicket.Error()) {
			return hostproto.AgentInput{}, fmt.Errorf("%w: %s", hostproto.ErrNoSingleTicket, decision.Output.Reason)
		}
		return hostproto.AgentInput{}, errors.New(decision.Output.Reason)
	}
	var launched hostproto.AgentInput
	if err := json.Unmarshal(decision.Output.Updated, &launched); err != nil {
		h.t.Fatalf("updated input %q: %v", decision.Output.Updated, err)
	}
	return launched, nil
}

// StartExecutor implements hostprototest.Host.
func (h *commandHost) StartExecutor(role string) string {
	if !h.hooks {
		return ""
	}
	role = moduleRole(role)
	h.counter++
	agentID := fmt.Sprintf(agentIDFormat, h.counter)
	h.roles[agentID] = role
	if _, code := h.runHook(hostproto.HookSubagentStart, map[string]any{
		"hook_event_name": "SubagentStart", "session_id": sessionID, "agent_id": agentID, "agent_type": h.harness.AgentType(role),
	}); code != cli.ExitClean {
		h.t.Fatalf("subagent-start exited %d", code)
	}
	return agentID
}

// Cap implements hostprototest.Host.
func (h *commandHost) Cap(agentID string) string {
	if !h.hooks || agentID == "" {
		return ""
	}
	stdout, code := h.runHook(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": sessionID, "agent_id": agentID, "agent_type": h.harness.AgentType(h.roles[agentID]),
		"tool_name":  h.harness.ToolName(hostproto.ToolFetchRequest),
		"tool_input": map[string]string{"ticket": h.current.TicketID.String(), "cap": ""},
	})
	if code != cli.ExitClean || stdout == "" {
		return ""
	}
	var decision struct {
		Output struct {
			Updated struct {
				Cap string `json:"cap"`
			} `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if err := json.Unmarshal([]byte(stdout), &decision); err != nil {
		return ""
	}
	return decision.Output.Updated.Cap
}

// Fetch implements hostprototest.Host.
func (h *commandHost) Fetch(ticket hostproto.TicketID, capability string) (hostproto.ExecutorView, error) {
	result := h.callTool(hostproto.ToolFetchRequest, map[string]string{"ticket": ticket.String(), "cap": capability})
	if result.IsError {
		return hostproto.ExecutorView{}, refusalOf(result.Text)
	}
	var view hostproto.ExecutorView
	if err := json.Unmarshal([]byte(result.Text), &view); err != nil {
		return hostproto.ExecutorView{}, err
	}
	return view, nil
}

// Submit implements hostprototest.Host.
func (h *commandHost) Submit(submission hostprototest.Submit) (hostprototest.Receipt, error) {
	result := h.callTool(hostproto.ToolSubmitResponse, map[string]any{
		"ticket": submission.Ticket.String(), "nonce": submission.Nonce, "output": closedAnswer(submission.Output), "cap": submission.Cap,
	})
	if !result.IsError {
		return hostprototest.Receipt{Text: result.Text}, nil
	}
	if strings.Contains(result.Text, "schemaErrors") {
		entry, err := h.store.GetEntry(submission.Ticket.Key())
		if err != nil {
			return hostprototest.Receipt{}, err
		}
		return hostprototest.Receipt{CorrectionAllowed: entry.State == hostproto.StateFetched}, hostprototest.ErrSchemaViolation
	}
	return hostprototest.Receipt{}, refusalOf(result.Text)
}

// AfterRun implements hostprototest.Host.
func (h *commandHost) AfterRun(run hostprototest.Run) error {
	if !h.hooks {
		return nil
	}
	path := filepath.Join(h.scratch, fmt.Sprintf("%s-%s", run.AgentID, transcriptFileName))
	transcript, err := hostprototest.RunTranscript(h.harness, h.roles[run.AgentID], run, h.moduleCalls(run.Transcript))
	if err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, transcript, fileMode); err != nil {
		h.t.Fatal(err)
	}
	role := h.roles[run.AgentID]
	if _, code := h.runHook(hostproto.HookSubagentStop, map[string]any{
		"hook_event_name": "SubagentStop", "session_id": sessionID, "agent_id": run.AgentID, "agent_type": h.harness.AgentType(role),
		"agent_transcript_path": path, "last_assistant_message": run.Reply,
	}); code != cli.ExitClean {
		h.t.Fatalf("subagent-stop exited %d", code)
	}
	if _, code := h.runHook(hostproto.HookPostTool, map[string]any{
		"hook_event_name": "PostToolUse", "session_id": sessionID, "tool_name": hostproto.ToolAgent,
		"tool_input": map[string]any{"subagent_type": role, "description": "decide", "prompt": hostproto.ExecutorPrompt(run.Ticket)},
		"tool_response": map[string]any{
			"status": "completed", "agentId": run.AgentID, "content": []map[string]string{{"type": "text", "text": run.Reply}},
			"resolvedModel": run.ResolvedModel, "totalTokens": usageTotalTokens,
			"usage": map[string]int{"input_tokens": usageInputTokens, "output_tokens": usageOutputTokens},
		},
	}); code != cli.ExitClean {
		h.t.Fatalf("post-tool exited %d", code)
	}
	for _, event := range h.Journal() {
		if event.Kind == hostproto.EventExposed && event.Ticket == run.Ticket.String() {
			return hostproto.ErrTranscriptDeviation
		}
	}
	return nil
}

// moduleCalls spells the executor tool calls of a generic transcript the way
// this harness names the module's tools.
func (h *commandHost) moduleCalls(calls []hostproto.ToolCall) []hostproto.ToolCall {
	spelled := make([]hostproto.ToolCall, len(calls))
	for index, call := range calls {
		spelled[index] = call
		for _, tool := range []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse} {
			if strings.HasSuffix(call.Name, "__"+tool) {
				spelled[index].Name = h.harness.ToolName(tool)
			}
		}
	}
	return spelled
}

// Advance implements hostprototest.Host.
func (h *commandHost) Advance(duration time.Duration) { h.now = h.now.Add(duration) }

// Sweep implements hostprototest.Host.
func (h *commandHost) Sweep(hostproto.RequestID) (hostproto.Ticket, error) { return h.want() }

// Entry implements hostprototest.Host.
func (h *commandHost) Entry(key hostproto.Key) (hostproto.Entry, error) { return h.store.GetEntry(key) }

// Profile implements hostprototest.Host: the profile a proof credits the answer with.
func (h *commandHost) Profile(key hostproto.Key) (format.ExecutorProfile, error) {
	entry, err := h.store.GetEntry(key)
	if err != nil {
		return format.ExecutorProfile{}, err
	}
	if entry.State != hostproto.StateAccepted {
		return format.ExecutorProfile{}, hostproto.ErrNotFound
	}
	answer, err := h.bridge().Want(hostbridge.Ask{Request: h.request, Votes: h.askVotes})
	if err != nil {
		return format.ExecutorProfile{}, err
	}
	return answer.Votes[key.Vote].Executor, nil
}

// Journal implements hostprototest.Host.
func (h *commandHost) Journal() []hostproto.Event {
	records, err := h.store.Records()
	if err != nil {
		h.t.Fatal(err)
	}
	events := make([]hostproto.Event, len(records))
	for index, record := range records {
		events[index] = record.Event
	}
	return events
}
