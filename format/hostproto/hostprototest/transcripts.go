package hostprototest

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"path"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/mcpstdio"
)

//go:embed testdata/transcripts/*.json
var transcriptFiles embed.FS

const (
	transcriptDirectory = "testdata/transcripts"
	serverName          = "musthold-fake"
	serverVersion       = "0.0.0"
	fakeNonce           = "0102030405060708090a0b0c0d0e0f10"
	sampledAnswer       = `{"validity":"VALID"}`
)

// Transcript is a golden JSON-RPC conversation: the lines a client sends and the
// lines the server must answer with, in order.
type Transcript struct {
	Name            string            `json:"name"`
	SamplingEnabled bool              `json:"samplingEnabled"`
	Client          []Frame           `json:"client"`
	Server          []json.RawMessage `json:"server"`
}

// Frame is one line a client sends. A valid JSON document is written as itself in
// a transcript file; text that is not JSON is written as a JSON string, so that
// the transcripts can hold malformed frames.
type Frame string

// MarshalJSON implements json.Marshaler.
func (f Frame) MarshalJSON() ([]byte, error) {
	if json.Valid([]byte(f)) {
		return []byte(f), nil
	}
	return json.Marshal(string(f))
}

// UnmarshalJSON implements json.Unmarshaler.
func (f *Frame) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*f = Frame(text)
		return nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return err
	}
	*f = Frame(compact.String())
	return nil
}

// FakeService is the executor service the transcripts run against: deterministic
// and independent of any store.
type FakeService struct{}

// FetchRequest implements mcpstdio.ExecutorService.
func (FakeService) FetchRequest(_ context.Context, args mcpstdio.FetchArgs) (mcpstdio.FetchResult, error) {
	if args.Ticket == "" {
		return mcpstdio.FetchResult{}, errors.New("unknown ticket")
	}
	return mcpstdio.FetchResult{View: hostproto.ExecutorView{
		Ticket: args.Ticket, Nonce: fakeNonce, System: systemText, User: userText,
		ResponseSchema: json.RawMessage(ResponseSchema),
	}}, nil
}

// SubmitResponse implements mcpstdio.ExecutorService.
func (FakeService) SubmitResponse(_ context.Context, args mcpstdio.SubmitArgs) (mcpstdio.SubmitResult, error) {
	if !satisfiesSchema(json.RawMessage(ResponseSchema), args.Output) {
		return mcpstdio.SubmitResult{SchemaErrors: []string{"validity must be one of VALID, EQUIVALENT, NOT_VIOLATING, UNDETERMINED"}}, nil
	}
	return mcpstdio.SubmitResult{Receipt: "receipt " + args.Ticket}, nil
}

// Drain implements mcpstdio.ExecutorService.
func (FakeService) Drain(ctx context.Context, args mcpstdio.DrainArgs, sampler mcpstdio.Sampler) (mcpstdio.DrainResult, error) {
	text, err := sampler.Sample(ctx, mcpstdio.SamplingRequest{System: systemText, User: userText + " " + args.Module})
	if err != nil {
		return mcpstdio.DrainResult{}, err
	}
	if text != sampledAnswer {
		return mcpstdio.DrainResult{}, fmt.Errorf("unexpected sampled answer %q", text)
	}
	return mcpstdio.DrainResult{Served: 1}, nil
}

// ServerInfo is the identity the transcripts' server announces.
func ServerInfo() mcpstdio.Info { return mcpstdio.Info{Name: serverName, Version: serverVersion} }

// RunServer feeds the client lines to a fresh server session and returns the lines
// it wrote.
func RunServer(samplingEnabled bool, service mcpstdio.ExecutorService, client []Frame) ([]json.RawMessage, error) {
	var input bytes.Buffer
	for _, line := range client {
		input.WriteString(string(line))
		input.WriteByte('\n')
	}
	var output bytes.Buffer
	server := mcpstdio.NewServer(mcpstdio.Options{Info: ServerInfo(), SamplingEnabled: samplingEnabled, Service: service}, &input, &output)
	if err := server.Serve(context.Background()); err != nil {
		return nil, err
	}
	var lines []json.RawMessage
	for _, line := range bytes.Split(bytes.TrimRight(output.Bytes(), "\n"), []byte("\n")) {
		if len(line) > 0 {
			lines = append(lines, json.RawMessage(slices.Clone(line)))
		}
	}
	return lines, nil
}

func line(text string) Frame { return Frame(text) }

func initialize(id int, capabilities string) Frame {
	return line(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":%s,"clientInfo":{"name":"fake-client","version":"1.0.0"}}}`, id, capabilities))
}

const initialized = `{"jsonrpc":"2.0","method":"notifications/initialized"}`

// TranscriptScripts returns the client side of every golden conversation.
func TranscriptScripts() []Transcript {
	ticket := string(MustID(Request())) + ".0.1"
	call := func(id int, name, arguments string) Frame {
		return line(fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, arguments))
	}
	return []Transcript{
		{Name: "handshake", Client: []Frame{
			initialize(1, `{}`), line(initialized),
			line(`{"jsonrpc":"2.0","id":2,"method":"ping"}`),
			line(`{"jsonrpc":"2.0","id":3,"method":"tools/list"}`),
		}},
		{Name: "handshake-older-protocol", Client: []Frame{
			line(`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"old","version":"0"}}}`),
			line(initialized),
		}},
		{Name: "handshake-unknown-protocol", Client: []Frame{
			line(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2099-01-01","capabilities":{},"clientInfo":{"name":"new","version":"9"}}}`),
		}},
		{Name: "sampling-declared-and-enabled", SamplingEnabled: true, Client: []Frame{
			initialize(1, `{"sampling":{}}`), line(initialized),
			line(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`),
		}},
		{Name: "sampling-declared-but-disabled", Client: []Frame{
			initialize(1, `{"sampling":{}}`), line(initialized),
			line(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`),
		}},
		{Name: "fetch-and-submit", Client: []Frame{
			initialize(1, `{}`), line(initialized),
			call(2, "fetch_request", fmt.Sprintf(`{"ticket":%q,"cap":""}`, ticket)),
			call(3, "submit_response", fmt.Sprintf(`{"ticket":%q,"nonce":%q,"output":{"validity":"VALID"},"cap":""}`, ticket, fakeNonce)),
			call(4, "submit_response", fmt.Sprintf(`{"ticket":%q,"nonce":%q,"output":{"validity":"MAYBE"},"cap":""}`, ticket, fakeNonce)),
			call(5, "fetch_request", `{"ticket":"","cap":""}`),
		}},
		{Name: "invalid-calls", Client: []Frame{
			line(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`),
			initialize(2, `{}`), line(initialized),
			initialize(3, `{}`),
			line(`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`),
			call(5, "fetch_request", `{"ticket":"t"}`),
			call(6, "fetch_request", `{"ticket":"t","cap":"","extra":1}`),
			call(7, "fetch_request", `{"ticket":1,"cap":""}`),
			call(8, "submit_response", `{"ticket":"t","nonce":"n","output":"text","cap":""}`),
			call(9, "drain", `{"module":"dp"}`),
			call(10, "no_such_tool", `{}`),
			line(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{}}`),
			line(`{not json`),
			line(`{"jsonrpc":"1.0","id":12,"method":"ping"}`),
			line(`{"jsonrpc":"2.0","id":[1],"method":"ping"}`),
			line(`[{"jsonrpc":"2.0","id":13,"method":"ping"}]`),
			line(`{"jsonrpc":"2.0","method":"notifications/unknown"}`),
			line(`{"jsonrpc":"2.0","id":14,"method":"ping"}`),
		}},
		{Name: "drain-through-sampling", SamplingEnabled: true, Client: []Frame{
			initialize(1, `{"sampling":{}}`), line(initialized),
			call(2, "drain", `{"module":"dp"}`),
			line(`{"jsonrpc":"2.0","id":"sp-sampling-1","result":{"role":"assistant","content":{"type":"text","text":"` + strings.ReplaceAll(sampledAnswer, `"`, `\"`) + `"},"model":"m"}}`),
			call(3, "drain", `{"module":"dp"}`),
			line(`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{}}`),
			line(`{"jsonrpc":"2.0","id":"sp-sampling-2","error":{"code":-1,"message":"user declined"}}`),
		}},
	}
}

// RecordTranscript runs the script's client lines against the fake service and
// returns the transcript with the server's lines filled in.
func RecordTranscript(script Transcript) (Transcript, error) {
	server, err := RunServer(script.SamplingEnabled, FakeService{}, script.Client)
	if err != nil {
		return Transcript{}, err
	}
	script.Server = server
	return script, nil
}

// LoadTranscripts reads the committed golden transcripts.
func LoadTranscripts() ([]Transcript, error) {
	entries, err := fs.ReadDir(transcriptFiles, transcriptDirectory)
	if err != nil {
		return nil, err
	}
	var transcripts []Transcript
	for _, entry := range entries {
		raw, err := fs.ReadFile(transcriptFiles, path.Join(transcriptDirectory, entry.Name()))
		if err != nil {
			return nil, err
		}
		var transcript Transcript
		if err := json.Unmarshal(raw, &transcript); err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		transcripts = append(transcripts, transcript)
	}
	return transcripts, nil
}

// ReplayTranscript runs the transcript's client lines against the fake service
// and checks the server's lines against the committed ones, member order aside.
func ReplayTranscript(transcript Transcript) error {
	server, err := RunServer(transcript.SamplingEnabled, FakeService{}, transcript.Client)
	if err != nil {
		return err
	}
	if len(server) != len(transcript.Server) {
		return fmt.Errorf("%s: the server wrote %d lines, the transcript has %d", transcript.Name, len(server), len(transcript.Server))
	}
	for index := range server {
		got, errGot := format.CanonicalizeJSON(server[index])
		want, errWant := format.CanonicalizeJSON(transcript.Server[index])
		if errGot != nil || errWant != nil || !bytes.Equal(got, want) {
			return fmt.Errorf("%s: server line %d is %s, want %s", transcript.Name, index, server[index], transcript.Server[index])
		}
	}
	return nil
}
