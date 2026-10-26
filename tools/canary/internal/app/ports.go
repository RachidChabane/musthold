// Package app is the canary's use cases: plan a session, prepare it, run it
// and evaluate what it recorded. It reaches processes, the clock and the lock
// only through the ports declared here.
package app

import (
	"context"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
)

// Output is what a finished command printed.
type Output struct {
	Stdout, Stderr string
	ExitCode       int
}

// CommandSpec is a command to run to completion.
type CommandSpec struct {
	Argv    []string
	Dir     string
	Env     []string
	Timeout time.Duration
}

// Processes runs commands to completion. A non-zero exit is not an error; failing
// to start or running out of time is.
type Processes interface {
	Run(ctx context.Context, spec CommandSpec) (Output, error)
}

// SessionSpec is the harness CLI process of a canary session.
type SessionSpec struct {
	Argv []string
	Dir  string
	Env  []string
	// StdoutPath and StderrPath receive what the CLI prints.
	StdoutPath, StderrPath string
	// TimesPath, when set, receives one JSON line per stdout line: its number and
	// the milliseconds since the CLI started at which it arrived.
	TimesPath string
	// Stdin, when set, is written to the CLI's standard input, which is then
	// closed; without it the CLI reads nothing.
	Stdin []byte
	// Counts says whether a stdout line is a step the cap counts.
	Counts func(line []byte) bool
	// Stops, when set, reads each stdout line in order and returns the reason to
	// end the session at that line, or an empty string.
	Stops   func(line []byte) string
	Cap     int
	Timeout time.Duration
}

// SessionOutcome is how a session ended.
type SessionOutcome struct {
	ExitCode int
	// Killed is the reason the runner ended the session, or empty.
	Killed string
}

// Sessions runs the harness CLI, enforcing the caps.
type Sessions interface {
	Run(spec SessionSpec) (SessionOutcome, error)
}

// ServerSpec says how to start the executor server of a module.
type ServerSpec struct {
	Binary  string
	Harness string
	Dir     string
	Env     []string
}

// ServerProber talks to an executor server over standard input and output.
type ServerProber interface {
	// Handshake starts the server, initializes it and asks for its tools.
	Handshake(ctx context.Context, spec ServerSpec) evidence.ServerStart
	// Probe asks the server for a ticket with no capability.
	Probe(ctx context.Context, spec ServerSpec, ticket string) evidence.CapProbe
}

// Lock is a held single-session lock.
type Lock interface{ Release() }

// Locker takes the lock that keeps two sessions from running at once.
type Locker interface {
	Acquire(path string) (Lock, error)
}

// Tokens draws the secrets of a run.
type Tokens interface {
	// New returns a token no agent of the session can know before it reads it.
	New() (string, error)
}

// Clock tells the time.
type Clock interface{ Now() time.Time }
