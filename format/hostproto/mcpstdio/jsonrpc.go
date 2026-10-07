// Package mcpstdio is a hand-rolled, pure MCP stdio codec and server loop over
// io.Reader and io.Writer: JSON-RPC 2.0 framing, the initialize handshake with
// sampling detection, tools/list, tools/call, ping and notifications, plus the one
// authoritative definition of the executor tools. It deliberately does not use the
// official MCP SDK, whose transports link net/http.
package mcpstdio

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// Framing limits and the JSON-RPC version.
const (
	// JSONRPCVersion is the only JSON-RPC version of MCP.
	JSONRPCVersion = "2.0"
	// MaxMessageBytes bounds one framed message, which keeps a hostile peer from
	// exhausting memory. It leaves room for the largest output an executor may
	// submit.
	MaxMessageBytes = 1 << 20
	frameDelimiter  = '\n'
)

// JSON-RPC error codes.
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Errors of the codec.
var (
	// ErrMessageTooLarge is returned for a frame above MaxMessageBytes. The frame is
	// consumed, so the reader stays usable.
	ErrMessageTooLarge = errors.New("message too large")
	// ErrMalformed is returned for a frame that is not a JSON-RPC 2.0 message.
	ErrMalformed = errors.New("malformed JSON-RPC message")
)

// RPCError is a JSON-RPC error object.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// Error implements error.
func (e *RPCError) Error() string { return fmt.Sprintf("json-rpc error %d: %s", e.Code, e.Message) }

// Message is any JSON-RPC 2.0 message: a request, a notification or a response.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *RPCError       `json:"error,omitempty"`
}

// Kind classifies a message.
type Kind int

// Message kinds.
const (
	KindInvalid Kind = iota
	KindRequest
	KindNotification
	KindResponse
)

// Kind classifies the message by its members.
func (m Message) Kind() Kind {
	hasID := len(m.ID) > 0
	switch {
	case m.JSONRPC != JSONRPCVersion:
		return KindInvalid
	case m.Method != "" && hasID:
		return KindRequest
	case m.Method != "":
		return KindNotification
	case hasID && (len(m.Result) > 0 || m.Error != nil):
		return KindResponse
	default:
		return KindInvalid
	}
}

// validID reports whether raw is a JSON string or number, the only id types MCP
// allows.
func validID(raw json.RawMessage) bool {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return false
	}
	return trimmed[0] == '"' || trimmed[0] == '-' || (trimmed[0] >= '0' && trimmed[0] <= '9')
}

// ParseMessage reads one framed message and checks that it is a JSON-RPC 2.0
// message of a known kind.
func ParseMessage(frame []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(frame, &message); err != nil {
		return Message{}, fmt.Errorf("%w: not a JSON object", ErrMalformed)
	}
	if message.Kind() == KindInvalid {
		return message, fmt.Errorf("%w: not a request, notification or response", ErrMalformed)
	}
	if len(message.ID) > 0 && !validID(message.ID) {
		return message, fmt.Errorf("%w: id must be a string or a number", ErrMalformed)
	}
	return message, nil
}

// FrameReader reads newline-delimited frames.
type FrameReader struct {
	reader *bufio.Reader
}

// NewFrameReader wraps r.
func NewFrameReader(r io.Reader) *FrameReader {
	return &FrameReader{reader: bufio.NewReaderSize(r, bufio.MaxScanTokenSize)}
}

// ReadFrame returns the next non-empty frame without its delimiter. At the end of
// input it returns io.EOF. A frame above MaxMessageBytes is consumed and reported
// as ErrMessageTooLarge.
func (f *FrameReader) ReadFrame() ([]byte, error) {
	for {
		frame, err := f.readLine()
		if err != nil {
			return nil, err
		}
		if trimmed := bytes.TrimSpace(frame); len(trimmed) > 0 {
			return trimmed, nil
		}
	}
}

func (f *FrameReader) readLine() ([]byte, error) {
	var line []byte
	oversized := false
	for {
		chunk, isPrefix, err := f.reader.ReadLine()
		if err != nil {
			if errors.Is(err, io.EOF) && (len(line) > 0 || oversized) {
				break
			}
			return nil, err
		}
		if !oversized {
			line = append(line, chunk...)
			oversized = len(line) > MaxMessageBytes
		}
		if oversized {
			line = nil
		}
		if !isPrefix {
			break
		}
	}
	if oversized {
		return nil, ErrMessageTooLarge
	}
	return line, nil
}

// FrameWriter writes newline-delimited frames.
type FrameWriter struct {
	writer io.Writer
}

// NewFrameWriter wraps w.
func NewFrameWriter(w io.Writer) *FrameWriter { return &FrameWriter{writer: w} }

// WriteMessage encodes the message on one line. Compact JSON has no raw newline,
// so the frame cannot be split by its own content.
func (f *FrameWriter) WriteMessage(message Message) error {
	message.JSONRPC = JSONRPCVersion
	encoded, err := json.Marshal(message)
	if err != nil {
		return fmt.Errorf("encode message: %w", err)
	}
	if bytes.IndexByte(encoded, frameDelimiter) >= 0 {
		return errors.New("encoded message contains a newline")
	}
	_, err = f.writer.Write(append(encoded, frameDelimiter))
	return err
}
