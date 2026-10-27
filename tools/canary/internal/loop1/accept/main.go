package accept

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The exit codes of the verbs.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// The defaults of the flags every verb takes.
const (
	DefaultConfig  = "experiments/loop-1/loop-1.json"
	WorkVariable   = "LOOP1_WORK"
	defaultWorkDir = ".musthold-loop-1"
)

// Flags are the flags every loop-1 verb takes.
type Flags struct {
	Repo   string
	Config string
	Work   string
}

// RegisterFlags declares --repo, --config and --work on a flag set.
func RegisterFlags(set *flag.FlagSet) *Flags {
	flags := &Flags{}
	set.StringVar(&flags.Repo, "repo", ".", "repository root")
	set.StringVar(&flags.Config, "config", DefaultConfig, "run configuration, relative to the repository root")
	set.StringVar(&flags.Work, "work", DefaultWork(), "work directory ($"+WorkVariable+", default $HOME/"+defaultWorkDir+")")
	return flags
}

// DefaultWork is $LOOP1_WORK, or $HOME/.musthold-loop-1.
func DefaultWork() string {
	if work := os.Getenv(WorkVariable); work != "" {
		return work
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return defaultWorkDir
	}
	return filepath.Join(home, defaultWorkDir)
}

// Layout is the work directory the flags name.
func (f Flags) Layout() study.Layout { return study.Layout{Root: f.Work} }

// ConfigPath is the run configuration's location on disk.
func (f Flags) ConfigPath() string {
	if filepath.IsAbs(f.Config) {
		return f.Config
	}
	return filepath.Join(f.Repo, f.Config)
}

// Wiring holds the adapters that reach outside the process: the box that runs a
// command in an offline container, and the equipping of the control arm.
type Wiring struct {
	Runner   box.Runner
	Equipper Equipper
}

// Wire builds the production adapters for a repository and a work directory. The
// program that assembles the verbs assigns it; the verbs refuse to run while it
// is unset.
var Wire func(repo string, layout study.Layout) (Wiring, error)

// ErrNotWired is returned when no production adapters were assigned.
var ErrNotWired = errors.New("no container runner is wired into this build")

// NewAcceptor builds the acceptor of a repository from the verb's flags.
func NewAcceptor(flags Flags, wiring Wiring, writer records.Writer) *Acceptor {
	return &Acceptor{
		Repo: flags.Repo, Config: flags.ConfigPath(), Layout: flags.Layout(),
		Runner: wiring.Runner, Equipper: wiring.Equipper, Records: writer,
	}
}

// Main runs the accept verb: it checks the tasks named on the command line, or
// every task directory in name order, prints each report, and appends each task
// that passes to the run configuration. It returns 0 when every task passes, 1
// when a check fails or cannot run, and 2 for a usage error.
func Main(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	set := flag.NewFlagSet("accept", flag.ContinueOnError)
	set.SetOutput(stderr)
	flags := RegisterFlags(set)
	if err := set.Parse(args); err != nil {
		return ExitUsage
	}
	if Wire == nil {
		fmt.Fprintf(stderr, "accept: %v\n", ErrNotWired)
		return ExitFailure
	}
	wiring, err := Wire(flags.Repo, flags.Layout())
	if err != nil {
		fmt.Fprintf(stderr, "accept: %v\n", err)
		return ExitFailure
	}
	names := set.Args()
	if len(names) == 0 {
		if names, err = TaskNames(flags.Repo); err != nil {
			fmt.Fprintf(stderr, "accept: %v\n", err)
			return ExitFailure
		}
	}
	return acceptAll(ctx, NewAcceptor(*flags, wiring, records.NewFileWriter(flags.Layout())), names, stdout, stderr)
}

func acceptAll(ctx context.Context, acceptor *Acceptor, names []string, stdout, stderr io.Writer) int {
	code := ExitOK
	for _, name := range names {
		report, err := acceptor.Accept(ctx, name, 1)
		if err != nil {
			fmt.Fprintf(stderr, "accept: %s: %v\n", name, err)
			code = ExitFailure
			continue
		}
		report.Print(stdout)
		if !report.Passed() {
			code = ExitFailure
		}
	}
	return code
}

// TaskNames lists the task directories of a repository in name order.
func TaskNames(repo string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(repo, filepath.FromSlash(TasksDir)))
	if err != nil {
		return nil, fmt.Errorf("list the tasks: %w", err)
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}
