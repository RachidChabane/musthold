package task

import (
	"flag"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// The exit codes of the construction verbs.
const (
	ExitOK      = 0
	ExitFailure = 1
	ExitUsage   = 2
)

// Flags are the flags every loop-2 construction verb takes.
type Flags struct {
	Repo, Config, Work string
}

// RegisterFlags declares --repo, --config and --work on a flag set.
func RegisterFlags(set *flag.FlagSet) *Flags {
	flags := &Flags{}
	set.StringVar(&flags.Repo, "repo", ".", "repository root")
	set.StringVar(&flags.Config, "config", ConfigFile, "run configuration, relative to the repository root")
	set.StringVar(&flags.Work, "work", DefaultWork(), "work directory ($"+WorkEnv+")")
	return flags
}

// ConfigPath is the run configuration's location on disk.
func (f Flags) ConfigPath() string { return f.under(f.Config) }

// Path is a slash path of the repository on disk.
func (f Flags) Path(relative string) string { return f.under(filepath.FromSlash(relative)) }

func (f Flags) under(location string) string {
	if filepath.IsAbs(location) {
		return location
	}
	return filepath.Join(f.Repo, location)
}

// Layout is the work directory the flags name, in loop-1's layout.
func (f Flags) Layout() study.Layout { return study.Layout{Root: f.Work} }
