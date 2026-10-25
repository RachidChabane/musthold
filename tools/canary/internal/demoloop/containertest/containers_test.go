// Package containertest runs the containers of a live run on the demonstration's
// engine and checks what they isolate: the scratch HOME, the absent SSH agent and
// docker socket, the egress that only the model endpoints pass, the token that no
// engine record, run record or log holds, the box office loop, blocked then
// passed, inside a container with the simulated Claude Code, and whole runs of the
// pinned Claude Code against a stand-in for the model API. The front door
// (demo/loop/run test-containers) builds the Linux product and the image, starts
// the engine, sets SPEC_PROOF_DEMO_CONTAINERS=1 and stops the engine after; without
// that variable every test skips.
package containertest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	canary "github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/container"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/egress"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/probe"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/window"
)

const (
	enableVariable = "SPEC_PROOF_DEMO_CONTAINERS"
	repoFromHere   = "../../../../.."
	dockerFront    = "demo/loop/container/docker"
	engineProfile  = "spec-proof-loop"
	// fakeToken has a token's shape, so the scans would find it anywhere.
	fakeToken     = "sk-ant-oat01-container-test-token-0123456789abcdef"
	deniedHost    = "example.com:443"
	directAddress = "1.1.1.1:443"
	runTimeout    = 20 * time.Minute
	// liveCap and liveSeconds bound each run against the stand-in.
	liveCap     = 3
	liveSeconds = 600
)

type world struct {
	repo, docker, product, image, work string
}

// setup readies the engine's world for a test whose checks are named. Off the
// engine it reports each check as skipped, so the results still name what was not
// checked, and returns false.
func setup(t *testing.T, checks ...string) (world, bool) {
	t.Helper()
	if os.Getenv(enableVariable) != "1" {
		for _, check := range checks {
			t.Run(check, func(t *testing.T) {
				t.Skipf("set %s=1 (demo/loop/run test-containers) to run the container tests", enableVariable)
			})
		}
		return world{}, false
	}
	repo, err := filepath.Abs(repoFromHere)
	if err != nil {
		t.Fatal(err)
	}
	w := world{repo: repo, docker: filepath.Join(repo, dockerFront), product: filepath.Join(repo, app.ProductRelative)}
	image, err := os.ReadFile(filepath.Join(w.product, app.ImageFile))
	if err != nil {
		t.Fatalf("no image: %v (run demo/loop/run build)", err)
	}
	w.image = strings.TrimSpace(string(image))
	base := filepath.Join(repo, app.WorkRelative)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	work, err := os.MkdirTemp(base, "containertest-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(work) })
	if w.work, err = filepath.EvalSymlinks(work); err != nil {
		t.Fatal(err)
	}
	if w.product, err = filepath.EvalSymlinks(w.product); err != nil {
		t.Fatal(err)
	}
	return w, true
}

func (w world) box(t *testing.T, name string) container.Box {
	t.Helper()
	dir := filepath.Join(w.work, name)
	for _, sub := range []string{plan.RecordsDir, plan.TmpDir} {
		if err := os.MkdirAll(filepath.Join(dir, sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return container.Box{Image: w.image, RunDir: dir, Product: w.product, UID: os.Getuid(), GID: os.Getgid(), Run: name}
}

// run runs docker against the engine and returns its output and exit code.
func (w world) run(t *testing.T, stdin string, argv ...string) (string, string, int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), runTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, w.docker, argv...)
	command.Dir = w.repo
	if stdin != "" {
		command.Stdin = strings.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	command.Stdout, command.Stderr = &stdout, &stderr
	err := command.Run()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("docker %v: %v", argv, err)
	}
	return stdout.String(), stderr.String(), code
}

// network starts the run's internal network and its egress proxy, and removes
// both when the test ends.
func (w world) network(t *testing.T, box container.Box) {
	t.Helper()
	t.Cleanup(func() {
		w.run(t, "", box.RemoveArgv()...)
		w.run(t, "", box.NetworkRemoveArgv()...)
	})
	for _, argv := range [][]string{box.NetworkArgv(), box.ProxyArgv(filepath.Join(box.RunDir, plan.RecordsDir, record.EgressFile)), box.ConnectArgv()} {
		if _, stderr, code := w.run(t, "", argv...); code != 0 {
			t.Fatalf("docker %v exited %d: %s", argv[:2], code, stderr)
		}
	}
}

func sessionVariables(box container.Box) []string {
	return append([]string{"HOME=" + container.ScratchHome, "PATH=" + container.ProductTarget + "/bin:" + container.SystemPath}, container.ProxyVariables()...)
}

// egressEntries reads the proxy's log in the records directory and prints it in
// the test's output.
func egressEntries(t *testing.T, records string) []egress.Entry {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(records, record.EgressFile))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("%s:\n%s", record.EgressFile, content)
	var entries []egress.Entry
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var entry egress.Entry
		if json.Unmarshal([]byte(line), &entry) == nil {
			entries = append(entries, entry)
		}
	}
	return entries
}

func TestEngine(t *testing.T) {
	const check = "every docker command reaches the demonstration's own engine [verifies SP-2415~1]"
	w, ok := setup(t, check)
	if !ok {
		return
	}
	t.Run(check, func(t *testing.T) {
		name, stderr, code := w.run(t, "", "info", "--format", "{{.Name}}")
		if code != 0 || !strings.Contains(name, engineProfile) {
			t.Fatalf("engine %q (exit %d): %s", name, code, stderr)
		}
	})
}

func TestSessionIsolation(t *testing.T) {
	const check1 = "HOME is an empty scratch directory and no SSH agent, docker socket or other socket is there [verifies SP-2415~1]"
	const check2 = "no host answers directly, and the proxy forwards to the model endpoints only, logging each request [verifies SP-2415~1]"
	w, ok := setup(t, check1, check2)
	if !ok {
		return
	}
	box := w.box(t, "isolation")
	w.network(t, box)
	allowed := container.ModelEndpoints()[0]
	argv := []string{container.Runner(), container.ProbeSubcommand, "--direct", directAddress, "--direct", allowed, "--via-proxy", deniedHost, "--via-proxy", allowed}
	stdout, stderr, code := w.run(t, fakeToken+"\n", box.SessionArgv(box.RunDir, sessionVariables(box), argv)...)
	if code != 0 {
		t.Fatalf("the probe exited %d: %s", code, stderr)
	}
	var report probe.Report
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &report); err != nil {
		t.Fatalf("probe printed %q: %v", stdout, err)
	}

	t.Run(check1, func(t *testing.T) {
		if report.Home != container.ScratchHome || len(report.HomeEntries) != 0 {
			t.Errorf("HOME %s holds %v", report.Home, report.HomeEntries)
		}
		if report.SSHAuthSock != "" || report.DockerSocket || len(report.Sockets) != 0 {
			t.Errorf("SSH_AUTH_SOCK %q, docker socket %t, sockets %v", report.SSHAuthSock, report.DockerSocket, report.Sockets)
		}
	})

	t.Run(check2, func(t *testing.T) {
		for target, reached := range report.Direct {
			if reached {
				t.Errorf("%s answered without the proxy", target)
			}
		}
		if report.ViaProxy[deniedHost] || !report.ViaProxy[allowed] {
			t.Errorf("through the proxy: %v", report.ViaProxy)
		}
		decisions := map[string]string{}
		for _, entry := range egressEntries(t, filepath.Join(box.RunDir, plan.RecordsDir)) {
			decisions[entry.Target] = entry.Decision
		}
		if decisions[deniedHost] != egress.DecisionDenied || decisions[allowed] != egress.DecisionAllowed {
			t.Errorf("egress log: %v", decisions)
		}
	})
}

func TestTokenStaysOutOfTheEngine(t *testing.T) {
	const check = "the session's command gets the token while docker inspect, the engine's logs and the records never hold it [verifies SP-2416~1]"
	w, ok := setup(t, check)
	if !ok {
		return
	}
	box := w.box(t, "token")
	w.network(t, box)
	script := `test -n "$` + launch.OAuthTokenVariable + `" && echo token-delivered; sleep 8`
	done := make(chan string, 1)
	go func() {
		stdout, _, _ := w.run(t, fakeToken+"\n", box.SessionArgv(box.RunDir, sessionVariables(box), []string{"/bin/sh", "-c", script})...)
		done <- stdout
	}()
	var inspected string
	for range 40 {
		time.Sleep(250 * time.Millisecond)
		if stdout, _, code := w.run(t, "", container.InspectArgv(box.SessionName())...); code == 0 {
			inspected = stdout
			break
		}
	}
	output := <-done
	logs, _, _ := w.run(t, "", "logs", box.SessionName())

	t.Run(check, func(t *testing.T) {
		if !strings.Contains(output, "token-delivered") {
			t.Errorf("the command did not get the token: %q", output)
		}
		if inspected == "" {
			t.Fatal("the running container was never inspected")
		}
		for name, content := range map[string]string{"docker inspect": inspected, "docker logs": logs} {
			if strings.Contains(content, fakeToken) {
				t.Errorf("%s holds the token", name)
			}
		}
		files := map[string][]byte{}
		_ = filepath.WalkDir(box.RunDir, func(path string, entry os.DirEntry, err error) error {
			if err == nil && !entry.IsDir() {
				content, _ := os.ReadFile(path)
				files[path] = content
			}
			return nil
		})
		if hits := record.TokenHits(files, fakeToken); len(hits) > 0 {
			t.Errorf("files hold the token: %v", hits)
		}
	})
}

func TestBoxOfficeLoopInAContainer(t *testing.T) {
	const check = "the gate blocks on the weak test, names the input it misses, and passes once it is strengthened, all offline in a container [verifies SP-2417~1]"
	w, ok := setup(t, check)
	if !ok {
		return
	}
	box := w.box(t, "boxoffice")
	gate := container.ProductTarget + "/tests/tools/canary/internal/demoloop/fixturegate"
	variables := []string{
		"HOME=" + container.ScratchHome, "PATH=" + container.ProductTarget + "/bin:" + container.SystemPath,
		"TMPDIR=" + filepath.Join(box.RunDir, plan.TmpDir), "SPEC_PROOF_DEMO_PRODUCT=" + container.ProductTarget, "PYTHONDONTWRITEBYTECODE=1",
	}
	argv := []string{container.ProductTarget + "/bin/fixturegate.test", "-test.run", "^TestFixtureGate$", "-test.v", "-test.count=1"}
	stdout, stderr, code := w.run(t, "", box.StepArgv(1, gate, variables, argv)...)

	t.Run(check, func(t *testing.T) {
		if code != 0 || !strings.Contains(stdout, "--- PASS: TestFixtureGate") {
			t.Fatalf("exit %d:\n%s\n%s", code, stdout, stderr)
		}
		if !strings.Contains(stdout, "ticket_price(age=65) => 8") {
			t.Errorf("the feedback is not in the output:\n%s", stdout)
		}
	})
}

// liveRun runs the committed configuration once with the pinned Claude Code
// against a stand-in for the model API, reachable or not, and without the plugin
// trees named, and returns the run's directory, the stand-in and the error.
func (w world) liveRun(t *testing.T, name string, reachable bool, without []string) (string, standIn, error) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(w.repo, "demo/loop/demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := config.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Caps = config.Caps{AssistantMessages: liveCap, Seconds: liveSeconds, BudgetUSD: parsed.Caps.BudgetUSD}
	pin, err := os.ReadFile(filepath.Join(w.repo, plan.ClaudePinRelative))
	if err != nil {
		t.Fatal(err)
	}
	stand := w.startStandIn(t, name)
	var stdout bytes.Buffer
	runner := app.Runner{
		Clock: realClock{}, Window: window.Nightly(),
		Processes: standInProcesses{inner: adapters.Exec{}, standIn: stand, reachable: reachable, w: w, t: t},
		Sessions:  standInSessions{inner: adapters.Sessions{}, standIn: stand, without: without},
		Files:     adapters.OSFiles{}, Locker: adapters.FileLocker{},
		Credentials: []canary.ClaudeCredential{adapters.EnvironmentCredential{Environ: []string{launch.OAuthTokenVariable + "=" + fakeToken}}},
		Env:         launch.WithoutOAuthToken(os.Environ()), Stdout: &stdout,
	}
	options := app.Options{
		RepoRoot: w.repo, Config: parsed, Work: w.work, Product: w.product, Image: w.image, Docker: []string{w.docker},
		UID: os.Getuid(), GID: os.Getgid(), ClaudePin: strings.TrimSpace(string(pin)), ForceWindow: true,
	}
	dirs, err := runner.Run(context.Background(), options)
	if len(dirs) != 1 {
		t.Fatalf("run: %v\n%s", err, stdout.String())
	}
	return dirs[0], stand, err
}

// streamEvents reads the session's stream.
func streamEvents(t *testing.T, records string) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(records, record.StreamFile))
	if err != nil {
		t.Fatal(err)
	}
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(string(content)), "\n") {
		var event map[string]any
		if json.Unmarshal([]byte(line), &event) == nil {
			events = append(events, event)
		}
	}
	return events
}

// gateBlockExit is the exit code of Shallnot's end-of-turn hook that holds the turn.
const gateBlockExit = 2

// gateBlocks counts the calls of Shallnot's end-of-turn hook the shims recorded
// that held the turn.
func gateBlocks(t *testing.T, records string) int {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(records, record.HooksFile))
	if err != nil {
		t.Fatal(err)
	}
	blocks := 0
	for _, line := range strings.Split(string(content), "\n") {
		if strings.Contains(line, `"args":["hook","claude-stop"]`) && strings.Contains(line, fmt.Sprintf(`"exitCode":%d`, gateBlockExit)) {
			blocks++
		}
	}
	return blocks
}

func TestClaudeCodeInContainers(t *testing.T) {
	const check1 = "the pinned Claude Code reaches the model API through the egress proxy by its alias, with the token the entry gave it, and the run leaves no container behind [verifies SP-2420~1]"
	const check2 = "every plugin the run gives the session loads, and Shallnot's end-of-turn gate runs once per end of turn [verifies SP-2420~1]"
	const check3 = "a session whose umbrella plugin lacks a dependency fails the run at its init event, naming the dependency [verifies SP-2419~1]"
	const check4 = "a session whose model API is out of reach fails the run at its first retry [verifies SP-2419~1]"
	w, ok := setup(t, check1, check2, check3, check4)
	if !ok {
		return
	}
	dir, stand, err := w.liveRun(t, "reach", true, nil)
	records := filepath.Join(dir, plan.RecordsDir)
	events := streamEvents(t, records)

	t.Run(check1, func(t *testing.T) {
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		allowed := 0
		for _, entry := range egressEntries(t, records) {
			if entry.Decision == egress.DecisionAllowed {
				allowed++
				if entry.Target != standInEndpoint {
					t.Errorf("the proxy let %s out", entry.Target)
				}
			}
		}
		requests := stand.requests(t)
		t.Logf("the stand-in served %+v", requests)
		messages := 0
		for _, r := range requests {
			if strings.HasPrefix(r.Path, "/v1/messages") && r.Method == "POST" {
				messages++
			}
			if !r.BearerIsToken {
				t.Errorf("%s %s did not carry the token as its bearer", r.Method, r.Path)
			}
		}
		if allowed == 0 || messages == 0 {
			t.Errorf("%d tunnels and %d Messages requests reached the stand-in", allowed, messages)
		}
		meta, err := os.ReadFile(filepath.Join(records, record.MetaFile))
		if err != nil || !strings.Contains(string(meta), `"claudeSha256": "`) || !strings.Contains(string(meta), `"image": "`+w.image) {
			t.Errorf("run.json: %v\n%s", err, meta)
		}
		listed, _, _ := w.run(t, "", "ps", "--all", "--quiet", "--filter", "name=spdl-")
		if strings.TrimSpace(listed) != "" {
			t.Errorf("containers are left: %s", listed)
		}
	})

	t.Run(check2, func(t *testing.T) {
		var loaded []string
		// blocks counts the Stop hooks that held an end of turn, the gate's block;
		// a gate registered twice would hold one end of turn twice.
		blocks, held := 0, 0
		for _, event := range events {
			switch {
			case event["subtype"] == "init":
				for _, plugin := range event["plugins"].([]any) {
					loaded = append(loaded, plugin.(map[string]any)["name"].(string))
				}
				if failed, _ := event["plugin_errors"].([]any); len(failed) != 0 {
					t.Errorf("plugin errors: %v", failed)
				}
			case event["type"] == "assistant":
				held = 0
			case event["subtype"] == "hook_response" && event["hook_event"] == "Stop" && event["exit_code"] == float64(gateBlockExit):
				blocks, held = blocks+1, held+1
				if held > 1 {
					t.Errorf("one end of turn was held %d times", held)
				}
			}
		}
		for _, name := range plan.PluginNames(config.ArmFourProofs) {
			if !slices.Contains(loaded, name) {
				t.Errorf("plugin %s is not loaded: %v", name, loaded)
			}
		}
		if calls := gateBlocks(t, records); blocks == 0 || calls != blocks {
			t.Errorf("the gate blocked %d times through the shim, and %d ends of turn were held", calls, blocks)
		}
		t.Logf("%d ends of turn held, each once, by the gate", blocks)
	})

	t.Run(check3, func(t *testing.T) {
		_, _, err := w.liveRun(t, "plugins", true, []string{container.ProductTarget + "/" + plan.ProductPluginsDir + "/" + plan.ProductShallnotPlugin})
		if !errors.Is(err, app.ErrSessionStart) || !strings.Contains(err.Error(), "plugins-not-loaded") || !strings.Contains(err.Error(), plan.ShallnotPlugin) {
			t.Errorf("run: %v", err)
		}
	})

	t.Run(check4, func(t *testing.T) {
		dir, _, err := w.liveRun(t, "unreachable", false, nil)
		if !errors.Is(err, app.ErrSessionStart) || !strings.Contains(err.Error(), "api-unreachable") {
			t.Fatalf("run: %v", err)
		}
		retries := 0
		for _, event := range streamEvents(t, filepath.Join(dir, plan.RecordsDir)) {
			if event["subtype"] == "api_retry" {
				retries++
			}
		}
		if retries != 1 {
			t.Errorf("the session retried %d times", retries)
		}
	})
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }
