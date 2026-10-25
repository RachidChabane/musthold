package app_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

const (
	repoFromHere = "../../../../.."
	testToken    = "sk-ant-oat01-test-token-that-must-stay-in-the-session"
	testPin      = "2.1.295"
	dockerFront  = "/repo/demo/loop/container/docker"
	testImage    = "sha256:0123456789abcdef"
)

// tickingClock gives each reading a step after the one before.
type tickingClock struct {
	mutex sync.Mutex
	at    time.Time
	step  time.Duration
}

func (c *tickingClock) Now() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	now := c.at
	c.at = c.at.Add(c.step)
	return now
}

var (
	night = time.Date(2026, time.October, 25, 22, 0, 0, 0, time.Local)
	noon  = time.Date(2026, time.October, 25, 12, 0, 0, 0, time.Local)
)

// fakeCredential is a credential source that counts the reads of its value.
type fakeCredential struct {
	present bool
	reads   *int
}

func (f fakeCredential) Source() string                        { return canary.SourceKeychain }
func (f fakeCredential) Present(context.Context) (bool, error) { return f.present, nil }
func (f fakeCredential) Token(context.Context) (string, error) { *f.reads++; return testToken, nil }

// dockerHost stands for the engine: it records every command and answers the
// version probe of the CLI.
type dockerHost struct {
	mutex   sync.Mutex
	calls   []canary.CommandSpec
	version string
}

func (d *dockerHost) Run(_ context.Context, spec canary.CommandSpec) (canary.Output, error) {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	d.calls = append(d.calls, spec)
	if slices.Contains(spec.Argv, "--version") {
		return canary.Output{Stdout: d.version + " (Claude Code)"}, nil
	}
	return canary.Output{}, nil
}

func (d *dockerHost) argvs() [][]string {
	d.mutex.Lock()
	defer d.mutex.Unlock()
	var all [][]string
	for _, call := range d.calls {
		all = append(all, call.Argv)
	}
	return all
}

// recordedSession stands for the session: it writes the recorded stream and,
// when asked, a record that holds the token.
type recordedSession struct {
	specs []canary.SessionSpec
	leak  bool
}

func (r *recordedSession) Run(spec canary.SessionSpec) (canary.SessionOutcome, error) {
	r.specs = append(r.specs, spec)
	for source, target := range map[string]string{record.StreamFile: spec.StdoutPath, record.TimesFile: spec.TimesPath} {
		content, err := os.ReadFile(filepath.Join(recording, source))
		if err != nil {
			return canary.SessionOutcome{}, err
		}
		if err := os.WriteFile(target, content, 0o644); err != nil {
			return canary.SessionOutcome{}, err
		}
	}
	stderr := []byte("session ended\n")
	if r.leak {
		stderr = []byte("env: " + launch.OAuthTokenVariable + "=" + testToken + "\n")
	}
	return canary.SessionOutcome{}, os.WriteFile(spec.StderrPath, stderr, 0o644)
}

type noLock struct{}

func (noLock) Acquire(string) (canary.Lock, error) { return noLock{}, nil }
func (noLock) Release()                            {}

type world struct {
	runner   app.Runner
	options  app.Options
	docker   *dockerHost
	sessions *recordedSession
	reads    *int
	stdout   *bytes.Buffer
}

func newWorld(t *testing.T, clock canary.Clock, present bool) world {
	t.Helper()
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(repo, "demo/loop/demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Caps = config.Caps{AssistantMessages: 12, Seconds: 7200, BudgetUSD: "1.00"}
	work, err := filepath.EvalSymlinks(mkdirTemp(t, "spdl-work-"))
	if err != nil {
		t.Fatal(err)
	}
	product := mkdirTemp(t, "spdl-product-")
	if err := os.MkdirAll(filepath.Join(product, plan.ProductClaudeDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(product, plan.ProductClaudeDir, plan.ClaudeBinary), []byte("linux claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	reads := 0
	w := world{docker: &dockerHost{version: testPin}, sessions: &recordedSession{}, reads: &reads, stdout: &bytes.Buffer{}}
	w.runner = app.Runner{
		Clock: clock, Window: window.Nightly(), Processes: w.docker, Sessions: w.sessions,
		Files: adapters.OSFiles{}, Locker: noLock{}, Credentials: []canary.ClaudeCredential{fakeCredential{present: present, reads: &reads}},
		Env: []string{"HOME=/Users/someone", "PATH=/usr/bin", "SSH_AUTH_SOCK=/private/tmp/agent.sock", launch.OAuthTokenVariable + "=" + testToken}, Stdout: w.stdout,
	}
	w.options = app.Options{
		RepoRoot: repo, Config: parsed, Work: work, Product: product, Image: testImage,
		Docker: []string{dockerFront}, UID: 501, GID: 20, ClaudePin: testPin,
	}
	return w
}

func joined(argv []string) string { return strings.Join(argv, " ") }

// mkdirTemp is a fresh directory whose path holds no character `--mount` refuses.
func mkdirTemp(t *testing.T, pattern string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", pattern)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestLiveRunInContainers(t *testing.T) {
	w := newWorld(t, &tickingClock{at: night, step: time.Second}, true)
	dirs, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 || len(w.sessions.specs) != 1 {
		t.Fatalf("dirs %v, sessions %d", dirs, len(w.sessions.specs))
	}
	spec := w.sessions.specs[0]
	box := container.Box{Run: filepath.Base(dirs[0])}

	t.Run("every step that touches the project runs in a container with no network, a scratch HOME and a fixed environment [verifies SP-2410~1]", func(t *testing.T) {
		steps := 0
		for _, argv := range w.docker.argvs() {
			if len(argv) < 2 || argv[0] != dockerFront || argv[1] != "run" || !slices.Contains(argv, "--rm") {
				continue
			}
			steps++
			line := joined(argv)
			for _, wanted := range []string{"--network none", "--read-only", "--cap-drop ALL", "--security-opt no-new-privileges", "--user 501:20", "--cpus 2", "--memory 2560m", "/usr/bin/env -i HOME=/home/demo", "--mount type=bind,source=" + w.options.Product + ",target=/opt/spec-proof,readonly"} {
				if !strings.Contains(line, wanted) {
					t.Errorf("step %q lacks %q", line, wanted)
				}
			}
			for _, leaked := range []string{"/Users/someone", "SSH_AUTH_SOCK", "docker.sock", testToken} {
				if strings.Contains(line, leaked) {
					t.Errorf("step %q holds %q", line, leaked)
				}
			}
		}
		if steps < 12 {
			t.Errorf("%d steps ran in containers", steps)
		}
		for _, call := range w.docker.calls {
			if call.Argv[0] != dockerFront && !strings.HasSuffix(call.Argv[0], plan.MemGuardRelative) {
				t.Errorf("%v ran on the host", call.Argv)
			}
		}
	})

	t.Run("the session runs on an internal network behind the egress proxy, its token on the entry's standard input only [verifies SP-2411~1]", func(t *testing.T) {
		line := joined(spec.Argv)
		for _, wanted := range []string{"--interactive", "--network " + box.NetworkName(), container.Runner() + " entry -- /opt/spec-proof/claude/claude -p", "HTTPS_PROXY=http://" + box.ProxyName() + ":3128", app.NonessentialTraffic, "--log-driver none", "--init"} {
			if !strings.Contains(line, wanted) {
				t.Errorf("the session lacks %q: %s", wanted, line)
			}
		}
		if strings.Contains(line, testToken) || slices.ContainsFunc(spec.Env, func(pair string) bool { return strings.Contains(pair, testToken) }) {
			t.Error("the token is on the session's command line or in the docker client's environment")
		}
		if string(spec.Stdin) != testToken+"\n" || *w.reads != 1 {
			t.Errorf("stdin %d bytes, %d reads", len(spec.Stdin), *w.reads)
		}
		calls := w.docker.argvs()
		order := []string{joined(box.NetworkArgv()), "--detach", joined(box.ConnectArgv()), joined(box.RemoveArgv()), joined(box.NetworkRemoveArgv())}
		at := 0
		for _, argv := range calls {
			if at < len(order) && strings.Contains(joined(argv), order[at]) {
				at++
			}
		}
		if at != len(order) {
			t.Errorf("the network, proxy and cleanup commands are not all there in order: %d of %d", at, len(order))
		}
		proxy := ""
		for _, argv := range calls {
			if slices.Contains(argv, "--detach") {
				proxy = joined(argv)
			}
		}
		if !strings.Contains(proxy, "egress-proxy --listen :3128 --log "+filepath.Join(dirs[0], plan.RecordsDir, record.EgressFile)+" --allow api.anthropic.com:443") {
			t.Errorf("proxy = %s", proxy)
		}
	})

	t.Run("the run records the image, the SHA-256 of the Linux CLI and the allowlist [verifies SP-2404~2]", func(t *testing.T) {
		content, err := os.ReadFile(filepath.Join(dirs[0], plan.RecordsDir, record.MetaFile))
		if err != nil {
			t.Fatal(err)
		}
		for _, wanted := range []string{`"image": "` + testImage + `"`, `"claudeSha256": "`, `"api.anthropic.com:443"`} {
			if !strings.Contains(string(content), wanted) {
				t.Errorf("run.json lacks %s:\n%s", wanted, content)
			}
		}
		if strings.Contains(string(content), testToken) {
			t.Error("run.json holds the token")
		}
	})
}

func TestTokenScan(t *testing.T) {
	t.Run("a run whose records hold the token fails, naming the file and not the token [verifies SP-2404~2]", func(t *testing.T) {
		w := newWorld(t, &tickingClock{at: night, step: time.Second}, true)
		w.sessions.leak = true
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrTokenInRecord) || !strings.Contains(err.Error(), record.StderrFile) || strings.Contains(err.Error(), testToken) {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLiveRefusals(t *testing.T) {
	t.Run("each session starts only if its time cap ends before the window closes [verifies SP-2403~2]", func(t *testing.T) {
		late := time.Date(2026, time.October, 26, 5, 0, 0, 0, time.Local)
		w := newWorld(t, &tickingClock{at: late, step: 20 * time.Minute}, true)
		w.options.Config.Repetitions = 3
		dirs, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrOutsideWindow) || len(w.sessions.specs) == 0 || len(w.sessions.specs) == 3 {
			t.Fatalf("error %v after %d sessions in %v", err, len(w.sessions.specs), dirs)
		}
	})

	t.Run("no credential, and a CLI that is not the pin, start no session [verifies SP-2403~2]", func(t *testing.T) {
		absent := newWorld(t, &tickingClock{at: night}, false)
		if _, err := absent.runner.Run(context.Background(), absent.options); !errors.Is(err, app.ErrNoCredential) || len(absent.docker.calls) != 0 {
			t.Errorf("no credential: %v, %d commands", err, len(absent.docker.calls))
		}
		skewed := newWorld(t, &tickingClock{at: noon}, true)
		skewed.options.ForceWindow = true
		skewed.docker.version = "2.1.999"
		if _, err := skewed.runner.Run(context.Background(), skewed.options); !errors.Is(err, app.ErrPinMismatch) || len(skewed.sessions.specs) != 0 {
			t.Errorf("skewed pin: %v", err)
		}
	})

	t.Run("a dry run prints the docker commands, the token as reaching standard input, and starts nothing [verifies SP-2403~2]", func(t *testing.T) {
		w := newWorld(t, &tickingClock{at: noon}, true)
		w.options.DryRun = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		printed := w.stdout.String()
		for _, wanted := range []string{
			"does not fit (--force-window needed)", "egress allowlist: api.anthropic.com:443",
			dockerFront + " network create --internal", dockerFront + " run --name 'spdl-proxy-",
			"(standard input: CLAUDE_CODE_OAUTH_TOKEN=<stdin>)", "--network none", "entry -- /opt/spec-proof/claude/claude -p",
		} {
			if !strings.Contains(printed, wanted) {
				t.Errorf("the plan lacks %q:\n%s", wanted, printed)
			}
		}
		if strings.Contains(printed, testToken) || len(w.docker.calls) != 0 || len(w.sessions.specs) != 0 || *w.reads != 0 {
			t.Errorf("the dry run read or started something: %d commands, %d sessions, %d reads", len(w.docker.calls), len(w.sessions.specs), *w.reads)
		}
	})
}
