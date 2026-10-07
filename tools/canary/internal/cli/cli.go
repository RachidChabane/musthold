// Package cli is the composition root of the canary binary: it parses the command
// line and wires the use cases to their adapters. It holds no rule of its own.
package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

// Exit codes beyond those of a report (verdict.Exit*).
const (
	ExitRefused = 2
	ExitUsage   = 64
)

// Subcommands.
const (
	commandRun      = "run"
	commandEvaluate = "evaluate"
	commandShim     = "shim"
	commandProbe    = "probe"
	commandModules  = "modules"
)

// Env is everything the binary takes from its surroundings.
type Env struct {
	Args   []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Environ is the process environment.
	Environ []string
	Now     func() time.Time
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
		fmt.Fprintln(os.Stderr, "spec-proof-canary:", err)
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
	case commandEvaluate:
		return env.evaluate(rest)
	case commandShim:
		return env.shim(rest)
	case commandProbe:
		return env.probe(rest)
	case commandModules:
		return env.listModules()
	default:
		return env.usage()
	}
}

func (e Env) usage() int {
	fmt.Fprintf(e.Stderr, "usage: spec-proof-canary %s|%s --harness claude|codex|copilot [flags]\n", commandRun, commandEvaluate)
	return ExitUsage
}

func (e Env) flags(name string) *flag.FlagSet {
	set := flag.NewFlagSet("spec-proof-canary "+name, flag.ContinueOnError)
	set.SetOutput(e.Stderr)
	return set
}

func (e Env) run(args []string) int {
	set := e.flags(commandRun)
	options := app.Options{Canary: e.Executable}
	set.StringVar(&options.Harness, "harness", "", "harness to run: claude, codex or copilot")
	set.StringVar(&options.RepoRoot, "repo", ".", "repository root")
	set.StringVar(&options.OutDir, "out", os.TempDir(), "directory the run directory is created in")
	set.BoolVar(&options.DryRun, "dry-run", false, "print the command lines and run nothing")
	set.BoolVar(&options.PrepareOnly, "prepare-only", false, "prepare the project and run the no-call probes, then stop before the session")
	set.BoolVar(&options.ForceWindow, "force-window", false, "run outside the nightly window")
	set.BoolVar(&options.SharedHome, "shared-home", false, "let Claude Code use the credentials of ~/.claude, and Copilot CLI the COPILOT_HOME of the user, instead of an isolated configuration directory")
	set.StringVar(&options.CodexHome, "codex-home", "", "a dedicated CODEX_HOME; empty uses the user's")
	if err := set.Parse(args); err != nil || set.NArg() > 0 {
		return ExitUsage
	}
	root, err := filepath.Abs(options.RepoRoot)
	if err != nil {
		fmt.Fprintln(e.Stderr, "spec-proof-canary:", err)
		return ExitRefused
	}
	options.RepoRoot = root
	if !options.DryRun {
		if err := adapters.Lower(); err != nil {
			fmt.Fprintln(e.Stderr, "spec-proof-canary: cannot lower the priority:", err)
			return ExitRefused
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	outcome, err := e.runner().Run(ctx, options)
	if err != nil {
		fmt.Fprintln(e.Stderr, "spec-proof-canary:", err)
		if outcome.RunDir != "" {
			fmt.Fprintln(e.Stderr, "run directory:", outcome.RunDir)
		}
		return ExitRefused
	}
	if options.DryRun || options.PrepareOnly {
		return verdict.ExitAllPass
	}
	if err := app.WriteReport(outcome.Report, e.Stdout, e.Stderr); err != nil {
		fmt.Fprintln(e.Stderr, "spec-proof-canary:", err)
		return ExitRefused
	}
	fmt.Fprintln(e.Stderr, "records:", filepath.Join(outcome.RunDir, "records"))
	return outcome.Report.ExitCode()
}

func (e Env) runner() app.Runner {
	return app.Runner{
		Clock: systemClock{e.Now}, Window: window.Nightly(),
		Processes: adapters.Exec{}, Sessions: adapters.Sessions{}, Prober: adapters.McpProber{},
		Locker: adapters.FileLocker{}, Files: adapters.OSFiles{},
		Env: e.Environ, Stdout: e.Stdout, Stderr: e.Stderr,
	}
}

func (e Env) evaluate(args []string) int {
	set := e.flags(commandEvaluate)
	var harnessFlag, records string
	set.StringVar(&harnessFlag, "harness", "", "harness the records come from: claude, codex or copilot")
	set.StringVar(&records, "records", "", "the records directory of a run")
	if err := set.Parse(args); err != nil || set.NArg() > 0 || harnessFlag == "" || records == "" {
		return ExitUsage
	}
	report, err := e.runner().Evaluate(records, harnessFlag)
	if err != nil {
		fmt.Fprintln(e.Stderr, "spec-proof-canary:", err)
		return ExitRefused
	}
	if err := app.WriteReport(report, e.Stdout, e.Stderr); err != nil {
		fmt.Fprintln(e.Stderr, "spec-proof-canary:", err)
		return ExitRefused
	}
	return report.ExitCode()
}

// shim and probe are what the harness runs: they take no flags of their own beyond
// the record, and they separate the arguments they forward with `--`.
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

func (e Env) probe(args []string) int {
	set := e.flags(commandProbe)
	var record, label string
	set.StringVar(&record, "record", "", "the hook log to append to")
	set.StringVar(&label, "label", "", "the label of the record")
	if err := set.Parse(args); err != nil || record == "" || label == "" {
		return 0
	}
	return adapters.RunProbe(record, label, e.Stdin)
}

// modules prints, one per line and tab-separated, the directory, the binary and the
// build tags of each module whose binary a session needs: the build script reads
// it, so that the list lives in one place.
func (e Env) listModules() int {
	for _, module := range modules.All() {
		fmt.Fprintf(e.Stdout, "%s\t%s\t%s\n", module.Dir, module.Binary, module.BuildTags)
	}
	return verdict.ExitAllPass
}
