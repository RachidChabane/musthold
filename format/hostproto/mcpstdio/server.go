package mcpstdio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// MCP methods and members.
const (
	MethodInitialize  = "initialize"
	MethodInitialized = "notifications/initialized"
	MethodToolsList   = "tools/list"
	MethodToolsCall   = "tools/call"
	MethodPing        = "ping"
	MethodSampling    = "sampling/createMessage"
	MethodRootsList   = "roots/list"

	// IncludeContextNone is the only context a sampled request asks for.
	IncludeContextNone = "none"
	contentTypeText    = "text"
	roleUser           = "user"

	samplingMaxTokens = 4096
	maxQueuedRequests = 16
	samplingIDPrefix  = "sp-sampling-"
	rootsIDPrefix     = "sp-roots-"
)

// ProtocolVersions lists the MCP revisions the server speaks, newest first.
var ProtocolVersions = []string{"2025-06-18", "2025-03-26", "2024-11-05"}

// Errors of the server loop.
var (
	// ErrNotInitialized is returned for a request that came before initialize.
	ErrNotInitialized = errors.New("the session is not initialized")
	// ErrSamplingRefused is returned when the client answers a sampling request with an error.
	ErrSamplingRefused = errors.New("sampling refused by the client")
	// ErrSamplingUnavailable is returned when a tool asks for sampling the client did not declare.
	ErrSamplingUnavailable = errors.New("the client declared no sampling capability")
)

// Info identifies the server in the initialize result.
type Info struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// FetchResult is what fetch_request returns: the view of the ticket's request.
type FetchResult struct {
	View hostproto.ExecutorView
}

// SubmitResult is what submit_response returns: the receipt of an accepted
// answer, or the schema errors of a refused one.
type SubmitResult struct {
	Receipt      string
	SchemaErrors []string
}

// DrainResult is what drain returns: how many tickets were served.
type DrainResult struct {
	Served int `json:"served"`
}

// SamplingMessage is one message of a sampling request.
type SamplingMessage struct {
	Role    string          `json:"role"`
	Content SamplingContent `json:"content"`
}

// SamplingContent is the text content of a sampling message.
type SamplingContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SamplingRequest is a request for the client to sample a model: no context from
// the client's session and no tools.
type SamplingRequest struct {
	System string
	User   string
}

// Sampler asks the client to sample a model. A Sampler is valid only during the
// tool call it was given to.
type Sampler interface {
	Sample(ctx context.Context, request SamplingRequest) (string, error)
}

// ExecutorService is the server side of the executor tools: the port a module
// implements over its Store and the pure host protocol.
type ExecutorService interface {
	FetchRequest(ctx context.Context, args FetchArgs) (FetchResult, error)
	SubmitResponse(ctx context.Context, args SubmitArgs) (SubmitResult, error)
	Drain(ctx context.Context, args DrainArgs, sampler Sampler) (DrainResult, error)
}

// Options configure a Server.
type Options struct {
	Info Info
	// SamplingEnabled is `host.sampling: on`: drain is offered only when it is on
	// and the client declared sampling.
	SamplingEnabled bool
	Service         ExecutorService
}

// Server is one MCP stdio session. It is not safe for concurrent use.
type Server struct {
	options     Options
	reader      *FrameReader
	writer      *FrameWriter
	initialized bool
	sampling    bool
	roots       bool
	version     string
	nextID      int
	queued      []Message
}

// NewServer builds a session over the given streams.
func NewServer(options Options, in io.Reader, out io.Writer) *Server {
	return &Server{options: options, reader: NewFrameReader(in), writer: NewFrameWriter(out)}
}

// SamplingDeclared reports whether the client declared the sampling capability in
// initialize.
func (s *Server) SamplingDeclared() bool { return s.sampling }

// ProtocolVersion is the revision negotiated by initialize, empty before it.
func (s *Server) ProtocolVersion() string { return s.version }

// Serve answers messages until the input ends, which is a clean stop, or the
// context is done.
func (s *Server) Serve(ctx context.Context) error {
	for ctx.Err() == nil {
		message, ok, err := s.nextMessage()
		if err != nil {
			return err
		}
		if !ok {
			return nil
		}
		if err := s.dispatch(ctx, message); err != nil {
			return err
		}
	}
	return ctx.Err()
}

// nextMessage returns the next message to handle: a queued one first, then one
// read from the input. Frames that are not messages are answered with their
// error and skipped.
func (s *Server) nextMessage() (Message, bool, error) {
	if len(s.queued) > 0 {
		message := s.queued[0]
		s.queued = s.queued[1:]
		return message, true, nil
	}
	for {
		frame, err := s.reader.ReadFrame()
		switch {
		case errors.Is(err, io.EOF):
			return Message{}, false, nil
		case errors.Is(err, ErrMessageTooLarge):
			if werr := s.writeError(nullID, CodeInvalidRequest, "message too large"); werr != nil {
				return Message{}, false, werr
			}
			continue
		case err != nil:
			return Message{}, false, err
		}
		message, err := ParseMessage(frame)
		if err != nil {
			code := CodeInvalidRequest
			if !json.Valid(frame) {
				code = CodeParseError
			}
			if werr := s.writeError(idOf(message), code, err.Error()); werr != nil {
				return Message{}, false, werr
			}
			continue
		}
		return message, true, nil
	}
}

var nullID = json.RawMessage("null")

func idOf(message Message) json.RawMessage {
	if len(message.ID) > 0 && validID(message.ID) {
		return message.ID
	}
	return nullID
}

func (s *Server) dispatch(ctx context.Context, message Message) error {
	switch message.Kind() {
	case KindNotification:
		return nil
	case KindRequest:
		return s.handleRequest(ctx, message)
	default:
		return nil
	}
}

func (s *Server) handleRequest(ctx context.Context, request Message) error {
	if request.Method == MethodPing {
		return s.writeResult(request.ID, struct{}{})
	}
	if request.Method == MethodInitialize {
		return s.handleInitialize(request)
	}
	if !s.initialized {
		return s.writeError(request.ID, CodeInvalidRequest, ErrNotInitialized.Error())
	}
	switch request.Method {
	case MethodToolsList:
		return s.writeResult(request.ID, map[string]any{"tools": s.tools()})
	case MethodToolsCall:
		return s.handleToolCall(ctx, request)
	default:
		return s.writeError(request.ID, CodeMethodNotFound, fmt.Sprintf("method %q not found", request.Method))
	}
}

func (s *Server) handleInitialize(request Message) error {
	var params struct {
		ProtocolVersion string `json:"protocolVersion"`
		Capabilities    struct {
			Sampling *json.RawMessage `json:"sampling"`
			Roots    *json.RawMessage `json:"roots"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.ProtocolVersion == "" {
		return s.writeError(request.ID, CodeInvalidParams, "initialize needs a protocolVersion and capabilities")
	}
	if s.initialized {
		return s.writeError(request.ID, CodeInvalidRequest, "the session is already initialized")
	}
	s.initialized = true
	s.sampling = params.Capabilities.Sampling != nil
	s.roots = params.Capabilities.Roots != nil
	s.version = ProtocolVersions[0]
	if slices.Contains(ProtocolVersions, params.ProtocolVersion) {
		s.version = params.ProtocolVersion
	}
	return s.writeResult(request.ID, map[string]any{
		"protocolVersion": s.version,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      s.options.Info,
	})
}

// tools lists the tools the session offers: drain only when sampling is both
// declared by the client and enabled by the policy.
func (s *Server) tools() []Tool {
	tools := ExecutorTools()
	if s.sampling && s.options.SamplingEnabled {
		tools = append(tools, DrainTool())
	}
	return tools
}

func (s *Server) handleToolCall(ctx context.Context, request Message) error {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
		return s.writeError(request.ID, CodeInvalidParams, "tools/call needs a tool name")
	}
	tool, offered := s.tool(params.Name)
	if !offered {
		return s.writeError(request.ID, CodeInvalidParams, fmt.Sprintf("unknown tool %q", params.Name))
	}
	if s.roots {
		ctx = WithRoots(ctx, sessionRoots{server: s})
	}
	result, err := s.callTool(ctx, tool, params.Arguments)
	var bad *argumentError
	switch {
	case errors.As(err, &bad):
		return s.writeError(request.ID, CodeInvalidParams, bad.Error())
	case err != nil:
		return s.writeResult(request.ID, toolResult(err.Error(), true))
	}
	return s.writeResult(request.ID, result)
}

func (s *Server) tool(name string) (Tool, bool) {
	for _, tool := range s.tools() {
		if tool.Name == name {
			return tool, true
		}
	}
	return Tool{}, false
}

// argumentError marks an error in the arguments, which is a protocol error rather
// than a tool failure.
type argumentError struct{ error }

func (s *Server) callTool(ctx context.Context, tool Tool, arguments json.RawMessage) (map[string]any, error) {
	switch tool.Name {
	case ToolFetchRequest:
		var args FetchArgs
		if err := decodeArguments(arguments, tool.InputSchema, &args); err != nil {
			return nil, &argumentError{err}
		}
		result, err := s.options.Service.FetchRequest(ctx, args)
		if err != nil {
			return nil, err
		}
		return jsonToolResult(result.View)
	case ToolSubmitResponse:
		var args SubmitArgs
		if err := decodeArguments(arguments, tool.InputSchema, &args); err != nil {
			return nil, &argumentError{err}
		}
		result, err := s.options.Service.SubmitResponse(ctx, args)
		if err != nil {
			return nil, err
		}
		if len(result.SchemaErrors) > 0 {
			return jsonToolErrorResult(map[string]any{"schemaErrors": result.SchemaErrors})
		}
		return toolResult(result.Receipt, false), nil
	default:
		var args DrainArgs
		if err := decodeArguments(arguments, tool.InputSchema, &args); err != nil {
			return nil, &argumentError{err}
		}
		result, err := s.options.Service.Drain(ctx, args, sessionSampler{server: s})
		if err != nil {
			return nil, err
		}
		return jsonToolResult(result)
	}
}

func toolResult(text string, isError bool) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": contentTypeText, "text": text}},
		"isError": isError,
	}
}

func jsonToolResult(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return toolResult(string(encoded), false), nil
}

func jsonToolErrorResult(value any) (map[string]any, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return toolResult(string(encoded), true), nil
}

func (s *Server) writeResult(id json.RawMessage, result any) error {
	encoded, err := json.Marshal(result)
	if err != nil {
		return s.writeError(id, CodeInternalError, err.Error())
	}
	return s.writer.WriteMessage(Message{ID: id, Result: encoded})
}

func (s *Server) writeError(id json.RawMessage, code int, message string) error {
	return s.writer.WriteMessage(Message{ID: id, Error: &RPCError{Code: code, Message: message}})
}

// sessionSampler implements Sampler over the session's streams.
type sessionSampler struct{ server *Server }

// Sample implements Sampler: it sends sampling/createMessage with no context and
// no tools, then reads until the client's response arrives. Pings are answered
// meanwhile; other requests wait in a bounded queue.
func (p sessionSampler) Sample(ctx context.Context, request SamplingRequest) (string, error) {
	s := p.server
	if !s.sampling {
		return "", ErrSamplingUnavailable
	}
	s.nextID++
	id := json.RawMessage(strconv.Quote(samplingIDPrefix + strconv.Itoa(s.nextID)))
	params, err := json.Marshal(map[string]any{
		"messages":       []SamplingMessage{{Role: roleUser, Content: SamplingContent{Type: contentTypeText, Text: request.User}}},
		"systemPrompt":   request.System,
		"includeContext": IncludeContextNone,
		"maxTokens":      samplingMaxTokens,
	})
	if err != nil {
		return "", err
	}
	if err := s.writer.WriteMessage(Message{ID: id, Method: MethodSampling, Params: params}); err != nil {
		return "", err
	}
	response, err := s.awaitResponse(ctx, id)
	if err != nil {
		return "", err
	}
	return samplingText(response)
}

// awaitResponse reads until the client's response to the request with the given
// id arrives. Pings are answered meanwhile; other requests wait in a bounded queue.
func (s *Server) awaitResponse(ctx context.Context, id json.RawMessage) (Message, error) {
	for ctx.Err() == nil {
		frame, err := s.reader.ReadFrame()
		if err != nil {
			return Message{}, fmt.Errorf("waiting for the response to a request of the server: %w", err)
		}
		message, err := ParseMessage(frame)
		if err != nil {
			continue
		}
		switch message.Kind() {
		case KindResponse:
			if string(message.ID) == string(id) {
				return message, nil
			}
		case KindRequest:
			if err := s.deferRequest(message); err != nil {
				return Message{}, err
			}
		}
	}
	return Message{}, ctx.Err()
}

func (s *Server) deferRequest(message Message) error {
	if message.Method == MethodPing {
		return s.writeResult(message.ID, struct{}{})
	}
	if len(s.queued) >= maxQueuedRequests {
		return s.writeError(message.ID, CodeInternalError, "too many requests while waiting for sampling")
	}
	s.queued = append(s.queued, message)
	return nil
}

func samplingText(response Message) (string, error) {
	if response.Error != nil {
		return "", fmt.Errorf("%w: %s", ErrSamplingRefused, response.Error.Message)
	}
	var result struct {
		Content SamplingContent `json:"content"`
	}
	if err := json.Unmarshal(response.Result, &result); err != nil || result.Content.Type != contentTypeText {
		return "", fmt.Errorf("%w: the sampled content is not text", ErrSamplingRefused)
	}
	return result.Content.Text, nil
}
