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
	// Profile holds what a study fixes beyond the demonstration's defaults.
	Profile Profile
}

// Profile holds the settings of a run that a study fixes beyond the
// demonstration's defaults. The zero value is the demonstration.
type Profile struct {
	// Claude is the Claude Code CLI inside the session image; empty, the product
	// tree's.
	Claude string
	// Egress are the endpoints the proxy forwards to, as host:port, or
	// container.AllowAll; empty, the model endpoints.
	Egress []string
	// Deny are the permission rules of the session that deny.
	Deny []string
	// OwnHome gives each run a HOME of its own, created empty under its directory.
	OwnHome bool
	// Memory and PidsLimit bound every container; empty and zero keep the
	// demonstration's bounds. ProxyImage is the proxy's image; empty, the session's.
	Memory     string
	PidsLimit  int
	ProxyImage string
	// CaptureStops records, after each end-of-turn hook call of Shallnot, the
	// gate's report and the project's tree.
	CaptureStops bool
	// InspectSession reads the exit code and the memory kill of the session's
	// container before it is removed.
	InspectSession bool
	// Probes is a shell script the session's container runs once the CLI has
	// exited, without the token in its environment, its output written to the
	// records' ProbesFile; the container's exit code stays the CLI's.
	Probes string
}

// ProbesFile is the record of what the session container's probes printed.
const ProbesFile = "probes.txt"

// probeWrapper runs the session's command, then the probes with the token and
// the proxy variables removed from their environment, and exits with the
// command's exit code. Its arguments are the probes' script, their output file
// and the command.
const probeWrapper = `script=$1; out=$2; shift 2; "$@"; status=$?; env -u ` + launch.OAuthTokenVariable + ` sh -c "$script" > "$out" 2>&1; exit $status`

// withProbes wraps a session's command line in the probe wrapper.
func withProbes(l plan.Layout, probes string, argv []string) []string {
	return append([]string{shellProgram, "-c", probeWrapper, shellProgram, probes, l.Record(ProbesFile)}, argv...)
}

const shellProgram = "sh"

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
	if err := Present(source, credentialErr); err != nil {
		return nil, err
	}
	lock, err := r.Lock(o)
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

// Present refuses to go on without a credential source that holds a token.
func Present(source canary.ClaudeCredential, err error) error {
	if err != nil {
		return err
	}
	if source == nil {
		return fmt.Errorf("%w: store a token (from `claude setup-token`) in the keychain as service %s, or set %s", ErrNoCredential, canary.KeychainService, launch.OAuthTokenVariable)
	}
	return nil
}

// Lock takes the lock that keeps two sessions from running at once.
func (r Runner) Lock(o Options) (canary.Lock, error) {
	return r.Locker.Acquire(filepath.Join(o.RepoRoot, canary.LockRelative))
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

// one prepares, conducts, collects and renders one run.
func (r Runner) one(ctx context.Context, o Options, configured config.Run, source canary.ClaudeCredential) (string, error) {
	if err := r.CheckMemory(ctx, o); err != nil {
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
	current, err := r.Open(o, dir)
	if err != nil {
		return dir, err
	}
	defer current.Close(ctx)
	l := current.Layout
	versions, err := r.Versions(ctx, current, o)
	if err != nil {
		return dir, err
	}
	preparer := r.Preparer(current)
	prepared, err := preparer.Prepare(ctx, l, configured, LeadingDirs(l))
	if err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	session, err := r.Ready(ctx, o, current, preparer, &prepared, configured)
	if err != nil {
		return dir, errors.Join(err, WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps))
	}
	meta := MetaOf(o, current, configured, session, versions)
	meta.ClaudeSHA256 = claudeDigest
	check := startup.New(plan.PluginNames(configured.Arm))
	conducted, err := r.Conduct(ctx, o, current, session, source, check.Line, &meta)
	if err != nil {
		return dir, err
	}
	outcome := conducted.Outcome
	meta.EndedAt, meta.ExitCode, meta.Killed = r.Clock.Now(), outcome.ExitCode, outcome.Killed
	collector := Collector{Processes: current.steps, Files: r.Files, Preparer: preparer}
	if err := collector.Collect(ctx, &prepared, meta); err != nil {
		return dir, err
	}
	if err := r.scanRecords(l, session.token); err != nil {
		return dir, err
	}
	if err := r.Render(l); err != nil {
		return dir, err
	}
	fmt.Fprintf(r.Stdout, "run %s: %s\ntranscript: %s\n", configured.Name(), dir, l.Record(record.TranscriptMarkdown))
	if startup.Failed(outcome.Killed) {
		return dir, fmt.Errorf("%w (%s): %s; the records are in %s", ErrSessionStart, outcome.Killed, check.Problem(), l.Records())
	}
	return dir, nil
}

// Attempt is one run directory and the containers that serve it.
type Attempt struct {
	Layout plan.Layout
	Box    container.Box
	steps  *containerSteps
}

// Open lays out a run directory and the containers of its run, refusing a
// directory a mount cannot carry.
func (r Runner) Open(o Options, dir string) (*Attempt, error) {
	attempt := r.layout(o, dir)
	return attempt, attempt.Box.Validate()
}

// LayoutOf lays out a run directory as the options' runs do.
func LayoutOf(o Options, dir string) plan.Layout {
	return plan.Layout{Repo: o.RepoRoot, Run: dir, Product: container.ProductTarget, ClaudePath: o.Profile.Claude, OwnHome: o.Profile.OwnHome}
}

func (r Runner) layout(o Options, dir string) *Attempt {
	l := LayoutOf(o, dir)
	box := container.Box{
		Image: o.Image, RunDir: dir, Product: o.Product, UID: o.UID, GID: o.GID, Run: filepath.Base(dir),
		Memory: o.Profile.Memory, PidsLimit: o.Profile.PidsLimit, Egress: o.Profile.Egress, ProxyImage: o.Profile.ProxyImage,
	}
	return &Attempt{Layout: l, Box: box, steps: &containerSteps{host: r.Processes, box: box, docker: o.Docker, env: r.Env}}
}

// Steps runs each command in a container of its own, with no network.
func (a *Attempt) Steps() canary.Processes { return a.steps }

// Close removes any step container a timeout left behind.
func (a *Attempt) Close(ctx context.Context) { a.steps.removeAll(ctx) }

// Preparer equips the attempt's project, every step in a container.
func (r Runner) Preparer(a *Attempt) Preparer {
	return Preparer{Processes: a.steps, Files: r.Files, Env: ContainerBase(a.Layout)}
}

// Conducted is how a session ended, and, when the profile asks, how its
// container ended.
type Conducted struct {
	Outcome canary.SessionOutcome
	State   *container.State
}

// Conduct starts the egress proxy on the run's internal network, runs the
// session with the token on its entry's standard input, ends it at the first line
// stops names a reason for, and removes the containers and the network, whatever
// the session did.
func (r Runner) Conduct(ctx context.Context, o Options, a *Attempt, s *Session, source canary.ClaudeCredential, stops func([]byte) string, meta *record.Meta) (Conducted, error) {
	defer r.teardown(ctx, o, a.Box)
	for _, argv := range [][]string{a.Box.NetworkArgv(), a.Box.ProxyArgv(a.Layout.Record(record.EgressFile)), a.Box.ConnectArgv()} {
		if err := r.docker(ctx, o, argv); err != nil {
			return Conducted{}, err
		}
	}
	token, err := source.Token(ctx)
	if err != nil {
		return Conducted{}, fmt.Errorf("%w: the %s source could not give the token: %v", ErrNoCredential, source.Source(), err)
	}
	s.token = token
	meta.StartedAt = r.Clock.Now()
	l := a.Layout
	outcome, err := r.Sessions.Run(canary.SessionSpec{
		Argv: s.argv, Dir: o.RepoRoot, Env: r.Env, Stdin: []byte(token + "\n"),
		StdoutPath: l.Record(record.StreamFile), StderrPath: l.Record(record.StderrFile), TimesPath: l.Record(record.TimesFile),
		Counts:  func(line []byte) bool { return launch.CountsAsEvent(harness.FlagClaude, line) },
		Stops:   stops,
		Cap:     o.Config.Caps.AssistantMessages,
		Timeout: time.Duration(o.Config.Caps.Seconds) * time.Second,
	})
	if err != nil {
		return Conducted{Outcome: outcome}, fmt.Errorf("the session could not run: %w", err)
	}
	conducted := Conducted{Outcome: outcome}
	if o.Profile.InspectSession {
		state, err := r.inspect(ctx, o, a.Box.SessionName())
		if err != nil {
			return conducted, err
		}
		conducted.State = &state
	}
	return conducted, nil
}

// inspect reads the exit code and the memory kill of a container, and nothing
// else the engine keeps for it.
func (r Runner) inspect(ctx context.Context, o Options, name string) (container.State, error) {
	output, err := r.dockerOutput(ctx, o, container.StateArgv(name))
	if err != nil {
		return container.State{}, err
	}
	return container.ParseState(output)
}

// ImageID is the id of the image the attempts run, as the engine names it.
func (r Runner) ImageID(ctx context.Context, o Options) (string, error) {
	output, err := r.dockerOutput(ctx, o, container.ImageIDArgv(o.Image))
	return strings.TrimSpace(output), err
}

// EngineVersion is the version of the engine the docker command names.
func (r Runner) EngineVersion(ctx context.Context, o Options) (string, error) {
	output, err := r.dockerOutput(ctx, o, container.EngineVersionArgv())
	return strings.TrimSpace(output), err
}

// teardown removes the session's and the proxy's containers and the network.
func (r Runner) teardown(ctx context.Context, o Options, box container.Box) {
	_ = r.docker(ctx, o, box.RemoveArgv())
	_ = r.docker(ctx, o, box.NetworkRemoveArgv())
}

// docker runs one docker command against the engine.
func (r Runner) docker(ctx context.Context, o Options, argv []string) error {
	_, err := r.dockerOutput(ctx, o, argv)
	return err
}

// dockerOutput runs one docker command against the engine and returns what it
// printed.
func (r Runner) dockerOutput(ctx context.Context, o Options, argv []string) (string, error) {
	command := append(append([]string{}, o.Docker...), argv...)
	output, err := r.Processes.Run(ctx, canary.CommandSpec{Argv: command, Dir: o.RepoRoot, Env: r.Env, Timeout: supportTimeout})
	if err != nil {
		return "", err
	}
	if output.ExitCode != 0 {
		return "", fmt.Errorf("docker %s exited %d: %s", strings.Join(argv[:min(2, len(argv))], " "), output.ExitCode, strings.TrimSpace(output.Stderr))
	}
	return output.Stdout, nil
}

// Session is what the session of a run needs once its project is prepared.
type Session struct {
	// command is the session's Claude Code command line inside its container.
	command launch.Command
	// argv is the docker command that runs it.
	argv []string
	// exports are the run's variables the shims set.
	exports []string
	// token is the token the entry received, kept to scan the records for it.
	token string
}

// Argv is the docker command that runs the session.
func (s *Session) Argv() []string { return append([]string(nil), s.argv...) }

// Token is the token the session's entry received, once the session has run.
func (s *Session) Token() string { return s.token }

// ContainerBase is the environment every container starts from, before the
// run's variables: the run's HOME, the image's directories, a locale and the
// project trusted by git.
func ContainerBase(l plan.Layout) []string {
	home := container.ScratchHome
	if l.OwnHome {
		home = l.Home()
	}
	base := []string{"HOME=" + home, "PATH=" + container.SystemPath, "LANG=C.UTF-8"}
	return append(base, container.TrustedRepository(l.Project())...)
}

// SessionOf builds the session of a run: its command line, the docker command
// that runs it and the variables the shims export.
func SessionOf(o Options, a *Attempt, configured config.Run) *Session {
	l := a.Layout
	command := plan.SessionCommand(plan.SessionInput{Layout: l, Run: configured, Config: o.Config, Claude: l.Claude()})
	variables := append(append(plan.Variables(l, LeadingDirs(l)), command.Env...), container.ProxyVariables()...)
	variables = append(variables, NonessentialTraffic)
	resolved := plan.Environment(ContainerBase(l), variables)
	inside := command.Argv
	if o.Profile.Probes != "" {
		inside = withProbes(l, o.Profile.Probes, inside)
	}
	argv := append(append([]string{}, o.Docker...), a.Box.SessionArgv(command.Dir, resolved, inside)...)
	return &Session{command: command, argv: argv, exports: plan.ShimExports(l, variables)}
}

// Ready runs the gate on the equipped project and lays the session's settings,
// its trusted isolated configuration and the recording shims, and returns the
// session.
func (r Runner) Ready(ctx context.Context, o Options, a *Attempt, preparer Preparer, prepared *Prepared, configured config.Run) (*Session, error) {
	s := SessionOf(o, a, configured)
	l := prepared.Layout
	gate := plan.Step{Argv: plan.GateArgv(l, l.Record(record.InitialGateFile)), Purpose: "run the gate on the equipped project"}
	outcome, err := preparer.Run(ctx, l, prepared.Environment, gate)
	prepared.Steps = append(prepared.Steps, outcome)
	if err != nil {
		return s, err
	}
	if err := WriteSteps(r.Files, l.Record(record.StepsFile), prepared.Steps); err != nil {
		return s, err
	}
	trusted, err := launch.ClaudeTrustedProject(l.Project())
	if err != nil {
		return s, err
	}
	if err := r.Files.Write(l.Path(plan.ClaudeConfigDir, launch.ClaudeGlobalConfigFile), trusted, false); err != nil {
		return s, err
	}
	settings, err := plan.SessionSettingsJSON(o.Config.AllowedTools, o.Profile.Deny)
	if err != nil {
		return s, err
	}
	if err := r.Files.Write(l.Path(plan.SessionSettings), settings, false); err != nil {
		return s, err
	}
	for _, name := range plan.Shimmed(configured.Arm) {
		if err := r.Files.Write(l.Path(plan.ShimDir, name), []byte(shimScript(o, l, name, s.exports)), true); err != nil {
			return s, err
		}
	}
	return s, nil
}

// shimScript is the shim of one binary: Shallnot's records each end-of-turn call
// when the profile asks.
func shimScript(o Options, l plan.Layout, name string, exports []string) string {
	if o.Profile.CaptureStops && name == plan.ShallnotBinary {
		return plan.StopShimScript(container.Runner(), l.Bin(name), l.Record(record.HooksFile), exports, l)
	}
	return plan.ShimScript(container.Runner(), l.Bin(name), l.Record(record.HooksFile), exports)
}

// MetaOf describes a run before its session starts.
func MetaOf(o Options, a *Attempt, configured config.Run, s *Session, versions map[string]string) record.Meta {
	return record.Meta{
		Task: configured.Task.Name, Arm: string(configured.Arm), Repetition: configured.Repetition, Model: o.Config.Model, Prompt: configured.Task.Prompt,
		Argv: s.argv, EventCap: o.Config.Caps.AssistantMessages, TimeoutSeconds: o.Config.Caps.Seconds,
		BudgetUSD: o.Config.Caps.BudgetUSD, Versions: versions, Image: o.Image, Egress: a.Box.Endpoints(),
	}
}

// LeadingDirs are the directories that lead the PATH of a container: the shims,
// the product's binaries and the pinned CLI.
func LeadingDirs(l plan.Layout) []string {
	return []string{l.Path(plan.ShimDir), l.BinDir(), filepath.Dir(l.Claude())}
}

// Environment is the environment of a command run in the attempt's project:
// the containers' base and the run's variables.
func (a *Attempt) Environment() []string {
	return plan.Environment(ContainerBase(a.Layout), plan.Variables(a.Layout, LeadingDirs(a.Layout)))
}

// CheckMemory refuses a run the memory guard refuses.
func (r Runner) CheckMemory(ctx context.Context, o Options) error {
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

// Versions records what each binary of the run and each command of the
// configuration's toolchain reports inside a container, and refuses a Claude
// Code binary whose version does not begin with the pin.
func (r Runner) Versions(ctx context.Context, a *Attempt, o Options) (map[string]string, error) {
	l := a.Layout
	found := map[string]string{}
	env := plan.Environment(ContainerBase(l), []string{plan.NoUpdater})
	ask := func(argv ...string) string {
		output, err := a.steps.Run(ctx, canary.CommandSpec{Argv: argv, Dir: l.Run, Env: env, Timeout: supportTimeout})
		if err != nil {
			return "unavailable: " + err.Error()
		}
		return strings.TrimSpace(output.Stdout + output.Stderr)
	}
	found[plan.ClaudeBinary] = ask(l.Claude(), versionArgument)
	if first, _, _ := strings.Cut(found[plan.ClaudeBinary], " "); first != o.ClaudePin {
		return found, fmt.Errorf("%w: %s reports %q, the pin is %s", ErrPinMismatch, l.Claude(), found[plan.ClaudeBinary], o.ClaudePin)
	}
	for _, module := range modules.All() {
		found[module.Binary] = ask(l.Bin(module.Binary), moduleVersionVerb)
	}
	for _, command := range o.Config.Toolchain {
		found[command] = ask(command, versionArgument)
	}
	if pin, err := r.Files.Read(filepath.Join(o.RepoRoot, plan.ShallnotPinRelative)); err == nil {
		found[plan.ShallnotBinary] = strings.TrimSpace(string(pin))
	}
	return found, nil
}

// Render writes the transcript of the attempt's records beside them.
func (r Runner) Render(l plan.Layout) error {
	return Renderer{Files: r.Files}.Write(l.Records(), l.Records(), false)
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

// EncodeJSON indents a value as the records write it.
func EncodeJSON(value any) ([]byte, error) {
	encoded, err := json.MarshalIndent(value, "", "  ")
	return append(encoded, '\n'), err
}
