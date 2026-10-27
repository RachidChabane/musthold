// Package docker implements the loop-1 box port with the docker client. Every
// command runs in a fresh, named container of the study's image with no network,
// the host user's identity, no capability, the study's resource limits, and
// only the command's mounts besides the product directory, which is read-only at
// /opt/musthold. The container is never started with --rm: once the command has
// ended the runner reads two fields of its state, `.State.ExitCode` and
// `.State.OOMKilled`, and removes the container, whatever happened.
package docker

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
)

// Limits and fixed options of every container.
const (
	CPUs      = "2"
	Memory    = "3g"
	PidsLimit = "1024"
	// ProductTarget is where the product directory is mounted, read-only.
	ProductTarget = "/opt/musthold"
	// SlotsPerContainer is the number of machine-wide execution slots a container
	// holds while it runs.
	SlotsPerContainer = 2

	networkNone   = "none"
	envProgram    = "/usr/bin/env"
	nameRandom    = 4
	namePrefix    = "loop1-box-"
	stateFormat   = "{{.State.ExitCode}} {{.State.OOMKilled}}"
	mountForbid   = ",="
	killRetry     = 100 * time.Millisecond
	controlLimit  = time.Minute
	stderrExcerpt = 512
)

var (
	// ErrConfig refuses a runner configuration that cannot run a container.
	ErrConfig = errors.New("invalid docker runner configuration")
	// ErrMountPath refuses a mounted path that `--mount` cannot carry.
	ErrMountPath = errors.New("a mounted path cannot hold ',' or '='")
	// ErrNotRun is returned when the container did not run to an end the engine records.
	ErrNotRun = errors.New("the container did not run")
	// ErrRemove is returned when the container could not be removed after it ran.
	ErrRemove = errors.New("the container could not be removed")
)

// Config says how containers are run.
type Config struct {
	// Client is the docker client command, which names the study's engine.
	Client []string
	// RunWrapper is the command the client of a `run` is started under, such as
	// SlotWrapper's; empty, the client starts directly.
	RunWrapper []string
	// Image is the study's image, by id or digest.
	Image string
	// Product is the host's product directory.
	Product string
	// UID and GID are the user every container runs as.
	UID, GID int
}

// Runner runs box commands in containers. It is safe for concurrent use.
type Runner struct {
	config Config
	prefix string
	count  atomic.Uint64
}

var _ box.Runner = (*Runner)(nil)

// New checks a configuration and returns a runner for it.
func New(config Config) (*Runner, error) {
	if len(config.Client) == 0 || config.Image == "" || config.Product == "" {
		return nil, fmt.Errorf("%w: a client, an image and a product directory are required", ErrConfig)
	}
	if err := checkPath(config.Product); err != nil {
		return nil, err
	}
	random := make([]byte, nameRandom)
	if _, err := rand.Read(random); err != nil {
		return nil, err
	}
	return &Runner{config: config, prefix: namePrefix + hex.EncodeToString(random) + "-"}, nil
}

// SlotWrapper is the wrapper that makes a command hold the slots of a container
// for as long as it runs, waiting up to wait for them: `with-slot 2 --wait N --`.
func SlotWrapper(withSlot string, wait time.Duration) []string {
	return []string{withSlot, strconv.Itoa(SlotsPerContainer), "--wait", strconv.FormatFloat(wait.Seconds(), 'f', -1, 64), "--"}
}

func checkPath(path string) error {
	if strings.ContainsAny(path, mountForbid) {
		return fmt.Errorf("%w: %q", ErrMountPath, path)
	}
	return nil
}

// RunArgv is the argument list of the docker client that runs a command in the
// container name. The command's environment is the only one the command sees.
func (r *Runner) RunArgv(name string, command box.Command) ([]string, error) {
	argv := []string{
		"run", "--name", name, "--init",
		"--network", networkNone,
		"--user", fmt.Sprintf("%d:%d", r.config.UID, r.config.GID),
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges",
		"--cpus", CPUs, "--memory", Memory, "--memory-swap", Memory,
		"--pids-limit", PidsLimit,
		"--log-driver", "none",
	}
	mounts := append(append([]box.Mount{}, command.Mounts...), box.Mount{Path: r.config.Product, ReadOnly: true})
	for _, mount := range mounts {
		if err := checkPath(mount.Path); err != nil {
			return nil, err
		}
	}
	for _, mount := range command.Mounts {
		argv = append(argv, "--mount", bindMount(mount.Path, mount.Path, mount.ReadOnly))
	}
	argv = append(argv, "--mount", bindMount(r.config.Product, ProductTarget, true))
	if command.Dir != "" {
		argv = append(argv, "--workdir", command.Dir)
	}
	argv = append(argv, r.config.Image, envProgram, "-i")
	argv = append(argv, command.Env...)
	return append(argv, command.Argv...), nil
}

func bindMount(source, target string, readOnly bool) string {
	mount := "type=bind,source=" + source + ",target=" + target
	if readOnly {
		mount += ",readonly"
	}
	return mount
}

// Run runs the command in a fresh container and removes the container.
func (r *Runner) Run(ctx context.Context, command box.Command) (result box.Result, err error) {
	name := r.prefix + strconv.FormatUint(r.count.Add(1), 10)
	args, err := r.RunArgv(name, command)
	if err != nil {
		return box.Result{}, err
	}
	defer func() {
		if removeErr := r.remove(ctx, name); err == nil {
			err = removeErr
		}
	}()
	finished := make(chan struct{})
	var timedOut atomic.Bool
	if command.Limit > 0 {
		limit := time.AfterFunc(command.Limit, func() {
			select {
			case <-finished:
			default:
				timedOut.Store(true)
				r.killUntil(ctx, name, finished)
			}
		})
		defer limit.Stop()
	}
	run, runErr := r.invoke(ctx, append(append([]string{}, r.config.RunWrapper...), r.client(args)...))
	close(finished)
	if runErr != nil {
		return box.Result{}, fmt.Errorf("%w: %w", ErrNotRun, runErr)
	}
	state, err := r.state(ctx, name)
	if err != nil {
		return box.Result{}, fmt.Errorf("%w: %w: %s", ErrNotRun, err, excerpt(run.stderr))
	}
	return box.Result{
		ExitCode: state.exitCode, Stdout: run.stdout, Stderr: run.stderr,
		TimedOut: timedOut.Load(), OOMKilled: state.oomKilled,
	}, nil
}

func (r *Runner) client(args []string) []string {
	return append(append([]string{}, r.config.Client...), args...)
}

type output struct {
	stdout, stderr []byte
	exitCode       int
}

// invoke runs a process to its end. A process that ends with a status is an
// output; one that cannot start, or that the context stops, is an error.
func (r *Runner) invoke(ctx context.Context, argv []string) (output, error) {
	var stdout, stderr bytes.Buffer
	process := exec.CommandContext(ctx, argv[0], argv[1:]...)
	process.Stdout, process.Stderr = &stdout, &stderr
	err := process.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return output{stdout: stdout.Bytes(), stderr: stderr.Bytes()}, nil
	case errors.As(err, &exit) && ctx.Err() == nil:
		return output{stdout: stdout.Bytes(), stderr: stderr.Bytes(), exitCode: exit.ExitCode()}, nil
	default:
		return output{}, err
	}
}

type containerState struct {
	exitCode  int
	oomKilled bool
}

// state reads the two fields of the container's state the study records.
func (r *Runner) state(ctx context.Context, name string) (containerState, error) {
	out, err := r.control(ctx, "inspect", "--format", stateFormat, name)
	if err != nil {
		return containerState{}, err
	}
	var state containerState
	if _, err := fmt.Sscanf(strings.TrimSpace(string(out.stdout)), "%d %t", &state.exitCode, &state.oomKilled); err != nil {
		return containerState{}, fmt.Errorf("read the container state %q: %w", out.stdout, err)
	}
	return state, nil
}

// remove removes the container, whatever its state. It outlives a cancelled context.
func (r *Runner) remove(ctx context.Context, name string) error {
	if _, err := r.control(ctx, "rm", "--force", name); err != nil {
		return fmt.Errorf("%w: %w", ErrRemove, err)
	}
	return nil
}

// killUntil kills the container until a kill succeeds or the run has ended.
func (r *Runner) killUntil(ctx context.Context, name string, finished <-chan struct{}) {
	for {
		if _, err := r.control(ctx, "kill", name); err == nil {
			return
		}
		select {
		case <-finished:
			return
		case <-time.After(killRetry):
		}
	}
}

// control runs a short docker command, outside the slots, with a time bound of its own.
func (r *Runner) control(ctx context.Context, args ...string) (output, error) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), controlLimit)
	defer cancel()
	out, err := r.invoke(ctx, r.client(args))
	if err == nil && out.exitCode != 0 {
		err = fmt.Errorf("docker %s exited %d: %s", args[0], out.exitCode, excerpt(out.stderr))
	}
	return out, err
}

func excerpt(data []byte) string {
	text := strings.TrimSpace(string(data))
	if len(text) > stderrExcerpt {
		text = text[:stderrExcerpt]
	}
	return text
}
