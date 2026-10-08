package app_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/adapters"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/launch"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/modules"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	copilotToken    = "COPILOT_GITHUB_TOKEN"
	portableSubpath = "plugin/portable"
	repoFromApp     = "../../../.."
)

// copyPortableTrees puts the committed portable trees of the modules into the
// temporary repository, which is what the canary inspects and mounts.
func copyPortableTrees(t *testing.T, repo string) {
	t.Helper()
	for _, module := range modules.All() {
		source, err := filepath.Abs(filepath.Join(repoFromApp, module.Dir, portableSubpath))
		if err != nil {
			t.Fatal(err)
		}
		if err := (adapters.OSFiles{}).CopyTree(source, filepath.Join(repo, module.Dir, portableSubpath)); err != nil {
			t.Fatal(err)
		}
	}
}

// copilotListing is what `copilot plugin list --json` or `skill list --json`
// prints with the module trees mounted.
func copilotListing(subject string) string {
	var entries []map[string]any
	for _, module := range modules.All() {
		switch subject {
		case "plugin":
			entries = append(entries, map[string]any{"name": module.Plugin, "version": "0.1.0", "enabled": true, "source": "external"})
		default:
			entries = append(entries, map[string]any{"name": module.Binary + "-fulfil", "source": "plugin", "enabled": true})
		}
	}
	encoded, _ := json.Marshal(entries)
	return string(encoded)
}

func copilotWorld(t *testing.T) *world {
	t.Helper()
	w := newWorld(t, "copilot", night)
	w.runner.Env = []string{"HOME=" + t.TempDir(), "PATH=/usr/bin", "GH_TOKEN=token-value", "COPILOT_ALLOW_ALL=true", "COPILOT_MODEL=other", "COPILOT_PROVIDER_BASE_URL=https://elsewhere"}
	return w
}

func environmentOf(pairs []string) map[string]string {
	found := map[string]string{}
	for _, pair := range pairs {
		name, value, _ := strings.Cut(pair, "=")
		found[name] = value
	}
	return found
}

func TestCopilotDryRun(t *testing.T) {
	t.Run("the dry run prints the Copilot command, the no-call probes, the credential source and what the run writes, and starts nothing [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		w.options.DryRun = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if len(w.processes.calls) != 0 || len(w.session.runs) != 0 || len(w.locker.acquired) != 0 {
			t.Fatalf("a dry run started something: %v", w.processes.calls)
		}
		out := w.stdout.String()
		for _, want := range []string{
			"copilot -p", "--model gpt-5.6-luna", "--output-format json", "--plugin-dir " + w.repo + "/discpower/plugin/portable",
			"COPILOT_HOME='<run-dir>/copilot-home'", "COPILOT_CACHE_HOME='<run-dir>/copilot-cache'", "--log-dir '<run-dir>/copilot-logs'",
			"copilot --version --no-auto-update", "plugin list --json", "skill list --json", "inspect " + w.repo + "/alignment/plugin/portable",
			"credentials: the GH_TOKEN environment variable, passed through unchanged", "nothing: Copilot CLI runs with COPILOT_HOME",
		} {
			if !strings.Contains(out, want) {
				t.Errorf("the plan lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "token-value") {
			t.Error("the plan prints the token")
		}
	})

	t.Run("without a token variable the plan says the login comes from the keychain, and a shared home sets no COPILOT_HOME [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		w.runner.Env = []string{"HOME=/home/u", "PATH=/usr/bin"}
		w.options.DryRun, w.options.SharedHome = true, true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		out := w.stdout.String()
		for _, want := range []string{"credentials: the Copilot CLI login in the system keychain", "$COPILOT_HOME/session-state", "COPILOT_CACHE_HOME='<run-dir>/copilot-cache'"} {
			if !strings.Contains(out, want) {
				t.Errorf("the plan lacks %q:\n%s", want, out)
			}
		}
		if strings.Contains(out, "COPILOT_HOME='<run-dir>") {
			t.Errorf("a shared home session sets COPILOT_HOME:\n%s", out)
		}
	})
}

func TestSyntheticSessionOfCopilot(t *testing.T) {
	w := copilotWorld(t)
	outcome, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("the session runs the built command with the state in the run and the trust variables dropped [verifies SP-1708~2]", func(t *testing.T) {
		spec := w.session.runs[0]
		environment := environmentOf(spec.Env)
		for variable, want := range map[string]string{
			"COPILOT_HOME": filepath.Join(outcome.RunDir, "copilot-home"), "COPILOT_CACHE_HOME": filepath.Join(outcome.RunDir, "copilot-cache"),
			"COPILOT_AUTO_UPDATE": "false", "GH_TOKEN": "token-value",
		} {
			if environment[variable] != want {
				t.Errorf("%s = %q, want %q", variable, environment[variable], want)
			}
		}
		for _, dropped := range []string{"COPILOT_ALLOW_ALL", "COPILOT_MODEL", "COPILOT_PROVIDER_BASE_URL"} {
			if _, present := environment[dropped]; present {
				t.Errorf("the session inherits %s", dropped)
			}
		}
		if got := flagArgument(spec.Argv, "--log-dir"); got != filepath.Join(outcome.RunDir, "copilot-logs") {
			t.Errorf("--log-dir = %s", got)
		}
		if spec.Argv[0] != "copilot" || spec.Cap != launch.CopilotEventCap || spec.Dir != filepath.Join(outcome.RunDir, "project") {
			t.Errorf("session = %+v", spec)
		}
		for _, dir := range []string{"copilot-home", "copilot-cache", "copilot-logs"} {
			if _, err := os.Stat(filepath.Join(outcome.RunDir, dir)); err != nil {
				t.Errorf("the run lacks %s", dir)
			}
		}
	})

	t.Run("every probe that runs the Copilot CLI uses the environment of the run, and the portable trees are inspected [verifies SP-1708~2]", func(t *testing.T) {
		for _, fragment := range []string{"copilot --version --no-auto-update", "plugin list --json", "skill list --json"} {
			if !w.processes.ran(fragment) {
				t.Errorf("no command ran %q", fragment)
			}
		}
		logs := filepath.Join(outcome.RunDir, "copilot-logs")
		for _, argv := range w.processes.calls {
			if argv[0] == "copilot" && flagArgument(argv, "--log-dir") != logs {
				t.Errorf("a probe logs outside the run: %v", argv)
			}
		}
		raw, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", "nocall.json"))
		if err != nil {
			t.Fatal(err)
		}
		var recorded evidence.NoCall
		if err := json.Unmarshal(raw, &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.CopilotListing == nil || !recorded.CopilotListing.Attempted || len(recorded.PluginTrees) != len(modules.All()) {
			t.Fatalf("no-call = %+v", recorded)
		}
		for _, tree := range recorded.PluginTrees {
			if len(tree.Problems) != 0 {
				t.Errorf("tree %s: %v", tree.Dir, tree.Problems)
			}
		}
	})

	t.Run("the Copilot session passes every check on the expected recordings and records the pinned model [verifies SP-1707~1]", func(t *testing.T) {
		if outcome.Report.ExitCode() != verdict.ExitAllPass {
			for _, v := range outcome.Report.Verdicts {
				t.Logf("%s %s %s", v.Status, v.Check, v.Summary)
			}
			t.Fatalf("exit code %d", outcome.Report.ExitCode())
		}
		raw, _ := os.ReadFile(filepath.Join(outcome.RunDir, "records", "session.json"))
		var recorded evidence.Session
		if err := json.Unmarshal(raw, &recorded); err != nil {
			t.Fatal(err)
		}
		if recorded.Model != launch.CopilotModel || recorded.ConfigMode != evidence.ConfigIsolated || recorded.EventCap != launch.CopilotEventCap {
			t.Errorf("session = %+v", recorded)
		}
		probe, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", "sandbox-probe.json"))
		if err != nil || !strings.Contains(string(probe), `"exists": false`) {
			t.Fatalf("allowlist probe = %s, %v", probe, err)
		}
	})
}

func TestOtherHarnessesLeaveCopilotDirectoriesOut(t *testing.T) {
	t.Run("a Claude Code run creates no Copilot directory [verifies SP-1508~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		for _, dir := range []string{"copilot-home", "copilot-cache", "copilot-logs"} {
			if _, err := os.Stat(filepath.Join(outcome.RunDir, dir)); err == nil {
				t.Errorf("the run holds %s", dir)
			}
		}
	})
}

func flagArgument(argv []string, flag string) string {
	index := slices.Index(argv, flag)
	if index < 0 || index+1 >= len(argv) {
		return ""
	}
	return argv[index+1]
}

func TestCopilotAgainstTheUsersCopilotDirectory(t *testing.T) {
	write := func(t *testing.T, path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touchedOf := func(t *testing.T, runDir string) evidence.Touched {
		t.Helper()
		raw, err := os.ReadFile(filepath.Join(runDir, "records", "touched.json"))
		if err != nil {
			t.Fatal(err)
		}
		var touched evidence.Touched
		if err := json.Unmarshal(raw, &touched); err != nil {
			t.Fatal(err)
		}
		return touched
	}
	verdictOf := func(t *testing.T, report verdict.Report, name string) verdict.Verdict {
		t.Helper()
		for _, v := range report.Verdicts {
			if v.Check == name {
				return v
			}
		}
		t.Fatalf("no verdict for %s", name)
		return verdict.Verdict{}
	}

	t.Run("an isolated session is fingerprinted against ~/.copilot and passes when it leaves it alone [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "HOME="+home)
		write(t, filepath.Join(home, ".copilot", "settings.json"))
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if touched := touchedOf(t, outcome.RunDir); !slices.Equal(touched.Roots, []string{filepath.Join(home, ".copilot")}) || len(touched.Unattributed) != 0 {
			t.Errorf("touched = %+v", touched)
		}
		if v := verdictOf(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Pass {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("an isolated session that writes an entry naming its run in ~/.copilot fails [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "HOME="+home)
		w.session.afterRun = func(spec app.SessionSpec) {
			write(t, filepath.Join(home, ".copilot", "logs", filepath.Base(filepath.Dir(spec.Dir))+".log"))
		}
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if v := verdictOf(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Fail {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a shared-home session lists what it wrote in ~/.copilot and the check leaves it open [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "HOME="+home)
		w.options.SharedHome = true
		w.session.afterRun = func(app.SessionSpec) {
			write(t, filepath.Join(home, ".copilot", "session-state", "uuid", "events.jsonl"))
		}
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if environment := environmentOf(w.session.runs[0].Env); environment["COPILOT_HOME"] != "" {
			t.Errorf("the shared session sets COPILOT_HOME=%s", environment["COPILOT_HOME"])
		}
		touched := touchedOf(t, outcome.RunDir)
		if !slices.ContainsFunc(touched.Unattributed, func(path string) bool { return strings.Contains(path, "session-state") }) {
			t.Errorf("touched = %+v", touched)
		}
		if v := verdictOf(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Inconclusive {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a COPILOT_HOME in the user's own environment is the directory that is fingerprinted [verifies SP-1708~2]", func(t *testing.T) {
		w := copilotWorld(t)
		own := t.TempDir()
		w.runner.Env = append(w.runner.Env, "COPILOT_HOME="+own)
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if touched := touchedOf(t, outcome.RunDir); !slices.Equal(touched.Roots, []string{own}) {
			t.Errorf("roots = %v", touched.Roots)
		}
		if environment := environmentOf(w.session.runs[0].Env); environment["COPILOT_HOME"] == own {
			t.Error("the session runs on the user's own COPILOT_HOME")
		}
	})
}
