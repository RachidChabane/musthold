package mcpstdio

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// Executor tool names, shared with the hooks that attest an executor.
const (
	ToolFetchRequest   = hostproto.ToolFetchRequest
	ToolSubmitResponse = hostproto.ToolSubmitResponse
	ToolDrain          = hostproto.ToolDrain
)

// Descriptions and member names of the executor tools.
const (
	// CapDescription is what the executor is told about the capability member.
	CapDescription = "set by the harness; leave empty"

	memberTicket = "ticket"
	memberNonce  = "nonce"
	memberOutput = "output"
	memberCap    = hostproto.CapMember
	memberModule = "module"

	fetchDescription  = "Fetch the prompt, the response schema and the nonce of your ticket."
	submitDescription = "Submit your answer: a JSON object that matches the response schema of your ticket."
	drainDescription  = "Serve the pending tickets of a module through sampling."

	ticketDescription = "The ticket named in your task."
	nonceDescription  = "The nonce returned by fetch_request."
	outputDescription = "Your answer, a JSON object matching the response schema."
	moduleDescription = "The module whose pending tickets to serve."
)

// Tool is the definition of one MCP tool as tools/list returns it.
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

// property describes one member of a closed tool schema.
type property struct {
	name        string
	typeName    string
	description string
}

// closedSchema builds the schema of a tool: an object whose members are exactly
// the listed ones, all required, with no other member allowed.
func closedSchema(properties ...property) json.RawMessage {
	required := make([]string, len(properties))
	members := make(map[string]any, len(properties))
	for index, p := range properties {
		required[index] = p.name
		members[p.name] = map[string]any{"type": p.typeName, "description": p.description}
	}
	encoded, err := json.Marshal(map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             required,
		"properties":           members,
	})
	if err != nil {
		panic(err)
	}
	return encoded
}

const (
	typeString = "string"
	typeObject = "object"
)

// ExecutorTools returns the two tools an executor holds, with closed schemas.
func ExecutorTools() []Tool {
	return []Tool{
		{
			Name: ToolFetchRequest, Description: fetchDescription,
			InputSchema: closedSchema(
				property{memberTicket, typeString, ticketDescription},
				property{memberCap, typeString, CapDescription},
			),
		},
		{
			Name: ToolSubmitResponse, Description: submitDescription,
			InputSchema: closedSchema(
				property{memberTicket, typeString, ticketDescription},
				property{memberNonce, typeString, nonceDescription},
				property{memberOutput, typeObject, outputDescription},
				property{memberCap, typeString, CapDescription},
			),
		},
	}
}

// DrainTool returns the optional tool that serves pending tickets through
// sampling.
func DrainTool() Tool {
	return Tool{
		Name: ToolDrain, Description: drainDescription,
		InputSchema: closedSchema(property{memberModule, typeString, moduleDescription}),
	}
}

// FetchArgs are the arguments of fetch_request.
type FetchArgs struct {
	Ticket string `json:"ticket"`
	Cap    string `json:"cap"`
}

// SubmitArgs are the arguments of submit_response.
type SubmitArgs struct {
	Ticket string          `json:"ticket"`
	Nonce  string          `json:"nonce"`
	Output json.RawMessage `json:"output"`
	Cap    string          `json:"cap"`
}

// DrainArgs are the arguments of drain.
type DrainArgs struct {
	Module string `json:"module"`
}

// errBadArguments is wrapped by every argument decoding failure.
var errBadArguments = errors.New("invalid tool arguments")

// decodeArguments decodes the arguments of a tool strictly: exactly the members
// the schema lists, each of its type. A missing, unknown or mistyped member is an
// error.
func decodeArguments(raw json.RawMessage, schema json.RawMessage, into any) error {
	var spec struct {
		Required   []string                   `json:"required"`
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(schema, &spec); err != nil {
		return err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		raw = json.RawMessage(`{}`)
	}
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil {
		return fmt.Errorf("%w: arguments are not an object", errBadArguments)
	}
	for name := range members {
		if _, known := spec.Properties[name]; !known {
			return fmt.Errorf("%w: unknown member %q", errBadArguments, name)
		}
	}
	for _, name := range spec.Required {
		if _, present := members[name]; !present {
			return fmt.Errorf("%w: missing member %q", errBadArguments, name)
		}
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		return fmt.Errorf("%w: a member has the wrong type", errBadArguments)
	}
	return checkOutputIsObject(members)
}

// checkOutputIsObject enforces the one member whose JSON type RawMessage would
// not check.
func checkOutputIsObject(members map[string]json.RawMessage) error {
	output, present := members[memberOutput]
	if present && !bytes.HasPrefix(bytes.TrimSpace(output), []byte("{")) {
		return fmt.Errorf("%w: %q must be an object", errBadArguments, memberOutput)
	}
	return nil
}
