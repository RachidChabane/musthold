package mcpstdio_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
)

// Each fuzz target has a body that states its invariants, a Fuzz function that
// feeds it generated input and a regular test that feeds it the seed corpus.

func framingSeeds() [][]byte {
	seeds := [][]byte{
		nil, []byte("\n"), []byte("{}\r\n"), []byte(`{"jsonrpc":"2.0","id":1,"method":"ping"}`),
		[]byte("{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{}}\n\n\n"), []byte("[]\n{"), []byte(`{"jsonrpc":"2.0","id":{},"method":"x"}`),
	}
	for _, script := range hostprototest.TranscriptScripts() {
		var joined []byte
		for _, frame := range script.Client {
			joined = append(append(joined, frame...), '\n')
		}
		seeds = append(seeds, joined)
	}
	return seeds
}

func framingBody(t *testing.T, input []byte) {
	t.Helper()
	reader := mcpstdio.NewFrameReader(bytes.NewReader(input))
	for {
		frame, err := reader.ReadFrame()
		if errors.Is(err, io.EOF) {
			return
		}
		if errors.Is(err, mcpstdio.ErrMessageTooLarge) {
			continue
		}
		if err != nil {
			t.Fatalf("unexpected error %v", err)
		}
		if len(frame) == 0 || len(frame) > mcpstdio.MaxMessageBytes || bytes.IndexByte(frame, '\n') >= 0 {
			t.Fatalf("frame %q breaks the framing rules", frame)
		}
		message, err := mcpstdio.ParseMessage(frame)
		if err != nil {
			if !errors.Is(err, mcpstdio.ErrMalformed) {
				t.Fatalf("error %v is not ErrMalformed", err)
			}
			continue
		}
		if message.Kind() == mcpstdio.KindInvalid {
			t.Fatalf("a parsed message has no kind: %+v", message)
		}
		var out bytes.Buffer
		if err := mcpstdio.NewFrameWriter(&out).WriteMessage(message); err != nil {
			t.Fatalf("a parsed message cannot be written: %v", err)
		}
	}
}

func FuzzFraming(f *testing.F) {
	for _, seed := range framingSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) { framingBody(t, input) })
}

func serverBody(t *testing.T, input []byte) {
	t.Helper()
	var output bytes.Buffer
	server := mcpstdio.NewServer(mcpstdio.Options{Info: hostprototest.ServerInfo(), SamplingEnabled: true, Service: hostprototest.FakeService{}}, bytes.NewReader(input), &output)
	if err := server.Serve(context.Background()); err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("Serve: %v", err)
	}
	for _, line := range bytes.Split(bytes.TrimRight(output.Bytes(), "\n"), []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var message mcpstdio.Message
		if err := json.Unmarshal(line, &message); err != nil || message.Kind() == mcpstdio.KindInvalid {
			t.Fatalf("the server wrote %q, which is not a JSON-RPC message (%v)", line, err)
		}
	}
}

func FuzzServer(f *testing.F) {
	for _, seed := range framingSeeds() {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, input []byte) { serverBody(t, input) })
}

func TestFuzzSeedCorpora(t *testing.T) {
	t.Run("the framing holds its invariants on the seed corpus [verifies SP-134~1]", func(t *testing.T) {
		for _, seed := range framingSeeds() {
			framingBody(t, seed)
		}
	})

	t.Run("the server writes only JSON-RPC messages on the seed corpus [verifies SP-138~1]", func(t *testing.T) {
		for _, seed := range framingSeeds() {
			serverBody(t, seed)
		}
	})
}
