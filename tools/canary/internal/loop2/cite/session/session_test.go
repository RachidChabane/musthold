package session_test

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/cite/session"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

const (
	token  = "sk-ant-oat01-citation-test-token-0123456789"
	prompt = "Cite the spec sentence behind each assertion."
	answer = `{"is_error":false,"structured_output":{"citations":[{"assertion":"test_rate_1.py:4","sentence":"s"}]}}`
)

type credential struct{}

func (credential) Source() string                        { return canary.SourceEnvironment }
func (credential) Present(context.Context) (bool, error) { return true, nil }
func (credential) Token(context.Context) (string, error) { return token, nil }

// processes records the docker commands around the session.
type processes struct{ commands [][]string }

func (p *processes) Run(_ context.Context, spec canary.CommandSpec) (canary.Output, error) {
	p.commands = append(p.commands, spec.Argv)
	return canary.Output{}, nil
}

// sessions records the session and writes what it prints.
type sessions struct {
	spec    canary.SessionSpec
	stdout  string
	outcome canary.SessionOutcome
}

func (s *sessions) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	s.spec = spec
	if err := os.WriteFile(spec.StdoutPath, []byte(s.stdout), 0o644); err != nil {
		return canary.SessionOutcome{}, err
	}
	return s.outcome, os.WriteFile(spec.StderrPath, nil, 0o644)
}

// versions answers `claude --version`.
type versions struct{}

func (versions) Run(context.Context, box.Command) (box.Result, error) {
	return box.Result{Stdout: []byte("2.1.295 (Claude Code)\n")}, nil
}

func call(t *testing.T, s *sessions) (*processes, construct.Reply, error) {
	t.Helper()
	p := &processes{}
	model := session.Claude{
		Docker: []string{"/repo/experiments/loop-2/script/docker"}, Image: "sha256:image", Product: "/work/product", UID: 501, GID: 20,
		Calls: t.TempDir(), Processes: p, Sessions: s, Box: versions{}, Credentials: []canary.ClaudeCredential{credential{}},
		Env: []string{"PATH=/usr/bin", "HOME=/Users/someone", "SSH_AUTH_SOCK=/tmp/agent.sock"}, Now: time.Now,
	}
	request := construct.Request{Role: "citer", Domain: "parcel-rates", Attempt: 1, Prompt: prompt, Schema: construct.Schema{JSON: []byte(`{"type":"object"}`)}}
	reply, err := model.Complete(context.Background(), request)
	return p, reply, err
}

func TestCitationSession(t *testing.T) {
	s := &sessions{stdout: answer}
	p, reply, err := call(t, s)
	if err != nil {
		t.Fatal(err)
	}
	argv := strings.Join(s.spec.Argv, " ")

	t.Run("the session runs under the live-session isolation: tmpfs HOME, read-only root, internal network, no host HOME, SSH agent or docker socket [verifies SP-2943~1]", func(t *testing.T) {
		for _, want := range []string{"--read-only", "--tmpfs " + container.ScratchHome, "--network spdl-net-", "--cap-drop ALL", "HOME=" + container.ScratchHome, "HTTPS_PROXY=" + container.ProxyURL()} {
			if !strings.Contains(argv, want) {
				t.Errorf("the session lacks %q:\n%s", want, argv)
			}
		}
		for _, forbidden := range []string{"/Users/someone", "SSH_AUTH_SOCK", "docker.sock", token} {
			if strings.Contains(argv, forbidden) {
				t.Errorf("the session's command holds %q", forbidden)
			}
		}
	})

	t.Run("the only route out is the logging proxy, which forwards only to the model endpoint [verifies SP-2943~1]", func(t *testing.T) {
		var proxy []string
		for _, command := range p.commands {
			if slices.Contains(command, container.ProxySubcommand) && slices.Contains(command, "--listen") {
				proxy = command
			}
		}
		var allowed []string
		for index, word := range proxy {
			if word == "--allow" {
				allowed = append(allowed, proxy[index+1])
			}
		}
		if !slices.Equal(allowed, container.ModelEndpoints()) || !slices.Contains(proxy, "--log") {
			t.Fatalf("proxy %v", proxy)
		}
		if !slices.ContainsFunc(p.commands, func(command []string) bool { return slices.Contains(command, "--internal") }) {
			t.Fatal("no internal network")
		}
	})

	t.Run("the token and then the prompt reach the entry on its standard input, which it forwards, and never a command line [verifies SP-2943~1] [verifies SP-2944~1]", func(t *testing.T) {
		if string(s.spec.Stdin) != token+"\n"+prompt {
			t.Fatalf("stdin %q", s.spec.Stdin)
		}
		if !strings.Contains(argv, container.EntrySubcommand+" --"+container.ForwardStdinFlag+" --") || strings.Contains(argv, prompt) {
			t.Fatalf("argv %s", argv)
		}
	})

	t.Run("claude runs in print mode for claude-opus-5-5 without tools, settings, MCP servers or a kept session [verifies SP-2936~1]", func(t *testing.T) {
		for _, want := range []string{"-p --model " + session.CitationModel + " --tools  --setting-sources  --strict-mcp-config", "--no-session-persistence", "--output-format json", "--json-schema"} {
			if !strings.Contains(argv, want) {
				t.Errorf("argv lacks %q", want)
			}
		}
	})

	t.Run("the answer is read as citations.json, with the client's version, and the containers and network are removed [verifies SP-2945~1]", func(t *testing.T) {
		if len(reply.Files) != 1 || reply.Files[0].Path != task.CitationsFile || reply.CLIVersion != "2.1.295 (Claude Code)" {
			t.Fatalf("%+v", reply)
		}
		last := strings.Join(p.commands[len(p.commands)-1], " ")
		removed := strings.Join(p.commands[len(p.commands)-2], " ")
		if !strings.Contains(last, "network rm") || !strings.Contains(removed, "rm --force") {
			t.Fatalf("teardown %v", p.commands)
		}
	})
}

func TestCitationSessionFailures(t *testing.T) {
	t.Run("a session ended at its time limit is a harness failure, and its containers are still removed [verifies SP-2945~1]", func(t *testing.T) {
		p, _, err := call(t, &sessions{outcome: canary.SessionOutcome{Killed: evidence.KilledTimeout, ExitCode: 143}})
		var harness *construct.HarnessFailure
		if !errors.As(err, &harness) || harness.Kind != construct.HarnessTimeout {
			t.Fatalf("got %v", err)
		}
		if !strings.Contains(strings.Join(p.commands[len(p.commands)-1], " "), "network rm") {
			t.Fatal("not torn down")
		}
	})

	t.Run("a usage limit without an answer is a harness failure, an error result a failure of the model [verifies SP-2945~1]", func(t *testing.T) {
		_, _, err := call(t, &sessions{stdout: "", outcome: canary.SessionOutcome{ExitCode: 1}})
		if !errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("no answer: %v", err)
		}
		_, _, err = call(t, &sessions{stdout: `{"is_error":true}`})
		if err == nil || errors.Is(err, construct.ErrHarnessFailure) {
			t.Fatalf("error result: %v", err)
		}
	})

	t.Run("a token the session printed is redacted from the reply and fails the call [verifies SP-2943~1]", func(t *testing.T) {
		_, reply, err := call(t, &sessions{stdout: `{"is_error":false,"result":"` + token + `"}`})
		if !errors.Is(err, session.ErrTokenInOutput) || strings.Contains(string(reply.Output), token) {
			t.Fatalf("%v %s", err, reply.Output)
		}
	})
}
