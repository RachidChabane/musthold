package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/checks"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

// Refusals of a run. They are reported before anything is started.
var (
	ErrOutsideWindow = errors.New("outside the nightly window")
	ErrNoClaudeAuth  = errors.New("no isolated Claude Code credential")
	ErrMissingBinary = errors.New("binary not built")
	ErrPrepare       = errors.New("the project could not be prepared")
	// ErrNeverStarted is the refusal of a session that ended before its first
	// turn: what its stream would show says nothing about the harness.
	ErrNeverStarted = errors.New("the session ended before its first turn")
)

// Options select what a run does.
type Options struct {
	Harness  string
	RepoRoot string
	// Canary is the path of the canary executable, which the shims and probes call.
	Canary string
	// OutDir is the directory the run directory is created in.
	OutDir string
	// DryRun prints the command lines and runs nothing.
	DryRun bool
	// PrepareOnly prepares the project and runs the probes that make no model call,
	// then stops before the session.
	PrepareOnly bool
	// ForceWindow allows a run outside the nightly window.
	ForceWindow bool
	// SharedHome lets Claude Code use the user's own configuration directory for its
	// credentials, when no isolated credential is given, and lets Copilot CLI use
	// the user's own COPILOT_HOME.
	SharedHome bool
	// CodexHome is a dedicated Codex home; empty uses the user's.
	CodexHome string
	// Model is the model the session runs on; empty uses the default of the harness.
	Model string
}

// Runner runs canary sessions.
type Runner struct {
	Clock     Clock
	Window    window.Window
	Processes Processes
	Sessions  Sessions
	Prober    ServerProber
	Locker    Locker
	Files     Files
	// Env is the environment of the canary process.
	Env    []string
	Stdout io.Writer
	Stderr io.Writer
}

// Outcome is what a run came to.
type Outcome struct {
	RunDir string
	Report verdict.Report
}

// Run plans, prepares and runs one session, then evaluates it. With DryRun it
// prints the plan and with PrepareOnly it stops before the session.
func (r Runner) Run(ctx context.Context, options Options) (Outcome, error) {
	if _, err := checks.ForHarness(options.Harness); err != nil {
		return Outcome{}, err
	}
	r = r.forHarness(options.Harness)
	now := r.Clock.Now()
	inWindow := r.Window.Allows(now)
	homes, authErr := r.homes(options)
	if options.DryRun {
		return Outcome{}, r.printPlan(options, now, inWindow, homes, authErr)
	}
	if !inWindow && !options.ForceWindow {
		return Outcome{}, fmt.Errorf("%w: it is %s and the window is %s; pass --force-window to run anyway", ErrOutsideWindow, now.Format("15:04"), r.Window.Describe())
	}
	if authErr != nil {
		return Outcome{}, authErr
	}
	lock, err := r.Locker.Acquire(filepath.Join(options.RepoRoot, lockRelative))
	if err != nil {
		return Outcome{}, err
	}
	defer lock.Release()
	if err := r.checkMemory(ctx, options); err != nil {
		return Outcome{}, err
	}
	session, err := r.prepare(ctx, options, homes)
	if err != nil {
		return Outcome{RunDir: session.layout.run}, err
	}
	if options.PrepareOnly {
		return Outcome{RunDir: session.layout.run}, r.reportPrepared(session)
	}
	return r.conduct(ctx, options, session)
}

// forHarness returns the runner with the environment a harness's session may
// inherit: a Copilot CLI session never inherits the variables that trust the
// working directory, approve every tool or reroute the model.
func (r Runner) forHarness(flag string) Runner {
	if flag != harness.FlagCopilot {
		return r
	}
	kept := make([]string, 0, len(r.Env))
	for _, pair := range r.Env {
		if name, _, _ := strings.Cut(pair, "="); !launch.DropsCopilotVariable(name) {
			kept = append(kept, pair)
		}
	}
	r.Env = kept
	return r
}

// homes says where each harness CLI keeps its credentials and state, and refuses
// a Claude Code run that has no credential to keep out of the user's directory.
func (r Runner) homes(options Options) (Homes, error) {
	isolated, err := r.isolatedClaude(options)
	return Homes{
		IsolatedClaude: isolated, CodexHome: options.CodexHome,
		IsolatedCopilot: options.Harness == harness.FlagCopilot && !options.SharedHome,
	}, err
}

// isolatedClaude decides where Claude Code keeps its credentials and state: an
// isolated directory inside the run when a credential is given in the environment,
// the user's own directory only on request, and otherwise a refusal. It reports
// whether the session is isolated.
func (r Runner) isolatedClaude(options Options) (bool, error) {
	if options.Harness != harness.FlagClaude || options.SharedHome {
		return false, nil
	}
	for _, pair := range r.Env {
		if strings.HasPrefix(pair, oauthTokenVariable+"=") {
			return true, nil
		}
	}
	return false, fmt.Errorf("%w: set %s (from `claude setup-token`) for a run that touches nothing of ~/.claude, or pass --shared-home to use the credentials of ~/.claude", ErrNoClaudeAuth, oauthTokenVariable)
}

func (r Runner) printPlan(options Options, now time.Time, inWindow bool, homes Homes, authErr error) error {
	lines, err := Plan(options.Harness, options.RepoRoot, options.Model, homes)
	if err != nil {
		return err
	}
	state := "open"
	if !inWindow {
		state = "closed (--force-window needed)"
	}
	fmt.Fprintf(r.Stdout, "harness: %s\nnow: %s, window %s: %s\n", options.Harness, now.Format("15:04"), r.Window.Describe(), state)
	if authErr != nil {
		fmt.Fprintf(r.Stdout, "credentials: %v\n", authErr)
	}
	if options.Harness == harness.FlagCopilot {
		fmt.Fprintf(r.Stdout, "credentials: %s\n", r.copilotCredentialSource())
	}
	fmt.Fprintln(r.Stdout, "commands, in order (the run directory and the ticket are decided at run time):")
	for index, line := range lines {
		fmt.Fprintf(r.Stdout, "%2d. %s\n", index+1, line)
	}
	fmt.Fprintln(r.Stdout, "outside the run directory the run writes:")
	for _, line := range touches(options, homes) {
		fmt.Fprintf(r.Stdout, "  - %s\n", line)
	}
	return nil
}

// touches says what a run writes outside its own directory, which is what the
// session's fingerprints of the user's configuration directories then confirm.
func touches(options Options, homes Homes) []string {
	switch {
	case options.Harness == harness.FlagCopilot && homes.IsolatedCopilot:
		return []string{"nothing: Copilot CLI runs with COPILOT_HOME, COPILOT_CACHE_HOME and --log-dir inside the run directory; the system keychain is read for the login"}
	case options.Harness == harness.FlagCopilot:
		return []string{
			"$COPILOT_HOME/session-state (default ~/.copilot/session-state): the events of the session and of its subagent",
			"~/.copilot/: any other file Copilot CLI keeps there (a refreshed login, its settings, the plugin cache); settings.json, mcp-config.json and the installed plugins are read; the cache and the logs stay in the run directory",
		}
	case options.Harness == harness.FlagClaude && homes.IsolatedClaude:
		return []string{"nothing: Claude Code runs with CLAUDE_CONFIG_DIR inside the run directory and CLAUDE_CODE_OAUTH_TOKEN for its credential"}
	case options.Harness == harness.FlagClaude:
		return []string{
			"~/.claude/projects/<the run's project path>/: the transcripts of the session and of its subagents",
			"~/.claude.json: the entry Claude Code adds for the new project and its usage statistics",
			"~/.claude/: any other file Claude Code keeps there (history, caches, a refreshed credential); settings.json is not read (--setting-sources project) and not written by the canary",
		}
	case options.CodexHome != "":
		return []string{"nothing outside " + options.CodexHome + ", the dedicated CODEX_HOME the credentials come from"}
	default:
		return []string{
			"$CODEX_HOME/sessions (default ~/.codex/sessions): the rollout of the session and of its subagents",
			"~/.codex/config.toml: read for the hooks of the session; the table Codex persists there for the trusted project is removed after the session",
			"~/.codex/: any other file Codex keeps there (a refreshed credential)",
		}
	}
}

func (r Runner) checkMemory(ctx context.Context, options Options) error {
	output, err := r.Processes.Run(ctx, CommandSpec{
		Argv: []string{filepath.Join(options.RepoRoot, memGuardRelative), "check"}, Dir: options.RepoRoot, Env: r.Env, Timeout: supportTimeout,
	})
	if err != nil {
		return err
	}
	if output.ExitCode != 0 {
		return fmt.Errorf("the memory guard refused the run: %s", strings.TrimSpace(output.Stderr))
	}
	return nil
}

// preparedSession is a run whose project is ready and whose no-call probes ran.
type preparedSession struct {
	layout    layout
	selection Selection
	session   evidence.Session
	noCall    evidence.NoCall
	command   launch.Command
	env       []string
	// before is the fingerprint of the user's configuration directories taken
	// before anything of the run touched them.
	before map[string]string
}

// prepare builds the run directory, the project and the shims, makes the module
// ask the host for tickets, and runs the probes that make no model call.
func (r Runner) prepare(ctx context.Context, options Options, homes Homes) (preparedSession, error) {
	var prepared preparedSession
	prepared.before = r.fingerprint(r.configRoots(options))
	run, err := r.Files.MakeDir(options.OutDir, runDirPrefix+options.Harness+"-")
	if err != nil {
		return prepared, err
	}
	l := newLayout(options.RepoRoot, run)
	prepared.layout = l
	input := l.input(Selection{}, homes, options.Model)
	dirs := []string{l.state, l.cache, l.tmp, l.records, l.shim, input.ClaudeConfigDir}
	if options.Harness == harness.FlagCopilot {
		dirs = append(dirs, input.Copilot.Home, input.Copilot.Cache, input.Copilot.Logs)
	}
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		if err := r.Files.MkdirAll(dir); err != nil {
			return prepared, err
		}
	}
	for _, module := range modules.All() {
		if !r.Files.Exists(l.binary(module.Binary)) {
			return prepared, fmt.Errorf("%w: %s (run script/canary-build)", ErrMissingBinary, l.binary(module.Binary))
		}
	}
	if !r.Files.Exists(l.shallnot()) {
		return prepared, fmt.Errorf("%w: %s (run script/canary-build)", ErrMissingBinary, l.shallnot())
	}
	if err := r.Files.CopyTree(l.fixture(), l.project); err != nil {
		return prepared, err
	}
	if err := r.Files.Write(l.projectFile("shallnot.yaml"), []byte(projectShallnotConfig), false); err != nil {
		return prepared, err
	}
	support := l.supportEnvironment(r.Env)
	if err := r.installHarnessFiles(ctx, options, l, support); err != nil {
		return prepared, err
	}
	for _, module := range modules.All() {
		if err := r.installShim(options, l, module); err != nil {
			return prepared, err
		}
	}
	cliEnvironment := support
	if options.Harness == harness.FlagCopilot {
		cliEnvironment = environment(support, l.copilotEnvironment(homes)...)
	}
	prepared.session = evidence.Session{Harness: options.Harness, Versions: r.versions(ctx, options, l, support, cliEnvironment)}
	listing, err := r.askHost(ctx, l, support)
	if err != nil {
		return prepared, err
	}
	prepared.selection, err = SelectTickets(listing)
	if err != nil {
		return prepared, fmt.Errorf("%w: %v", ErrPrepare, err)
	}
	prepared.command, err = launch.Build(options.Harness, l.input(prepared.selection, homes, options.Model))
	if err != nil {
		return prepared, err
	}
	prepared.noCall = r.noCallProbes(ctx, options, l, probeEnvironments{support: support, cli: environment(support, prepared.command.Env...)}, prepared.selection)
	prepared.env = l.sessionEnvironment(r.Env, prepared.command.Env)
	prepared.session = r.describe(options, prepared, configMode(options, homes))
	return prepared, r.writeJSON(l.record(evidence.NoCallFile), prepared.noCall)
}

// configMode says where the harness CLI keeps its credentials and state.
func configMode(options Options, homes Homes) string {
	switch {
	case options.Harness == harness.FlagClaude && homes.IsolatedClaude, options.Harness == harness.FlagCopilot && homes.IsolatedCopilot:
		return evidence.ConfigIsolated
	case options.Harness == harness.FlagCodex && options.CodexHome != "":
		return evidence.ConfigDedicated
	default:
		return evidence.ConfigShared
	}
}

func (r Runner) describe(options Options, prepared preparedSession, mode string) evidence.Session {
	session := prepared.session
	session.ConfigMode = mode
	model := launch.Model(options.Harness, options.Model)
	session.Model, session.Ticket, session.Role, session.ProbeTicket = model, prepared.selection.Ticket, prepared.selection.Role, prepared.selection.ProbeTicket
	session.Argv = prepared.command.Argv
	session.EventCap, session.TimeoutSeconds = launch.EventCap(options.Harness), launch.TimeoutSeconds
	session.EditFile, session.EditFrom, session.EditTo = launch.EditFile, launch.EditFrom, launch.EditTo
	return session
}

func (r Runner) installHarnessFiles(ctx context.Context, options Options, l layout, env []string) error {
	switch options.Harness {
	case harness.FlagClaude:
		settings, err := launch.ClaudeSettings(options.Canary, l.hooksLog())
		if err != nil {
			return err
		}
		return r.Files.Write(l.projectFile(launch.ClaudeSettingsPath), settings, false)
	case harness.FlagCodex:
		for _, module := range modules.All() {
			output, err := r.Processes.Run(ctx, CommandSpec{Argv: initArgv(l, module), Dir: l.project, Env: env, Timeout: supportTimeout})
			if err != nil || output.ExitCode != 0 {
				return fmt.Errorf("%w: %s init: %v %s", ErrPrepare, module.Binary, err, strings.TrimSpace(output.Stderr))
			}
		}
		hooks, err := r.Files.Read(l.projectFile(launch.CodexHooksPath))
		if err != nil {
			return err
		}
		merged, err := launch.MergeCodexProbes(hooks, options.Canary, l.hooksLog())
		if err != nil {
			return err
		}
		return r.Files.Write(l.projectFile(launch.CodexHooksPath), merged, false)
	case harness.FlagCopilot:
		return nil
	default:
		return fmt.Errorf("%w %q", checks.ErrUnknownHarness, options.Harness)
	}
}

// installShim writes the executable that stands in for a module binary on the PATH
// of the session. It exports the variables of the run itself, because a harness may
// start an MCP server or a hook with a reduced environment, and the module would
// then keep its state in the user's home.
func (r Runner) installShim(options Options, l layout, module modules.Module) error {
	var script strings.Builder
	script.WriteString("#!/bin/sh\n")
	for _, variable := range l.shimExports() {
		name, value, _ := strings.Cut(variable, "=")
		fmt.Fprintf(&script, "export %s=%s\n", name, launch.Quote(value))
	}
	fmt.Fprintf(&script, "exec %s %s %s %s %s %s -- \"$@\"\n",
		launch.Quote(options.Canary), launch.ShimSubcommand, launch.FlagReal, launch.Quote(l.binary(module.Binary)), launch.FlagRecord, launch.Quote(l.hooksLog()))
	return r.Files.Write(filepath.Join(l.shim, module.Binary), []byte(script.String()), true)
}

// versions records the versions of the CLI and of the binaries the run uses.
func (r Runner) versions(ctx context.Context, options Options, l layout, env, cliEnv []string) map[string]string {
	found := map[string]string{}
	ask := func(name string, argv []string, env []string) {
		output, err := r.Processes.Run(ctx, CommandSpec{Argv: argv, Dir: l.project, Env: env, Timeout: supportTimeout})
		switch {
		case err != nil:
			found[name] = "unavailable: " + err.Error()
		default:
			found[name] = strings.TrimSpace(output.Stdout + output.Stderr)
		}
	}
	ask(options.Harness, cliVersionArgv(l, options.Harness), cliEnv)
	for _, module := range modules.All() {
		ask(module.Binary, versionOfBinaryArgv(l.binary(module.Binary)), env)
	}
	if pin, err := r.Files.Read(filepath.Join(l.repo, "script", "shallnot.pin")); err == nil {
		found[shallnotBinary] = strings.TrimSpace(string(pin))
	}
	return found
}

// askHost makes the primary module run its proof until it needs the host, and
// returns the listing of the tickets it issued.
func (r Runner) askHost(ctx context.Context, l layout, env []string) ([]byte, error) {
	proved, err := r.Processes.Run(ctx, CommandSpec{Argv: proveArgv(l), Dir: l.project, Env: env, Timeout: proveTimeout})
	if err != nil {
		return nil, fmt.Errorf("%w: prove: %v", ErrPrepare, err)
	}
	if proved.ExitCode != exitAwaiting {
		return nil, fmt.Errorf("%w: prove exited %d, expected %d: %s%s", ErrPrepare, proved.ExitCode, exitAwaiting, proved.Stdout, proved.Stderr)
	}
	listed, err := r.Processes.Run(ctx, CommandSpec{Argv: listArgv(l), Dir: l.project, Env: env, Timeout: supportTimeout})
	if err != nil || listed.ExitCode != 0 {
		return nil, fmt.Errorf("%w: request list: %v %s", ErrPrepare, err, listed.Stderr)
	}
	return []byte(listed.Stdout), nil
}

// probeEnvironments are the environments of the no-call probes: the one the module
// binaries run in, and the one the harness CLI runs in, which carries its own
// configuration directory when the session has one.
type probeEnvironments struct{ support, cli []string }

// noCallProbes runs the checks that need no model: the start of each module's
// executor server, the harness CLI's own plugin validation (Claude Code) and a
// fetch_request with no capability.
func (r Runner) noCallProbes(ctx context.Context, options Options, l layout, environments probeEnvironments, selection Selection) evidence.NoCall {
	var found evidence.NoCall
	for _, module := range modules.All() {
		found.Servers = append(found.Servers, r.Prober.Handshake(ctx, ServerSpec{Binary: l.binary(module.Binary), Harness: options.Harness, Dir: l.project, Env: environments.support}))
	}
	if options.Harness == harness.FlagClaude {
		for _, module := range modules.All() {
			argv := validateArgv(l, module)
			output, err := r.Processes.Run(ctx, CommandSpec{Argv: argv, Dir: l.project, Env: environments.cli, Timeout: supportTimeout})
			result := evidence.PluginValidation{Dir: argv[len(argv)-1], ExitCode: output.ExitCode, Output: strings.TrimSpace(output.Stdout + output.Stderr)}
			if err != nil {
				result.ExitCode, result.Output = -1, err.Error()
			}
			found.PluginValidation = append(found.PluginValidation, result)
		}
	}
	if options.Harness == harness.FlagCopilot {
		found.CopilotListing = r.copilotListing(ctx, l, environments.cli)
		found.PluginTrees = r.inspectTrees(l)
	}
	found.CapProbe = r.Prober.Probe(ctx, ServerSpec{Binary: l.primary(), Harness: options.Harness, Dir: l.project, Env: environments.support}, selection.ProbeTicket)
	return found
}

func (r Runner) reportPrepared(prepared preparedSession) error {
	fmt.Fprintf(r.Stdout, "prepared: %s\nticket: %s (%s), probe ticket %s\n", prepared.layout.run, prepared.selection.Ticket, prepared.selection.Role, prepared.selection.ProbeTicket)
	fmt.Fprintf(r.Stdout, "the session would run:\n  %s\n  (in %s)\n", prepared.command.Line(), prepared.command.Dir)
	report, err := checks.Evaluate(prepared.session.Harness, evidence.Evidence{Session: prepared.session, NoCall: prepared.noCall})
	if err != nil {
		return err
	}
	fmt.Fprintln(r.Stdout, "no-call checks:")
	for _, v := range report.Verdicts {
		if slices.Contains(checks.NoCallNames(), v.Check) {
			fmt.Fprintf(r.Stdout, "  %-13s %-26s %s\n", v.Status, v.Check, v.Summary)
		}
	}
	return nil
}

// conduct runs the session, collects what it left, and evaluates it.
func (r Runner) conduct(ctx context.Context, options Options, prepared preparedSession) (Outcome, error) {
	l := prepared.layout
	session := prepared.session
	session.StartedAt = r.Clock.Now()
	outcome, err := r.Sessions.Run(SessionSpec{
		Argv: prepared.command.Argv, Dir: prepared.command.Dir, Env: prepared.env,
		StdoutPath: l.record(evidence.StreamFile), StderrPath: l.record(evidence.StderrFile),
		Counts: func(line []byte) bool { return launch.CountsAsEvent(options.Harness, line) },
		Cap:    launch.EventCap(options.Harness), Timeout: time.Duration(launch.TimeoutSeconds) * time.Second,
	})
	session.EndedAt = r.Clock.Now()
	session.ExitCode, session.Killed = outcome.ExitCode, outcome.Killed
	if err != nil {
		return Outcome{RunDir: l.run}, fmt.Errorf("the session could not run: %w", err)
	}
	touched := r.touched(r.configRoots(options), prepared.before, filepath.Base(l.run), r.restoreCodexTrust(options, l))
	if err := r.collect(options, l, session, touched); err != nil {
		return Outcome{RunDir: l.run}, err
	}
	if err := r.requireATurn(options, l, session); err != nil {
		return Outcome{RunDir: l.run}, err
	}
	report, err := r.Evaluate(l.records, options.Harness)
	return Outcome{RunDir: l.run, Report: report}, err
}

// collect writes the session's own record and copies what the module and the
// project left behind into the records directory.
func (r Runner) collect(options Options, l layout, session evidence.Session, touched evidence.Touched) error {
	if err := r.writeJSON(l.record(evidence.SessionFile), session); err != nil {
		return err
	}
	if err := r.writeJSON(l.record(evidence.TouchedFile), touched); err != nil {
		return err
	}
	journals, err := r.Files.Glob(filepath.Join(l.state, "spec-proof", modules.Primary().Dir, "*", "host", evidence.JournalFile))
	if err != nil {
		return err
	}
	if len(journals) > 0 {
		journal, err := r.Files.Read(journals[0])
		if err != nil {
			return err
		}
		if err := r.Files.Write(l.record(evidence.JournalFile), journal, false); err != nil {
			return err
		}
	}
	edited, err := r.Files.Read(l.projectFile(launch.EditFile))
	if err != nil {
		return err
	}
	if err := r.Files.Write(l.record(evidence.EditedFile), edited, false); err != nil {
		return err
	}
	if options.Harness == harness.FlagCodex || options.Harness == harness.FlagCopilot {
		probe := evidence.SandboxProbe{File: launch.SandboxProbeFile, Exists: r.Files.Exists(l.projectFile(launch.SandboxProbeFile))}
		if err := r.writeJSON(l.record(evidence.SandboxProbeFile), probe); err != nil {
			return err
		}
	}
	return r.collectRollouts(options, l, session)
}

// requireATurn refuses a session whose stream shows no model turn: the harness
// CLI never reached the model, so the setup is at fault and not the harness. The
// error carries the line the CLI printed to say why.
func (r Runner) requireATurn(options Options, l layout, session evidence.Session) error {
	data, err := r.Files.Read(l.record(evidence.StreamFile))
	if err != nil {
		return err
	}
	lines, err := evidence.Lines(evidence.StreamFile, data)
	if err != nil {
		return err
	}
	if evidence.StartedATurn(options.Harness, lines) {
		return nil
	}
	stderr, err := r.Files.Read(l.record(evidence.StderrFile))
	if err != nil {
		return err
	}
	reason := evidence.ErrorLine(string(stderr))
	if reason == "" {
		reason = fmt.Sprintf("the CLI printed nothing to its error stream (exit code %d)", session.ExitCode)
	}
	return fmt.Errorf("%w: %s", ErrNeverStarted, reason)
}

func (r Runner) writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return r.Files.Write(path, append(encoded, '\n'), false)
}

// configDirDepth is how deep below a configuration directory the fingerprint
// looks: its entries and, among them, the per-project and per-session folders.
const configDirDepth = 2

// configRoots are the places in the user's home where the harness CLI keeps its
// credentials, history and sessions.
func (r Runner) configRoots(options Options) []string {
	home := variable(r.Env, "HOME")
	switch options.Harness {
	case harness.FlagClaude:
		return []string{filepath.Join(home, ".claude.json"), filepath.Join(home, ".claude")}
	case harness.FlagCodex:
		return []string{r.codexHome(options)}
	case harness.FlagCopilot:
		copilotHome := variable(r.Env, launch.CopilotHomeVariable)
		if copilotHome == "" {
			copilotHome = filepath.Join(home, copilotUserDir)
		}
		return []string{copilotHome}
	default:
		return nil
	}
}

// variable is the value of an environment variable; as in a process environment,
// the last of several definitions wins.
func variable(environ []string, name string) string {
	value := ""
	for _, pair := range environ {
		if found, ok := strings.CutPrefix(pair, name+"="); ok {
			value = found
		}
	}
	return value
}

// fingerprint records the size and modification time of what the roots hold. A
// root that cannot be read has no entries: the check that reads the result then
// has nothing to attribute.
func (r Runner) fingerprint(roots []string) map[string]string {
	found := map[string]string{}
	for _, root := range roots {
		entries, err := r.Files.Fingerprint(root, configDirDepth)
		if err != nil {
			continue
		}
		maps.Copy(found, entries)
	}
	return found
}

// touched compares the fingerprint of the roots after the session with the one
// before it. A change whose path names the run's directory, or that is a file the
// session wrote a table naming its run into, is the session's own; any other
// change is counted, since another process of the user may have made it.
func (r Runner) touched(roots []string, before map[string]string, runName string, writes []evidence.ConfigWrite) evidence.Touched {
	after := r.fingerprint(roots)
	result := evidence.Touched{Roots: roots, Attributed: []string{}, Unattributed: []string{}, ConfigWrites: writes}
	written := func(path string) bool {
		return slices.ContainsFunc(writes, func(write evidence.ConfigWrite) bool { return write.Path == path })
	}
	changed := func(path string) {
		if strings.Contains(path, runName) || written(path) {
			result.Attributed = append(result.Attributed, path)
		} else {
			result.Unattributed = append(result.Unattributed, path)
		}
	}
	for path, now := range after {
		if was, existed := before[path]; !existed || was != now {
			changed(path)
		}
	}
	for path := range before {
		if _, remains := after[path]; !remains {
			changed(path)
		}
	}
	slices.Sort(result.Attributed)
	slices.Sort(result.Unattributed)
	return result
}
