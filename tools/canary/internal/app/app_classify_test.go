package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/evidence"
	"github.com/RachidChabane/spec-proof/tools/canary/internal/domain/verdict"
)

func TestClassificationIsPreparedForAClaudeCodeRun(t *testing.T) {
	t.Run("the project classifies the completeness requests in shadow and the raw API log directory exists [verifies SP-1925~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.options.PrepareOnly = true
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		config, err := os.ReadFile(filepath.Join(outcome.RunDir, "project", ".spec-proof", "config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		bodies := filepath.Join(outcome.RunDir, "bodies")
		for _, wanted := range []string{"completeness: true", "default: off", "completeness: shadow", "witness_dir: " + bodies} {
			if !strings.Contains(string(config), wanted) {
				t.Errorf("the configuration lacks %q:\n%s", wanted, config)
			}
		}
		if info, err := os.Stat(bodies); err != nil || !info.IsDir() {
			t.Errorf("the raw API log directory: %v", err)
		}
	})

	t.Run("the plan names the raw API log in the environment of the session [verifies SP-1925~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		w.options.DryRun = true
		if _, err := w.runner.Run(context.Background(), w.options); err != nil {
			t.Fatal(err)
		}
		if out := w.stdout.String(); !strings.Contains(out, "OTEL_LOG_RAW_API_BODIES='file:<run-dir>/bodies'") {
			t.Errorf("the plan lacks the raw API log:\n%s", out)
		}
	})
}

func TestClassifyingModuleJournalIsRecorded(t *testing.T) {
	t.Run("a Claude Code run copies the journal of the classifying module apart from the primary journal and the check reads it [verifies SP-1925~1]", func(t *testing.T) {
		w := newWorld(t, "claude", night)
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		recorded, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", evidence.ClassifyJournalFile))
		if err != nil {
			t.Fatal(err)
		}
		fixture, err := os.ReadFile(filepath.Join(fixturesRoot, "claude-session", evidence.ClassifyJournalFile))
		if err != nil {
			t.Fatal(err)
		}
		if string(recorded) != string(fixture) {
			t.Errorf("the classifying module's journal was not copied:\n%s", recorded)
		}
		primary, err := os.ReadFile(filepath.Join(outcome.RunDir, "records", evidence.JournalFile))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(primary), "classify_shadow") {
			t.Errorf("the primary journal holds the classifying module's events:\n%s", primary)
		}
		for _, v := range outcome.Report.Verdicts {
			if v.Check == "classify_witness_recorded" && v.Status != verdict.Pass {
				t.Errorf("classify_witness_recorded: %s %s", v.Status, v.Summary)
			}
		}
	})

	t.Run("a run of another harness records no journal of the classifying module [verifies SP-1925~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		outcome, err := w.runner.Run(context.Background(), w.options)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(outcome.RunDir, "records", evidence.ClassifyJournalFile)); !os.IsNotExist(err) {
			t.Errorf("stat: %v", err)
		}
	})
}
