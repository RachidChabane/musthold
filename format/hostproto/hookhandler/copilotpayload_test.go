package hookhandler_test

import (
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
)

// copilotExecutorPayload is the preToolUse payload Copilot CLI sends for a call of
// the fetch tool: the tool name joins the server and the tool with a hyphen and
// the payload names no agent, for the executor and the main agent alike.
func copilotExecutorPayload(t *testing.T, ticket string) []byte {
	t.Helper()
	return []byte(marshal(t, map[string]any{
		"sessionId": "00726893-8f86-477f-a613-48e93e4efa1a", "timestamp": 1791420110352, "cwd": "/run/project",
		"toolName": "spec-proof-dp-exec-fetch_request", "toolArgs": map[string]string{"cap": "", "ticket": ticket},
	}))
}

func TestCopilotPreToolPayload(t *testing.T) {
	copilot := harness.Copilot(identity)

	t.Run("a call of the fetch tool named with the hyphen-joined spelling and carrying no agent identity concludes nothing [verifies SP-709~1]", func(t *testing.T) {
		f := newFixture(t)
		output, err := f.handler(copilot).Handle(hostproto.HookPreTool, copilotExecutorPayload(t, f.ticketText()))
		if err != nil || len(output) != 0 {
			t.Fatalf("output %q, error %v", output, err)
		}
	})

	t.Run("a call of the fetch tool for a ticket the module did not issue is denied [verifies SP-705~1]", func(t *testing.T) {
		f := newFixture(t)
		unknown := "88ec618dd2f5f5a73120577c8324765babf222a1fe52b144337a61bc9116d709.2.1"
		output, err := f.handler(copilot).Handle(hostproto.HookPreTool, copilotExecutorPayload(t, unknown))
		if err != nil || !strings.Contains(string(output), `"permissionDecision":"deny"`) {
			t.Fatalf("output %q, error %v", output, err)
		}
	})
}
