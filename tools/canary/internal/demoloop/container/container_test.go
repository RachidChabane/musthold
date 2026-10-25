package container_test

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
)

// maxLabel is the longest label a DNS name may hold.
const maxLabel = 63

var box = container.Box{Image: "sha256:abc", RunDir: "/work/r1", Product: "/product", UID: 501, GID: 20, Run: "r1"}

func TestStepArgv(t *testing.T) {
	argv := box.StepArgv(3, "/work/r1/project", []string{"HOME=/home/demo", "PATH=/opt/spec-proof/bin:/usr/bin"}, []string{"git", "status"})
	line := strings.Join(argv, " ")

	t.Run("a step runs as the user, without capability, network or writable root, within the limits, with its environment emptied and refilled [verifies SP-2410~1]", func(t *testing.T) {
		for _, wanted := range []string{
			"run --name spdl-step-r1-3 --init --user 501:20 --cap-drop ALL --security-opt no-new-privileges --read-only",
			"--tmpfs /home/demo:size=256m,mode=1777", "--cpus 2 --memory 2560m --memory-swap 2560m", "--log-driver none",
			"--mount type=bind,source=/work/r1,target=/work/r1", "--mount type=bind,source=/product,target=/opt/spec-proof,readonly",
			"--rm --network none --workdir /work/r1/project sha256:abc /usr/bin/env -i HOME=/home/demo PATH=/opt/spec-proof/bin:/usr/bin git status",
		} {
			if !strings.Contains(line, wanted) {
				t.Errorf("the step lacks %q:\n%s", wanted, line)
			}
		}
		if strings.Contains(line, "docker.sock") || slices.Contains(argv, "--privileged") {
			t.Errorf("the step reaches the engine: %s", line)
		}
	})
}

func TestSessionAndProxy(t *testing.T) {
	t.Run("the session reads standard input, sits on the internal network and starts its command through the entry [verifies SP-2411~2]", func(t *testing.T) {
		line := strings.Join(box.SessionArgv("/work/r1/project", container.ProxyVariables(), []string{"/opt/spec-proof/claude/claude", "-p", "task"}), " ")
		for _, wanted := range []string{"--interactive --network spdl-net-r1", "HTTPS_PROXY=http://egress-proxy:3128", "/opt/spec-proof/bin/spec-proof-demo-loop entry -- /opt/spec-proof/claude/claude -p task"} {
			if !strings.Contains(line, wanted) {
				t.Errorf("the session lacks %q:\n%s", wanted, line)
			}
		}
		if got := strings.Join(box.NetworkArgv(), " "); got != "network create --internal spdl-net-r1" {
			t.Errorf("network = %s", got)
		}
	})

	t.Run("the session reaches the proxy by an alias that fits a DNS label, however long the run's name [verifies SP-2411~2]", func(t *testing.T) {
		long := box
		long.Run = "spec-proof-demo-loop-boxoffice-four-proofs-1-3156254532"
		if got := strings.Join(long.ConnectArgv(), " "); got != "network connect --alias egress-proxy spdl-net-"+long.Run+" spdl-proxy-"+long.Run {
			t.Errorf("connect = %s", got)
		}
		for _, variable := range container.ProxyVariables() {
			_, url, _ := strings.Cut(variable, "=")
			host := strings.TrimSuffix(strings.TrimPrefix(url, "http://"), ":"+container.ProxyPort)
			if len(host) > maxLabel || strings.Contains(host, long.Run) {
				t.Errorf("the session addresses the proxy as %q", host)
			}
		}
	})

	t.Run("the proxy forwards to the model endpoints only and logs into the run's records [verifies SP-2415~1]", func(t *testing.T) {
		line := strings.Join(box.ProxyArgv("/work/r1/records/egress.log"), " ")
		if !strings.Contains(line, "--detach sha256:abc /usr/bin/env -i PATH=/usr/local/bin:/usr/bin:/bin /opt/spec-proof/bin/spec-proof-demo-loop egress-proxy --listen :3128 --log /work/r1/records/egress.log --allow api.anthropic.com:443") {
			t.Errorf("proxy = %s", line)
		}
		if !container.Allowed("api.anthropic.com:443") || container.Allowed("api.anthropic.com:80") || container.Allowed("statsig.anthropic.com:443") {
			t.Error("the allowlist is not exactly the model endpoint")
		}
	})

	t.Run("a printed session names the token as standard input, and a mounted path docker cannot carry is refused [verifies SP-2403~2]", func(t *testing.T) {
		if line := container.Line("docker", []string{"run", "a b"}, true); line != "docker run 'a b'   (standard input: CLAUDE_CODE_OAUTH_TOKEN=<stdin>)" {
			t.Errorf("line = %s", line)
		}
		bad := box
		bad.RunDir = "/work/a,b"
		if err := bad.Validate(); !errors.Is(err, container.ErrMountPath) || box.Validate() != nil {
			t.Errorf("validate: %v", err)
		}
	})
}
