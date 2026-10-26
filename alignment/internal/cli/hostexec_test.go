package cli_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/alignment/internal/cli"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/hostprototest"
)

const samplingOnYAML = "host:\n  sampling: on\n"

func claudeHost(t *testing.T) *commandHost {
	t.Helper()
	return newCommandHost(t, hostprototest.ClaudeCodeObserved(), hostprototest.RequireAttestation())
}

// toolNames runs one conversation that lists the tools, with or without the
// sampling capability declared by the client.
func (h *commandHost) toolNames(declareSampling bool) []string {
	h.t.Helper()
	capabilities := `{}`
	if declareSampling {
		capabilities = `{"sampling":{}}`
	}
	frames := []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":%s,"clientInfo":{"name":"test","version":"1"}}}`, protocolVersion, capabilities),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
	}
	code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor", "--harness", "claude")
	if code != cli.ExitClean {
		h.t.Fatalf("mcp exited %d: %s", code, stderr)
	}
	var names []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		var message struct {
			ID     int `json:"id"`
			Result struct {
				Tools []struct {
					Name string `json:"name"`
				} `json:"tools"`
			} `json:"result"`
		}
		if json.Unmarshal([]byte(line), &message) == nil && message.ID == 2 {
			for _, tool := range message.Result.Tools {
				names = append(names, tool.Name)
			}
		}
	}
	return names
}

func TestExecutorServer(t *testing.T) {
	t.Run("the server offers exactly fetch_request and submit_response [verifies SP-741~1]", func(t *testing.T) {
		h := claudeHost(t)
		if got := h.toolNames(false); !slices.Equal(got, []string{"fetch_request", "submit_response"}) {
			t.Fatalf("tools = %v", got)
		}
	})

	t.Run("drain is offered only when the client declares sampling and the project turns it on [verifies SP-741~1]", func(t *testing.T) {
		with := []string{"fetch_request", "submit_response", "drain"}
		without := []string{"fetch_request", "submit_response"}
		h := claudeHost(t)
		if got := h.toolNames(true); !slices.Equal(got, without) {
			t.Errorf("declared, not enabled: %v", got)
		}
		h.writeConfig(samplingOnYAML)
		if got := h.toolNames(false); !slices.Equal(got, without) {
			t.Errorf("enabled, not declared: %v", got)
		}
		if got := h.toolNames(true); !slices.Equal(got, with) {
			t.Errorf("declared and enabled: %v", got)
		}
	})

	t.Run("a drain through the command line serves the pending ticket as a sampled response [verifies SP-741~1]", func(t *testing.T) {
		h := claudeHost(t)
		h.writeConfig(samplingOnYAML)
		ticket, err := h.Emit(judgeRequest())
		if err != nil {
			t.Fatal(err)
		}
		frames := []string{
			fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{"sampling":{}},"clientInfo":{"name":"test","version":"1"}}}`, protocolVersion),
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"alignment"}}}`,
			`{"jsonrpc":"2.0","id":"sp-sampling-1","result":{"role":"assistant","content":{"type":"text","text":"{\"validity\":\"VALID\"}"},"model":"m"}}`,
		}
		code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor", "--harness", "claude")
		if code != cli.ExitClean || !strings.Contains(stdout, `\"served\":1`) {
			t.Fatalf("exit %d, stdout %s, stderr %s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, `"includeContext":"none"`) {
			t.Fatalf("the sampling request carried context: %s", stdout)
		}
		response, err := h.store.GetResponse(ticket.Key())
		if err != nil || response.Executor.Channel != format.ChannelHostSampled {
			t.Fatalf("response = %+v, %v", response, err)
		}
	})

	t.Run("a drain names the module as alignment and serves no other module's tickets [verifies SP-741~1]", func(t *testing.T) {
		h := claudeHost(t)
		h.writeConfig(samplingOnYAML)
		ticket, err := h.Emit(judgeRequest())
		if err != nil {
			t.Fatal(err)
		}
		frames := []string{
			fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":%q,"capabilities":{"sampling":{}},"clientInfo":{"name":"test","version":"1"}}}`, protocolVersion),
			`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"drain","arguments":{"module":"dp"}}}`,
		}
		code, stdout, stderr := h.run(strings.Join(frames, "\n")+"\n", "mcp", "--role", "executor", "--harness", "claude")
		if code != cli.ExitClean {
			t.Fatalf("exit %d, stdout %s, stderr %s", code, stdout, stderr)
		}
		if _, err := h.store.GetResponse(ticket.Key()); err == nil {
			t.Fatalf("a drain of another module answered the ticket: %s", stdout)
		}
	})

	t.Run("the server of a harness that cannot attest accepts an answer without a capability as restricted-unverified on Copilot [verifies SP-741~1]", func(t *testing.T) {
		h := newCommandHost(t, hostproto.Observed{PluginPresent: true, ServerPresent: true, Allowlist: hostproto.AllowlistUnverified}, hostproto.Policy{})
		ticket, err := h.Emit(judgeRequest())
		if err != nil {
			t.Fatal(err)
		}
		view, err := h.Fetch(ticket.TicketID, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := h.Submit(hostprototest.Submit{Ticket: ticket.TicketID, Nonce: view.Nonce, Output: json.RawMessage(hostprototest.HonestAnswer)}); err != nil {
			t.Fatal(err)
		}
		response, err := h.store.GetResponse(ticket.Key())
		if err != nil || response.Executor.Isolation != format.IsolationRestrictedUnverified || response.Executor.Harness.Name != hostproto.HarnessCopilot {
			t.Fatalf("response = %+v, %v", response, err)
		}
	})

	t.Run("a closed request of the decider is served to its executor like a judge request [verifies SP-741~1]", func(t *testing.T) {
		h := claudeHost(t)
		ticket, err := h.Emit(deciderRequest())
		if err != nil {
			t.Fatal(err)
		}
		agent := h.StartExecutor(deciderRequest().Role)
		view, err := h.Fetch(ticket.TicketID, h.Cap(agent))
		if err != nil || len(view.Closed) == 0 {
			t.Fatalf("view = %+v, %v", view, err)
		}
	})
}

func TestExecutorCommandRefusals(t *testing.T) {
	h := claudeHost(t)
	for name, args := range map[string][]string{
		"an mcp role other than executor": {"mcp", "--role", "judge"},
		"an mcp without a role":           {"mcp"},
		"an mcp harness that is unknown":  {"mcp", "--role", "executor", "--harness", "vim"},
		"a hook without a kind":           {"hook"},
		"a hook of an unknown kind":       {"hook", "stop"},
		"a hook of an unknown harness":    {"hook", "pre-tool", "--harness", "vim"},
		"a hook with two kinds":           {"hook", "pre-tool", "post-tool"},
	} {
		t.Run(name+" is refused with exit 1 [verifies SP-740~1]", func(t *testing.T) {
			if code, _, _ := h.run("{}", args...); code != cli.ExitBlocked {
				t.Fatalf("exit = %d", code)
			}
		})
	}

	t.Run("a stray argument is a usage failure, as for prove [verifies SP-740~1]", func(t *testing.T) {
		if code, _, _ := h.run("{}", "mcp", "--role", "executor", "extra"); code != cli.ExitToolFailure {
			t.Fatalf("exit = %d", code)
		}
	})

	t.Run("a hook that cannot read its payload only reports, so that it never blocks a call that is not the module's [verifies SP-740~1]", func(t *testing.T) {
		for _, kind := range []string{"pre-tool", "subagent-start", "subagent-stop", "post-tool"} {
			if code, _, _ := h.run("not json", "hook", kind); code != cli.ExitBlocked {
				t.Errorf("%s exit = %d", kind, code)
			}
		}
	})

	t.Run("the usage names the executor commands [verifies SP-740~1]", func(t *testing.T) {
		_, _, stderr := h.run("")
		for _, command := range []string{"mcp", "hook", "init", "enable", "disable", "host"} {
			if !strings.Contains(stderr, command) {
				t.Fatalf("usage = %q lacks %q", stderr, command)
			}
		}
	})
}

func TestHostRoot(t *testing.T) {
	t.Run("the hook, the server and the tickets of a project share one host directory, whatever the spelling of its path [verifies SP-740~1]", func(t *testing.T) {
		h := claudeHost(t)
		ticket, err := h.Emit(judgeRequest())
		if err != nil {
			t.Fatal(err)
		}
		link := filepath.Join(filepath.Dir(h.root), "alias")
		if err := os.Symlink(h.root, link); err != nil {
			t.Skipf("symbolic links are unavailable: %v", err)
		}
		variables := map[string]string{stateVariable: h.state, "CLAUDE_PROJECT_DIR": link}
		run := func(workDir, stdin string, args ...string) (int, string) {
			var out, errOut bytes.Buffer
			code := cli.Run(cli.Env{
				Binary: binaryName, Stdin: strings.NewReader(stdin), Stdout: &out, Stderr: &errOut, WorkDir: workDir, HomeDir: h.state,
				LookupEnv: func(name string) (string, bool) { value, found := variables[name]; return value, found },
				Now:       func() time.Time { return h.now }, Entropy: h.entropy,
			}, args)
			return code, out.String() + errOut.String()
		}
		payload := executorCallPayload(t, h, ticket.TicketID.String())
		code, output := run(t.TempDir(), payload, "hook", "pre-tool")
		if code != cli.ExitClean || !strings.Contains(output, `"permissionDecision":"deny"`) {
			t.Fatalf("exit %d, output %s", code, output)
		}
		if !slices.ContainsFunc(h.Journal(), func(e hostproto.Event) bool { return e.Kind == hostproto.EventExposureAttempt }) {
			t.Fatalf("the hook journaled elsewhere than the store of the tickets: %v", h.Journal())
		}
	})
}

// executorCallPayload is the PreToolUse payload of the main agent calling fetch_request.
func executorCallPayload(t *testing.T, h *commandHost, ticket string) string {
	t.Helper()
	return marshalJSON(t, map[string]any{
		"hook_event_name": "PreToolUse", "tool_name": h.harness.ToolName(hostproto.ToolFetchRequest),
		"tool_input": map[string]string{"ticket": ticket, "cap": ""},
	})
}

func marshalJSON(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func TestHooksInAProjectWithoutTheModule(t *testing.T) {
	t.Run("a hook in a project that has no shared configuration does nothing and creates no state [verifies SP-740~1]", func(t *testing.T) {
		h := claudeHost(t)
		if err := os.Remove(filepath.Join(h.root, ".musthold", "config.yaml")); err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(h.state); err != nil {
			t.Fatal(err)
		}
		payload := executorCallPayload(t, h, "bbb0107006fad4abd4746fc925c050dff762f20370b46e56c92e74fe9ef5f7a5.0.1")
		code, stdout, stderr := h.run(payload, "hook", "pre-tool")
		if code != cli.ExitClean || stdout != "" || stderr != "" {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
		}
		if _, err := os.Stat(h.state); !os.IsNotExist(err) {
			t.Fatalf("the hook created state: %v", err)
		}
	})

	t.Run("a call that is not the module's passes through although the state directory is unusable [verifies SP-740~1]", func(t *testing.T) {
		h := claudeHost(t)
		blocker := filepath.Join(t.TempDir(), "file")
		writeFile(t, blocker, "not a directory")
		variables := map[string]string{stateVariable: filepath.Join(blocker, "state")}
		var out, errOut bytes.Buffer
		code := cli.Run(cli.Env{
			Binary: binaryName, Stdin: strings.NewReader(`{"tool_name":"Agent","tool_input":{"subagent_type":"general-purpose","prompt":"x"}}`),
			Stdout: &out, Stderr: &errOut, WorkDir: h.root, HomeDir: h.state,
			LookupEnv: func(name string) (string, bool) { value, found := variables[name]; return value, found },
			Now:       func() time.Time { return h.now }, Entropy: h.entropy,
		}, []string{"hook", "pre-tool"})
		if code != cli.ExitClean || out.Len() != 0 {
			t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), errOut.String())
		}
	})

	t.Run("the host settings are read from the shared configuration, and a malformed one is a tool failure [verifies SP-740~1]", func(t *testing.T) {
		h := claudeHost(t)
		h.writeConfig("host:\n  sampling: [unterminated\n")
		if code, _, _ := h.run("", "mcp", "--role", "executor", "--harness", "claude"); code == cli.ExitClean {
			t.Fatalf("a malformed configuration was served")
		}
	})
}
