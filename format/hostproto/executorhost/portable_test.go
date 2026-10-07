package executorhost_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/executorhost"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/hostprototest"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
)

const (
	projectDirectory = "/work/project"
	rootsID          = "sp-roots-1"
)

func portableIdentity() harness.Identity {
	return harness.Identity{Plugin: hostprototest.PluginName, Agents: hostproto.Agents()}
}

// portableService is the service a server started without `--harness` builds: the
// context of the portable harness, whatever the project's policy says.
func (f *fixture) portableService(requireAttestation bool) executorhost.Service {
	portable := harness.Portable(portableIdentity())
	service := f.requests
	service.RequireAttestation = portable.AttestationRequired(requireAttestation)
	service.Executor.Harness = format.Harness{Name: portable.Name, Version: portable.Version}
	service.Executor.Isolation = portable.Isolation()
	service.Executor.Channel = portable.AnswerChannel()
	return executorhost.Service{Requests: service, ModuleNames: []string{moduleAlias}}
}

func (f *fixture) toolCall(id int, name string, arguments any) string {
	encoded, err := json.Marshal(arguments)
	if err != nil {
		f.t.Fatal(err)
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, id, name, encoded)
}

func serve(t *testing.T, service mcpstdio.ExecutorService, client ...string) []map[string]any {
	t.Helper()
	var input, output bytes.Buffer
	for _, line := range client {
		input.WriteString(line + "\n")
	}
	err := executorhost.Serve(context.Background(), executorhost.Options{ServerName: "portable", Version: "0", Service: service}, &input, &output)
	if err != nil {
		t.Fatal(err)
	}
	var messages []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
		var message map[string]any
		if err := json.Unmarshal([]byte(line), &message); err != nil {
			t.Fatalf("server line %q: %v", line, err)
		}
		messages = append(messages, message)
	}
	return messages
}

func initialize(capabilities string) []string {
	return []string{
		fmt.Sprintf(`{"jsonrpc":"2.0","id":0,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":%s}}`, capabilities),
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
	}
}

func TestPortableServerAnswersInline(t *testing.T) {
	t.Run("an answer taken through the portable server is recorded inline and journaled as exposed, whatever the project requires [verifies SP-1700~1]", func(t *testing.T) {
		for _, requireAttestation := range []bool{true, false} {
			f := newFixture(t, requireAttestation)
			service := f.portableService(requireAttestation)
			client := append(initialize(`{}`),
				f.toolCall(1, hostproto.ToolFetchRequest, map[string]string{"ticket": f.ticket.TicketID.String(), "cap": ""}),
				f.toolCall(2, hostproto.ToolSubmitResponse, map[string]any{
					"ticket": f.ticket.TicketID.String(), "nonce": f.ticket.Nonce, "output": json.RawMessage(validAnswer), "cap": "",
				}))
			serve(t, service, client...)
			executor := f.response().Executor
			if executor.Channel != format.ChannelHostInline || executor.Isolation != format.IsolationInline ||
				executor.Harness.Name != hostproto.HarnessAgentPlugins || executor.Attestation.AgentID != "" {
				t.Errorf("require attestation %v: executor = %+v", requireAttestation, executor)
			}
			if !slices.Contains(f.kinds(), hostproto.EventExposed) || slices.Contains(f.kinds(), hostproto.EventRestricted) {
				t.Errorf("require attestation %v: events = %v", requireAttestation, f.kinds())
			}
		}
	})

	t.Run("the isolation and the channel come from the harness of the server, not from the request it serves [verifies SP-1700~1]", func(t *testing.T) {
		portable := harness.Portable(portableIdentity())
		if portable.Isolation() != format.IsolationInline || portable.AnswerChannel() != format.ChannelHostInline || portable.AttestationRequired(true) {
			t.Errorf("portable: isolation %s, channel %s, attestation %v", portable.Isolation(), portable.AnswerChannel(), portable.AttestationRequired(true))
		}
		for _, other := range harness.All(portableIdentity()) {
			if other.AnswerChannel() != format.ChannelHostSubagent || other.Isolation() == format.IsolationInline {
				t.Errorf("%s: channel %s, isolation %s", other.Flag, other.AnswerChannel(), other.Isolation())
			}
		}
		if found, err := harness.ByFlag(portableIdentity(), harness.FlagPortable); err != nil || found.Flag != harness.FlagPortable {
			t.Errorf("ByFlag(portable) = %+v, %v", found, err)
		}
		if slices.Contains(harness.Flags(), harness.FlagPortable) {
			t.Errorf("the portable harness is no harness a module installs files for: %v", harness.Flags())
		}
	})
}

func TestPortableServerFindsTheProject(t *testing.T) {
	t.Run("the project is the first root the client lists, learned at the first tool call [verifies SP-1701~1]", func(t *testing.T) {
		f := newFixture(t, false)
		var asked []string
		lazy := &executorhost.LazyService{Build: func(roots []string) (executorhost.Service, error) {
			asked = roots
			return f.portableService(false), nil
		}}
		client := append(initialize(`{"roots":{}}`),
			f.toolCall(1, hostproto.ToolFetchRequest, map[string]string{"ticket": f.ticket.TicketID.String(), "cap": ""}),
			fmt.Sprintf(`{"jsonrpc":"2.0","id":%q,"result":{"roots":[{"uri":"file://%s","name":"project"},{"uri":"https://example.com/x"},{"uri":"file://localhost/work/other%%20dir"},{"uri":"file://remote/x"},{"uri":"file:///bad%%zz"}]}}`, rootsID, projectDirectory),
			f.toolCall(2, hostproto.ToolFetchRequest, map[string]string{"ticket": f.ticket.TicketID.String(), "cap": ""}))
		messages := serve(t, lazy, client...)
		if !slices.Equal(asked, []string{projectDirectory, "/work/other dir"}) {
			t.Errorf("roots given to the build = %v", asked)
		}
		methods := 0
		for _, message := range messages {
			if message["method"] == mcpstdio.MethodRootsList {
				methods++
			}
		}
		if methods != 1 {
			t.Errorf("roots/list asked %d times, want once", methods)
		}
	})

	t.Run("a client with no roots capability is never asked, and the working directory stands in [verifies SP-1701~1]", func(t *testing.T) {
		f := newFixture(t, false)
		var asked []string
		lazy := &executorhost.LazyService{Build: func(roots []string) (executorhost.Service, error) {
			asked = roots
			return f.portableService(false), nil
		}}
		messages := serve(t, lazy, append(initialize(`{}`), f.toolCall(1, hostproto.ToolFetchRequest, map[string]string{"ticket": f.ticket.TicketID.String(), "cap": ""}))...)
		for _, message := range messages {
			if message["method"] == mcpstdio.MethodRootsList {
				t.Errorf("roots/list was asked of a client that declared no roots: %v", message)
			}
		}
		if len(asked) != 0 || executorhost.PickRoot("", "", asked) != "" {
			t.Errorf("roots = %v", asked)
		}
	})

	t.Run("the directory given, then the one the client exports, then the first root wins [verifies SP-1701~1]", func(t *testing.T) {
		roots := []string{"/a", "/b"}
		cases := []struct{ given, exported, want string }{
			{"/g", "/e", "/g"}, {"", "/e", "/e"}, {"", "", "/a"},
		}
		for _, c := range cases {
			if got := executorhost.PickRoot(c.given, c.exported, roots); got != c.want {
				t.Errorf("PickRoot(%q, %q) = %q, want %q", c.given, c.exported, got, c.want)
			}
		}
		if got := executorhost.PickRoot("", "", nil); got != "" {
			t.Errorf("PickRoot with nothing = %q", got)
		}
	})

	t.Run("a directory with no project is refused with a message that says so, and the next call tries again [verifies SP-1701~1]", func(t *testing.T) {
		f := newFixture(t, false)
		builds := 0
		lazy := &executorhost.LazyService{Build: func([]string) (executorhost.Service, error) {
			builds++
			if builds == 1 {
				return executorhost.Service{}, executorhost.ErrNoProject
			}
			return f.portableService(false), nil
		}}
		call := f.toolCall(1, hostproto.ToolFetchRequest, map[string]string{"ticket": f.ticket.TicketID.String(), "cap": ""})
		messages := serve(t, lazy, append(initialize(`{}`), call, strings.Replace(call, `"id":1`, `"id":2`, 1))...)
		var texts []string
		for _, message := range messages {
			if result, ok := message["result"].(map[string]any); ok && result["isError"] == true {
				texts = append(texts, fmt.Sprint(result["content"]))
			}
		}
		if len(texts) != 1 || !strings.Contains(texts[0], executorhost.ErrNoProject.Error()) || builds != 2 {
			t.Errorf("errors = %v, builds = %d", texts, builds)
		}
		if _, err := (&executorhost.LazyService{Build: func([]string) (executorhost.Service, error) {
			return executorhost.Service{}, executorhost.ErrNoProject
		}}).FetchRequest(context.Background(), mcpstdio.FetchArgs{}); !errors.Is(err, executorhost.ErrNoProject) {
			t.Errorf("FetchRequest error = %v", err)
		}
	})
}
