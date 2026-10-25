package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

// Refusals of a live run, reported before anything starts.
var (
	ErrOutsideWindow = errors.New("outside the nightly window")
	ErrNoCredential  = errors.New("no Claude Code credential")
	ErrPinMismatch   = errors.New("the Claude Code binary is not the pinned version")
	ErrToolchain     = errors.New("a toolchain directory could not be resolved")
	ErrMemory        = errors.New("the memory guard refused the run")
)

// Names and bounds of a live run.
const (
	runDirPrefix      = "spec-proof-demo-loop-"
	resolveToolDirs   = "script/resolve-tool-dirs"
	supportTimeout    = 2 * time.Minute
	versionArgument   = "--version"
	moduleVersionVerb = "version"
	placeholderRun    = "<run-dir>"
)

// Runner runs the configured sessions of the agent loop.
type Runner struct {
	Clock     canary.Clock
	Window    window.Window
	Processes canary.Processes
	Sessions  canary.Sessions
	Files     canary.Files
	Locker    canary.Locker
	// Credentials are the sources of the Claude Code token, in order of preference.
	Credentials []canary.ClaudeCredential
	// Env is the environment of the runner. The token is dropped from it: only
	// the session receives it.
	Env    []string
	Stdout io.Writer
}

// Options select what a live run does.
type Options struct {
	RepoRoot string
	Config   config.Config
	// OutDir is the directory the run directories are created in.
	OutDir string
	// Executable is the runner's own binary, which the shims call.
	Executable string
	// Claude is the pinned Claude Code binary and ClaudePin the version it must report.
	Claude, ClaudePin string
	// DryRun prints the commands and starts nothing.
	DryRun bool
	// ForceWindow allows a run outside the nightly window.
	ForceWindow bool
}

// Run runs every session the configuration asks for, one at a time, and returns
// their run directories. With DryRun it prints what it would do and reads no
// credential value.
func (r Runner) Run(ctx context.Context, o Options) ([]string, error) {
	r.Env = launch.WithoutOAuthToken(r.Env)
	now := r.Clock.Now()
	open := r.Window.Allows(now)
	source, credentialErr := canary.FirstCredential(ctx, r.Credentials)
	if o.DryRun {
		return nil, r.printPlan(o, now, open, source, credentialErr)
	}
	if !open && !o.ForceWindow {
		return nil, fmt.Errorf("%w: it is %s and the window is %s; pass --force-window to run anyway", ErrOutsideWindow, now.Format("15:04"), r.Window.Describe())
	}
	if credentialErr != nil {
		return nil, credentialErr
	}
	if source == nil {
		return nil, fmt.Errorf("%w: store a token (from `claude setup-token`) in the keychain as service %s, or set %s", ErrNoCredential, canary.KeychainService, launch.OAuthTokenVariable)
	}
	lock, err := r.Locker.Acquire(filepath.Join(o.RepoRoot, canary.LockRelative))
	if err != nil {
		return nil, err
	}
	defer lock.Release()
	var dirs []string
	for _, run := range o.Config.Runs() {
		dir, err := r.one(ctx, o, run, source)
		if dir != "" {
			dirs = append(dirs, dir)
		}
		if err != nil {
			return dirs, err
		}
	}
	return dirs, nil
}

// one prepares, conducts, collects and renders one run.
func (r Runner) one(ctx context.Context, o Options, run config.Run, source canary.ClaudeCredential) (string, error) {
	if err := r.checkMemory(ctx, o); err != nil {
		return "", err
	}
	versions, err := r.versions(ctx, o)
	if err != nil {
		return "", err
	}
	toolchain, err := r.toolchain(ctx, o)
	if err != nil {
		return "", err
	}
	dir, err := r.Files.MakeDir(o.OutDir, runDirPrefix+run.Name()+"-")
	if err != nil {
		return "", err
	}
	l := plan.Layout{Repo: o.RepoRoot, Run: dir}
	leading := leadingDirs(l, toolchain, o.Claude)
	preparer := Preparer{Processes: r.Processes, Files: r.Files, Env: r.Env}
	prepared, err := preparer.Prepare(ctx, l, run, leading)
	if err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	session := sessionOf(o, l, run, prepared, leading)
	if err := r.ready(ctx, o, preparer, &prepared, session, run); err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	token, err := source.Token(ctx)
	if err != nil {
		return dir, fmt.Errorf("%w: the %s source could not give the token: %v", ErrNoCredential, source.Source(), err)
	}
	meta := metaOf(o, run, session, versions)
	meta.StartedAt = r.Clock.Now()
	outcome, err := r.Sessions.Run(canary.SessionSpec{
		Argv: session.command.Argv, Dir: session.command.Dir,
		Env:        append(append([]string{}, session.env...), launch.OAuthTokenVariable+"="+token),
		StdoutPath: l.Record(record.StreamFile), StderrPath: l.Record(record.StderrFile), TimesPath: l.Record(record.TimesFile),
		Counts:  func(line []byte) bool { return launch.CountsAsEvent(harness.FlagClaude, line) },
		Cap:     o.Config.Caps.AssistantMessages,
		Timeout: time.Duration(o.Config.Caps.Seconds) * time.Second,
	})
	meta.EndedAt, meta.ExitCode, meta.Killed = r.Clock.Now(), outcome.ExitCode, outcome.Killed
	if err != nil {
		return dir, fmt.Errorf("the session could not run: %w", err)
	}
	collector := Collector{Processes: r.Processes, Files: r.Files, Preparer: preparer}
	if err := collector.Collect(ctx, &prepared, meta); err != nil {
		return dir, err
	}
	if err := (Renderer{Files: r.Files}).Write(l.Records(), l.Records(), false); err != nil {
		return dir, err
	}
	fmt.Fprintf(r.Stdout, "run %s: %s\ntranscript: %s\n", run.Name(), dir, l.Record(record.TranscriptMarkdown))
	return dir, nil
}

// session is what the session of a run needs once its project is prepared.
type session struct {
	command launch.Command
	env     []string
	// exports are the run's variables the shims set.
	exports []string
}

func sessionOf(o Options, l plan.Layout, run config.Run, prepared Prepared, leading []string) session {
	command := plan.SessionCommand(plan.SessionInput{Layout: l, Run: run, Config: o.Config, Claude: o.Claude})
	variables := append(plan.Variables(l, leading), command.Env...)
	return session{command: command, env: plan.Environment(prepared.Environment, variables), exports: plan.Environment(nil, variables)}
}

// ready runs the gate on the equipped project and lays the session's settings,
// its trusted isolated configuration and the recording shims.
func (r Runner) ready(ctx context.Context, o Options, preparer Preparer, prepared *Prepared, s session, run config.Run) error {
	l := prepared.Layout
	gate := plan.Step{Argv: plan.GateArgv(l, l.Record(record.InitialGateFile)), Purpose: "run the gate on the equipped project"}
	outcome, err := preparer.Run(ctx, l, prepared.Environment, gate)
	prepared.Steps = append(prepared.Steps, outcome)
	if err != nil {
		return err
	}
	if err := WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps); err != nil {
		return err
	}
	trusted, err := launch.ClaudeTrustedProject(l.Project())
	if err != nil {
		return err
	}
	if err := r.Files.Write(l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile), trusted, false); err != nil {
		return err
	}
	settings, err := plan.SessionSettingsJSON(o.Config.AllowedTools)
	if err != nil {
		return err
	}
	if err := r.Files.Write(l.Path(plan.SessionSettings), settings, false); err != nil {
		return err
	}
	for _, name := range plan.Shimmed(run.Arm) {
		script := plan.ShimScript(o.Executable, l.Bin(name), l.Record(record.HooksFile), s.exports)
		if err := r.Files.Write(l.Path(plan.ShimDir, name), []byte(script), true); err != nil {
			return err
		}
	}
	return nil
}

func metaOf(o Options, run config.Run, s session, versions map[string]string) record.Meta {
	return record.Meta{
		Task: run.Task.Name, Arm: string(run.Arm), Repetition: run.Repetition, Model: o.Config.Model, Prompt: run.Task.Prompt,
		Argv: s.command.Argv, EventCap: o.Config.Caps.AssistantMessages, TimeoutSeconds: o.Config.Caps.Seconds,
		BudgetUSD: o.Config.Caps.BudgetUSD, Versions: versions,
	}
}

// leadingDirs are the directories that lead the PATH of the run: the shims, the
// product's binaries, the configured toolchain and the pinned CLI.
func leadingDirs(l plan.Layout, toolchain []string, claude string) []string {
	return append(append([]string{l.Path(plan.ShimDir), filepath.Join(l.Repo, plan.ToolsBinRelative)}, toolchain...), filepath.Dir(claude))
}

func (r Runner) checkMemory(ctx context.Context, o Options) error {
	output, err := r.Processes.Run(ctx, canary.CommandSpec{Argv: []string{filepath.Join(o.RepoRoot, plan.MemGuardRelative), "check"}, Dir: o.RepoRoot, Env: r.Env, Timeout: supportTimeout})
	if err != nil {
		return err
	}
	if output.ExitCode != 0 {
		return fmt.Errorf("%w: %s", ErrMemory, strings.TrimSpace(output.Stderr+output.Stdout))
	}
	return nil
}

// versions records what each binary of the run reports, and refuses a Claude
// Code binary that is not the pinned version.
func (r Runner) versions(ctx context.Context, o Options) (map[string]string, error) {
	found := map[string]string{}
	env := plan.Environment(r.Env, []string{plan.NoUpdater})
	ask := func(argv ...string) string {
		output, err := r.Processes.Run(ctx, canary.CommandSpec{Argv: argv, Dir: o.RepoRoot, Env: env, Timeout: supportTimeout})
		if err != nil {
			return "unavailable: " + err.Error()
		}
		return strings.TrimSpace(output.Stdout + output.Stderr)
	}
	found[plan.ClaudeBinary] = ask(o.Claude, versionArgument)
	if first, _, _ := strings.Cut(found[plan.ClaudeBinary], " "); first != o.ClaudePin {
		return found, fmt.Errorf("%w: %s reports %q, the pin is %s (run script/claude-code-install)", ErrPinMismatch, o.Claude, found[plan.ClaudeBinary], o.ClaudePin)
	}
	for _, module := range modules.All() {
		found[module.Binary] = ask(filepath.Join(o.RepoRoot, plan.ToolsBinRelative, module.Binary), moduleVersionVerb)
	}
	if pin, err := r.Files.Read(filepath.Join(o.RepoRoot, plan.ShallnotPinRelative)); err == nil {
		found[plan.ShallnotBinary] = strings.TrimSpace(string(pin))
	}
	return found, nil
}

// toolchain resolves the install directory of each configured tool.
func (r Runner) toolchain(ctx context.Context, o Options) ([]string, error) {
	if len(o.Config.Toolchain) == 0 {
		return nil, nil
	}
	argv := append([]string{filepath.Join(o.RepoRoot, resolveToolDirs)}, o.Config.Toolchain...)
	output, err := r.Processes.Run(ctx, canary.CommandSpec{Argv: argv, Dir: o.RepoRoot, Env: r.Env, Timeout: supportTimeout})
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrToolchain, err)
	}
	resolved := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(output.Stdout), "\n") {
		if tool, dir, found := strings.Cut(line, "\t"); found {
			resolved[tool] = dir
		}
	}
	var dirs []string
	for _, tool := range o.Config.Toolchain {
		dir, found := resolved[tool]
		if !found {
			return nil, fmt.Errorf("%w: %s", ErrToolchain, tool)
		}
		dirs = append(dirs, dir)
	}
	return dirs, nil
}

// encodeJSON indents a value as the records write it.
func encodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	return append(encoded, '\n'), err
}
