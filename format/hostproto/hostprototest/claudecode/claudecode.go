// Package claudecode plays the part of Claude Code around a proof module for the
// end-to-end tests: it fires the hooks with the payloads Claude Code documents,
// and serves the executor tools through the module's real MCP server with the
// input the hooks hand back. It is test support and ships nowhere.
package claudecode

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
)

// What the simulated Claude Code reports about itself and its subagents.
const (
	protocolVersion = "2025-06-18"
	// ResolvedModel is the model the host reports for every executor.
	ResolvedModel = "claude-sonnet-5-5"
	// SessionID is the session of the simulated main agent.
	SessionID     = "e2e-session"
	subagentLabel = "judge a mutant"
	paddingPrefix = "Mind the boundary at 50. "
	swappedModel  = "claude-haiku-4-5"
	transcriptExt = ".jsonl"
	fileMode      = 0o644
	// receiptPrefix starts the text `submit_response` returns.
	receiptPrefix = "receipt "
)

// Result is what one run of the module binary left behind.
type Result struct {
	Code   int
	Stdout string
	Stderr string
}

// Module is the proof module under test: its binary run in the project, and the
// MCP executor server it starts.
type Module interface {
	// Run runs the binary with the arguments and the standard input.
	Run(stdin string, args ...string) Result
	// Server prepares the long-lived executor server process.
	Server() *exec.Cmd
	// ToolName spells the executor tool as the harness shows it to its hooks.
	ToolName(tool string) string
}

// Server is a client of the executor server, which Claude Code starts once with
// its plugin and shares between the subagents.
type Server struct {
	t       *testing.T
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	next    int
}

// StartServer starts the executor server of the module and initializes it. The
// server stops with the test.
func StartServer(t *testing.T, module Module) *Server {
	t.Helper()
	command := module.Server()
	stdin, err := command.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	server := &Server{t: t, command: command, stdin: stdin, stdout: bufio.NewReader(stdout)}
	t.Cleanup(server.stop)
	server.send(map[string]any{
		"jsonrpc": "2.0", "id": server.id(), "method": "initialize",
		"params": map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "e2e", "version": "1"}},
	})
	server.readResponse()
	server.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return server
}

func (s *Server) id() int {
	s.next++
	return s.next
}

func (s *Server) send(message map[string]any) {
	s.t.Helper()
	encoded, err := json.Marshal(message)
	if err != nil {
		s.t.Fatal(err)
	}
	if _, err := s.stdin.Write(append(encoded, '\n')); err != nil {
		s.t.Fatal(err)
	}
}

func (s *Server) readResponse() map[string]json.RawMessage {
	s.t.Helper()
	line, err := s.stdout.ReadBytes('\n')
	if err != nil {
		s.t.Fatalf("the server closed its output: %v", err)
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(line, &message); err != nil {
		s.t.Fatalf("response %q: %v", line, err)
	}
	return message
}

// ToolText is what a tool call returned.
type ToolText struct {
	Text    string
	IsError bool
}

// Call calls an executor tool with the arguments.
func (s *Server) Call(tool string, arguments json.RawMessage) ToolText {
	s.t.Helper()
	s.send(map[string]any{"jsonrpc": "2.0", "id": s.id(), "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
	message := s.readResponse()
	if failure, found := message["error"]; found {
		return ToolText{Text: string(failure), IsError: true}
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(message["result"], &result); err != nil || len(result.Content) == 0 {
		s.t.Fatalf("result %s: %v", message["result"], err)
	}
	return ToolText{Text: result.Content[0].Text, IsError: result.IsError}
}

func (s *Server) stop() {
	_ = s.stdin.Close()
	_ = s.command.Wait()
}

// Host plays Claude Code around the module.
type Host struct {
	t      *testing.T
	module Module
	// Server is the executor server the host serves the tools through.
	Server *Server
	tmp    string
	agents int
}

// NewHost starts the executor server of the module and returns the host that
// serves it. Transcripts of the runs are written below tmp.
func NewHost(t *testing.T, module Module, tmp string) *Host {
	t.Helper()
	return &Host{t: t, module: module, Server: StartServer(t, module), tmp: tmp}
}

// HookOutput is the part of a PreToolUse answer the host acts on.
type HookOutput struct {
	Permission string
	Reason     string
	Updated    json.RawMessage
}

// Fire runs one hook of the module with the payload and returns what the host
// would act on and the exit code.
func (h *Host) Fire(kind hostproto.HookKind, payload map[string]any) (HookOutput, int) {
	h.t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatal(err)
	}
	r := h.module.Run(string(encoded), "hook", string(kind), "--harness", "claude")
	var answer struct {
		Output struct {
			Permission string          `json:"permissionDecision"`
			Reason     string          `json:"permissionDecisionReason"`
			Updated    json.RawMessage `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if strings.TrimSpace(r.Stdout) != "" {
		if err := json.Unmarshal([]byte(r.Stdout), &answer); err != nil {
			h.t.Fatalf("hook %s printed %q: %v", kind, r.Stdout, err)
		}
	}
	return HookOutput{Permission: answer.Output.Permission, Reason: answer.Output.Reason, Updated: answer.Output.Updated}, r.Code
}

// ListPending returns the tickets that wait for an answer, with their roles.
func (h *Host) ListPending() hostproto.Listing {
	h.t.Helper()
	r := h.module.Run("", hostproto.ListArgs()...)
	if r.Code != 0 {
		h.t.Fatalf("request list: %s", r.Stdout+r.Stderr)
	}
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(r.Stdout), &listing); err != nil {
		h.t.Fatal(err)
	}
	return listing
}

// Serve runs one subagent for each pending ticket the way Claude Code does, as the
// honest host: launch through the rewrite hook, start, fetch and submit with the
// capability the hook adds, stop and report. It returns the number of tickets served.
func (h *Host) Serve(answers hostprototest.AnswerSource) int {
	h.t.Helper()
	served := 0
	for _, item := range h.ListPending().Items {
		h.runExecutor(item, answers)
		served++
	}
	return served
}

func (h *Host) runExecutor(item hostproto.ListingItem, answers hostprototest.AnswerSource) {
	h.t.Helper()
	ticket := item.Ticket.TicketID
	launch, code := h.Fire(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": SessionID, "tool_name": "Agent",
		"tool_input": map[string]any{
			"subagent_type": item.Role, "description": subagentLabel, "model": swappedModel,
			"run_in_background": true, "prompt": paddingPrefix + hostproto.ExecutorPrompt(ticket),
		},
	})
	if code != 0 || launch.Permission != "allow" {
		h.t.Fatalf("the launch of %s was not allowed: %+v (exit %d)", ticket, launch, code)
	}
	var launched hostproto.AgentInput
	if err := json.Unmarshal(launch.Updated, &launched); err != nil || launched.Model != "" || launched.RunInBackground || launched.Prompt != hostproto.ExecutorPrompt(ticket) {
		h.t.Fatalf("the launch was not rewritten to the bare ticket: %s (%v)", launch.Updated, err)
	}

	h.agents++
	agentID := fmt.Sprintf("agent-%04d", h.agents)
	if _, code := h.Fire(hostproto.HookSubagentStart, map[string]any{
		"hook_event_name": "SubagentStart", "session_id": SessionID, "agent_id": agentID, "agent_type": item.Role,
	}); code != 0 {
		h.t.Fatalf("subagent-start exited %d", code)
	}

	view := h.fetch(agentID, item.Role, ticket)
	answer, err := answers.Answer(hostprototest.PersonaHonest, view)
	if err != nil {
		h.t.Fatalf("no answer for %s: %v", ticket, err)
	}
	receipt := h.submit(agentID, item.Role, view, answer)

	transcript := h.writeTranscript(agentID, ticket)
	if _, code := h.Fire(hostproto.HookSubagentStop, map[string]any{
		"hook_event_name": "SubagentStop", "session_id": SessionID, "agent_id": agentID, "agent_type": item.Role,
		"agent_transcript_path": transcript, "last_assistant_message": receipt,
	}); code != 0 {
		h.t.Fatalf("subagent-stop exited %d", code)
	}
	if _, code := h.Fire(hostproto.HookPostTool, map[string]any{
		"hook_event_name": "PostToolUse", "session_id": SessionID, "tool_name": "Agent",
		"tool_input": launched,
		"tool_response": map[string]any{
			"status": "completed", "agentId": agentID, "content": []map[string]string{{"type": "text", "text": receipt}},
			"resolvedModel": ResolvedModel, "totalTokens": 100, "usage": map[string]int{"input_tokens": 60, "output_tokens": 40},
		},
	}); code != 0 {
		h.t.Fatalf("post-tool exited %d", code)
	}
}

// toolInput runs the PreToolUse hook on an executor tool call and returns the
// input Claude Code would use, with the capability the hook added.
func (h *Host) toolInput(agentID, role, tool string, input map[string]any) json.RawMessage {
	h.t.Helper()
	decision, code := h.Fire(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": SessionID, "agent_id": agentID, "agent_type": role,
		"tool_name": h.module.ToolName(tool), "tool_input": input,
	})
	if code != 0 || decision.Permission != "allow" || len(decision.Updated) == 0 {
		h.t.Fatalf("the call of %s was not allowed: %+v (exit %d)", tool, decision, code)
	}
	return decision.Updated
}

func (h *Host) fetch(agentID, role string, ticket hostproto.TicketID) hostproto.ExecutorView {
	h.t.Helper()
	input := h.toolInput(agentID, role, hostproto.ToolFetchRequest, map[string]any{"ticket": ticket.String(), "cap": ""})
	fetched := h.Server.Call(hostproto.ToolFetchRequest, input)
	if fetched.IsError {
		h.t.Fatalf("fetch_request refused %s: %s", ticket, fetched.Text)
	}
	var view hostproto.ExecutorView
	if err := json.Unmarshal([]byte(fetched.Text), &view); err != nil {
		h.t.Fatal(err)
	}
	return view
}

func (h *Host) submit(agentID, role string, view hostproto.ExecutorView, answer string) string {
	h.t.Helper()
	input := h.toolInput(agentID, role, hostproto.ToolSubmitResponse, map[string]any{
		"ticket": view.Ticket, "nonce": view.Nonce, "output": json.RawMessage(answer), "cap": "",
	})
	submitted := h.Server.Call(hostproto.ToolSubmitResponse, input)
	if submitted.IsError || submitted.Text != receiptPrefix+view.Ticket {
		h.t.Fatalf("submit_response of %s: %+v", view.Ticket, submitted)
	}
	return submitted.Text
}

// writeTranscript writes the transcript of a run that fetched and submitted once.
func (h *Host) writeTranscript(agentID string, ticket hostproto.TicketID) string {
	h.t.Helper()
	input, _ := json.Marshal(map[string]string{"ticket": ticket.String()})
	var lines []string
	for _, tool := range []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse} {
		line, _ := json.Marshal(map[string]any{
			"type": "assistant", "message": map[string]any{"content": []map[string]any{{"type": "tool_use", "name": h.module.ToolName(tool), "input": json.RawMessage(input)}}},
		})
		lines = append(lines, string(line))
	}
	path := filepath.Join(h.tmp, agentID+transcriptExt)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), fileMode); err != nil {
		h.t.Fatal(err)
	}
	return path
}
