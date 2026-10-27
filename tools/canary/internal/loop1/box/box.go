// Package box is the port through which the verbs of the loop-1 study execute a
// command in an offline container of the study's image (PREREG §7.3): accept,
// resolve, oracle and detect run every project command through it, never on the
// host.
package box

import (
	"context"
	"time"
)

// Mount is a host directory the container sees at the same path.
type Mount struct {
	Path     string
	ReadOnly bool
}

// Command is one command to run in a fresh offline container.
type Command struct {
	// Argv is the command, run under `env -i` with Env alone.
	Argv []string
	Env  []string
	// Dir is the working directory, a path under one of the mounts.
	Dir string
	// Mounts are the only host paths the container sees, besides the product
	// directory that every container mounts read-only.
	Mounts []Mount
	// Limit ends the command when it runs longer; zero means no limit.
	Limit time.Duration
}

// Result is how the command ended.
type Result struct {
	ExitCode int
	Stdout   []byte
	Stderr   []byte
	// TimedOut is set when the command reached its limit; OOMKilled when the
	// container was killed for memory.
	TimedOut  bool
	OOMKilled bool
}

// Runner runs commands in offline containers. An error means the container could
// not be run at all; a command that ran and failed is a Result.
type Runner interface {
	Run(ctx context.Context, command Command) (Result, error)
}
