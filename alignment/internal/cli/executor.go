package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/alignment/internal/adapters/responsecheck"
	"github.com/RachidChabane/musthold/alignment/internal/adapters/transcriptfile"
	"github.com/RachidChabane/musthold/alignment/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/alignment/internal/version"
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/executorhost"
	"github.com/RachidChabane/musthold/format/hostproto/hookhandler"
	"github.com/RachidChabane/musthold/format/hostproto/hostsettings"
	"github.com/RachidChabane/musthold/format/hostproto/requests"
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
	serverName = pluginName + "-exec"
	// drainAlias is the short name `drain` accepts for the module.
	drainAlias = "alignment"
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

// hostSettingsOf reads the `host` section of the configuration the modules
// share; a project without one has the defaults.
func hostSettingsOf(root string) (hostsettings.Settings, error) {
	data, err := os.ReadFile(filepath.Join(root, stateDirectory, sharedConfigFile))
	if errors.Is(err, os.ErrNotExist) {
		return hostsettings.Settings{}, nil
	}
	if err != nil {
		return hostsettings.Settings{}, fmt.Errorf("read the configuration: %w", err)
	}
	return hostsettings.Parse(data)
}

func (e Env) executorService(root string, harness hostharness.Harness) (executorhost.Service, bool, error) {
	settings, err := hostSettingsOf(root)
	if err != nil {
		return executorhost.Service{}, false, err
	}
	store, err := e.hostStore(root)
	if err != nil {
		return executorhost.Service{}, false, err
	}
	service := requests.Service{
		Plugin: pluginName, Store: store, Validator: responsecheck.New(), Clock: e.Now,
		RequireAttestation: harness.AttestationRequired(settings.AttestationRequired()),
		Executor: requests.ExecutorContext{
			Harness:   format.Harness{Name: harness.Name, Version: harness.Version},
			Isolation: harness.Isolation(),
			Channel:   harness.AnswerChannel(),
		},
	}
	return executorhost.Service{Requests: service, ModuleNames: []string{drainAlias, moduleName}}, settings.Sampling, nil
}

// mcp serves the executor tools over standard input and output until the client
// closes the stream.
func (e Env) mcp(args []string) outcome {
	set := e.newFlags(commandMCP, "")
	role := set.String(flagRole, "", "the role the server plays: "+roleExecutor)
	harnessFlag := set.String(flagHarness, hostharness.FlagPortable, "the host harness: one of "+fmt.Sprint(hostharness.Flags())+", or "+hostharness.FlagPortable+" for a client of the Agent Plugins standard")
	root := set.String(flagRoot, "", usageRoot)
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
	if harness.Flag == hostharness.FlagPortable {
		return e.servePortable(*root, harness)
	}
	resolved, err := e.hostRoot(*root)
	if err != nil {
		return outcome{err: err}
	}
	service, sampling, err := e.executorService(resolved, harness)
	if err != nil {
		return outcome{err: err}
	}
	return outcome{err: executorhost.Serve(context.Background(), executorhost.Options{
		ServerName: serverName, Version: version.Version, Sampling: sampling, Service: service,
	}, e.Stdin, e.Stdout)}
}

// servePortable serves the executor tools to a client of the Agent Plugins
// standard, which starts the server in the plugin root: the project is learned
// from the client's roots at the first tool call, and the answers are inline.
func (e Env) servePortable(given string, harness hostharness.Harness) outcome {
	service := &executorhost.LazyService{Build: func(roots []string) (executorhost.Service, error) {
		resolved, err := e.hostRoot(executorhost.PickRoot(given, e.lookup(envClaudeProjectDir), roots))
		if err != nil {
			return executorhost.Service{}, err
		}
		if _, err := os.Stat(filepath.Join(resolved, stateDirectory, sharedConfigFile)); err != nil {
			return executorhost.Service{}, fmt.Errorf("%w (looked in %s)", executorhost.ErrNoProject, resolved)
		}
		built, _, err := e.executorService(resolved, harness)
		return built, err
	}}
	return outcome{err: executorhost.Serve(context.Background(), executorhost.Options{
		ServerName: serverName, Version: version.Version, Service: service,
	}, e.Stdin, e.Stdout)}
}

// hook serves one hook of a harness: the payload comes on standard input and the
// decision, if any, goes to standard output.
func (e Env) hook(args []string) outcome {
	set := e.newFlags(commandHook, "<kind>")
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
	if !kind.Valid() {
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
		return outcome{err: refusal{err}}
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
	// The plugin's hooks run in every project of the user: one that has no
	// configuration of the modules is none of their business.
	if _, err := os.Stat(filepath.Join(root, stateDirectory, sharedConfigFile)); err != nil {
		return nil, nil
	}
	open := func() (hookhandler.Store, error) { return e.hostStore(root) }
	return hookhandler.Handler{Harness: harness, Open: open, Transcripts: transcriptfile.Reader{}}.Handle(kind, payload)
}
