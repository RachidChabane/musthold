package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/hostinstall"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// fixtureAgents is an AGENTS.md the fixture project could already hold.
const fixtureAgents = "<!-- shallnot:begin -->\n## Tracing requirements\n\nRun the gate.\n<!-- shallnot:end -->\n"

func preparedAgents(t *testing.T, w *world) string {
	t.Helper()
	w.options.PrepareOnly = true
	outcome, err := w.runner.Run(context.Background(), w.options)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(outcome.RunDir, "project", hostinstall.AgentsPath))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func TestProjectAgentsFile(t *testing.T) {
	for _, flag := range []string{"claude", "codex", "copilot"} {
		t.Run("on "+flag+" the prepared project holds the AGENTS.md that enabling every module writes [verifies SP-1943~1]", func(t *testing.T) {
			want, err := hostinstall.WithAgentsBlocks(nil, modules.Installs()...)
			if err != nil {
				t.Fatal(err)
			}
			if got := preparedAgents(t, newWorld(t, flag, night)); got != string(want) {
				t.Fatalf("AGENTS.md:\n%s\nwant:\n%s", got, want)
			}
			for _, module := range modules.All() {
				if !strings.Contains(string(want), module.Binary+" prove --changed") {
					t.Errorf("the rendered file lacks the block of %s", module.Binary)
				}
			}
		})
	}

	t.Run("an AGENTS.md the fixture already holds keeps its text and gains the blocks of the modules [verifies SP-1943~1]", func(t *testing.T) {
		w := newWorld(t, "codex", night)
		fixturePath := filepath.Join(w.repo, "discpower/fixtures/prove/parcel", hostinstall.AgentsPath)
		if err := os.WriteFile(fixturePath, []byte(fixtureAgents), 0o644); err != nil {
			t.Fatal(err)
		}
		want, err := hostinstall.WithAgentsBlocks([]byte(fixtureAgents), modules.Installs()...)
		if err != nil {
			t.Fatal(err)
		}
		if got := preparedAgents(t, w); got != string(want) || !strings.HasPrefix(got, fixtureAgents) {
			t.Fatalf("AGENTS.md:\n%s", got)
		}
	})
}
