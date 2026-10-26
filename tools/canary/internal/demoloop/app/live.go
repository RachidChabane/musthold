package app

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/startup"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
)

// Refusals of a live run, reported before the session they concern starts.
var (
	ErrOutsideWindow = errors.New("outside the nightly window")
	ErrNoCredential  = errors.New("no Claude Code credential")
	ErrPinMismatch   = errors.New("the Claude Code binary is not the pinned version")
	ErrMemory        = errors.New("the memory guard refused the run")
	ErrNoProduct     = errors.New("the Linux product is not built")
	ErrTokenInRecord = errors.New("a record holds the token")
	// ErrSessionStart is a session the runner ended because its start failed:
	// a plugin did not load, or the model API was out of reach.
	ErrSessionStart = errors.New("the session did not start")
)

// Names and bounds of a live run.
const (
	runDirPrefix      = "musthold-demo-loop-"
	supportTimeout    = 2 * time.Minute
	versionArgument   = "--version"
	moduleVersionVerb = "version"
	placeholderRun    = "<run-dir>"
	// NonessentialTraffic asks Claude Code to make no call but the model's.
	NonessentialTraffic = "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC=1"
	// WorkRelative is the default work area, inside the repository's ignored tools directory.
	WorkRelative = ".tools/demo-loop/work"
	// ProductRelative is the Linux product tree the containers mount read-only.
	ProductRelative = ".tools/demo-loop/product"
	// ImageFile, in the product tree, names the session image by its id.
	ImageFile = "IMAGE"
)

// Runner runs the configured sessions of the agent loop, each in its own
// containers.
type Runner struct {
	Clock     canary.Clock
	Window    window.Window
	Processes canary.Processes
	Sessions  canary.Sessions
	Files     canary.Files
	Locker    canary.Locker
	// Credentials are the sources of the Claude Code token, in order of preference.
	Credentials []canary.ClaudeCredential
	// Env is the environment of the runner and of its docker client. The token is
	// dropped from it: only the session's entry receives it, on its standard input.
	Env    []string
	Stdout io.Writer
}

// Options select what a live run does.
type Options struct {
	RepoRoot string
	Config   config.Config
	// Work is the directory the run directories are created in, mounted into the
	// engine read-write.
	Work string
	// Product is the host's Linux product tree, mounted read-only, and Image the
	// session image, by id.
	Product, Image string
	// Docker is the docker command, as a prefix, that names the demonstration's engine.
	Docker []string
	// UID and GID are the user every container runs as.
	UID, GID int
	// ClaudePin is the version the product's Claude Code must report.
	ClaudePin string
	// DryRun prints the commands and starts nothing.
	DryRun bool
	// ForceWindow allows a session outside the nightly window.
	ForceWindow bool
}

// Run runs every session the configuration asks for, one at a time, and returns
// their run directories. With DryRun it prints what it would do and reads no
// credential value.
func (r Runner) Run(ctx context.Context, o Options) ([]string, error) {
	r.Env = launch.WithoutOAuthToken(r.Env)
	now := r.Clock.Now()
	source, credentialErr := canary.FirstCredential(ctx, r.Credentials)
	if o.DryRun {
		return nil, r.printPlan(o, now, source, credentialErr)
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
		if err := r.windowAllows(o); err != nil {
			return dirs, err
		}
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

// windowAllows refuses a session whose time cap would end after the window closes.
func (r Runner) windowAllows(o Options) error {
	now := r.Clock.Now()
	length := time.Duration(o.Config.Caps.Seconds) * time.Second
	if o.ForceWindow || r.Window.Holds(now, length) {
		return nil
	}
	return fmt.Errorf("%w: a session that starts at %s may last %s and the window is %s; pass --force-window to run anyway",
		ErrOutsideWindow, now.Format("15:04"), length, r.Window.Describe())
}

// run is one run directory and the containers that serve it.
type run struct {
	layout plan.Layout
	box    container.Box
	steps  *containerSteps
}

// newRun lays out a run directory and the containers of its run.
func (r Runner) newRun(o Options, dir string) run {
	l := plan.Layout{Repo: o.RepoRoot, Run: dir, Product: container.ProductTarget}
	box := container.Box{Image: o.Image, RunDir: dir, Product: o.Product, UID: o.UID, GID: o.GID, Run: filepath.Base(dir)}
	return run{layout: l, box: box, steps: &containerSteps{host: r.Processes, box: box, docker: o.Docker, env: r.Env}}
}

// one prepares, conducts, collects and renders one run.
func (r Runner) one(ctx context.Context, o Options, configured config.Run, source canary.ClaudeCredential) (string, error) {
	if err := r.checkMemory(ctx, o); err != nil {
		return "", err
	}
	claudeDigest, err := r.claudeDigest(o)
	if err != nil {
		return "", err
	}
	dir, err := r.Files.MakeDir(o.Work, runDirPrefix+configured.Name()+"-")
	if err != nil {
		return "", err
	}
	current := r.newRun(o, dir)
	if err := current.box.Validate(); err != nil {
		return dir, err
	}
	defer current.steps.removeAll(ctx)
	l := current.layout
	versions, err := r.versions(ctx, current.steps, l, o)
	if err != nil {
		return dir, err
	}
	preparer := Preparer{Processes: current.steps, Files: r.Files, Env: containerBase(current.layout)}
	prepared, err := preparer.Prepare(ctx, l, configured, leadingDirs(l))
	if err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	session := sessionOf(o, l, current.box, configured)
	if err := r.ready(ctx, o, preparer, &prepared, session, configured); err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	meta := metaOf(o, configured, session, versions)
	meta.ClaudeSHA256, meta.Image, meta.Egress = claudeDigest, o.Image, container.ModelEndpoints()
	check := startup.New(plan.PluginNames(configured.Arm))
	outcome, err := r.conduct(ctx, o, current, session, source, check, &meta)
	if err != nil {
		return dir, err
	}
	meta.EndedAt, meta.ExitCode, meta.Killed = r.Clock.Now(), outcome.ExitCode, outcome.Killed
	collector := Collector{Processes: current.steps, Files: r.Files, Preparer: preparer}
	if err := collector.Collect(ctx, &prepared, meta); err != nil {
		return dir, err
	}
	if err := r.scanRecords(l, session.token); err != nil {
		return dir, err
	}
	if err := (Renderer{Files: r.Files}).Write(l.Records(), l.Records(), false); err != nil {
		return dir, err
	}
	fmt.Fprintf(r.Stdout, "run %s: %s\ntranscript: %s\n", configured.Name(), dir, l.Record(record.TranscriptMarkdown))
	if startup.Failed(outcome.Killed) {
		return dir, fmt.Errorf("%w (%s): %s; the records are in %s", ErrSessionStart, outcome.Killed, check.Problem(), l.Records())
	}
	return dir, nil
}

// conduct starts the egress proxy on the run's internal network, runs the
// session with the token on its entry's standard input, ends it at once when the
// check finds its start failed, and removes the containers and the network,
// whatever the session did.
func (r Runner) conduct(ctx context.Context, o Options, current run, s *session, source canary.ClaudeCredential, check *startup.Check, meta *record.Meta) (canary.SessionOutcome, error) {
	defer r.teardown(ctx, o, current.box)
	for _, argv := range [][]string{current.box.NetworkArgv(), current.box.ProxyArgv(current.layout.Record(record.EgressFile)), current.box.ConnectArgv()} {
		if err := r.docker(ctx, o, argv); err != nil {
			return canary.SessionOutcome{}, err
		}
	}
	token, err := source.Token(ctx)
	if err != nil {
		return canary.SessionOutcome{}, fmt.Errorf("%w: the %s source could not give the token: %v", ErrNoCredential, source.Source(), err)
	}
	s.token = token
	meta.StartedAt = r.Clock.Now()
	l := current.layout
	outcome, err := r.Sessions.Run(canary.SessionSpec{
		Argv: s.argv, Dir: o.RepoRoot, Env: r.Env, Stdin: []byte(token + "\n"),
		StdoutPath: l.Record(record.StreamFile), StderrPath: l.Record(record.StderrFile), TimesPath: l.Record(record.TimesFile),
		Counts:  func(line []byte) bool { return launch.CountsAsEvent(harness.FlagClaude, line) },
		Stops:   check.Line,
		Cap:     o.Config.Caps.AssistantMessages,
		Timeout: time.Duration(o.Config.Caps.Seconds) * time.Second,
	})
	if err != nil {
		return outcome, fmt.Errorf("the session could not run: %w", err)
	}
	return outcome, nil
}

// teardown removes the session's and the proxy's containers and the network.
func (r Runner) teardown(ctx context.Context, o Options, box container.Box) {
	_ = r.docker(ctx, o, box.RemoveArgv())
	_ = r.docker(ctx, o, box.NetworkRemoveArgv())
}

// docker runs one docker command against the engine.
func (r Runner) docker(ctx context.Context, o Options, argv []string) error {
	command := append(append([]string{}, o.Docker...), argv...)
	output, err := r.Processes.Run(ctx, canary.CommandSpec{Argv: command, Dir: o.RepoRoot, Env: r.Env, Timeout: supportTimeout})
	if err != nil {
		return err
	}
	if output.ExitCode != 0 {
		return fmt.Errorf("docker %s exited %d: %s", strings.Join(argv[:min(2, len(argv))], " "), output.ExitCode, strings.TrimSpace(output.Stderr))
	}
	return nil
}

// session is what the session of a run needs once its project is prepared.
type session struct {
	// command is the session's Claude Code command line inside its container.
	command launch.Command
	// argv is the docker command that runs it.
	argv []string
	// exports are the run's variables the shims set.
	exports []string
	// token is the token the entry received, kept to scan the records for it.
	token string
}

// containerBase is the environment every container starts from, before the
// run's variables: a scratch HOME, the image's directories, a locale and the
// project trusted by git.
func containerBase(l plan.Layout) []string {
	base := []string{"HOME=" + container.ScratchHome, "PATH=" + container.SystemPath, "LANG=C.UTF-8"}
	return append(base, container.TrustedRepository(l.Project())...)
}

func sessionOf(o Options, l plan.Layout, box container.Box, configured config.Run) *session {
	command := plan.SessionCommand(plan.SessionInput{Layout: l, Run: configured, Config: o.Config, Claude: l.Claude()})
	variables := append(append(plan.Variables(l, leadingDirs(l)), command.Env...), container.ProxyVariables()...)
	variables = append(variables, NonessentialTraffic)
	resolved := plan.Environment(containerBase(l), variables)
	argv := append(append([]string{}, o.Docker...), box.SessionArgv(command.Dir, resolved, command.Argv)...)
	return &session{command: command, argv: argv, exports: plan.ShimExports(l, variables)}
}

// ready runs the gate on the equipped project and lays the session's settings,
// its trusted isolated configuration and the recording shims.
func (r Runner) ready(ctx context.Context, o Options, preparer Preparer, prepared *Prepared, s *session, configured config.Run) error {
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
	for _, name := range plan.Shimmed(configured.Arm) {
		script := plan.ShimScript(container.Runner(), l.Bin(name), l.Record(record.HooksFile), s.exports)
		if err := r.Files.Write(l.Path(plan.ShimDir, name), []byte(script), true); err != nil {
			return err
		}
	}
	return nil
}

func metaOf(o Options, configured config.Run, s *session, versions map[string]string) record.Meta {
	return record.Meta{
		Task: configured.Task.Name, Arm: string(configured.Arm), Repetition: configured.Repetition, Model: o.Config.Model, Prompt: configured.Task.Prompt,
		Argv: s.argv, EventCap: o.Config.Caps.AssistantMessages, TimeoutSeconds: o.Config.Caps.Seconds,
		BudgetUSD: o.Config.Caps.BudgetUSD, Versions: versions,
	}
}

// leadingDirs are the directories that lead the PATH of a container: the shims,
// the product's binaries and the pinned CLI.
func leadingDirs(l plan.Layout) []string {
	return []string{l.Path(plan.ShimDir), l.BinDir(), filepath.Dir(l.Claude())}
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

// claudeDigest is the SHA-256 of the product's Claude Code binary.
func (r Runner) claudeDigest(o Options) (string, error) {
	content, err := r.Files.Read(filepath.Join(o.Product, plan.ProductClaudeDir, plan.ClaudeBinary))
	if err != nil {
		return "", err
	}
	if content == nil {
		return "", fmt.Errorf("%w: %s holds no Claude Code binary (run demo/loop/run build)", ErrNoProduct, o.Product)
	}
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:]), nil
}

// versions records what each binary of the run reports inside a container, and
// refuses a Claude Code binary that is not the pinned version.
func (r Runner) versions(ctx context.Context, steps *containerSteps, l plan.Layout, o Options) (map[string]string, error) {
	found := map[string]string{}
	env := plan.Environment(containerBase(l), []string{plan.NoUpdater})
	ask := func(argv ...string) string {
		output, err := steps.Run(ctx, canary.CommandSpec{Argv: argv, Dir: l.Run, Env: env, Timeout: supportTimeout})
		if err != nil {
			return "unavailable: " + err.Error()
		}
		return strings.TrimSpace(output.Stdout + output.Stderr)
	}
	found[plan.ClaudeBinary] = ask(l.Claude(), versionArgument)
	if first, _, _ := strings.Cut(found[plan.ClaudeBinary], " "); first != o.ClaudePin {
		return found, fmt.Errorf("%w: %s reports %q, the pin is %s (run demo/loop/run build)", ErrPinMismatch, l.Claude(), found[plan.ClaudeBinary], o.ClaudePin)
	}
	for _, module := range modules.All() {
		found[module.Binary] = ask(l.Bin(module.Binary), moduleVersionVerb)
	}
	if pin, err := r.Files.Read(filepath.Join(o.RepoRoot, plan.ShallnotPinRelative)); err == nil {
		found[plan.ShallnotBinary] = strings.TrimSpace(string(pin))
	}
	return found, nil
}

// scanRecords refuses a run whose records hold the token, naming the files.
func (r Runner) scanRecords(l plan.Layout, token string) error {
	files, err := recordFiles(r.Files, l.Records())
	if err != nil {
		return err
	}
	if hits := record.TokenHits(files, token); len(hits) > 0 {
		return fmt.Errorf("%w: %s", ErrTokenInRecord, strings.Join(hits, ", "))
	}
	return nil
}

// recordFiles reads every file of a records directory, by name.
func recordFiles(files canary.Files, dir string) (map[string][]byte, error) {
	paths, err := files.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return nil, err
	}
	found := map[string][]byte{}
	for _, path := range paths {
		content, err := files.Read(path)
		if err != nil {
			return nil, err
		}
		if content != nil {
			found[filepath.Base(path)] = content
		}
	}
	return found, nil
}

// containerSteps runs each preparation and collection command in a container of
// its own, with no network.
type containerSteps struct {
	host   canary.Processes
	box    container.Box
	docker []string
	env    []string
	count  int
	names  []string
}

// Run implements canary.Processes.
func (c *containerSteps) Run(ctx context.Context, spec canary.CommandSpec) (canary.Output, error) {
	c.count++
	argv := append(append([]string{}, c.docker...), c.box.StepArgv(c.count, spec.Dir, spec.Env, spec.Argv)...)
	c.names = append(c.names, c.box.StepName(c.count))
	return c.host.Run(ctx, canary.CommandSpec{Argv: argv, Dir: "", Env: c.env, Timeout: spec.Timeout})
}

// removeAll removes any step container a timeout left behind.
func (c *containerSteps) removeAll(ctx context.Context) {
	if len(c.names) == 0 {
		return
	}
	argv := append(append(append([]string{}, c.docker...), "rm", "--force"), c.names...)
	_, _ = c.host.Run(ctx, canary.CommandSpec{Argv: argv, Env: c.env, Timeout: supportTimeout})
}

// encodeJSON indents a value as the records write it.
func encodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	return append(encoded, '\n'), err
}
