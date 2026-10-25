// Package cli is the composition root of the agent-loop runner: it parses the
// command line and wires the use cases to the canary's adapters. It holds no rule
// of its own.
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

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

// Exit codes.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitRefused = 2
	ExitUsage   = 64
)

// Subcommands.
const (
	commandRun    = "run"
	commandRender = "render"
	commandKeep   = "keep"
	commandShim   = "shim"
	binaryName    = "spec-proof-demo-loop"
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
	switch command {
	case commandRun:
		return env.run(rest)
	case commandRender:
		return env.render(rest)
	case commandKeep:
		return env.keep(rest)
	case commandShim:
		return env.shim(rest)
	default:
		return env.usage()
	}
}

func (e Env) usage() int {
	fmt.Fprintf(e.Stderr, "usage: %s %s|%s|%s [flags]\n", binaryName, commandRun, commandRender, commandKeep)
	return ExitUsage
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

func (e Env) run(args []string) int {
	set := e.flags(commandRun)
	configPath := set.String("config", "", "the configuration file of the runs")
	repo := set.String("repo", ".", "the repository root")
	out := set.String("out", os.TempDir(), "the directory the run directories are created in")
	claude := set.String("claude", "", "the pinned Claude Code binary (default: the one script/claude-code-install installs)")
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
		RepoRoot: root, Config: parsed, OutDir: *out, Executable: e.Executable,
		Claude: *claude, ClaudePin: strings.TrimSpace(string(pin)), DryRun: *dryRun, ForceWindow: *force,
	}
	if options.Claude == "" {
		options.Claude = filepath.Join(root, plan.ClaudeToolsRelative, options.ClaudePin, "bin", plan.ClaudeBinary)
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
	if err := (app.Keeper{Files: adapters.OSFiles{}}).Keep(runDir, root, variable(e.Environ, homeVariable), *to); err != nil {
		return e.fail(err)
	}
	return ExitOK
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
