package cli

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/responsecheck"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/transcriptfile"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/executorhost"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/hooks"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/requests"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/discpower/internal/version"
	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/mcpstdio"
)

// Flags and values of `mcp` and `hook`.
const (
	flagRole     = "role"
	flagHarness  = "harness"
	roleExecutor = "executor"

	// envClaudeProjectDir is the project directory Claude Code gives its hooks.
	envClaudeProjectDir = "CLAUDE_PROJECT_DIR"
	// maxHookPayloadBytes bounds what a hook reads from standard input.
	maxHookPayloadBytes = 8 << 20
	// serverName is the name the executor server announces.
	serverName = "spec-proof-dp-exec"
)

var (
	errUnknownRole = errors.New("--role must be " + roleExecutor)
	errHookKind    = errors.New("hook needs one of pre-tool, subagent-start, subagent-stop, post-tool")
)

// hostRoot resolves the project root of a process a harness starts: the root
// given, the project directory the harness exports, or the working directory.
func (e Env) hostRoot(given string) (string, error) {
	if given == "" {
		given = e.lookup(envClaudeProjectDir)
	}
	return e.rootOf(given)
}

func (e Env) executorService(root string, harness hostharness.Harness) (executorhost.Service, bool, error) {
	settings, err := settingsOf(root)
	if err != nil {
		return executorhost.Service{}, false, err
	}
	store, err := e.hostStore(root)
	if err != nil {
		return executorhost.Service{}, false, err
	}
	service := requests.Service{
		Store: store, Validator: responsecheck.New(), Clock: e.Now,
		RequireAttestation: harness.AttestationRequired(settings.Host),
		Executor: requests.ExecutorContext{
			Harness:   format.Harness{Name: harness.Name, Version: harness.Version},
			Isolation: harness.Isolation(),
		},
	}
	return executorhost.Service{Requests: service}, settings.Host.Sampling, nil
}

// mcp serves the executor tools over standard input and output until the client
// closes the stream.
func (e Env) mcp(args []string) outcome {
	set := e.newFlags(commandMCP)
	role := set.String(flagRole, "", "the role the server plays: "+roleExecutor)
	harnessFlag := set.String(flagHarness, hostharness.FlagClaude, "the host harness: one of "+fmt.Sprint(hostharness.Flags()))
	root := set.String(flagRoot, "", "project root (default: the working directory)")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) > 0:
		return outcome{err: errUnexpectedParams}
	case *role != roleExecutor:
		return outcome{err: refusal{errUnknownRole}}
	}
	harness, err := hostharness.ByFlag(*harnessFlag)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	resolved, err := e.hostRoot(*root)
	if err != nil {
		return outcome{err: err}
	}
	service, sampling, err := e.executorService(resolved, harness)
	if err != nil {
		return outcome{err: err}
	}
	server := mcpstdio.NewServer(mcpstdio.Options{
		Info:            mcpstdio.Info{Name: serverName, Version: version.Version},
		SamplingEnabled: sampling,
		Service:         service,
	}, e.Stdin, e.Stdout)
	return outcome{err: server.Serve(context.Background())}
}

// hook serves one hook of a harness: the payload comes on standard input and the
// decision, if any, goes to standard output.
func (e Env) hook(args []string) outcome {
	set := e.newFlags(commandHook)
	harnessFlag := set.String(flagHarness, hostharness.FlagClaude, "the host harness: one of "+fmt.Sprint(hostharness.Flags()))
	root := set.String(flagRoot, "", "project root (default: the project directory of the harness, else the working directory)")
	rest, err := parseInterleaved(set, args)
	switch {
	case err != nil:
		return outcome{err: err}
	case len(rest) != 1:
		return outcome{err: refusal{errHookKind}}
	}
	kind := hostproto.HookKind(rest[0])
	if !isHookKind(kind) {
		return outcome{err: refusal{errHookKind}}
	}
	harness, err := hostharness.ByFlag(*harnessFlag)
	if err != nil {
		return outcome{err: refusal{err}}
	}
	payload, err := io.ReadAll(io.LimitReader(e.Stdin, maxHookPayloadBytes))
	if err != nil {
		return outcome{err: fmt.Errorf("read the hook payload: %w", err)}
	}
	output, err := e.runHook(*root, harness, kind, payload)
	if err != nil {
		return outcome{err: hookFailure(kind, err)}
	}
	if len(output) > 0 {
		if _, err := e.Stdout.Write(output); err != nil {
			return outcome{err: err}
		}
	}
	return outcome{code: ExitClean}
}

func (e Env) runHook(rootFlag string, harness hostharness.Harness, kind hostproto.HookKind, payload []byte) ([]byte, error) {
	root, err := e.hostRoot(rootFlag)
	if err != nil {
		return nil, err
	}
	store, err := e.hostStore(root)
	if err != nil {
		return nil, err
	}
	return hooks.Handler{Harness: harness, Store: store, Transcripts: transcriptfile.Reader{}}.Handle(kind, payload)
}

func isHookKind(kind hostproto.HookKind) bool {
	for _, known := range hostproto.HookKinds() {
		if known == kind {
			return true
		}
	}
	return false
}

// hookFailure classifies the error of a hook that failed: a PreToolUse hook
// blocks the call it could not check, as a tool failure does, and the others
// report without blocking, as a refusal does.
func hookFailure(kind hostproto.HookKind, err error) error {
	if kind == hostproto.HookPreTool {
		return err
	}
	return refusal{err}
}
