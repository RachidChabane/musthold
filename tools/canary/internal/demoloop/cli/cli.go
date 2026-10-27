// Package cli is the composition root of the agent-loop runner: it parses the
// command line and wires the use cases to the canary's adapters, and hands each
// verb of the loop-1 study to its own composition root. It holds no rule of its
// own.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/window"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/verbs"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitRefused = 2
	ExitUsage   = 64
)

// Subcommands of the demonstration.
const (
	commandLive   = "live"
	commandRender = "render"
	commandKeep   = "keep"
	commandShim   = "shim"
	binaryName    = "musthold-demo-loop"
	homeVariable  = "HOME"
)

// Env is everything the binary takes from its surroundings.
type Env struct {
	Args           []string
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Environ        []string
	Now            func() time.Time
	// Executable is the path of the running binary.
	Executable string
}

type systemClock struct{ now func() time.Time }

func (c systemClock) Now() time.Time { return c.now() }

// Main runs the binary in the process's own surroundings.
func Main() int {
	executable, err := os.Executable()
	if err == nil {
		executable, err = filepath.EvalSymlinks(executable)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, binaryName+":", err)
		return ExitRefused
	}
	return Run(Env{Args: os.Args[1:], Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Environ: os.Environ(), Now: time.Now, Executable: executable})
}

// Run runs a command line and returns the exit code.
func Run(env Env) int {
	if len(env.Args) == 0 {
		return env.usage()
	}
	command, rest := env.Args[0], env.Args[1:]
	if verb, found := verbs.Lookup(command); found {
		return env.loop1(verb, rest)
	}
	switch command {
	case commandLive:
		return env.live(rest)
	case commandRender:
		return env.render(rest)
	case commandKeep:
		return env.keep(rest)
	case commandShim:
		return env.shim(rest)
	case container.EntrySubcommand:
		return env.entry(rest)
	case container.ProxySubcommand:
		return env.egressProxy(rest)
	case container.ProbeSubcommand:
		return env.probeIsolation(rest)
	default:
		return env.usage()
	}
}

func (e Env) usage() int {
	commands := append([]string{commandLive, commandRender, commandKeep}, verbs.Names()...)
	fmt.Fprintf(e.Stderr, "usage: %s %s [flags]\n", binaryName, strings.Join(commands, "|"))
	return ExitUsage
}

// loop1 runs a verb of the loop-1 study, stopped by an interrupt or a
// termination signal.
func (e Env) loop1(verb verbs.Main, args []string) int {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return verb(ctx, args, e.Stdout, e.Stderr)
}

func (e Env) flags(name string) *flag.FlagSet {
	set := flag.NewFlagSet(binaryName+" "+name, flag.ContinueOnError)
	set.SetOutput(e.Stderr)
	return set
}

func (e Env) fail(err error) int {
	fmt.Fprintln(e.Stderr, binaryName+":", err)
	return ExitRefused
}

func (e Env) live(args []string) int {
	set := e.flags(commandLive)
	configPath := set.String("config", "", "the configuration file of the runs")
	repo := set.String("repo", ".", "the repository root")
	work := set.String("work", "", "the directory the run directories are created in (default: <repo>/"+app.WorkRelative+")")
	product := set.String("product", "", "the Linux product tree the containers mount read-only (default: <repo>/"+app.ProductRelative+")")
	docker := set.String("docker", "", "the docker command that names the demonstration's engine (default: <repo>/"+dockerRelative+")")
	dryRun := set.Bool("dry-run", false, "print what a run would do and start nothing")
	force := set.Bool("force-window", false, "run outside the nightly window")
	if err := set.Parse(args); err != nil || set.NArg() > 0 || *configPath == "" {
		return ExitUsage
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return e.fail(err)
	}
	data, err := os.ReadFile(*configPath)
	if err != nil {
		return e.fail(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		return e.fail(fmt.Errorf("%s: %w", *configPath, err))
	}
	pin, err := os.ReadFile(filepath.Join(root, plan.ClaudePinRelative))
	if err != nil {
		return e.fail(err)
	}
	options := app.Options{
		RepoRoot: root, Config: parsed, Work: orDefault(*work, root, app.WorkRelative), Product: orDefault(*product, root, app.ProductRelative),
		Docker: []string{orDefault(*docker, root, dockerRelative)}, UID: os.Getuid(), GID: os.Getgid(),
		ClaudePin: strings.TrimSpace(string(pin)), DryRun: *dryRun, ForceWindow: *force,
	}
	image, err := os.ReadFile(filepath.Join(options.Product, app.ImageFile))
	switch {
	case err == nil:
		options.Image = strings.TrimSpace(string(image))
	case options.DryRun:
		options.Image = "<image: run demo/loop/run build>"
	default:
		return e.fail(fmt.Errorf("%w: %v", app.ErrNoProduct, err))
	}
	if !options.DryRun {
		if err := adapters.Lower(); err != nil {
			return e.fail(fmt.Errorf("cannot lower the priority: %w", err))
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	dirs, err := e.runner().Run(ctx, options)
	for _, dir := range dirs {
		fmt.Fprintln(e.Stderr, "run directory:", dir)
	}
	if err != nil {
		return e.fail(err)
	}
	return ExitOK
}

// dockerRelative is the docker command that names the demonstration's engine.
const dockerRelative = "demo/loop/container/docker"

// orDefault is the value, made absolute, or the path below the repository.
func orDefault(value, root, relative string) string {
	if value == "" {
		return filepath.Join(root, relative)
	}
	if absolute, err := filepath.Abs(value); err == nil {
		return absolute
	}
	return value
}

func (e Env) runner() app.Runner {
	return app.Runner{
		Clock: systemClock{e.Now}, Window: window.Nightly(),
		Processes: adapters.Exec{}, Sessions: adapters.Sessions{}, Files: adapters.OSFiles{}, Locker: adapters.FileLocker{},
		Credentials: []canary.ClaudeCredential{adapters.EnvironmentCredential{Environ: e.Environ}, adapters.NewKeychainCredential()},
		Env:         launch.WithoutOAuthToken(e.Environ), Stdout: e.Stdout,
	}
}

func (e Env) render(args []string) int {
	set := e.flags(commandRender)
	records := set.String("records", "", "the records of a run, or a kept recording")
	out := set.String("out", "", "the directory the transcript is written to (default: the records)")
	check := set.Bool("check", false, "write nothing and fail when the transcript there differs from what the records render")
	if err := set.Parse(args); err != nil || set.NArg() > 0 || *records == "" {
		return ExitUsage
	}
	if *out == "" {
		*out = *records
	}
	err := app.Renderer{Files: adapters.OSFiles{}}.Write(*records, *out, *check)
	switch {
	case errors.Is(err, app.ErrStale):
		fmt.Fprintln(e.Stderr, binaryName+":", err)
		return ExitFailure
	case err != nil:
		return e.fail(err)
	}
	return ExitOK
}

func (e Env) keep(args []string) int {
	set := e.flags(commandKeep)
	run := set.String("run", "", "the run directory whose records are kept")
	to := set.String("to", "", "the recording directory to write")
	repo := set.String("repo", ".", "the repository root, written as a placeholder")
	if err := set.Parse(args); err != nil || set.NArg() > 0 || *run == "" || *to == "" {
		return ExitUsage
	}
	runDir, err := filepath.EvalSymlinks(*run)
	if err != nil {
		return e.fail(err)
	}
	root, err := filepath.Abs(*repo)
	if err != nil {
		return e.fail(err)
	}
	if err := (app.Keeper{Files: adapters.OSFiles{}}).Keep(runDir, root, variable(e.Environ, homeVariable), *to, e.knownToken()); err != nil {
		return e.fail(err)
	}
	return ExitOK
}

// knownToken is the token a credential source holds, to scan the records for its
// exact value, or empty when none holds one; the scan then looks for a token's shape.
func (e Env) knownToken() string {
	ctx := context.Background()
	source, err := canary.FirstCredential(ctx, e.runner().Credentials)
	if err != nil || source == nil {
		return ""
	}
	token, err := source.Token(ctx)
	if err != nil {
		return ""
	}
	return token
}

// shim is what the session's PATH runs in place of a binary: it records the hook
// calls of the real binary and hands every other command over to it.
func (e Env) shim(args []string) int {
	set := e.flags(commandShim)
	var real, record string
	set.StringVar(&real, "real", "", "the binary the shim stands for")
	set.StringVar(&record, "record", "", "the hook log to append to")
	if err := set.Parse(args); err != nil {
		return ExitUsage
	}
	return adapters.RunShim(real, record, set.Args(), e.Stdin, e.Stdout, e.Stderr)
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
