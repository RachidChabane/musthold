package docker_test

import (
	"context"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/freeze"
)

const (
	longSleep   = "60"
	sleepLimit  = 3 * time.Second
	claudePin   = "2.1.295"
	pythonPin   = "3.13.14"
	pytestPin   = "9.1.1"
	unreachable = "import socket; socket.create_connection(('1.1.1.1', 443), timeout=3)"
)

// engineWorld is the engine, the product and a work directory a container may mount.
type engineWorld struct {
	runner *docker.Runner
	client string
	work   string
	repo   string
}

func (w engineWorld) run(t *testing.T, limit time.Duration, argv ...string) box.Result {
	t.Helper()
	result, err := w.runner.Run(context.Background(), box.Command{
		Argv: argv, Env: []string{box.SystemPath, "HOME=" + w.work}, Dir: w.work,
		Mounts: []box.Mount{{Path: w.work}}, Limit: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

// containerChecks run each check on the engine when the container tests are enabled, and
// report it as skipped otherwise, so that the results still name what was not checked.
func containerChecks(t *testing.T, checks map[string]func(t *testing.T, w engineWorld)) {
	names := slices.Sorted(maps.Keys(checks))
	if !dockertest.Ready(t, names...) {
		return
	}
	world := dockertest.New(t)
	w := engineWorld{runner: world.Runner, client: world.Client, work: world.Work, repo: world.Repo}
	for _, name := range names {
		t.Run(name, func(t *testing.T) { checks[name](t, w) })
	}
}

type tool struct {
	argv []string
	want string
}

func TestContainers(t *testing.T) {
	containerChecks(t, map[string]func(*testing.T, engineWorld){
		"a command is offline and runs as the host user [verifies SP-2639~1]": func(t *testing.T, w engineWorld) {
			if result := w.run(t, 0, "python3", "-c", unreachable); result.ExitCode == 0 {
				t.Error("a container reached an outside address")
			}
			if result := w.run(t, 0, "curl", "--silent", "--max-time", "5", "https://github.com"); result.ExitCode == 0 {
				t.Error("curl reached github.com")
			}
			result := w.run(t, 0, "id", "-u")
			if got := strings.TrimSpace(string(result.Stdout)); got != strconv.Itoa(os.Getuid()) {
				t.Errorf("uid = %q (stderr %q)", got, result.Stderr)
			}
		},
		"curl, git, Python, pytest and Claude Code are at their pins on the image's path [verifies SP-2639~1]": func(t *testing.T, w engineWorld) {
			for _, tool := range []tool{
				{[]string{"git", "--version"}, "git version"},
				{[]string{"curl", "--version"}, "curl"},
				{[]string{"python3", "--version"}, pythonPin},
				{[]string{"pytest", "--version"}, pytestPin},
				{[]string{"python3", "-m", "pytest", "--version"}, pytestPin},
				{[]string{"claude", "--version"}, claudePin},
			} {
				result := w.run(t, 0, tool.argv...)
				if output := string(result.Stdout) + string(result.Stderr); result.ExitCode != 0 || !strings.Contains(output, tool.want) {
					t.Errorf("%v: exit %d, output %q", tool.argv, result.ExitCode, output)
				}
			}
		},
		"the image's Claude Code CLI lies at the path every session and probe runs [verifies SP-2647~1]": func(t *testing.T, w engineWorld) {
			result := w.run(t, 0, freeze.ClaudeProgram, freeze.VersionFlag)
			if output := string(result.Stdout) + string(result.Stderr); result.ExitCode != 0 || !strings.Contains(output, claudePin) {
				t.Errorf("%s: exit %d, output %q", freeze.ClaudeProgram, result.ExitCode, output)
			}
		},
		"the product is read-only and the pinned Shallnot runs from it [verifies SP-2639~1]": func(t *testing.T, w engineWorld) {
			if result := w.run(t, 0, "touch", docker.ProductTarget+"/written"); result.ExitCode == 0 {
				t.Error("the product directory is writable")
			}
			if result := w.run(t, 0, docker.ProductTarget+"/bin/shallnot", "--version"); result.ExitCode != 0 {
				t.Errorf("shallnot --version: exit %d, stderr %q", result.ExitCode, result.Stderr)
			}
		},
		"only the asked mounts are there, and the work directory is writable [verifies SP-2639~1]": func(t *testing.T, w engineWorld) {
			if result := w.run(t, 0, "sh", "-c", "echo made > "+filepath.Join(w.work, "made")); result.ExitCode != 0 {
				t.Fatalf("exit %d: %s", result.ExitCode, result.Stderr)
			}
			if data, err := os.ReadFile(filepath.Join(w.work, "made")); err != nil || string(data) != "made\n" {
				t.Errorf("the host sees %q (%v)", data, err)
			}
			if result := w.run(t, 0, "ls", w.repo); result.ExitCode == 0 {
				t.Errorf("the repository %s is visible: %s", w.repo, result.Stdout)
			}
		},
		"a command past its limit is killed and no container is left [verifies SP-2639~1]": func(t *testing.T, w engineWorld) {
			if result := w.run(t, sleepLimit, "sleep", longSleep); !result.TimedOut {
				t.Errorf("result = %+v", result)
			}
			left, err := exec.Command(w.client, "ps", "--all", "--quiet", "--filter", "name=loop1-box-").Output()
			if err != nil || strings.TrimSpace(string(left)) != "" {
				t.Errorf("containers left: %q (%v)", left, err)
			}
		},
	})
}
