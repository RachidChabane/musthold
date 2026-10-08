package app_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/app"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

const (
	codexFixture    = "codex-session"
	rolloutDayDir   = "sessions/2026/10/07"
	mainRolloutName = "rollout-2026-10-07T21-05-12-01a11334-f5df-7a33-ba1e-3dd65b19d059.jsonl"
	childRollout    = "rollout-2026-10-07T21-05-15-01a11334-f9a0-7c11-8e02-1b3f0c2d4e55.jsonl"
	strangerName    = "rollout-2026-10-07T20-10-00-0199aaaa-0000-7000-8000-000000000001.jsonl"
	strangerChild   = "rollout-2026-10-07T20-10-03-0199aaaa-0000-7000-8000-000000000002.jsonl"
	trustedTable    = "[projects.%q]\ntrust_level = \"trusted\"\n"
	userConfig      = "model = \"gpt-6-luna\"\n\n[projects.\"/home/u/work\"]\ntrust_level = \"trusted\"\n"
)

func fixtureBytes(t *testing.T, elements ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{fixturesRoot, codexFixture}, elements...)...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path string, data []byte, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, mode); err != nil {
		t.Fatal(err)
	}
}

// leaveCodexRollouts files the rollouts of the recorded session where Codex files
// them, beside the rollouts of two threads of other sessions.
func leaveCodexRollouts(t *testing.T, codexHome string) {
	t.Helper()
	day := filepath.Join(codexHome, rolloutDayDir)
	writeFile(t, filepath.Join(day, mainRolloutName), fixtureBytes(t, "rollout-main.jsonl"), 0o644)
	writeFile(t, filepath.Join(day, childRollout), fixtureBytes(t, "rollout-children", childRollout), 0o644)
	writeFile(t, filepath.Join(day, strangerName), []byte(`{"type":"session_meta","payload":{"id":"0199aaaa-0000-7000-8000-000000000001","source":"cli"}}`+"\n"), 0o644)
	writeFile(t, filepath.Join(day, strangerChild), []byte(`{"type":"session_meta","payload":{"id":"0199aaaa-0000-7000-8000-000000000002","source":{"subagent":{"thread_spawn":{"parent_thread_id":"0199aaaa-0000-7000-8000-000000000001","depth":1}}}}}`+"\n"), 0o644)
}

func verdictNamed(t *testing.T, report verdict.Report, name string) verdict.Verdict {
	t.Helper()
	for _, v := range report.Verdicts {
		if v.Check == name {
			return v
		}
	}
	t.Fatalf("no verdict for %s", name)
	return verdict.Verdict{}
}

func readTouched(t *testing.T, runDir string) evidence.Touched {
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

func TestCodexRollouts(t *testing.T) {
	t.Run("the rollout of the main thread and those of its children are copied into the records, and no other thread's [verifies SP-1510~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		w.options.CodexHome = t.TempDir()
		w.session.afterRun = func(app.SessionSpec) { leaveCodexRollouts(t, w.options.CodexHome) }
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		records := filepath.Join(outcome.RunDir, "records")
		main, err := os.ReadFile(filepath.Join(records, evidence.RolloutMainFile))
		if err != nil || string(main) != string(fixtureBytes(t, "rollout-main.jsonl")) {
			t.Fatalf("main rollout %q, %v", main, err)
		}
		children, err := os.ReadDir(filepath.Join(records, evidence.RolloutChildrenDir))
		if err != nil || len(children) != 1 || children[0].Name() != childRollout {
			t.Fatalf("children %v, %v", children, err)
		}
		if v := verdictNamed(t, outcome.Report, "executor_spawn_blind"); v.Status != verdict.Pass || v.Evidence[0].File != evidence.RolloutMainFile {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a recorded run is evaluated again from its rollouts without starting anything [verifies SP-1510~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		w.options.CodexHome = t.TempDir()
		w.session.afterRun = func(app.SessionSpec) { leaveCodexRollouts(t, w.options.CodexHome) }
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		before := len(w.processes.calls)
		report, err := w.runner.Evaluate(filepath.Join(outcome.RunDir, "records"), "codex")
		if err != nil || len(w.processes.calls) != before {
			t.Fatalf("error %v", err)
		}
		if v := verdictNamed(t, report, "executor_spawn_blind"); v.Status != verdict.Pass {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a session that left no rollout leaves the spawn inconclusive and the run still reports [verifies SP-1510~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		w.options.CodexHome = t.TempDir()
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(outcome.RunDir, "records", evidence.RolloutMainFile)); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("a rollout was recorded: %v", err)
		}
		v := verdictNamed(t, outcome.Report, "executor_spawn_blind")
		if v.Status != verdict.Inconclusive || !strings.Contains(v.Summary, "no rollout") {
			t.Fatalf("verdict = %+v", v)
		}
		if verdictNamed(t, outcome.Report, "agent_files_load").Status != verdict.Pass {
			t.Error("the stream fallback no longer finds the spawn")
		}
	})
}

func TestCodexTrustTable(t *testing.T) {
	configOf := func(home string) string { return filepath.Join(home, "config.toml") }
	run := func(t *testing.T, appended func(project string) string) (w *world, outcome app.Outcome, home string) {
		t.Helper()
		w = newWorld(t, "codex", night)
		home = t.TempDir()
		w.options.CodexHome = home
		writeFile(t, configOf(home), []byte(userConfig), 0o600)
		w.session.afterRun = func(spec app.SessionSpec) {
			leaveCodexRollouts(t, home)
			file, err := os.OpenFile(configOf(home), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer file.Close()
			if _, err := file.WriteString(appended(spec.Dir)); err != nil {
				t.Fatal(err)
			}
		}
		var err error
		if outcome, err = w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		return w, outcome, home
	}

	t.Run("the table the session persisted is removed, the file keeps its content and mode, and the check passes with what was removed [verifies SP-1512~1]", func(t *testing.T) {
		_, outcome, home := run(t, func(project string) string { return "\n" + strings.Replace(trustedTable, "%q", `"`+project+`"`, 1) })
		content, err := os.ReadFile(configOf(home))
		if err != nil || string(content) != userConfig {
			t.Fatalf("config = %q, %v", content, err)
		}
		if info, err := os.Stat(configOf(home)); err != nil || info.Mode().Perm() != 0o600 {
			t.Fatalf("mode = %v, %v", info.Mode(), err)
		}
		touched := readTouched(t, outcome.RunDir)
		if len(touched.ConfigWrites) != 1 {
			t.Fatalf("writes = %+v", touched.ConfigWrites)
		}
		write := touched.ConfigWrites[0]
		if write.Path != configOf(home) || !write.Removed || !write.Verified || !strings.Contains(write.Table, "trust_level") || !strings.Contains(write.Table, "[projects.") {
			t.Errorf("write = %+v", write)
		}
		if !slices.Contains(touched.Attributed, configOf(home)) {
			t.Errorf("the change of config.toml is not attributed: %+v", touched)
		}
		v := verdictNamed(t, outcome.Report, "user_config_untouched")
		if v.Status != verdict.Pass || !strings.Contains(v.Summary, "removed exactly those") || !strings.Contains(v.Summary, configOf(home)) {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a table that names the project and cannot be removed stays in the file and fails the check [verifies SP-1512~1]", func(t *testing.T) {
		appended := func(project string) string { return "\n[projects.\"" + project + "\".extra]\nk = 1\n" }
		_, outcome, home := run(t, appended)
		content, err := os.ReadFile(configOf(home))
		if err != nil || !strings.Contains(string(content), ".extra]") {
			t.Fatalf("config = %q, %v", content, err)
		}
		touched := readTouched(t, outcome.RunDir)
		if len(touched.ConfigWrites) != 1 || touched.ConfigWrites[0].Removed || touched.ConfigWrites[0].Verified {
			t.Fatalf("writes = %+v", touched.ConfigWrites)
		}
		if v := verdictNamed(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Fail {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("a configuration the session left alone is not rewritten and nothing is recorded [verifies SP-1512~1]", func(t *testing.T) {
		_, outcome, home := run(t, func(string) string { return "" })
		content, _ := os.ReadFile(configOf(home))
		if string(content) != userConfig {
			t.Fatalf("config = %q", content)
		}
		if touched := readTouched(t, outcome.RunDir); len(touched.ConfigWrites) != 0 || len(touched.Attributed) != 0 {
			t.Fatalf("touched = %+v", touched)
		}
		if v := verdictNamed(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Pass {
			t.Fatalf("verdict = %+v", v)
		}
	})

	t.Run("the user's own home is restored the same way, and the other Codex configuration of it is not touched [verifies SP-1512~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		home := t.TempDir()
		w.runner.Env = append(w.runner.Env, "CODEX_HOME="+home)
		writeFile(t, configOf(home), []byte(userConfig), 0o644)
		writeFile(t, filepath.Join(home, "auth.json"), []byte("{}"), 0o600)
		w.session.afterRun = func(spec app.SessionSpec) {
			writeFile(t, configOf(home), []byte(userConfig+"\n[projects.\""+spec.Dir+"\"]\ntrust_level = \"trusted\"\n"), 0o644)
		}
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if content, _ := os.ReadFile(configOf(home)); string(content) != userConfig {
			t.Fatalf("config = %q", content)
		}
		if auth, _ := os.ReadFile(filepath.Join(home, "auth.json")); string(auth) != "{}" {
			t.Errorf("auth.json = %q", auth)
		}
		if v := verdictNamed(t, outcome.Report, "user_config_untouched"); v.Status != verdict.Pass {
			t.Fatalf("verdict = %+v", v)
		}
	})
}

func TestSessionThatNeverStarted(t *testing.T) {
	const refused = "Error: Model \"claude-haiku-4.5\" from --model flag is not available."
	for _, flag := range []string{"claude", "codex", "copilot"} {
		t.Run(flag+": a session that ends before its first turn is a setup error with the CLI's own error line and no report [verifies SP-1513~1]", func(t *testing.T) {
			w := newWorld(t, flag, night)
			w.options.CodexHome = t.TempDir()
			w.session.stderr = "starting\n" + refused + "\n"
			outcome, err := w.runner.Run(context.Background(), w.options)
			if !errors.Is(err, app.ErrNeverStarted) || !strings.Contains(err.Error(), refused) {
				t.Fatalf("error = %v", err)
			}
			if outcome.RunDir == "" || len(outcome.Report.Verdicts) != 0 {
				t.Fatalf("outcome = %+v", outcome)
			}
			if _, err := os.Stat(filepath.Join(outcome.RunDir, "records", app.ReportFile)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("a report was written: %v", err)
			}
			if _, err := os.Stat(filepath.Join(outcome.RunDir, "records", "stderr.txt")); err != nil {
				t.Errorf("the CLI's error output was not kept: %v", err)
			}
		})
	}

	t.Run("a CLI that printed nothing is named as such with its exit code [verifies SP-1513~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.session.stderr = "\n"
		_, err := w.runner.Run(context.Background(), w.options)
		if !errors.Is(err, app.ErrNeverStarted) || !strings.Contains(err.Error(), "exit code 1") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestModelOption(t *testing.T) {
	for _, tc := range []struct{ flag, defaultModel, option string }{
		{"claude", "claude-sonnet-5-5", "--model"},
		{"codex", "gpt-6-luna", "-m"},
		{"copilot", "gpt-5.6-luna", "--model"},
	} {
		t.Run(tc.flag+": the model the operator names replaces the default in the command and in the session record [verifies SP-1514~2]", func(t *testing.T) {
			w := newWorld(t, tc.flag, night)
			w.options.CodexHome = t.TempDir()
			w.options.Model = "custom-model"
			outcome, err := w.runner.Run(context.Background(), w.options)
			if err != nil {
				t.Fatal(err)
			}
			argv := w.session.runs[0].Argv
			at := slices.Index(argv, tc.option)
			if at < 0 || argv[at+1] != "custom-model" || slices.Contains(argv, tc.defaultModel) {
				t.Fatalf("argv = %v", argv)
			}
			raw, _ := os.ReadFile(filepath.Join(outcome.RunDir, "records", "session.json"))
			if !strings.Contains(string(raw), `"model": "custom-model"`) {
				t.Errorf("session = %s", raw)
			}
		})

		t.Run(tc.flag+": the plan prints the model the operator names [verifies SP-1514~2]", func(t *testing.T) {
			lines, err := app.Plan(tc.flag, "/repo", "custom-model", app.Homes{})
			if err != nil || !strings.Contains(strings.Join(lines, "\n"), tc.option+" custom-model") {
				t.Fatalf("plan %v, %v", lines, err)
			}
		})
	}
}
