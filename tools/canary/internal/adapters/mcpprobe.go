package adapters

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
)

const (
	mcpProtocolVersion = "2025-06-18"
	mcpClientName      = "spec-proof-canary"
	probeTimeout       = 30 * time.Second
	servedMarker       = "responseSchema"
)

// McpProber probes the executor server over standard input and output.
type McpProber struct{}

// Probe starts the executor server and calls fetch_request for the ticket with an
// empty capability, the way an executor does when no hook fills it in. It makes no model
// call. The outcome says whether the server refused and whether it served the
// ticket.
func (McpProber) Probe(ctx context.Context, spec app.ServerSpec, ticket string) evidence.CapProbe {
	probe := evidence.CapProbe{Attempted: true}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, spec.Binary, "mcp", "--role", "executor", "--harness", spec.Harness)
	command.Dir, command.Env = spec.Dir, spec.Env
	stdin, err := command.StdinPipe()
	if err != nil {
		probe.Detail = err.Error()
		return probe
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		probe.Detail = err.Error()
		return probe
	}
	if err := command.Start(); err != nil {
		probe.Detail = err.Error()
		return probe
	}
	defer func() {
		_ = stdin.Close()
		_ = command.Wait()
	}()
	client := stdioClient{in: stdin, out: bufio.NewReaderSize(stdout, mcpstdio.MaxMessageBytes)}
	if _, err := client.call(1, "initialize", map[string]any{
		"protocolVersion": mcpProtocolVersion, "capabilities": map[string]any{},
		"clientInfo": map[string]string{"name": mcpClientName, "version": "0"},
	}); err != nil {
		probe.Detail = "initialize: " + err.Error()
		return probe
	}
	if err := client.notify("notifications/initialized"); err != nil {
		probe.Detail = err.Error()
		return probe
	}
	response, err := client.call(2, "tools/call", map[string]any{
		"name": hostproto.ToolFetchRequest, "arguments": map[string]string{"ticket": ticket, hostproto.CapMember: ""},
	})
	if err != nil {
		probe.Detail = "tools/call: " + err.Error()
		return probe
	}
	probe.Responded = true
	return interpretFetch(probe, response)
}

// interpretFetch reads the answer to a fetch_request.
func interpretFetch(probe evidence.CapProbe, response mcpstdio.Message) evidence.CapProbe {
	if response.Error != nil {
		probe.Refused, probe.Detail = true, response.Error.Error()
		return probe
	}
	var result struct {
		IsError bool `json:"isError"`
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil {
		probe.Detail = "unreadable result: " + err.Error()
		return probe
	}
	texts := make([]string, len(result.Content))
	for index, part := range result.Content {
		texts[index] = part.Text
	}
	text := strings.Join(texts, "\n")
	probe.Refused = result.IsError
	probe.ServedTicket = !result.IsError && strings.Contains(text, servedMarker)
	const detailLimit = 200
	if len(text) > detailLimit {
		text = text[:detailLimit] + "..."
	}
	probe.Detail = text
	return probe
}

type stdioClient struct {
	in  io.Writer
	out *bufio.Reader
}

func (c stdioClient) send(message mcpstdio.Message) error {
	message.JSONRPC = mcpstdio.JSONRPCVersion
	encoded, err := json.Marshal(message)
	if err != nil {
		return err
	}
	_, err = c.in.Write(append(encoded, '\n'))
	return err
}

func (c stdioClient) notify(method string) error {
	return c.send(mcpstdio.Message{Method: method})
}

// call sends a request and returns the response with the same id, skipping the
// notifications the server sends in between.
func (c stdioClient) call(id int, method string, params any) (mcpstdio.Message, error) {
	encodedParams, err := json.Marshal(params)
	if err != nil {
		return mcpstdio.Message{}, err
	}
	if err := c.send(mcpstdio.Message{ID: json.RawMessage(fmt.Sprint(id)), Method: method, Params: encodedParams}); err != nil {
		return mcpstdio.Message{}, err
	}
	for {
		line, err := c.out.ReadBytes('\n')
		if err != nil {
			return mcpstdio.Message{}, fmt.Errorf("no response: %w", err)
		}
		var message mcpstdio.Message
		if json.Unmarshal(line, &message) != nil || string(message.ID) != fmt.Sprint(id) || message.Method != "" {
			continue
		}
		return message, nil
	}
}
