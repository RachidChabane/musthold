package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
