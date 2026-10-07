package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// credentials, when no isolated credential is given.
	SharedHome bool
	// CodexHome is a dedicated Codex home; empty uses the user's.
	CodexHome string
}

// Runner runs canary sessions.
type Runner struct {
	Clock     Clock
	Window    window.Window
	Processes Processes
	Sessions  Sessions
	Prober    CapProber
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
	now := r.Clock.Now()
	inWindow := r.Window.Allows(now)
	isolated, authErr := r.isolatedClaude(options)
	if options.DryRun {
		return Outcome{}, r.printPlan(options, now, inWindow, isolated, authErr)
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
	session, err := r.prepare(ctx, options, isolated)
	if err != nil {
		return Outcome{RunDir: session.layout.run}, err
	}
	if options.PrepareOnly {
		return Outcome{RunDir: session.layout.run}, r.reportPrepared(session)
	}
	return r.conduct(ctx, options, session)
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

func (r Runner) printPlan(options Options, now time.Time, inWindow bool, isolated bool, authErr error) error {
	lines, err := Plan(options.Harness, options.RepoRoot, isolated, options.CodexHome)
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
	fmt.Fprintln(r.Stdout, "commands, in order (the run directory and the ticket are decided at run time):")
	for index, line := range lines {
		fmt.Fprintf(r.Stdout, "%2d. %s\n", index+1, line)
	}
	return nil
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
}

// prepare builds the run directory, the project and the shims, makes the module
// ask the host for tickets, and runs the probes that make no model call.
func (r Runner) prepare(ctx context.Context, options Options, isolated bool) (preparedSession, error) {
	var prepared preparedSession
	run, err := r.Files.MakeDir(options.OutDir, runDirPrefix+options.Harness+"-")
	if err != nil {
		return prepared, err
	}
	l := newLayout(options.RepoRoot, run)
	prepared.layout = l
	claudeConfigDir := ""
	if isolated {
		claudeConfigDir = l.claudeConfig
	}
	for _, dir := range []string{l.state, l.cache, l.tmp, l.records, l.shim, claudeConfigDir} {
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
	prepared.session = evidence.Session{Harness: options.Harness, Versions: r.versions(ctx, options, l, support)}
	listing, err := r.askHost(ctx, l, support)
	if err != nil {
		return prepared, err
	}
	prepared.selection, err = SelectTickets(listing)
	if err != nil {
		return prepared, fmt.Errorf("%w: %v", ErrPrepare, err)
	}
	prepared.noCall = r.noCallProbes(ctx, options, l, support, prepared.selection)
	prepared.command, err = launch.Build(options.Harness, l.input(prepared.selection, claudeConfigDir, options.CodexHome))
	if err != nil {
		return prepared, err
	}
	prepared.env = l.sessionEnvironment(r.Env, prepared.command.Env)
	prepared.session = r.describe(options, prepared)
	return prepared, r.writeJSON(l.record(evidence.NoCallFile), prepared.noCall)
}

func (r Runner) describe(options Options, prepared preparedSession) evidence.Session {
	session := prepared.session
	model := launch.ClaudeModel
	if options.Harness == harness.FlagCodex {
		model = launch.CodexModel
	}
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
func (r Runner) versions(ctx context.Context, options Options, l layout, env []string) map[string]string {
	found := map[string]string{}
	ask := func(name string, argv []string) {
		output, err := r.Processes.Run(ctx, CommandSpec{Argv: argv, Dir: l.project, Env: env, Timeout: supportTimeout})
		switch {
		case err != nil:
			found[name] = "unavailable: " + err.Error()
		default:
			found[name] = strings.TrimSpace(output.Stdout + output.Stderr)
		}
	}
	ask(options.Harness, cliVersionArgv(options.Harness))
	for _, module := range modules.All() {
		ask(module.Binary, versionOfBinaryArgv(l.binary(module.Binary)))
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

// noCallProbes runs the checks that need no model: the harness CLI's own plugin
// validation (Claude Code) and a fetch_request with no capability.
func (r Runner) noCallProbes(ctx context.Context, options Options, l layout, env []string, selection Selection) evidence.NoCall {
	var found evidence.NoCall
	if options.Harness == harness.FlagClaude {
		for _, module := range modules.All() {
			argv := validateArgv(l, module)
			output, err := r.Processes.Run(ctx, CommandSpec{Argv: argv, Dir: l.project, Env: r.Env, Timeout: supportTimeout})
			result := evidence.PluginValidation{Dir: argv[len(argv)-1], ExitCode: output.ExitCode, Output: strings.TrimSpace(output.Stdout + output.Stderr)}
			if err != nil {
				result.ExitCode, result.Output = -1, err.Error()
			}
			found.PluginValidation = append(found.PluginValidation, result)
		}
	}
	found.CapProbe = r.Prober.Probe(ctx, ServerSpec{Binary: l.primary(), Harness: options.Harness, Dir: l.project, Env: env}, selection.ProbeTicket)
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
	if err := r.collect(options, l, session); err != nil {
		return Outcome{RunDir: l.run}, err
	}
	report, err := r.Evaluate(l.records, options.Harness)
	return Outcome{RunDir: l.run, Report: report}, err
}

// collect writes the session's own record and copies what the module and the
// project left behind into the records directory.
func (r Runner) collect(options Options, l layout, session evidence.Session) error {
	if err := r.writeJSON(l.record(evidence.SessionFile), session); err != nil {
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
	if options.Harness == harness.FlagCodex {
		probe := evidence.SandboxProbe{File: launch.SandboxProbeFile, Exists: r.Files.Exists(l.projectFile(launch.SandboxProbeFile))}
		return r.writeJSON(l.record(evidence.SandboxProbeFile), probe)
	}
	return nil
}

func (r Runner) writeJSON(path string, value any) error {
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return r.Files.Write(path, append(encoded, '\n'), false)
}
