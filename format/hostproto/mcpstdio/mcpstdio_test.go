package mcpstdio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
)

const (
	transcriptDir  = "../hostprototest/testdata/transcripts"
	transcriptMode = 0o644
	dirMode        = 0o755
)

var updateGolden = flag.Bool("update-golden", false, "rewrite the golden transcripts from hostprototest")

func TestMain(m *testing.M) {
	flag.Parse()
	if *updateGolden {
		if err := writeTranscripts(); err != nil {
			panic(err)
		}
	}
	os.Exit(m.Run())
}

func writeTranscripts() error {
	if err := os.MkdirAll(transcriptDir, dirMode); err != nil {
		return err
	}
	entries, err := os.ReadDir(transcriptDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.Remove(filepath.Join(transcriptDir, entry.Name())); err != nil {
			return err
		}
	}
	for _, script := range hostprototest.TranscriptScripts() {
		recorded, err := hostprototest.RecordTranscript(script)
		if err != nil {
			return err
		}
		encoded, err := json.MarshalIndent(recorded, "", "  ")
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(transcriptDir, recorded.Name+".json"), append(encoded, '\n'), transcriptMode); err != nil {
			return err
		}
	}
	return nil
}

// fataler is what the helpers need of a *testing.T or a *rapid.T.
type fataler interface {
	Helper()
	Fatal(args ...any)
	Fatalf(format string, args ...any)
}

func run(t fataler, samplingEnabled bool, client ...string) []map[string]any {
	t.Helper()
	lines := make([]hostprototest.Frame, len(client))
	for index, text := range client {
		lines[index] = hostprototest.Frame(text)
	}
	output, err := hostprototest.RunServer(samplingEnabled, hostprototest.FakeService{}, lines)
	if err != nil {
		t.Fatal(err)
	}
	decoded := make([]map[string]any, len(output))
	for index, line := range output {
		if err := json.Unmarshal(line, &decoded[index]); err != nil {
			t.Fatalf("server line %d is not JSON: %v", index, err)
		}
	}
	return decoded
}

const (
	initializeText  = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{}}}`
	initializedText = `{"jsonrpc":"2.0","method":"notifications/initialized"}`
)

func errorCode(t fataler, message map[string]any) int {
	t.Helper()
	failure, ok := message["error"].(map[string]any)
	if !ok {
		t.Fatalf("message %v is not an error", message)
	}
	return int(failure["code"].(float64))
}

func TestFraming(t *testing.T) {
	t.Run("frames are lines, blank lines are skipped and CRLF is accepted [verifies SP-134~1]", func(t *testing.T) {
		reader := mcpstdio.NewFrameReader(strings.NewReader("{\"a\":1}\r\n\n  \n{\"b\":2}"))
		for _, want := range []string{`{"a":1}`, `{"b":2}`} {
			frame, err := reader.ReadFrame()
			if err != nil || string(frame) != want {
				t.Fatalf("frame = %q, %v, want %q", frame, err, want)
			}
		}
		if _, err := reader.ReadFrame(); !errors.Is(err, io.EOF) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a frame at the limit is read and a larger one is consumed and reported [verifies SP-134~1]", func(t *testing.T) {
		exact := strings.Repeat("a", mcpstdio.MaxMessageBytes)
		over := strings.Repeat("b", mcpstdio.MaxMessageBytes+1)
		reader := mcpstdio.NewFrameReader(strings.NewReader(exact + "\n" + over + "\nafter\n"))
		if frame, err := reader.ReadFrame(); err != nil || len(frame) != mcpstdio.MaxMessageBytes {
			t.Fatalf("exact frame: %d bytes, %v", len(frame), err)
		}
		if _, err := reader.ReadFrame(); !errors.Is(err, mcpstdio.ErrMessageTooLarge) {
			t.Fatalf("oversized frame: %v", err)
		}
		if frame, err := reader.ReadFrame(); err != nil || string(frame) != "after" {
			t.Fatalf("the reader did not recover: %q, %v", frame, err)
		}
	})

	t.Run("an oversized frame without a final newline is reported at the end of input [verifies SP-134~1]", func(t *testing.T) {
		reader := mcpstdio.NewFrameReader(strings.NewReader(strings.Repeat("c", mcpstdio.MaxMessageBytes+10)))
		if _, err := reader.ReadFrame(); !errors.Is(err, mcpstdio.ErrMessageTooLarge) {
			t.Fatalf("error = %v", err)
		}
		if _, err := reader.ReadFrame(); !errors.Is(err, io.EOF) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a message is classified as a request, a notification or a response [verifies SP-134~1]", func(t *testing.T) {
		cases := []struct {
			frame string
			want  mcpstdio.Kind
			ok    bool
		}{
			{`{"jsonrpc":"2.0","id":1,"method":"ping"}`, mcpstdio.KindRequest, true},
			{`{"jsonrpc":"2.0","id":"a","method":"ping"}`, mcpstdio.KindRequest, true},
			{`{"jsonrpc":"2.0","method":"notifications/initialized"}`, mcpstdio.KindNotification, true},
			{`{"jsonrpc":"2.0","id":1,"result":{}}`, mcpstdio.KindResponse, true},
			{`{"jsonrpc":"2.0","id":1,"error":{"code":-1,"message":"x"}}`, mcpstdio.KindResponse, true},
			{`{"jsonrpc":"1.0","id":1,"method":"ping"}`, mcpstdio.KindInvalid, false},
			{`{"id":1,"method":"ping"}`, mcpstdio.KindInvalid, false},
			{`{"jsonrpc":"2.0"}`, mcpstdio.KindInvalid, false},
			{`{"jsonrpc":"2.0","id":1}`, mcpstdio.KindInvalid, false},
			{`{"jsonrpc":"2.0","id":null,"method":"ping"}`, mcpstdio.KindRequest, false},
			{`{"jsonrpc":"2.0","id":true,"method":"ping"}`, mcpstdio.KindRequest, false},
			{`{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`, mcpstdio.KindRequest, false},
			{`[]`, mcpstdio.KindInvalid, false},
			{`{`, mcpstdio.KindInvalid, false},
		}
		for _, c := range cases {
			message, err := mcpstdio.ParseMessage([]byte(c.frame))
			if (err == nil) != c.ok || (c.ok && message.Kind() != c.want) {
				t.Errorf("%s: kind %v, error %v", c.frame, message.Kind(), err)
			}
			if err != nil && !errors.Is(err, mcpstdio.ErrMalformed) {
				t.Errorf("%s: error %v is not ErrMalformed", c.frame, err)
			}
		}
	})

	t.Run("a written message is one line carrying the JSON-RPC version [verifies SP-134~1]", func(t *testing.T) {
		var out bytes.Buffer
		err := mcpstdio.NewFrameWriter(&out).WriteMessage(mcpstdio.Message{ID: json.RawMessage(`1`), Result: json.RawMessage(`{"text":"a\nb"}`)})
		if err != nil {
			t.Fatal(err)
		}
		written := out.String()
		if strings.Count(written, "\n") != 1 || !strings.HasSuffix(written, "\n") || !strings.Contains(written, `"jsonrpc":"2.0"`) {
			t.Fatalf("written = %q", written)
		}
	})

	t.Run("a message round-trips through the writer and the reader [verifies SP-134~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			text := rapid.String().Draw(t, "text")
			result, err := json.Marshal(map[string]string{"text": text})
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			if err := mcpstdio.NewFrameWriter(&out).WriteMessage(mcpstdio.Message{ID: json.RawMessage(`7`), Result: result}); err != nil {
				t.Fatal(err)
			}
			frame, err := mcpstdio.NewFrameReader(&out).ReadFrame()
			if err != nil {
				t.Fatal(err)
			}
			message, err := mcpstdio.ParseMessage(frame)
			if err != nil || message.Kind() != mcpstdio.KindResponse {
				t.Fatalf("message %+v, %v", message, err)
			}
			var decoded map[string]string
			if err := json.Unmarshal(message.Result, &decoded); err != nil || decoded["text"] != text {
				t.Fatalf("decoded %v, %v", decoded, err)
			}
		})
	})
}

func TestGoldenTranscripts(t *testing.T) {
	transcripts, err := hostprototest.LoadTranscripts()
	if err != nil {
		t.Fatal(err)
	}
	scripts := hostprototest.TranscriptScripts()

	t.Run("every golden transcript is replayed to the committed answers [verifies SP-141~1]", func(t *testing.T) {
		if len(transcripts) != len(scripts) {
			t.Fatalf("%d committed transcripts, %d scripts (run go test ./format/hostproto/mcpstdio -update-golden)", len(transcripts), len(scripts))
		}
		for _, transcript := range transcripts {
			if err := hostprototest.ReplayTranscript(transcript); err != nil {
				t.Errorf("%v (run go test ./format/hostproto/mcpstdio -update-golden if the change is intended)", err)
			}
		}
	})

	t.Run("the committed transcripts are the ones the scripts record [verifies SP-141~1]", func(t *testing.T) {
		for _, script := range scripts {
			recorded, err := hostprototest.RecordTranscript(script)
			if err != nil {
				t.Fatal(err)
			}
			index := slices.IndexFunc(transcripts, func(c hostprototest.Transcript) bool { return c.Name == script.Name })
			if index < 0 {
				t.Errorf("transcript %s is not committed", script.Name)
				continue
			}
			if err := hostprototest.ReplayTranscript(recorded); err != nil || len(transcripts[index].Client) != len(script.Client) {
				t.Errorf("transcript %s is stale: %v", script.Name, err)
			}
		}
	})
}

func TestHandshake(t *testing.T) {
	t.Run("initialize negotiates the protocol version and announces the tools capability [verifies SP-135~1]", func(t *testing.T) {
		for requested, want := range map[string]string{
			"2025-06-18": "2025-06-18", "2025-03-26": "2025-03-26", "2024-11-05": "2024-11-05", "2099-01-01": mcpstdio.ProtocolVersions[0],
		} {
			out := run(t, false, strings.Replace(initializeText, "2025-06-18", requested, 1))
			result := out[0]["result"].(map[string]any)
			if result["protocolVersion"] != want {
				t.Errorf("requested %s: negotiated %v, want %s", requested, result["protocolVersion"], want)
			}
			if _, ok := result["capabilities"].(map[string]any)["tools"]; !ok {
				t.Error("the tools capability is not announced")
			}
			if info := result["serverInfo"].(map[string]any); info["name"] != "spec-proof-fake" {
				t.Errorf("serverInfo = %v", info)
			}
		}
	})

	t.Run("a client that declares sampling is detected, one that does not is not [verifies SP-135~1]", func(t *testing.T) {
		for declared, capabilities := range map[bool]string{true: `{"sampling":{}}`, false: `{"roots":{}}`} {
			input := strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":` + capabilities + `}}` + "\n")
			server := mcpstdio.NewServer(mcpstdio.Options{Service: hostprototest.FakeService{}}, input, io.Discard)
			if err := server.Serve(context.Background()); err != nil {
				t.Fatal(err)
			}
			if server.SamplingDeclared() != declared {
				t.Errorf("capabilities %s: SamplingDeclared = %v", capabilities, server.SamplingDeclared())
			}
		}
	})

	t.Run("initialize without a protocol version, or twice, is refused [verifies SP-135~1]", func(t *testing.T) {
		out := run(t, false, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`, initializeText, initializeText)
		if errorCode(t, out[0]) != mcpstdio.CodeInvalidParams || out[1]["result"] == nil || errorCode(t, out[2]) != mcpstdio.CodeInvalidRequest {
			t.Fatalf("out = %v", out)
		}
	})

	t.Run("a request other than ping before initialize is refused and ping is answered [verifies SP-135~1]", func(t *testing.T) {
		out := run(t, false, `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, `{"jsonrpc":"2.0","id":2,"method":"ping"}`)
		if errorCode(t, out[0]) != mcpstdio.CodeInvalidRequest || out[1]["result"] == nil {
			t.Fatalf("out = %v", out)
		}
	})
}

func toolNames(t fataler, out map[string]any) []string {
	t.Helper()
	var names []string
	for _, tool := range out["result"].(map[string]any)["tools"].([]any) {
		names = append(names, tool.(map[string]any)["name"].(string))
	}
	return names
}

func TestToolList(t *testing.T) {
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`

	t.Run("the executor tools are listed with closed schemas and the capability description [verifies SP-136~1]", func(t *testing.T) {
		tools := mcpstdio.ExecutorTools()
		if len(tools) != 2 || tools[0].Name != "fetch_request" || tools[1].Name != "submit_response" {
			t.Fatalf("tools = %+v", tools)
		}
		wantMembers := map[string][]string{"fetch_request": {"cap", "ticket"}, "submit_response": {"cap", "nonce", "output", "ticket"}, "drain": {"module"}}
		for _, tool := range append(tools, mcpstdio.DrainTool()) {
			var schema struct {
				Type                 string                       `json:"type"`
				AdditionalProperties *bool                        `json:"additionalProperties"`
				Required             []string                     `json:"required"`
				Properties           map[string]map[string]string `json:"properties"`
			}
			if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
				t.Fatal(err)
			}
			members := slices.Sorted(func(yield func(string) bool) {
				for name := range schema.Properties {
					if !yield(name) {
						return
					}
				}
			})
			if schema.Type != "object" || schema.AdditionalProperties == nil || *schema.AdditionalProperties || !slices.Equal(members, wantMembers[tool.Name]) {
				t.Errorf("%s: schema %s is not closed over %v", tool.Name, tool.InputSchema, wantMembers[tool.Name])
			}
			if !slices.Equal(slices.Sorted(slices.Values(schema.Required)), wantMembers[tool.Name]) {
				t.Errorf("%s: required = %v", tool.Name, schema.Required)
			}
			if cap, has := schema.Properties["cap"]; has && cap["description"] != "set by the harness; leave empty" {
				t.Errorf("%s: cap description = %q", tool.Name, cap["description"])
			}
			if tool.Description == "" {
				t.Errorf("%s has no description", tool.Name)
			}
		}
		if schemaOf(t, tools[1])["output"]["type"] != "object" {
			t.Error("the output member is not an object")
		}
	})

	t.Run("the executor tool names are those the hooks reason about [verifies SP-136~1]", func(t *testing.T) {
		if mcpstdio.ToolFetchRequest != hostproto.ToolFetchRequest || mcpstdio.ToolSubmitResponse != hostproto.ToolSubmitResponse || mcpstdio.ToolDrain != hostproto.ToolDrain {
			t.Fatal("the tool names diverge from the hook constants")
		}
	})

	t.Run("drain is listed only when sampling is both declared and enabled [verifies SP-136~1]", func(t *testing.T) {
		cases := []struct {
			name      string
			sampling  string
			enabled   bool
			wantNames []string
		}{
			{"declared and enabled", `{"sampling":{}}`, true, []string{"fetch_request", "submit_response", "drain"}},
			{"declared, policy off", `{"sampling":{}}`, false, []string{"fetch_request", "submit_response"}},
			{"enabled, not declared", `{}`, true, []string{"fetch_request", "submit_response"}},
		}
		for _, c := range cases {
			initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":` + c.sampling + `}}`
			out := run(t, c.enabled, initialize, initializedText, list)
			if got := toolNames(t, out[1]); !slices.Equal(got, c.wantNames) {
				t.Errorf("%s: tools %v, want %v", c.name, got, c.wantNames)
			}
		}
	})
}

func schemaOf(t fataler, tool mcpstdio.Tool) map[string]map[string]any {
	t.Helper()
	var schema struct {
		Properties map[string]map[string]any `json:"properties"`
	}
	if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
		t.Fatal(err)
	}
	return schema.Properties
}

func TestToolCall(t *testing.T) {
	ready := []string{initializeText, initializedText}
	call := func(arguments string, name string) string {
		return `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"` + name + `","arguments":` + arguments + `}}`
	}
	runCall := func(line string) map[string]any {
		return run(t, false, append(slices.Clone(ready), line)...)[1]
	}
	text := func(response map[string]any) string {
		return response["result"].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	}

	t.Run("fetch_request returns the view and submit_response the bare receipt [verifies SP-137~1]", func(t *testing.T) {
		fetched := runCall(call(`{"ticket":"tk","cap":""}`, "fetch_request"))
		var view hostproto.ExecutorView
		if err := json.Unmarshal([]byte(text(fetched)), &view); err != nil || view.Ticket != "tk" || view.User == "" {
			t.Fatalf("view = %+v, %v (%v)", view, err, fetched)
		}
		submitted := runCall(call(`{"ticket":"tk","nonce":"n","output":{"validity":"VALID"},"cap":""}`, "submit_response"))
		if text(submitted) != "receipt tk" || submitted["result"].(map[string]any)["isError"] != false {
			t.Fatalf("submitted = %v", submitted)
		}
	})

	t.Run("schema errors and service failures are tool errors, not protocol errors [verifies SP-137~1]", func(t *testing.T) {
		refused := runCall(call(`{"ticket":"tk","nonce":"n","output":{"validity":"MAYBE"},"cap":""}`, "submit_response"))
		if refused["result"].(map[string]any)["isError"] != true || !strings.Contains(text(refused), "schemaErrors") {
			t.Fatalf("refused = %v", refused)
		}
		failed := runCall(call(`{"ticket":"","cap":""}`, "fetch_request"))
		if failed["result"].(map[string]any)["isError"] != true || text(failed) != "unknown ticket" {
			t.Fatalf("failed = %v", failed)
		}
	})

	t.Run("arguments are decoded strictly against the closed schema [verifies SP-137~1]", func(t *testing.T) {
		bad := map[string]string{
			"a missing member":        call(`{"ticket":"t"}`, "fetch_request"),
			"an unknown member":       call(`{"ticket":"t","cap":"","extra":1}`, "fetch_request"),
			"a mistyped member":       call(`{"ticket":1,"cap":""}`, "fetch_request"),
			"a string output":         call(`{"ticket":"t","nonce":"n","output":"x","cap":""}`, "submit_response"),
			"an array output":         call(`{"ticket":"t","nonce":"n","output":[],"cap":""}`, "submit_response"),
			"arguments not an object": call(`[]`, "fetch_request"),
			"no arguments":            `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"fetch_request"}}`,
		}
		for name, line := range bad {
			if code := errorCode(t, runCall(line)); code != mcpstdio.CodeInvalidParams {
				t.Errorf("%s: code %d", name, code)
			}
		}
	})

	t.Run("an unknown tool, a missing tool name and an unlisted drain are refused [verifies SP-137~1]", func(t *testing.T) {
		for _, line := range []string{call(`{}`, "no_such_tool"), call(`{"module":"dp"}`, "drain"), `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{}}`, `{"jsonrpc":"2.0","id":9,"method":"tools/call"}`} {
			if code := errorCode(t, runCall(line)); code != mcpstdio.CodeInvalidParams {
				t.Errorf("%s: code %d", line, code)
			}
		}
	})
}

func TestPingNotificationsAndErrors(t *testing.T) {
	t.Run("ping is answered with an empty result and a notification gets no answer [verifies SP-138~1]", func(t *testing.T) {
		out := run(t, false, `{"jsonrpc":"2.0","id":"p","method":"ping"}`, `{"jsonrpc":"2.0","method":"notifications/anything"}`, initializeText, initializedText)
		if len(out) != 2 || out[0]["id"] != "p" || len(out[0]["result"].(map[string]any)) != 0 || out[1]["id"].(float64) != 1 {
			t.Fatalf("out = %v", out)
		}
	})

	t.Run("an unknown method is not found and the answer carries the request id [verifies SP-138~1]", func(t *testing.T) {
		out := run(t, false, initializeText, initializedText, `{"jsonrpc":"2.0","id":"x","method":"resources/list"}`)
		if errorCode(t, out[1]) != mcpstdio.CodeMethodNotFound || out[1]["id"] != "x" {
			t.Fatalf("out = %v", out)
		}
	})

	t.Run("a frame that is not JSON-RPC is answered with an error and the session continues [verifies SP-138~1]", func(t *testing.T) {
		out := run(t, false, `{not json`, `{"jsonrpc":"1.0","id":3,"method":"ping"}`, `[{"jsonrpc":"2.0","id":4,"method":"ping"}]`, `{"jsonrpc":"2.0","id":5,"method":"ping"}`)
		if len(out) != 4 || errorCode(t, out[0]) != mcpstdio.CodeParseError || errorCode(t, out[1]) != mcpstdio.CodeInvalidRequest ||
			errorCode(t, out[2]) != mcpstdio.CodeParseError && errorCode(t, out[2]) != mcpstdio.CodeInvalidRequest || out[3]["result"] == nil {
			t.Fatalf("out = %v", out)
		}
		if out[0]["id"] != nil {
			t.Errorf("a parse error carries id %v", out[0]["id"])
		}
	})

	t.Run("the server stops cleanly at the end of input and at a cancelled context [verifies SP-138~1]", func(t *testing.T) {
		server := mcpstdio.NewServer(mcpstdio.Options{Service: hostprototest.FakeService{}}, strings.NewReader(""), io.Discard)
		if err := server.Serve(context.Background()); err != nil {
			t.Fatalf("end of input: %v", err)
		}
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		if err := server.Serve(cancelled); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled: %v", err)
		}
	})

	t.Run("an oversized frame is refused and the session continues [verifies SP-138~1]", func(t *testing.T) {
		huge := strings.Repeat("x", mcpstdio.MaxMessageBytes+1)
		out := run(t, false, huge, `{"jsonrpc":"2.0","id":1,"method":"ping"}`)
		if len(out) != 2 || errorCode(t, out[0]) != mcpstdio.CodeInvalidRequest || out[1]["result"] == nil {
			t.Fatalf("out = %v", out)
		}
	})

	t.Run("each request of a valid sequence gets exactly one response [verifies SP-138~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			methods := rapid.SliceOfN(rapid.SampledFrom([]string{"ping", "tools/list", "unknown/method"}), 0, 12).Draw(t, "methods")
			lines := []string{initializeText, initializedText}
			for index, method := range methods {
				lines = append(lines, fmt.Sprintf(`{"jsonrpc":"2.0","id":"r%d","method":%q}`, index, method))
			}
			out := run(t, false, lines...)
			if len(out) != len(methods)+1 {
				t.Fatalf("%d responses for %d requests", len(out), len(methods)+1)
			}
		})
	})
}

func TestSampling(t *testing.T) {
	t.Run("drain asks the client to sample with no context and no tools and serves the answer [verifies SP-139~1]", func(t *testing.T) {
		out := run(t, true,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"sampling":{}}}}`, initializedText,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"dp"}}}`,
			`{"jsonrpc":"2.0","id":"sp-sampling-1","result":{"role":"assistant","content":{"type":"text","text":"{\"validity\":\"VALID\"}"}}}`,
		)
		request := out[1]
		params := request["params"].(map[string]any)
		if request["method"] != "sampling/createMessage" || params["includeContext"] != "none" || params["tools"] != nil || params["maxTokens"] == nil {
			t.Fatalf("sampling request = %v", request)
		}
		if served := out[2]["result"].(map[string]any); served["isError"] != false || !strings.Contains(served["content"].([]any)[0].(map[string]any)["text"].(string), `"served":1`) {
			t.Fatalf("result = %v", out[2])
		}
	})

	t.Run("a client that refuses to sample makes the drain a tool error [verifies SP-139~1]", func(t *testing.T) {
		out := run(t, true,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"sampling":{}}}}`, initializedText,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"dp"}}}`,
			`{"jsonrpc":"2.0","id":"sp-sampling-1","error":{"code":-1,"message":"declined"}}`,
		)
		if result := out[2]["result"].(map[string]any); result["isError"] != true {
			t.Fatalf("result = %v", out[2])
		}
	})

	t.Run("pings during sampling are answered at once and other requests wait their turn [verifies SP-139~1]", func(t *testing.T) {
		out := run(t, true,
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"sampling":{}}}}`, initializedText,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"dp"}}}`,
			`{"jsonrpc":"2.0","id":"pp","method":"ping"}`,
			`{"jsonrpc":"2.0","id":"tl","method":"tools/list"}`,
			`{"jsonrpc":"2.0","id":"sp-sampling-1","result":{"role":"assistant","content":{"type":"text","text":"{\"validity\":\"VALID\"}"}}}`,
		)
		ids := make([]any, len(out))
		for index, message := range out {
			ids[index] = message["id"]
		}
		want := []any{float64(1), "sp-sampling-1", "pp", float64(2), "tl"}
		if !slices.Equal(ids, want) {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	})

	t.Run("sampling without the capability is unavailable [verifies SP-139~1]", func(t *testing.T) {
		var sampler mcpstdio.Sampler
		server := mcpstdio.NewServer(mcpstdio.Options{SamplingEnabled: true, Service: samplerCapture{out: &sampler}}, strings.NewReader(strings.Join([]string{
			initializeText, initializedText,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"dp"}}}`,
		}, "\n")+"\n"), io.Discard)
		if err := server.Serve(context.Background()); err != nil {
			t.Fatal(err)
		}
		if sampler != nil {
			t.Fatal("drain was reachable although the client declared no sampling")
		}
	})
}

// samplerCapture records the sampler a drain call was given.
type samplerCapture struct{ out *mcpstdio.Sampler }

func (samplerCapture) FetchRequest(context.Context, mcpstdio.FetchArgs) (mcpstdio.FetchResult, error) {
	return mcpstdio.FetchResult{}, nil
}
func (samplerCapture) SubmitResponse(context.Context, mcpstdio.SubmitArgs) (mcpstdio.SubmitResult, error) {
	return mcpstdio.SubmitResult{}, nil
}
func (c samplerCapture) Drain(_ context.Context, _ mcpstdio.DrainArgs, sampler mcpstdio.Sampler) (mcpstdio.DrainResult, error) {
	*c.out = sampler
	return mcpstdio.DrainResult{}, nil
}
