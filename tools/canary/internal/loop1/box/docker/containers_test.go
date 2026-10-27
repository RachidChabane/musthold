package docker_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/product"
)

const (
	containersVariable = "MUSTHOLD_LOOP1_CONTAINERS"
	repoFromHere       = "../../../../../.."
	dockerFront        = "experiments/loop-1/script/docker"
	slotFront          = "script/with-slot"
	defaultWork        = ".musthold-loop-1"
	defaultProduct     = ".musthold-loop-1-product"
	imagePath          = "/usr/local/bin:/usr/bin:/bin"
	slotWait           = 10 * time.Minute
	longSleep          = "60"
	sleepLimit         = 3 * time.Second
	claudePin          = "2.1.295"
	pythonPin          = "3.13.14"
	pytestPin          = "9.1.1"
	unreachable        = "import socket; socket.create_connection(('1.1.1.1', 443), timeout=3)"
)

// engineWorld is the engine, the product and a work directory a container may mount.
type engineWorld struct {
	runner *docker.Runner
	client string
	work   string
}

// containerWorld readies the engine's world, or skips the check when the container
// tests are not enabled (script/build builds the product; script/engine start starts the engine).
func containerWorld(t *testing.T) engineWorld {
	t.Helper()
	if os.Getenv(containersVariable) != "1" {
		t.Skipf("set %s=1 with the engine running (experiments/loop-1/script/engine start) to run the container tests", containersVariable)
	}
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.UserHomeDir()
	productDir := envOr("LOOP1_PRODUCT", filepath.Join(home, defaultProduct))
	data, err := os.ReadFile(filepath.Join(productDir, product.ManifestFile))
	if err != nil {
		t.Fatalf("no product: %v (run experiments/loop-1/script/build)", err)
	}
	manifest, err := product.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	productDir, err = filepath.EvalSymlinks(productDir)
	if err != nil {
		t.Fatal(err)
	}
	client := filepath.Join(repo, dockerFront)
	runner, err := docker.New(docker.Config{
		Client:     []string{client},
		RunWrapper: docker.SlotWrapper(filepath.Join(repo, slotFront), slotWait),
		Image:      manifest.Images.Built, Product: productDir, UID: os.Getuid(), GID: os.Getgid(),
	})
	if err != nil {
		t.Fatal(err)
	}
	base := envOr("LOOP1_WORK", filepath.Join(home, defaultWork))
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(base, "boxtest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	if work, err = filepath.EvalSymlinks(work); err != nil {
		t.Fatal(err)
	}
	return engineWorld{runner: runner, client: client, work: work}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}

func (w engineWorld) run(t *testing.T, limit time.Duration, argv ...string) box.Result {
	t.Helper()
	result, err := w.runner.Run(context.Background(), box.Command{
		Argv: argv, Env: []string{"PATH=" + imagePath, "HOME=" + w.work}, Dir: w.work,
		Mounts: []box.Mount{{Path: w.work}}, Limit: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestContainers(t *testing.T) {
	w := containerWorld(t)

	t.Run("a command is offline and runs as the host user [verifies SP-2639~1]", func(t *testing.T) {
		if result := w.run(t, 0, "python3", "-c", unreachable); result.ExitCode == 0 {
			t.Error("a container reached an outside address")
		}
		result := w.run(t, 0, "id", "-u")
		if got := strings.TrimSpace(string(result.Stdout)); got != strconv.Itoa(os.Getuid()) {
			t.Errorf("uid = %q (stderr %q)", got, result.Stderr)
		}
	})

	t.Run("git, Python, pytest and Claude Code are at their pins on the image's path [verifies SP-2639~1]", func(t *testing.T) {
		for _, tool := range []struct {
			argv []string
			want string
		}{
			{[]string{"git", "--version"}, "git version"},
			{[]string{"python3", "--version"}, pythonPin},
			{[]string{"pytest", "--version"}, pytestPin},
			{[]string{"claude", "--version"}, claudePin},
		} {
			result := w.run(t, 0, tool.argv...)
			if output := string(result.Stdout) + string(result.Stderr); result.ExitCode != 0 || !strings.Contains(output, tool.want) {
				t.Errorf("%v: exit %d, output %q", tool.argv, result.ExitCode, output)
			}
		}
	})

	t.Run("the product is read-only and the pinned Shallnot runs from it [verifies SP-2639~1]", func(t *testing.T) {
		if result := w.run(t, 0, "touch", docker.ProductTarget+"/written"); result.ExitCode == 0 {
			t.Error("the product directory is writable")
		}
		if result := w.run(t, 0, docker.ProductTarget+"/bin/shallnot", "--version"); result.ExitCode != 0 {
			t.Errorf("shallnot --version: exit %d, stderr %q", result.ExitCode, result.Stderr)
		}
	})

	t.Run("only the asked mounts are there, and the work directory is writable [verifies SP-2639~1]", func(t *testing.T) {
		if result := w.run(t, 0, "sh", "-c", "echo made > "+filepath.Join(w.work, "made")); result.ExitCode != 0 {
			t.Fatalf("exit %d: %s", result.ExitCode, result.Stderr)
		}
		if data, err := os.ReadFile(filepath.Join(w.work, "made")); err != nil || string(data) != "made\n" {
			t.Errorf("the host sees %q (%v)", data, err)
		}
		home, _ := os.UserHomeDir()
		if result := w.run(t, 0, "ls", home); result.ExitCode == 0 {
			t.Errorf("the home directory %s is visible: %s", home, result.Stdout)
		}
	})

	t.Run("a command past its limit is killed and no container is left [verifies SP-2639~1]", func(t *testing.T) {
		result := w.run(t, sleepLimit, "sleep", longSleep)
		if !result.TimedOut {
			t.Errorf("result = %+v", result)
		}
		left, err := exec.Command(w.client, "ps", "--all", "--quiet", "--filter", "name=loop1-box-").Output()
		if err != nil || strings.TrimSpace(string(left)) != "" {
			t.Errorf("containers left: %q (%v)", left, err)
		}
	})
}
