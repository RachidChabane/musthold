package docker_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
)

const (
	unitSeparator = "\x1f"
	productDir    = "/var/product"
	imageID       = "sha256:0123456789abcdef"
	// fakeDocker records its argument list, one line per call, and behaves as the
	// files of $FAKE_DIR say.
	fakeDocker = `#!/bin/sh
{ printf '%s\037' "$@"; printf '\n'; } >>"$FAKE_DIR/calls"
case "$1" in
run)
  echo "standard output"; echo "standard error" >&2
  if [ -f "$FAKE_DIR/block" ]; then
    while [ ! -f "$FAKE_DIR/killed" ]; do sleep 0.02; done
  fi
  exit "$(cat "$FAKE_DIR/run-exit" 2>/dev/null || echo 0)" ;;
inspect)
  case "$3" in *Running*) [ -f "$FAKE_DIR/block" ] && [ ! -f "$FAKE_DIR/killed" ] && echo true || echo false; exit 0 ;; esac
  [ -f "$FAKE_DIR/no-container" ] && { echo "Error: No such object" >&2; exit 1; }
  cat "$FAKE_DIR/state" 2>/dev/null || echo "0 false" ;;
kill) touch "$FAKE_DIR/killed" ;;
rm) ;;
esac
`
	// fakeSlots records its argument list and runs the command after "--".
	fakeSlots = `#!/bin/sh
{ printf '%s\037' "$@"; printf '\n'; } >>"$FAKE_DIR/slots"
while [ "$1" != "--" ]; do shift; done
shift
exec "$@"
`
)

type fake struct {
	dir    string
	docker string
	slots  string
}

func newFake(t *testing.T) fake {
	t.Helper()
	dir := t.TempDir()
	f := fake{dir: dir, docker: filepath.Join(dir, "docker"), slots: filepath.Join(dir, "with-slot")}
	for path, script := range map[string]string{f.docker: fakeDocker, f.slots: fakeSlots} {
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("FAKE_DIR", dir)
	return f
}

func (f fake) set(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// calls are the recorded argument lists of one fake, in order.
func (f fake) calls(t *testing.T, file string) [][]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, file))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		calls = append(calls, strings.Split(strings.TrimSuffix(line, unitSeparator), unitSeparator))
	}
	return calls
}

func (f fake) verbs(t *testing.T) []string {
	var verbs []string
	for _, call := range f.calls(t, "calls") {
		if !slices.Contains(call, "{{.State.Running}}") {
			verbs = append(verbs, call[0])
		}
	}
	return verbs
}

func (f fake) runner(t *testing.T, wrapper ...string) *docker.Runner {
	t.Helper()
	runner, err := docker.New(docker.Config{
		Client: []string{f.docker}, RunWrapper: wrapper, Image: imageID, Product: productDir, UID: 501, GID: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func sample() box.Command {
	return box.Command{
		Argv: []string{"pytest", "-q"}, Env: []string{"HOME=/work/run/home", "LANG=C.UTF-8"}, Dir: "/work/run/project",
		Mounts: []box.Mount{{Path: "/work/run"}, {Path: "/work/ref", ReadOnly: true}},
	}
}

func contains(argv []string, sequence ...string) bool {
	for i := range argv {
		if i+len(sequence) <= len(argv) && slices.Equal(argv[i:i+len(sequence)], sequence) {
			return true
		}
	}
	return false
}

func TestRunOptions(t *testing.T) {
	f := newFake(t)
	runner := f.runner(t)
	if _, err := runner.Run(context.Background(), sample()); err != nil {
		t.Fatal(err)
	}
	run := f.calls(t, "calls")[0]

	t.Run("a command runs with no network, the host user, no capability, no new privilege and the study's limits [verifies SP-2633~1]", func(t *testing.T) {
		for _, sequence := range [][]string{
			{"--network", "none"}, {"--user", "501:20"}, {"--cap-drop", "ALL"}, {"--security-opt", "no-new-privileges"},
			{"--cpus", "2"}, {"--memory", "3g"}, {"--memory-swap", "3g"}, {"--pids-limit", "1024"},
		} {
			if !contains(run, sequence...) {
				t.Errorf("the run lacks %v: %v", sequence, run)
			}
		}
	})

	t.Run("a mounted path that --mount cannot carry is refused, before any container is started [verifies SP-2633~1]", func(t *testing.T) {
		g := newFake(t)
		command := sample()
		command.Mounts = []box.Mount{{Path: "/work/a,b"}}
		if _, err := g.runner(t).Run(context.Background(), command); !errors.Is(err, docker.ErrMountPath) {
			t.Errorf("error = %v", err)
		}
		if calls := g.calls(t, "calls"); len(calls) != 0 {
			t.Errorf("docker was called: %v", calls)
		}
		if _, err := docker.New(docker.Config{Client: []string{"docker"}, Image: imageID, Product: "/p=q"}); !errors.Is(err, docker.ErrMountPath) {
			t.Errorf("a product path with '=' gives %v", err)
		}
	})

	t.Run("a configuration without a client, an image or a product directory is refused [verifies SP-2633~1]", func(t *testing.T) {
		if _, err := docker.New(docker.Config{Image: imageID, Product: productDir}); !errors.Is(err, docker.ErrConfig) {
			t.Errorf("error = %v", err)
		}
	})
}

func TestRunMountsAndEnvironment(t *testing.T) {
	f := newFake(t)
	runner := f.runner(t)
	for range 2 {
		if _, err := runner.Run(context.Background(), sample()); err != nil {
			t.Fatal(err)
		}
	}
	calls := f.calls(t, "calls")
	run := calls[0]

	t.Run("the command's mounts are at their own path and the product directory is read-only at /opt/musthold [verifies SP-2634~1]", func(t *testing.T) {
		for _, mount := range []string{
			"type=bind,source=/work/run,target=/work/run",
			"type=bind,source=/work/ref,target=/work/ref,readonly",
			"type=bind,source=/var/product,target=/opt/musthold,readonly",
		} {
			if !contains(run, "--mount", mount) {
				t.Errorf("the run lacks mount %s: %v", mount, run)
			}
		}
		if got := strings.Count(strings.Join(run, " "), "--mount"); got != 3 {
			t.Errorf("%d mounts, want 3", got)
		}
		if !contains(run, "--workdir", "/work/run/project") {
			t.Errorf("no working directory: %v", run)
		}
	})

	t.Run("the command runs under env -i with its own environment alone, and the container is not started with --rm [verifies SP-2634~1]", func(t *testing.T) {
		if !contains(run, imageID, "/usr/bin/env", "-i", "HOME=/work/run/home", "LANG=C.UTF-8", "pytest", "-q") {
			t.Errorf("the command line is %v", run)
		}
		if slices.Contains(run, "--rm") {
			t.Error("the container is started with --rm")
		}
	})

	t.Run("each command runs in a container of its own name [verifies SP-2634~1]", func(t *testing.T) {
		names := map[string]bool{}
		for _, call := range calls {
			if call[0] == "run" {
				names[call[slices.Index(call, "--name")+1]] = true
			}
		}
		if len(names) != 2 {
			t.Errorf("names = %v", names)
		}
	})
}

func TestRunResult(t *testing.T) {
	t.Run("the exit code and the memory kill come from the container's state, the output from the client [verifies SP-2635~1]", func(t *testing.T) {
		f := newFake(t)
		f.set(t, "state", "3 true\n")
		f.set(t, "run-exit", "0")
		result, err := f.runner(t).Run(context.Background(), sample())
		if err != nil {
			t.Fatal(err)
		}
		if result.ExitCode != 3 || !result.OOMKilled || result.TimedOut {
			t.Errorf("result = %+v", result)
		}
		if string(result.Stdout) != "standard output\n" || string(result.Stderr) != "standard error\n" {
			t.Errorf("output = %q, %q", result.Stdout, result.Stderr)
		}
	})

	t.Run("only the exit code and the memory flag are read, then the container is removed by name [verifies SP-2635~1]", func(t *testing.T) {
		f := newFake(t)
		if _, err := f.runner(t).Run(context.Background(), sample()); err != nil {
			t.Fatal(err)
		}
		if got := f.verbs(t); !slices.Equal(got, []string{"run", "inspect", "rm"}) {
			t.Fatalf("calls = %v", got)
		}
		calls := f.calls(t, "calls")
		name := calls[0][slices.Index(calls[0], "--name")+1]
		if !slices.Equal(calls[1], []string{"inspect", "--format", "{{.State.ExitCode}} {{.State.OOMKilled}}", name}) {
			t.Errorf("inspect = %v", calls[1])
		}
		if !slices.Equal(calls[2], []string{"rm", "--force", name}) {
			t.Errorf("rm = %v", calls[2])
		}
	})

	t.Run("a container that cannot be inspected is an error and is still removed [verifies SP-2635~1]", func(t *testing.T) {
		f := newFake(t)
		f.set(t, "no-container", "")
		f.set(t, "run-exit", "125")
		if _, err := f.runner(t).Run(context.Background(), sample()); !errors.Is(err, docker.ErrNotRun) {
			t.Errorf("error = %v", err)
		}
		if got := f.verbs(t); !slices.Equal(got, []string{"run", "inspect", "rm"}) {
			t.Errorf("calls = %v", got)
		}
	})

	t.Run("a cancelled context stops the command and the container is still removed [verifies SP-2635~1]", func(t *testing.T) {
		f := newFake(t)
		f.set(t, "block", "")
		ctx, cancel := context.WithCancel(context.Background())
		time.AfterFunc(200*time.Millisecond, cancel)
		if _, err := f.runner(t).Run(ctx, sample()); err == nil {
			t.Error("the cancelled run returned no error")
		}
		if verbs := f.verbs(t); verbs[len(verbs)-1] != "rm" {
			t.Errorf("calls = %v", verbs)
		}
	})
}

func TestRunLimit(t *testing.T) {
	t.Run("a command that outlives its limit is killed and its result is timed out [verifies SP-2636~1]", func(t *testing.T) {
		f := newFake(t)
		f.set(t, "block", "")
		f.set(t, "state", "137 false")
		command := sample()
		command.Limit = 100 * time.Millisecond
		result, err := f.runner(t).Run(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		if !result.TimedOut || result.ExitCode != 137 {
			t.Errorf("result = %+v", result)
		}
		if got := f.verbs(t); !slices.Equal(got, []string{"run", "kill", "inspect", "rm"}) {
			t.Errorf("calls = %v", got)
		}
	})

	t.Run("a command that ends within its limit is not killed [verifies SP-2636~1]", func(t *testing.T) {
		f := newFake(t)
		command := sample()
		command.Limit = time.Minute
		result, err := f.runner(t).Run(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		if result.TimedOut || slices.Contains(f.verbs(t), "kill") {
			t.Errorf("result = %+v, calls = %v", result, f.verbs(t))
		}
	})
}

func TestSlots(t *testing.T) {
	f := newFake(t)
	wrapper := docker.SlotWrapper(f.slots, 90*time.Second)

	t.Run("the slot wrapper holds two slots and waits for them [verifies SP-2637~1]", func(t *testing.T) {
		want := []string{f.slots, strconv.Itoa(docker.SlotsPerContainer), "--wait", "90", "--"}
		if !slices.Equal(wrapper, want) || docker.SlotsPerContainer != 2 {
			t.Errorf("wrapper = %v", wrapper)
		}
	})

	t.Run("the docker client of the run starts under the wrapper and the short commands do not [verifies SP-2637~1]", func(t *testing.T) {
		if _, err := f.runner(t, wrapper...).Run(context.Background(), sample()); err != nil {
			t.Fatal(err)
		}
		slots := f.calls(t, "slots")
		held := len(wrapper) - 1
		if len(slots) != 1 || len(slots[0]) < held+2 || !slices.Equal(slots[0][:held], wrapper[1:]) || slots[0][held] != f.docker || slots[0][held+1] != "run" {
			t.Errorf("wrapper calls = %v", slots)
		}
		if got := f.verbs(t); !slices.Equal(got, []string{"run", "inspect", "rm"}) {
			t.Errorf("client calls = %v", got)
		}
	})
}

func TestRunArgvProperties(t *testing.T) {
	f := newFake(t)
	runner := f.runner(t)
	word := rapid.StringMatching(`[A-Za-z0-9_./:-]{1,12}`)
	t.Run("whatever the command, the isolation options come first and the command's words come last, unchanged [verifies SP-2633~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			command := box.Command{
				Argv:   rapid.SliceOfN(word, 1, 6).Draw(t, "argv"),
				Env:    rapid.SliceOfN(rapid.StringMatching(`[A-Z]{1,6}=[a-z0-9 ,=]{0,10}`), 0, 4).Draw(t, "env"),
				Mounts: []box.Mount{{Path: "/work/" + word.Draw(t, "mount"), ReadOnly: rapid.Bool().Draw(t, "ro")}},
			}
			argv, err := runner.RunArgv("name", command)
			if err != nil {
				t.Fatal(err)
			}
			tail := append(append([]string{imageID, "/usr/bin/env", "-i"}, command.Env...), command.Argv...)
			if !slices.Equal(argv[len(argv)-len(tail):], tail) {
				t.Fatalf("tail = %v, want %v", argv[len(argv)-len(tail):], tail)
			}
			for _, option := range []string{"--network", "--cap-drop", "--security-opt", "--memory"} {
				if slices.Index(argv, option) < 0 || slices.Index(argv, option) > slices.Index(argv, imageID) {
					t.Fatalf("%s is missing or after the image: %v", option, argv)
				}
			}
		})
	})
}
