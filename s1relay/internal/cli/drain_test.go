package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/s1relay/internal/adapters/memguard"
	"github.com/RachidChabane/musthold/s1relay/internal/app/ports"
	"github.com/RachidChabane/musthold/s1relay/internal/cli"
	"github.com/RachidChabane/musthold/s1relay/internal/domain/endpoint"
	"github.com/RachidChabane/musthold/s1relay/internal/testkit/stubendpoint"
)

const (
	recordedModel = "tev1:0.8b"
	exportTwo     = `[
	  {"ticket":"aa.0.1","nonce":"n1","closed":{"state":"the sky is green","questions":[{"id":"color","type":"choice","instructions":"Which color does the text say the sky is?","options":[{"name":"blue","description":"blue"},{"name":"green","description":"green"},{"name":"grey","description":"grey"}]}]}},
	  {"ticket":"bb.0.1","nonce":"n2","closed":{"state":"another state","questions":[{"id":"q1","type":"choice","instructions":"Does the change break requirement 1?","options":[{"name":"VIOLATES","description":"The change breaks the requirement."},{"name":"CONSISTENT","description":"The requirement is still met."}]}]}}
	]`
)

type runnerCall struct {
	dir   string
	name  string
	args  string
	stdin string
}

// scriptedRunner plays a proof module: export prints a fixed list, import records
// its input and succeeds.
type scriptedRunner struct {
	export string
	calls  []runnerCall
	failOn string
}

func (s *scriptedRunner) Run(_ context.Context, dir, name string, args []string, stdin []byte) ([]byte, []byte, error) {
	joined := strings.Join(args, " ")
	s.calls = append(s.calls, runnerCall{dir: dir, name: name, args: joined, stdin: string(stdin)})
	if s.failOn != "" && strings.Contains(joined, s.failOn) {
		return nil, []byte("the module refused"), errors.New("exit status 1")
	}
	if strings.HasPrefix(joined, "systemone export") {
		return []byte(s.export), nil, nil
	}
	return []byte("accepted\n"), nil, nil
}

func (s *scriptedRunner) imports() []runnerCall {
	var imports []runnerCall
	for _, call := range s.calls {
		if strings.HasPrefix(call.args, "systemone import") {
			imports = append(imports, call)
		}
	}
	return imports
}

type fixedGuard struct{ verdict ports.MemoryVerdict }

func (g fixedGuard) Check(context.Context) (ports.MemoryVerdict, error) { return g.verdict, nil }

type harness struct {
	t         *testing.T
	root      string
	runner    *scriptedRunner
	environ   map[string]string
	guard     ports.MemoryGuard
	guardMins []int
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	return &harness{
		t: t, root: t.TempDir(), runner: &scriptedRunner{export: exportTwo},
		environ: map[string]string{}, guard: fixedGuard{verdict: ports.MemoryVerdict{Allowed: true}},
	}
}

func (h *harness) writeConfig(content string) {
	h.t.Helper()
	dir := filepath.Join(h.root, ".musthold")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

type result struct {
	code           int
	stdout, stderr string
}

func (r result) report(t *testing.T) map[string]any {
	t.Helper()
	var decoded map[string]any
	if err := json.Unmarshal([]byte(r.stdout), &decoded); err != nil {
		t.Fatalf("stdout %q is not a JSON report: %v", r.stdout, err)
	}
	return decoded
}

func (h *harness) run(args ...string) result {
	h.t.Helper()
	var stdout, stderr bytes.Buffer
	env := cli.Env{
		Binary: "musthold-s1", Stdout: &stdout, Stderr: &stderr, WorkDir: h.root, Runner: h.runner,
		LookupEnv: func(name string) (string, bool) { value, found := h.environ[name]; return value, found },
		NewGuard: func(minFree int) ports.MemoryGuard {
			h.guardMins = append(h.guardMins, minFree)
			return h.guard
		},
	}
	code := cli.Run(env, append([]string{"drain"}, args...))
	return result{code: code, stdout: stdout.String(), stderr: stderr.String()}
}

func TestDrainCommand(t *testing.T) {
	t.Run("the closed requests of the module are posted to Ollama one at a time and imported under the model and endpoint digest [verifies SP-600~2]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL, "--model", recordedModel)
		if got.code != cli.ExitClean {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		parsed, _ := endpoint.Parse(stub.URL)
		source := "system-one:" + recordedModel + "@" + parsed.Digest()
		if len(stub.Requests()) != 2 || stub.MaxInFlight() != 1 {
			t.Fatalf("%d requests, %d in flight at most", len(stub.Requests()), stub.MaxInFlight())
		}
		if string(stub.Requests()[0]) != stubendpoint.RecordedOllamaChoiceRequest {
			t.Errorf("first request %s", stub.Requests()[0])
		}
		imports := h.runner.imports()
		if len(imports) != 2 || imports[0].name != "musthold-dp" || imports[0].dir != h.root {
			t.Fatalf("imports %+v", imports)
		}
		if imports[0].args != "systemone import aa.0.1 --nonce n1 --source "+source || imports[1].args != "systemone import bb.0.1 --nonce n2 --source "+source {
			t.Errorf("import arguments %q, %q", imports[0].args, imports[1].args)
		}
		var reply struct {
			Answers map[string]struct {
				Choice     string  `json:"choice"`
				Confidence float64 `json:"confidence"`
			} `json:"answers"`
		}
		if err := json.Unmarshal([]byte(imports[1].stdin), &reply); err != nil || reply.Answers["q1"].Choice != "CONSISTENT" || reply.Answers["q1"].Confidence != 0.9674686077174837 {
			t.Errorf("the module did not get the endpoint's reply: %s (%v)", imports[1].stdin, err)
		}
	})

	t.Run("--session relays the closed requests of that session only, and a text that names no session runs nothing [verifies SP-2156~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		h := newHarness(t)
		if got := h.run("--module", "dp", "--endpoint", stub.URL, "--session", "prove"); got.code != cli.ExitClean {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		if h.runner.calls[0].args != "systemone export --session prove" {
			t.Fatalf("the export ran %q", h.runner.calls[0].args)
		}
		refused := newHarness(t)
		if got := refused.run("--module", "dp", "--endpoint", stub.URL, "--session", "scan"); got.code != cli.ExitToolFailure || len(refused.runner.calls) != 0 {
			t.Fatalf("exit %d with %d module runs, stderr %s", got.code, len(refused.runner.calls), got.stderr)
		}
	})

	t.Run("the report names the digest, never the address with its credentials, and counts the outcomes [verifies SP-613~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		address, _ := url.Parse(stub.URL)
		address.User = url.UserPassword("operator", "hunter2")
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", address.String())
		report := got.report(t)
		if strings.Contains(got.stdout+got.stderr, "hunter2") {
			t.Fatalf("a credential leaked: %s%s", got.stdout, got.stderr)
		}
		plain, _ := endpoint.Parse(stub.URL)
		if report["endpointDigest"] != plain.Digest() || report["answered"] != float64(2) || report["unanswered"] != float64(0) || report["module"] != "dp" {
			t.Fatalf("report %v", report)
		}
		items := report["items"].([]any)
		if len(items) != 2 || items[0].(map[string]any)["status"] != "answered" || !strings.HasPrefix(items[0].(map[string]any)["source"].(string), "system-one:unknown@") {
			t.Fatalf("items %v", items)
		}
	})

	t.Run("an unreadable reply leaves the request and exits 1 with the kind in the report [verifies SP-613~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, func(index int, body []byte) stubendpoint.Reply {
			if index == 0 {
				return stubendpoint.Reply{Status: http.StatusBadRequest, Body: `{"error":"too large"}`}
			}
			return stubendpoint.Reply{Status: http.StatusOK, Body: `{"model":"kev-latest","answers":{"q1":{"type":"choice","choice":"VIOLATES","confidence":0.8}},"latency_ms":29.9}`}
		})
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL)
		if got.code != cli.ExitUnanswered {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		report := got.report(t)
		first := report["items"].([]any)[0].(map[string]any)
		if first["kind"] != "rejected" || !strings.Contains(first["detail"].(string), "too large") || report["unanswered"] != float64(1) {
			t.Fatalf("report %v", report)
		}
	})
}

func TestDrainConfiguration(t *testing.T) {
	t.Run("the endpoint and the model come from the project configuration [verifies SP-601~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		h.writeConfig("systemone:\n  endpoint: " + stub.URL + "\n  model: kev-latest\n")
		if got := h.run("--module", "alignment"); got.code != cli.ExitClean {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		if body := string(stub.Requests()[0]); !strings.Contains(body, `"model":"kev-latest"`) {
			t.Errorf("request %s", body)
		}
		if imports := h.runner.imports(); !strings.Contains(imports[0].args, "--source system-one:kev-latest@") {
			t.Errorf("import %s", imports[0].args)
		}
	})

	t.Run("the flags win over the configuration [verifies SP-601~1]", func(t *testing.T) {
		configured := stubendpoint.New(stubendpoint.Kev, nil)
		defer configured.Close()
		flagged := stubendpoint.New(stubendpoint.Kev, nil)
		defer flagged.Close()
		h := newHarness(t)
		h.writeConfig("systemone:\n  endpoint: " + configured.URL + "\n  model: from-config\n")
		if got := h.run("--module", "dp", "--endpoint", flagged.URL, "--model", "from-flag"); got.code != cli.ExitClean {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		if len(configured.Requests()) != 0 || len(flagged.Requests()) != 2 || !strings.Contains(string(flagged.Requests()[0]), `"model":"from-flag"`) {
			t.Fatalf("configured got %d, flagged got %d requests", len(configured.Requests()), len(flagged.Requests()))
		}
	})

	t.Run("--root names the project whose configuration and module are used [verifies SP-601~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		other := newHarness(t)
		other.writeConfig("systemone:\n  endpoint: " + stub.URL + "\n")
		if got := h.run("--module", "dp", "--root", other.root); got.code != cli.ExitClean {
			t.Fatalf("exit %d, stderr %s", got.code, got.stderr)
		}
		if h.runner.calls[0].dir != other.root {
			t.Errorf("the module ran in %s", h.runner.calls[0].dir)
		}
	})

	t.Run("without a usable endpoint the command fails without running a module or a connection [verifies SP-601~1]", func(t *testing.T) {
		for name, args := range map[string][]string{
			"no endpoint":      {"--module", "dp"},
			"a bad endpoint":   {"--module", "dp", "--endpoint", "ftp://example.test"},
			"no module":        {"--endpoint", "http://localhost:1"},
			"a bad module":     {"--module", "../dp", "--endpoint", "http://localhost:1"},
			"a stray argument": {"--module", "dp", "--endpoint", "http://localhost:1", "extra"},
			"an unknown flag":  {"--module", "dp", "--bogus"},
			"a negative limit": {"--module", "dp", "--endpoint", "http://localhost:1", "--max-requests", "-1"},
			"a zero timeout":   {"--module", "dp", "--endpoint", "http://localhost:1", "--timeout", "0s"},
		} {
			h := newHarness(t)
			got := h.run(args...)
			if got.code != cli.ExitToolFailure || got.stderr == "" || got.stdout != "" || len(h.runner.calls) != 0 {
				t.Errorf("%s: exit %d, stderr %q, %d module calls", name, got.code, got.stderr, len(h.runner.calls))
			}
		}
	})

	t.Run("a broken configuration file fails the command [verifies SP-601~1]", func(t *testing.T) {
		h := newHarness(t)
		h.writeConfig("systemone: [unclosed")
		if got := h.run("--module", "dp"); got.code != cli.ExitToolFailure {
			t.Fatalf("exit %d", got.code)
		}
	})

	t.Run("a module that cannot export fails the command [verifies SP-602~1]", func(t *testing.T) {
		h := newHarness(t)
		h.runner.failOn = "systemone export"
		if got := h.run("--module", "dp", "--endpoint", "http://localhost:1"); got.code != cli.ExitToolFailure || !strings.Contains(got.stderr, "the module refused") {
			t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
		}
	})
}

func TestDrainImportRefusal(t *testing.T) {
	t.Run("an import the module refuses is reported with its reason and exits 1 [verifies SP-614~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		h.runner.failOn = "import aa.0.1"
		got := h.run("--module", "dp", "--endpoint", stub.URL)
		if got.code != cli.ExitUnanswered {
			t.Fatalf("exit %d", got.code)
		}
		first := got.report(t)["items"].([]any)[0].(map[string]any)
		if first["kind"] != "import" || !strings.Contains(first["detail"].(string), "the module refused") || len(h.runner.imports()) != 2 {
			t.Fatalf("first item %v, %d imports attempted", first, len(h.runner.imports()))
		}
	})
}

func TestDrainMaxRequests(t *testing.T) {
	t.Run("--max-requests posts at most that many requests and reports the rest [verifies SP-610~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL, "--max-requests", "1")
		report := got.report(t)
		if got.code != cli.ExitClean || len(stub.Requests()) != 1 || report["remaining"] != float64(1) || report["answered"] != float64(1) {
			t.Fatalf("exit %d, %d requests, report %v", got.code, len(stub.Requests()), report)
		}
	})
}

func TestDrainUnload(t *testing.T) {
	t.Run("an Ollama endpoint is unloaded after the batch [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL, "--model", recordedModel)
		unloads := stub.Unloads()
		if len(unloads) != 1 || string(unloads[0]) != `{"model":"tev1:0.8b","keep_alive":0}` {
			t.Fatalf("unloads %q", unloads)
		}
		if unload := got.report(t)["unload"].(map[string]any); unload["attempted"] != true || unload["done"] != true {
			t.Fatalf("unload %v", unload)
		}
	})

	t.Run("--no-unload keeps the model loaded [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		defer stub.Close()
		h := newHarness(t)
		h.run("--module", "dp", "--endpoint", stub.URL, "--no-unload")
		if len(stub.Unloads()) != 0 {
			t.Fatalf("unloads %q", stub.Unloads())
		}
	})

	t.Run("a Kev endpoint is left alone and the batch still succeeds [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL)
		unload := got.report(t)["unload"].(map[string]any)
		if got.code != cli.ExitClean || unload["done"] != false || unload["detail"] == "" {
			t.Fatalf("exit %d, unload %v", got.code, unload)
		}
	})

	t.Run("an unload that fails does not fail the batch [verifies SP-611~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Ollama, nil)
		stub.UnloadStatus = http.StatusInternalServerError
		defer stub.Close()
		h := newHarness(t)
		got := h.run("--module", "dp", "--endpoint", stub.URL, "--model", recordedModel)
		unload := got.report(t)["unload"].(map[string]any)
		if unload["done"] != false || !strings.Contains(unload["detail"].(string), "HTTP 500") {
			t.Fatalf("unload %v", unload)
		}
		if got.code != cli.ExitClean {
			t.Fatalf("exit %d", got.code)
		}
	})
}

func TestDrainMemoryRule(t *testing.T) {
	refusal := fixedGuard{verdict: ports.MemoryVerdict{Reason: "12% of memory is free, the rule needs 40%"}}

	t.Run("a refused batch exits 4, prints no report and touches neither the module nor the endpoint [verifies SP-612~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		h.guard = refusal
		got := h.run("--module", "dp", "--endpoint", stub.URL, "--mem-guard")
		if got.code != cli.ExitMemoryRefused || got.stdout != "" || !strings.Contains(got.stderr, "12%") || len(h.runner.calls) != 0 || len(stub.Requests()) != 0 {
			t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
		}
	})

	t.Run("without --mem-guard the rule does not stop the batch [verifies SP-612~1]", func(t *testing.T) {
		stub := stubendpoint.New(stubendpoint.Kev, nil)
		defer stub.Close()
		h := newHarness(t)
		h.guard = refusal
		if got := h.run("--module", "dp", "--endpoint", stub.URL); got.code != cli.ExitClean {
			t.Fatalf("exit %d", got.code)
		}
	})

	t.Run("the threshold is 40 percent unless MEM_GUARD_MIN_FREE_PCT holds a number [verifies SP-612~1]", func(t *testing.T) {
		for value, want := range map[string]int{"": memguard.DefaultMinFreePercent, "55": 55, "abc": memguard.DefaultMinFreePercent, "-3": memguard.DefaultMinFreePercent} {
			h := newHarness(t)
			if value != "" {
				h.environ[memguard.MinFreeVariable] = value
			}
			h.run("--module", "dp", "--endpoint", "http://127.0.0.1:1", "--mem-guard")
			if len(h.guardMins) != 1 || h.guardMins[0] != want {
				t.Errorf("MEM_GUARD_MIN_FREE_PCT=%q: threshold %v, want %d", value, h.guardMins, want)
			}
		}
	})
}
