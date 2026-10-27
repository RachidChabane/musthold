package canary_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
	"github.com/RachidChabane/musthold/tools/canary/internal/adapters"
	ports "github.com/RachidChabane/musthold/tools/canary/internal/app"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/egress"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/record"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box/docker/dockertest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/canary"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/runs/runstest"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

func entries(lines ...string) []egress.Entry {
	parsed, err := canary.LogEntries([]byte(strings.Join(lines, "\n")))
	if err != nil {
		panic(err)
	}
	return parsed
}

func TestEgressHosts(t *testing.T) {
	log := entries(
		runstest.ProxyEntry("api.anthropic.com:443", egress.DecisionAllowed),
		runstest.ProxyEntry("statsig.anthropic.com:443", egress.DecisionAllowed),
		runstest.ProxyEntry("api.anthropic.com:443", egress.DecisionAllowed),
		runstest.ProxyEntry("claude.ai:443", egress.DecisionFailed),
		runstest.ProxyEntry("evilanthropic.com:443", egress.DecisionAllowed),
		runstest.ProxyEntry("example.com:443", egress.DecisionDenied),
	)

	t.Run("the observed hosts are every host C1 contacted, once each, and the allowlist keeps those of the model's domains [verifies SP-2535~1]", func(t *testing.T) {
		observed := canary.Observed(log)
		if !slices.Equal(observed, []string{"api.anthropic.com", "claude.ai", "evilanthropic.com", "statsig.anthropic.com"}) {
			t.Errorf("observed %v", observed)
		}
		if got := canary.Allowlist(observed); !slices.Equal(got, []string{"api.anthropic.com", "claude.ai", "statsig.anthropic.com"}) {
			t.Errorf("allowlist %v", got)
		}
	})

	t.Run("a host is in a domain only when it is the domain or one of its subdomains [verifies SP-2535~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			label := rapid.StringMatching(`[a-z0-9-]{1,12}`).Draw(t, "label")
			domain := rapid.SampledFrom(canary.ModelDomains()).Draw(t, "domain")
			if !canary.InDomains(label+"."+domain, canary.ModelDomains()) || !canary.InDomains(domain, canary.ModelDomains()) {
				t.Fatalf("%s.%s is not in its domain", label, domain)
			}
			if canary.InDomains(label+domain, canary.ModelDomains()) || canary.InDomains(domain+"."+label, canary.ModelDomains()) {
				t.Fatalf("a look-alike of %s is in it", domain)
			}
		})
	})

	t.Run("C2 names each host the proxy refused that C1 did not observe [verifies SP-2536~1]", func(t *testing.T) {
		refused := entries(runstest.ProxyEntry("example.com:443", egress.DecisionDenied), runstest.ProxyEntry("claude.ai:443", egress.DecisionDenied))
		if got := canary.RefusedBeyond(refused, []string{"claude.ai"}); !slices.Equal(got, []string{"example.com"}) {
			t.Errorf("refused beyond %v", got)
		}
	})
}

func TestProbes(t *testing.T) {
	t.Run("the probes run curl to github.com with no proxy and a read of the host path, and must both fail [verifies SP-2536~1]", func(t *testing.T) {
		script := canary.ProbeScript("/Users/someone/repo/PREREG.md")
		for _, wanted := range []string{"env -u HTTPS_PROXY -u HTTP_PROXY", "curl", "https://github.com", "cat '/Users/someone/repo/PREREG.md'"} {
			if !strings.Contains(script, wanted) {
				t.Errorf("the script lacks %q:\n%s", wanted, script)
			}
		}
		for output, want := range map[string]int{
			"curl=7\nread=1\n":   0,
			"curl=0\nread=1\n":   1,
			"curl=127\nread=1\n": 1,
			"read=0\n":           2,
		} {
			if got := canary.ProbeProblems([]byte(output)); len(got) != want {
				t.Errorf("%q: %v", output, got)
			}
		}
	})
}

// alignmentStatement is a verified alignment verdict made at the isolation given.
func alignmentStatement(t *testing.T, isolation format.Isolation) []byte {
	t.Helper()
	statement := format.Statement[format.Alignment]{
		Type: "https://in-toto.io/Statement/v1", PredicateType: format.PredicateTypeAlignment,
		Predicate: format.Alignment{Executor: format.Executor{ExecutorProfile: format.ExecutorProfile{Isolation: isolation}}},
	}
	encoded, err := json.Marshal(statement)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestLoopChecks(t *testing.T) {
	t.Run("C3 names a plugin system/init does not list and a model that is not the session's [verifies SP-2537~2]", func(t *testing.T) {
		lines, _ := evidence.Lines(record.StreamFile, []byte(runstest.InitLine(t)))
		stream := evidence.DecodeClaude(lines)
		if got := canary.InitProblems(stream, plan.PluginNames(config.ArmTreatment), runstest.Model); len(got) != 0 {
			t.Errorf("the recorded init: %v", got)
		}
		if got := canary.InitProblems(stream, append(plan.PluginNames(config.ArmTreatment), "absent-plugin"), "claude-other"); len(got) != 2 {
			t.Errorf("problems %v", got)
		}
		if got := canary.InitProblems(evidence.ClaudeStream{}, nil, runstest.Model); len(got) != 1 {
			t.Errorf("no init: %v", got)
		}
	})

	t.Run("C3 needs an acceptance gate that passes with no blocking finding [verifies SP-2537~2]", func(t *testing.T) {
		if got := canary.GateProblems([]byte(`{"verdict":"pass","exit_code":0,"findings":[{"category":"note","blocking":false}]}`)); len(got) != 0 {
			t.Errorf("a pass: %v", got)
		}
		if got := canary.GateProblems([]byte(`{"verdict":"blocked","exit_code":1,"findings":[{"category":"surviving_valid_mutant","blocking":true,"requirement_id":"BOX-2"}]}`)); len(got) != 2 {
			t.Errorf("a block: %v", got)
		}
		if got := canary.GateProblems(nil); len(got) != 1 {
			t.Errorf("no report: %v", got)
		}
	})

	t.Run("C3 names every alignment verdict below the isolation floor, signed or not, and counts the verdicts [verifies SP-2537~2]", func(t *testing.T) {
		files := map[string][]byte{
			"a.json": alignmentStatement(t, format.IsolationRestrictedAttested),
			"b.json": alignmentStatement(t, format.IsolationRestricted),
			"c.json": alignmentStatement(t, format.IsolationUnrestricted),
			"d.json": []byte(`{"predicateType":"` + format.PredicateTypeDiscriminatingPower + `"}`),
		}
		problems, verdicts := canary.IsolationProblems(files)
		if verdicts != 3 || len(problems) != 1 || !strings.Contains(problems[0], "c.json") {
			t.Errorf("%d verdicts, problems %v", verdicts, problems)
		}
	})

	t.Run("the auxiliary models are the usage's models beside the session model [verifies SP-2537~2]", func(t *testing.T) {
		if got := canary.AuxiliaryModels([]string{"claude-haiku-5", runstest.Model, "claude-haiku-5"}, runstest.Model); !slices.Equal(got, []string{"claude-haiku-5"}) {
			t.Errorf("auxiliary %v", got)
		}
	})
}

// detectors records the run directories it is asked about and answers with the
// hits it holds.
type detectors struct {
	dirs     []string
	observed []string
	hits     []study.Hit
}

func (d *detectors) Detect(_ context.Context, dir string, observed []string) (study.Detection, error) {
	d.dirs, d.observed = append(d.dirs, dir), observed
	return study.Detection{Hits: d.hits}, nil
}

// canaryWorld is the run verb's world with what the canary sessions see: a proxy
// that contacted the model's hosts and one other, failing probes, and a C3
// session that leaves a verified alignment verdict.
func canaryWorld(t *testing.T) (*runstest.World, *detectors) {
	t.Helper()
	w := runstest.NewWorld(t)
	w.Engine.ProxyLog = []string{
		runstest.ProxyEntry(runstest.AllowedHost+":443", egress.DecisionAllowed),
		runstest.ProxyEntry("statsig.anthropic.com:443", egress.DecisionAllowed),
		runstest.ProxyEntry("telemetry.example.org:443", egress.DecisionAllowed),
	}
	w.Session.Probes = "curl=7\nread=1\n"
	w.Session.Stream = runstest.ShortStream(t, runstest.ResultLine("success", false, "the gate passes", runstest.Model, "claude-haiku-5"))
	verified := alignmentStatement(t, format.IsolationRestricted)
	w.Session.Edit = func(project string) error {
		return runstest.WriteAll(project, map[string]string{filepath.Join(pluginfiles.VerifiedDirectory, "alignment", "BOX-1.json"): string(verified)})
	}
	stageModuleHooks(t, w)
	w.Runner.App.Sessions = &hookLogging{inner: w.Session, log: moduleHookLog(t, w)}
	return w, &detectors{}
}

// stageModuleHooks copies the hook declarations of the repository's module plugin
// trees into the product, where the canary reads them.
func stageModuleHooks(t *testing.T, w *runstest.World) {
	t.Helper()
	for _, module := range modules.All() {
		declaration := runstest.Read(t, filepath.Join(w.Repo, module.PluginDir(harness.FlagClaude), canary.PluginHooksFile))
		tree := filepath.Join(w.Product, plan.ProductPluginsDir, module.PluginDir(harness.FlagClaude))
		if err := runstest.WriteAll(tree, map[string]string{canary.PluginHooksFile: declaration}); err != nil {
			t.Fatal(err)
		}
	}
}

// moduleHookLog is a hook log in which every hook the module plugin trees declare
// was called once.
func moduleHookLog(t *testing.T, w *runstest.World) string {
	t.Helper()
	var lines []string
	for _, module := range modules.All() {
		declaration := runstest.Read(t, filepath.Join(w.Repo, module.PluginDir(harness.FlagClaude), canary.PluginHooksFile))
		declared, err := canary.DeclaredHooks(module.Plugin, []byte(declaration))
		if err != nil {
			t.Fatal(err)
		}
		for _, hook := range declared {
			lines = append(lines, hookLine(hook.Binary, hook.Kind))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func hookLine(binary, kind string) string {
	return `{"module":"` + binary + `","args":["hook","` + kind + `","--harness","claude"],"exitCode":0}`
}

// hookLogging is a Sessions that writes the hook log the recording shims would
// have written in a session's records.
type hookLogging struct {
	inner ports.Sessions
	log   string
}

func (h *hookLogging) Run(spec ports.SessionSpec) (ports.SessionOutcome, error) {
	outcome, err := h.inner.Run(spec)
	if err != nil {
		return outcome, err
	}
	return outcome, os.WriteFile(filepath.Join(filepath.Dir(spec.StdoutPath), record.HooksFile), []byte(h.log), 0o644)
}

func (d *detectors) canary(w *runstest.World) canary.Canary {
	return canary.Canary{Runner: w.Runner, Detectors: d, Stdout: w.Stdout}
}

func invocation(w *runstest.World) canary.Invocation {
	return canary.Invocation{Plan: runs.Plan{Options: w.Options}, Config: w.Config, Layout: study.Layout{Root: w.Work}, HostPath: filepath.Join(w.Repo, canary.HostPathRelative)}
}

func TestCanary(t *testing.T) {
	w, detected := canaryWorld(t)
	output, err := detected.canary(w).Run(context.Background(), invocation(w))
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(w.Work, study.CanaryDir, "1")
	proxies := map[string][]string{}
	for _, argv := range w.Engine.Argvs() {
		if slices.Contains(argv, "--detach") {
			var allows []string
			for index, word := range argv {
				if word == "--allow" {
					allows = append(allows, argv[index+1])
				}
			}
			log := argv[slices.Index(argv, "--log")+1]
			proxies[filepath.Base(filepath.Dir(filepath.Dir(log)))] = allows
		}
	}

	t.Run("C1 runs in an empty project through the allow-all proxy and gives the observed hosts and the allowlist [verifies SP-2535~1]", func(t *testing.T) {
		if !output.C1.Held || !slices.Equal(proxies["c1"], []string{"*"}) {
			t.Errorf("C1 %+v, proxy %v", output.C1, proxies["c1"])
		}
		if !slices.Equal(output.EgressObserved, []string{runstest.AllowedHost, "statsig.anthropic.com", "telemetry.example.org"}) ||
			!slices.Equal(output.EgressAllowlist, []string{runstest.AllowedHost, "statsig.anthropic.com"}) {
			t.Errorf("observed %v, allowlist %v", output.EgressObserved, output.EgressAllowlist)
		}
		if before := runstest.Read(t, filepath.Join(dir, "c1", plan.RecordsDir, study.HashesBeforeFile)); strings.TrimSpace(before) != "{}" {
			t.Errorf("C1's project held files when its session started: %s", before)
		}
		if strings.Contains(strings.Join(w.Session.Specs[0].Argv, " "), "--plugin-dir") {
			t.Error("C1 loads plugins")
		}
	})

	t.Run("C2 enforces C1's allowlist and probes its container with the token kept out of the probes [verifies SP-2536~1]", func(t *testing.T) {
		if !output.C2.Held || !slices.Equal(proxies["c2"], []string{runstest.AllowedHost + ":443", "statsig.anthropic.com:443"}) {
			t.Errorf("C2 %+v, proxy %v", output.C2, proxies["c2"])
		}
		session := strings.Join(w.Session.Specs[1].Argv, " ")
		for _, wanted := range []string{"env -u CLAUDE_CODE_OAUTH_TOKEN", "https://github.com", filepath.Join(w.Repo, canary.HostPathRelative)} {
			if !strings.Contains(session, wanted) {
				t.Errorf("C2's session lacks %q", wanted)
			}
		}
	})

	t.Run("C3 runs the treatment arm on boxoffice in its done state with the prove-all prompt, and holds [verifies SP-2537~2]", func(t *testing.T) {
		if !output.C3.Held {
			t.Fatalf("C3 %+v", output.C3)
		}
		session := strings.Join(w.Session.Specs[2].Argv, " ")
		if !strings.Contains(session, canary.C3Prompt) || strings.Count(session, "--plugin-dir") != len(plan.PluginNames(config.ArmTreatment)) {
			t.Errorf("C3's session: %s", session)
		}
		project := filepath.Join(dir, "c3", plan.ProjectDir)
		if runstest.Read(t, filepath.Join(project, "boxoffice", "prices.py")) != runstest.Read(t, filepath.Join(w.Repo, canary.DoneSource, "boxoffice", "prices.py")) ||
			runstest.Read(t, filepath.Join(project, "tests", "test_prices.py")) != runstest.Read(t, filepath.Join(w.Repo, canary.DoneTests, "tests", "test_prices.py")) {
			t.Error("C3's project is not boxoffice in its done state")
		}
		if !slices.Equal(detected.dirs, []string{filepath.Join(dir, "c3")}) || !slices.Equal(detected.observed, output.EgressObserved) {
			t.Errorf("detectors asked about %v with %v", detected.dirs, detected.observed)
		}
		if !slices.Equal(output.CLIAuxiliaryModels, []string{"claude-haiku-5"}) {
			t.Errorf("auxiliary models %v", output.CLIAuxiliaryModels)
		}
	})

	t.Run("the output is written in the canary's directory with the runner's quota patterns, and is what resolve reads [verifies SP-2538~2]", func(t *testing.T) {
		var written map[string]json.RawMessage
		_ = json.Unmarshal([]byte(runstest.Read(t, filepath.Join(dir, study.CanaryOutputFile))), &written)
		for _, key := range []string{"c1", "c2", "c3", "egress_observed", "egress_allowlist", "cli_auxiliary_models", "quota_patterns", "records"} {
			if _, found := written[key]; !found {
				t.Errorf("output.json lacks %s", key)
			}
		}
		latest, err := canary.Latest{Files: adapters.OSFiles{}, Layout: study.Layout{Root: w.Work}}.Output(context.Background())
		if err != nil || !slices.Equal(latest.QuotaPatterns, runs.QuotaPatterns()) || !latest.Held() {
			t.Errorf("latest %+v, %v", latest, err)
		}
		sessions := map[string]bool{}
		for _, recorded := range latest.Records {
			content := runstest.Read(t, filepath.Join(w.Work, study.CanaryDir, filepath.FromSlash(recorded.Name)))
			if recorded.SHA256 != runs.SHA256([]byte(content)) || recorded.Start.IsZero() {
				t.Errorf("record %+v", recorded)
			}
			sessions[strings.SplitN(recorded.Name, "/", 3)[1]] = true
		}
		if len(sessions) != 3 {
			t.Errorf("the records name the sessions %v", sessions)
		}
		if err := os.WriteFile(filepath.Join(dir, study.CanaryOutputFile), []byte(`{"unknown":1}`), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := canary.LatestOutput(adapters.OSFiles{}, study.Layout{Root: w.Work}); !errors.Is(err, canary.ErrMalformedOutput) {
			t.Errorf("an unknown member: %v", err)
		}
		for _, session := range []string{"c1", "c2", "c3"} {
			if _, err := os.Stat(filepath.Join(dir, session, plan.RecordsDir, record.MetaFile)); err != nil {
				t.Error(err)
			}
		}
	})
}

func TestCanaryFailures(t *testing.T) {
	t.Run("a declared hook that no call of the session shows, or a module plugin tree without hooks, fails C3 [verifies SP-2537~2]", func(t *testing.T) {
		w, detected := canaryWorld(t)
		withoutFirst := strings.Join(strings.Split(moduleHookLog(t, w), "\n")[1:], "\n")
		w.Runner.App.Sessions = &hookLogging{inner: w.Session, log: withoutFirst}
		output, err := detected.canary(w).Run(context.Background(), invocation(w))
		if err != nil {
			t.Fatal(err)
		}
		if output.C3.Held || len(output.C3.Problems) != 1 || !strings.Contains(output.C3.Problems[0], "was not called") {
			t.Errorf("a missing call: %+v", output.C3)
		}

		w, detected = canaryWorld(t)
		if err := os.RemoveAll(filepath.Join(w.Product, plan.ProductPluginsDir, modules.All()[0].PluginDir(harness.FlagClaude))); err != nil {
			t.Fatal(err)
		}
		output, err = detected.canary(w).Run(context.Background(), invocation(w))
		if err != nil {
			t.Fatal(err)
		}
		if output.C3.Held || len(output.C3.Problems) != 1 || !strings.Contains(output.C3.Problems[0], "declares no hooks") {
			t.Errorf("a plugin tree without hooks: %+v", output.C3)
		}
	})

	t.Run("a refused host C1 did not record, or a probe that succeeds, fails C2 and C3 does not run [verifies SP-2536~1]", func(t *testing.T) {
		w, detected := canaryWorld(t)
		w.Engine.ProxyLog = append(w.Engine.ProxyLog, runstest.ProxyEntry("new.example.net:443", egress.DecisionDenied))
		w.Session.Probes = "curl=0\nread=1\n"
		output, err := detected.canary(w).Run(context.Background(), invocation(w))
		if err != nil {
			t.Fatal(err)
		}
		if output.C2.Held || len(output.C2.Problems) != 2 || len(w.Session.Specs) != 2 || output.Held() {
			t.Errorf("C2 %+v, %d sessions", output.C2, len(w.Session.Specs))
		}
	})

	t.Run("a detector hit or an alignment verdict below the floor fails C3 [verifies SP-2537~2]", func(t *testing.T) {
		w, detected := canaryWorld(t)
		detected.hits = []study.Hit{{Detector: study.DetectorGateFiles, Evidence: "shallnot.yaml changed"}}
		low := alignmentStatement(t, format.IsolationInline)
		w.Session.Edit = func(project string) error {
			return runstest.WriteAll(project, map[string]string{filepath.Join(pluginfiles.VerifiedDirectory, "a.json"): string(low)})
		}
		output, err := detected.canary(w).Run(context.Background(), invocation(w))
		if err != nil {
			t.Fatal(err)
		}
		if output.C3.Held || len(output.C3.Problems) != 2 {
			t.Errorf("C3 %+v", output.C3)
		}
	})
}

func TestPluginHooks(t *testing.T) {
	const declaration = `{"hooks":{"PreToolUse":[{"matcher":"Agent","hooks":[{"type":"command","command":"musthold-dp hook pre-tool --harness claude","timeout":30}]}],
		"Stop":[{"hooks":[{"type":"command","command":"\"${CLAUDE_PLUGIN_ROOT}/hooks/stop.sh\""}]}],
		"SubagentStart":[{"hooks":[{"type":"command","command":"musthold-dp hook subagent-start --harness claude"}]}]}}`

	t.Run("a plugin tree's declared hooks are its module commands, and a script of the plugin is not one [verifies SP-2537~2]", func(t *testing.T) {
		declared, err := canary.DeclaredHooks("musthold-dp", []byte(declaration))
		if err != nil {
			t.Fatal(err)
		}
		want := []canary.DeclaredHook{
			{Plugin: "musthold-dp", Event: "PreToolUse", Binary: "musthold-dp", Kind: "pre-tool"},
			{Plugin: "musthold-dp", Event: "SubagentStart", Binary: "musthold-dp", Kind: "subagent-start"},
		}
		if !slices.Equal(declared, want) {
			t.Errorf("%+v", declared)
		}
		if _, err := canary.DeclaredHooks("p", []byte("{")); err == nil {
			t.Error("a malformed declaration was read")
		}
	})

	t.Run("every declared hook must appear in the hook log by its binary and kind, a probe or another binary's hook does not count [verifies SP-2537~2]", func(t *testing.T) {
		declared, _ := canary.DeclaredHooks("musthold-dp", []byte(declaration))
		partial := hookLine("musthold-dp", "pre-tool") + "\n" + hookLine("musthold-alignment", "subagent-start") + "\n" + `{"label":"Stop-probe","exitCode":0}` + "\n"
		calls, err := evidence.DecodeHooks([]byte(partial))
		if err != nil {
			t.Fatal(err)
		}
		problems := canary.HookProblems(declared, calls)
		if len(problems) != 1 || !strings.Contains(problems[0], "SubagentStart") || !strings.Contains(problems[0], "musthold-dp") {
			t.Errorf("%v", problems)
		}
		all, _ := evidence.DecodeHooks([]byte(hookLine("musthold-dp", "pre-tool") + "\n" + hookLine("musthold-dp", "subagent-start") + "\n"))
		if got := canary.HookProblems(declared, all); len(got) != 0 {
			t.Errorf("%v", got)
		}
	})

	t.Run("the recorded init event carries no hook, and the recorded hook log shows every hook of the module plugin trees [verifies SP-2537~2]", func(t *testing.T) {
		repo := dockertest.RepoRoot(t)
		stream, err := os.ReadFile(filepath.Join(repo, "demo/loop/recording/stream.jsonl"))
		if err != nil {
			t.Fatal(err)
		}
		lines, _ := evidence.Lines(record.StreamFile, stream)
		init, found := evidence.DecodeClaude(lines).Init()
		if !found || len(init.Init.Plugins) == 0 || strings.Contains(string(lines[0].Raw), "hooks") {
			t.Fatalf("init %+v", init)
		}
		log, err := os.ReadFile(filepath.Join(repo, "demo/loop/recording", record.HooksFile))
		if err != nil {
			t.Fatal(err)
		}
		calls, err := evidence.DecodeHooks(log)
		if err != nil {
			t.Fatal(err)
		}
		for _, module := range modules.All() {
			declaration, err := os.ReadFile(filepath.Join(repo, module.PluginDir(harness.FlagClaude), canary.PluginHooksFile))
			if err != nil {
				t.Fatal(err)
			}
			declared, err := canary.DeclaredHooks(module.Plugin, declaration)
			if err != nil || len(declared) == 0 {
				t.Fatalf("%s: %v %v", module.Plugin, declared, err)
			}
			if problems := canary.HookProblems(declared, calls); len(problems) != 0 {
				t.Errorf("%s: %v", module.Plugin, problems)
			}
		}
	})
}

func TestCanaryMain(t *testing.T) {
	repo := dockertest.RepoRoot(t)
	work := runstest.TempDir(t)
	main := func(args ...string) (int, string, string) {
		var stdout, stderr bytes.Buffer
		code := canary.Command{Environ: []string{"HOME=" + work, "LOOP1_WORK=" + work}}.Main(context.Background(), args, &stdout, &stderr)
		return code, stdout.String(), stderr.String()
	}

	t.Run("a dry run prints the three sessions and exits 0, an unknown flag exits 2, and a run with no image exits 1 before any session [verifies SP-2539~2]", func(t *testing.T) {
		code, stdout, stderr := main("--repo", repo, "--dry-run", "--inputs", filepath.Join(work, "absent.json"))
		if code != runs.ExitOK || !strings.Contains(stdout, "C1, in ") || !strings.Contains(stdout, "C3, in ") || !strings.Contains(stdout, "--allow '*'") {
			t.Errorf("dry run: %d\n%s\n%s", code, stdout, stderr)
		}
		if code, _, _ := main("--bogus"); code != runs.ExitUsage {
			t.Errorf("unknown flag: %d", code)
		}
		if code, _, stderr := main("--repo", repo, "--inputs", filepath.Join(work, "absent.json")); code != runs.ExitFailure || !strings.Contains(stderr, "--image") {
			t.Errorf("no image: %d %s", code, stderr)
		}
	})
}
