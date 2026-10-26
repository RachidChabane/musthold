package ports

import (
	"context"
	"time"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
)

// Command is one process to run.
type Command struct {
	Argv []string
	Dir  string
	// Env lists KEY=VALUE entries added to the inherited environment.
	Env []string
	// Timeout is the deadline after which the process group is killed; zero means
	// no deadline beyond the context's.
	Timeout time.Duration
	// StdoutFile, when set, receives the whole standard output, which
	// CommandResult.Stdout then leaves out.
	StdoutFile string
}

// CommandResult is what a finished process left behind.
type CommandResult struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
	// TimedOut is true when the deadline killed the process group.
	TimedOut bool
}

// ProcessRunner runs a command with a process group, a deadline and captured
// output. A non-zero exit status is a result, not an error.
type ProcessRunner interface {
	Run(ctx context.Context, command Command) (CommandResult, error)
}

// ResultsReader reads per-test outcomes from JUnit XML files. A pattern that
// matches no file contributes nothing.
type ResultsReader interface {
	Read(patterns []string, format ecosystem.ReportFormat) (map[ecosystem.TestID]ecosystem.TestOutcome, error)
}

// Workspaces manages the working copies a mutant run executes in. They live
// outside the repository.
type Workspaces interface {
	// Prepare copies the project at source once, honouring the layout's
	// exclusions, and returns the reference workspace.
	Prepare(ctx context.Context, source string, layout ecosystem.Layout) (ecosystem.Workspace, error)
	// Clone derives a mutant workspace from the built reference workspace.
	Clone(ctx context.Context, reference ecosystem.Workspace, layout ecosystem.Layout) (ecosystem.Workspace, error)
	// ReadFile reads a file, relative POSIX, of the workspace.
	ReadFile(workspace ecosystem.Workspace, file string) ([]byte, error)
	// WriteFile replaces a file of the workspace and gives it a modification time
	// later than every build output of the workspace.
	WriteFile(workspace ecosystem.Workspace, layout ecosystem.Layout, file string, content []byte) error
	// Release removes the workspace.
	Release(workspace ecosystem.Workspace) error
}
