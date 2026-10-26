package cli_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/cli"
)

func TestPortableExecutorServer(t *testing.T) {
	t.Run("a server started with no harness refuses a directory that holds no project of the module [verifies SP-1713~1]", func(t *testing.T) {
		h := claudeHost(t)
		frames := []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"fetch_request","arguments":{"ticket":"x","cap":""}}}`,
		}
		code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor", "--root", t.TempDir())
		if code != cli.ExitClean {
			t.Fatalf("mcp exited %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, `"isError":true`) || !strings.Contains(stdout, "no project of musthold here") {
			t.Fatalf("the tool call was not refused: %s", stdout)
		}
	})

	t.Run("a server with no harness is the portable harness, whose tools stay the two executor tools [verifies SP-1713~1]", func(t *testing.T) {
		h := claudeHost(t)
		frames := []string{
			`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{"sampling":{}},"clientInfo":{"name":"test","version":"1"}}}`,
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		}
		code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor")
		if code != cli.ExitClean {
			t.Fatalf("mcp exited %d: %s", code, stderr)
		}
		if !strings.Contains(stdout, `"fetch_request"`) || !strings.Contains(stdout, `"submit_response"`) || strings.Contains(stdout, `"drain"`) {
			t.Fatalf("tools: %s", stdout)
		}
	})
}
