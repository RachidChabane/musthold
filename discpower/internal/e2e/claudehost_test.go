package e2e_test

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

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/executorrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

const (
	protocolVersion = "2025-06-18"
	resolvedModel   = "claude-sonnet-5-5"
	sessionID       = "e2e-session"
	subagentLabel   = "judge a mutant"
	paddingPrefix   = "Mind the boundary at 50. "
	swappedModel    = "claude-haiku-4-5"
)

// mcpClient is a client of the executor server, which Claude Code starts once
// with its plugin and shares between the subagents.
type mcpClient struct {
	t       *testing.T
	command *exec.Cmd
	stdin   io.WriteCloser
	stdout  *bufio.Reader
	next    int
}

func (p *project) startServer() *mcpClient {
	p.t.Helper()
	command := exec.Command(p.binary, "mcp", "--role", "executor", "--harness", "claude")
	command.Dir = p.root
	command.Env = p.environment()
	stdin, err := command.StdinPipe()
	if err != nil {
		p.t.Fatal(err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		p.t.Fatal(err)
	}
	if err := command.Start(); err != nil {
		p.t.Fatal(err)
	}
	client := &mcpClient{t: p.t, command: command, stdin: stdin, stdout: bufio.NewReader(stdout)}
	p.t.Cleanup(client.stop)
	client.send(map[string]any{
		"jsonrpc": "2.0", "id": client.id(), "method": "initialize",
		"params": map[string]any{"protocolVersion": protocolVersion, "capabilities": map[string]any{}, "clientInfo": map[string]string{"name": "e2e", "version": "1"}},
	})
	client.readResponse()
	client.send(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized"})
	return client
}

func (c *mcpClient) id() int {
	c.next++
	return c.next
}

func (c *mcpClient) send(message map[string]any) {
	c.t.Helper()
	encoded, err := json.Marshal(message)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := c.stdin.Write(append(encoded, '\n')); err != nil {
		c.t.Fatal(err)
	}
}

func (c *mcpClient) readResponse() map[string]json.RawMessage {
	c.t.Helper()
	line, err := c.stdout.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("the server closed its output: %v", err)
	}
	var message map[string]json.RawMessage
	if err := json.Unmarshal(line, &message); err != nil {
		c.t.Fatalf("response %q: %v", line, err)
	}
	return message
}

// toolText is what a tool call returned.
type toolText struct {
	Text    string
	IsError bool
}

func (c *mcpClient) call(tool string, arguments json.RawMessage) toolText {
	c.t.Helper()
	c.send(map[string]any{"jsonrpc": "2.0", "id": c.id(), "method": "tools/call", "params": map[string]any{"name": tool, "arguments": arguments}})
	message := c.readResponse()
	if failure, found := message["error"]; found {
		return toolText{Text: string(failure), IsError: true}
	}
	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(message["result"], &result); err != nil || len(result.Content) == 0 {
		c.t.Fatalf("result %s: %v", message["result"], err)
	}
	return toolText{Text: result.Content[0].Text, IsError: result.IsError}
}

func (c *mcpClient) stop() {
	_ = c.stdin.Close()
	_ = c.command.Wait()
}

// claudeHost plays the part of Claude Code around the module: it fires the hooks
// with the payloads Claude Code documents and serves the executor tools through
// the real server, with the input the hooks hand back.
type claudeHost struct {
	t       *testing.T
	project *project
	server  *mcpClient
	harness hostharness.Harness
	agents  int
}

func (p *project) claudeHost() *claudeHost {
	return &claudeHost{t: p.t, project: p, server: p.startServer(), harness: hostharness.Claude()}
}

// hookOutput is the part of a PreToolUse answer the host acts on.
type hookOutput struct {
	Permission string
	Reason     string
	Updated    json.RawMessage
}

func (h *claudeHost) fire(kind hostproto.HookKind, payload map[string]any) (hookOutput, int) {
	h.t.Helper()
	encoded, err := json.Marshal(payload)
	if err != nil {
		h.t.Fatal(err)
	}
	r := h.project.dp(string(encoded), "hook", string(kind), "--harness", "claude")
	var answer struct {
		Output struct {
			Permission string          `json:"permissionDecision"`
			Reason     string          `json:"permissionDecisionReason"`
			Updated    json.RawMessage `json:"updatedInput"`
		} `json:"hookSpecificOutput"`
	}
	if strings.TrimSpace(r.stdout) != "" {
		if err := json.Unmarshal([]byte(r.stdout), &answer); err != nil {
			h.t.Fatalf("hook %s printed %q: %v", kind, r.stdout, err)
		}
	}
	return hookOutput{Permission: answer.Output.Permission, Reason: answer.Output.Reason, Updated: answer.Output.Updated}, r.code
}

// answerSource gives the answer an executor derives from what it fetched.
type answerSource interface {
	Answer(persona string, view hostproto.ExecutorView) (string, error)
}

// listPending returns the tickets that wait for an answer, with their roles.
func (h *claudeHost) listPending() hostproto.Listing {
	h.t.Helper()
	r := h.project.dp("", hostproto.ListArgs()...)
	if r.code != 0 {
		h.t.Fatalf("request list: %s", r.output())
	}
	var listing hostproto.Listing
	if err := json.Unmarshal([]byte(r.stdout), &listing); err != nil {
		h.t.Fatal(err)
	}
	return listing
}

// serve runs one subagent for each pending ticket the way Claude Code does, as the
// honest host: launch through the rewrite hook, start, fetch and submit with the
// capability the hook adds, stop and report.
func (h *claudeHost) serve(answers answerSource) int {
	h.t.Helper()
	served := 0
	for _, item := range h.listPending().Items {
		h.runExecutor(item, answers)
		served++
	}
	return served
}

func (h *claudeHost) runExecutor(item hostproto.ListingItem, answers answerSource) {
	h.t.Helper()
	ticket := item.Ticket.TicketID
	launch, code := h.fire(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": sessionID, "tool_name": "Agent",
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
	if _, code := h.fire(hostproto.HookSubagentStart, map[string]any{
		"hook_event_name": "SubagentStart", "session_id": sessionID, "agent_id": agentID, "agent_type": item.Role,
	}); code != 0 {
		h.t.Fatalf("subagent-start exited %d", code)
	}

	view := h.fetch(agentID, item.Role, ticket)
	answer, err := answers.Answer("honest", view)
	if err != nil {
		h.t.Fatalf("no answer for %s: %v", ticket, err)
	}
	receipt := h.submit(agentID, item.Role, view, answer)

	transcript := h.writeTranscript(agentID, ticket)
	if _, code := h.fire(hostproto.HookSubagentStop, map[string]any{
		"hook_event_name": "SubagentStop", "session_id": sessionID, "agent_id": agentID, "agent_type": item.Role,
		"agent_transcript_path": transcript, "last_assistant_message": receipt,
	}); code != 0 {
		h.t.Fatalf("subagent-stop exited %d", code)
	}
	if _, code := h.fire(hostproto.HookPostTool, map[string]any{
		"hook_event_name": "PostToolUse", "session_id": sessionID, "tool_name": "Agent",
		"tool_input": launched,
		"tool_response": map[string]any{
			"status": "completed", "agentId": agentID, "content": []map[string]string{{"type": "text", "text": receipt}},
			"resolvedModel": resolvedModel, "totalTokens": 100, "usage": map[string]int{"input_tokens": 60, "output_tokens": 40},
		},
	}); code != 0 {
		h.t.Fatalf("post-tool exited %d", code)
	}
}

// toolInput runs the PreToolUse hook on an executor tool call and returns the
// input Claude Code would use, with the capability the hook added.
func (h *claudeHost) toolInput(agentID, role, tool string, input map[string]any) json.RawMessage {
	h.t.Helper()
	decision, code := h.fire(hostproto.HookPreTool, map[string]any{
		"hook_event_name": "PreToolUse", "session_id": sessionID, "agent_id": agentID, "agent_type": role,
		"tool_name": h.harness.ToolName(tool), "tool_input": input,
	})
	if code != 0 || decision.Permission != "allow" || len(decision.Updated) == 0 {
		h.t.Fatalf("the call of %s was not allowed: %+v (exit %d)", tool, decision, code)
	}
	return decision.Updated
}

func (h *claudeHost) fetch(agentID, role string, ticket hostproto.TicketID) hostproto.ExecutorView {
	h.t.Helper()
	input := h.toolInput(agentID, role, hostproto.ToolFetchRequest, map[string]any{"ticket": ticket.String(), "cap": ""})
	fetched := h.server.call(hostproto.ToolFetchRequest, input)
	if fetched.IsError {
		h.t.Fatalf("fetch_request refused %s: %s", ticket, fetched.Text)
	}
	var view hostproto.ExecutorView
	if err := json.Unmarshal([]byte(fetched.Text), &view); err != nil {
		h.t.Fatal(err)
	}
	return view
}

func (h *claudeHost) submit(agentID, role string, view hostproto.ExecutorView, answer string) string {
	h.t.Helper()
	input := h.toolInput(agentID, role, hostproto.ToolSubmitResponse, map[string]any{
		"ticket": view.Ticket, "nonce": view.Nonce, "output": json.RawMessage(answer), "cap": "",
	})
	submitted := h.server.call(hostproto.ToolSubmitResponse, input)
	ticket, err := hostproto.ParseTicketID(view.Ticket)
	if err != nil {
		h.t.Fatal(err)
	}
	if submitted.IsError || submitted.Text != executorrun.Receipt(ticket) {
		h.t.Fatalf("submit_response of %s: %+v", view.Ticket, submitted)
	}
	return submitted.Text
}

// writeTranscript writes the transcript of a run that fetched and submitted once.
func (h *claudeHost) writeTranscript(agentID string, ticket hostproto.TicketID) string {
	h.t.Helper()
	input, _ := json.Marshal(map[string]string{"ticket": ticket.String()})
	var lines []string
	for _, tool := range []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse} {
		line, _ := json.Marshal(map[string]any{
			"type": "assistant", "message": map[string]any{"content": []map[string]any{{"type": "tool_use", "name": h.harness.ToolName(tool), "input": json.RawMessage(input)}}},
		})
		lines = append(lines, string(line))
	}
	path := filepath.Join(h.project.tmp, agentID+".jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), fileMode); err != nil {
		h.t.Fatal(err)
	}
	return path
}
