package runs

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/deviation"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Exit codes of the verbs.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Defaults of the verbs' flags, relative to the repository root or the home
// directory, and the variables that override the work and product directories.
const (
	DefaultConfig = "experiments/loop-1/loop-1.json"
	DefaultInputs = "experiments/loop-1/freeze/" + study.InputsFile
	DefaultDocker = "experiments/loop-1/script/docker"
	DefaultClaude = "/usr/local/bin/claude"
	homeVariable  = "HOME"
)

// The bounds of every container of the study (PREREG §7.3), and the slots each
// `docker run` holds (PREREG §7.7): the script, relative to the repository root,
// and how long a run waits for them.
const (
	StudyMemory      = "3g"
	StudyPidsLimit   = 1024
	WithSlotRelative = "script/with-slot"
	SlotWait         = time.Hour
)

// Flags are what the run and canary verbs read from their command line.
type Flags struct {
	Repo, Config, Work, Inputs, Product, Image, Claude, Docker string
	DryRun                                                     bool
}

// ParseFlags reads the verbs' flags; environ gives the defaults of the work and
// product directories.
func ParseFlags(name string, args []string, environ []string, stderr io.Writer) (Flags, error) {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(stderr)
	home := variable(environ, homeVariable)
	var f Flags
	set.StringVar(&f.Repo, "repo", ".", "the repository root")
	set.StringVar(&f.Config, "config", DefaultConfig, "the study's configuration, relative to the repository root")
	set.StringVar(&f.Work, "work", orElse(variable(environ, study.WorkEnv), filepath.Join(home, study.DefaultWorkName)), "the work directory ($"+study.WorkEnv+")")
	set.StringVar(&f.Inputs, "inputs", DefaultInputs, "the inputs freeze, relative to the repository root")
	set.StringVar(&f.Product, "product", orElse(variable(environ, study.ProductEnv), filepath.Join(home, study.DefaultProductName)), "the Linux product the containers mount read-only ($"+study.ProductEnv+")")
	set.StringVar(&f.Image, "image", "", "the study's image (default: images.built of the inputs freeze)")
	set.StringVar(&f.Claude, "claude", DefaultClaude, "the Claude Code CLI inside the image")
	set.StringVar(&f.Docker, "docker", DefaultDocker, "the docker command that names the study's engine, relative to the repository root")
	set.BoolVar(&f.DryRun, "dry-run", false, "print every command and start nothing")
	if err := set.Parse(args); err != nil {
		return Flags{}, err
	}
	if set.NArg() > 0 {
		return Flags{}, fmt.Errorf("unexpected arguments: %v", set.Args())
	}
	return f, nil
}

func variable(environ []string, name string) string {
	value := ""
	for _, pair := range environ {
		if found, ok := strings.CutPrefix(pair, name+"="); ok {
			value = found
		}
	}
	return value
}

func orElse(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

// Setup is what the flags resolve to: the configuration, the inputs freeze and
// the agent-loop runner's options with the study's profile.
type Setup struct {
	Config  config.Config
	Inputs  study.Inputs
	Options app.Options
	Layout  study.Layout
}

// ErrNoInputs refuses a verb whose inputs freeze is absent.
var ErrNoInputs = errors.New("the inputs freeze is absent")

// Resolve reads the configuration, the inputs freeze and the pin, and lays out the
// options. An absent inputs freeze is refused unless absentOK, which gives an
// empty freeze.
func (f Flags) Resolve(files canary.Files, absentOK bool) (Setup, error) {
	root, err := filepath.Abs(f.Repo)
	if err != nil {
		return Setup{}, err
	}
	at := func(path string) string {
		if filepath.IsAbs(path) {
			return path
		}
		return filepath.Join(root, path)
	}
	data, err := files.Read(at(f.Config))
	if err != nil || data == nil {
		return Setup{}, fmt.Errorf("%s: cannot be read: %v", at(f.Config), err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		return Setup{}, fmt.Errorf("%s: %w", at(f.Config), err)
	}
	inputs, err := readInputs(files, at(f.Inputs), absentOK)
	if err != nil {
		return Setup{}, err
	}
	pin, err := files.Read(filepath.Join(root, plan.ClaudePinRelative))
	if err != nil || pin == nil {
		return Setup{}, fmt.Errorf("%s: cannot be read: %v", plan.ClaudePinRelative, err)
	}
	work, product := resolved(f.Work), resolved(f.Product)
	options := app.Options{
		RepoRoot: root, Config: parsed, Work: work, Product: product, Image: orElse(f.Image, inputs.Images.Built),
		Docker: []string{at(f.Docker)}, UID: os.Getuid(), GID: os.Getgid(), ClaudePin: strings.TrimSpace(string(pin)), DryRun: f.DryRun,
		Profile: StudyProfile(f.Claude),
	}
	options.Profile.ProxyImage = inputs.Images.Proxy
	options.Profile.RunWrapper = docker.SlotWrapper(filepath.Join(root, WithSlotRelative), SlotWait)
	return Setup{Config: parsed, Inputs: inputs, Options: options, Layout: study.Layout{Root: work}}, nil
}

// resolved is a directory as the engine mounts it: absolute, with its symbolic
// links resolved when it exists.
func resolved(dir string) string {
	absolute, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	if real, err := filepath.EvalSymlinks(absolute); err == nil {
		return real
	}
	return absolute
}

// StudyProfile is what the study fixes beyond the demonstration's defaults
// (PREREG §5, §7): the image's CLI, the web tools denied, a HOME of the run's own,
// the containers' bounds, the stops recorded and the session container's state
// read.
func StudyProfile(claude string) app.Profile {
	return app.Profile{
		Claude: claude, Deny: plan.WebTools(), OwnHome: true, Memory: StudyMemory, PidsLimit: StudyPidsLimit,
		CaptureStops: true, InspectSession: true,
	}
}

func readInputs(files canary.Files, path string, absentOK bool) (study.Inputs, error) {
	data, err := files.Read(path)
	if err != nil {
		return study.Inputs{}, err
	}
	if data == nil {
		if absentOK {
			return study.Inputs{}, nil
		}
		return study.Inputs{}, fmt.Errorf("%w: %s", ErrNoInputs, path)
	}
	return study.ParseInputs(data)
}

// systemClock is the host's clock.
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// AppRunner is the agent-loop runner wired to the canary's adapters.
func AppRunner(environ []string, stdout io.Writer) app.Runner {
	return app.Runner{
		Clock: systemClock{}, Window: window.Nightly(),
		Processes: adapters.Exec{}, Sessions: adapters.Sessions{}, Files: adapters.OSFiles{}, Locker: adapters.FileLocker{},
		Credentials: []canary.ClaudeCredential{adapters.EnvironmentCredential{Environ: environ}, adapters.NewKeychainCredential()},
		Env:         launch.WithoutOAuthToken(environ), Stdout: stdout,
	}
}

// Command is the run verb wired to its adapters.
type Command struct {
	// Deviations writes the deviation log of a work directory; nil, it is the
	// log-deviation verb's appender with its RFC 3161 stamper.
	Deviations func(study.Layout) DeviationWriter
	// Environ is the process's environment; nil reads it.
	Environ []string
}

// RunMain is the run verb, its deviations written by the log-deviation verb's
// appender.
func RunMain(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return Command{}.Main(ctx, args, stdout, stderr)
}

// Main runs the run verb and returns its exit code.
func (c Command) Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	environ := c.environ()
	flags, err := ParseFlags("run", args, environ, stderr)
	if err != nil {
		return ExitUsage
	}
	setup, err := flags.Resolve(adapters.OSFiles{}, flags.DryRun)
	if err != nil {
		return fail(stderr, err)
	}
	if setup.Inputs.Order == nil {
		setup.Inputs.Order = Order(setup.Config)
	}
	batch := Batch{Runner: c.runner(environ, stdout, setup), Stdout: stdout}
	err = batch.Run(ctx, Invocation{Plan: Plan{Options: setup.Options, Inputs: setup.Inputs}, Config: setup.Config, Layout: setup.Layout, DryRun: flags.DryRun})
	if err != nil {
		return fail(stderr, err)
	}
	return ExitOK
}

func (c Command) environ() []string {
	if c.Environ != nil {
		return c.Environ
	}
	return os.Environ()
}

func (c Command) runner(environ []string, stdout io.Writer, setup Setup) Runner {
	writer := c.Deviations
	if writer == nil {
		writer = func(layout study.Layout) DeviationWriter {
			return deviation.NewAppender(layout, deviation.DefaultStamper(), time.Now)
		}
	}
	runner := StudyRunner(environ, stdout, setup.Options)
	runner.Deviations = writer(setup.Layout)
	return runner
}

// StudyRunner wires the attempts to the canary's adapters, the study's engine and
// the host, as the resolve verb reads them.
func StudyRunner(environ []string, stdout io.Writer, options app.Options) Runner {
	command := freeze.OSExec{}
	return Runner{
		App: AppRunner(environ, stdout), Tree: OSTree{},
		Engine: freeze.CLIEngine{Exec: command, Docker: options.Docker}, Host: freeze.UnameHost{Exec: command},
	}
}

func fail(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "loop-1:", err)
	return ExitFailure
}
