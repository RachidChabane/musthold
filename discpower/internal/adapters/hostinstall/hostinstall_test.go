package hostinstall_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/hostinstall"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
)

const (
	traceabilityOnly = "version: 1\nspecs:\n  - specs\ntests:\n  - tests\n"
	shallnotAgents   = "<!-- shallnot:begin -->\n## Tracing requirements\n<!-- shallnot:end -->\n"
	seedYAML         = "proofs:\n  alignment: false\n  discriminating-power: false\n  completeness: false\necosystems:\n  - pytest\n"
)

func write(t *testing.T, root, path, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, root, path string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
	if errors.Is(err, os.ErrNotExist) {
		return "<absent>"
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "shallnot.yaml", traceabilityOnly)
	write(t, root, "AGENTS.md", shallnotAgents)
	return root
}

func apply(t *testing.T, plan hostinstall.Plan, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.Apply(); err != nil {
		t.Fatal(err)
	}
}

func TestInstallOnDisk(t *testing.T) {
	t.Run("init writes the seed, ignores the module's state and enables the plugin, and a second run plans nothing [verifies SP-243~2]", func(t *testing.T) {
		root := repository(t)
		all := []hostharness.Harness{hostharness.Claude(), hostharness.Codex(), hostharness.Copilot()}
		plan, err := hostinstall.BuildInit(hostinstall.InitOptions{Root: root, Harnesses: all, Seed: hostinstall.ConfigSeed(seedYAML)})
		apply(t, plan, err)
		if got := read(t, root, ".spec-proof/config.yaml"); got != seedYAML+"\n"+hostinstall.OperationsExample {
			t.Errorf("config = %q", got)
		}
		if got := read(t, root, ".gitignore"); got != ".spec-proof/cache/\n.spec-proof/verified/\n.spec-proof/pending/\n" {
			t.Errorf(".gitignore = %q", got)
		}
		if got := read(t, root, ".claude/settings.json"); !strings.Contains(got, `"spec-proof-dp@spec-proof": true`) {
			t.Errorf("settings = %s", got)
		}
		for _, path := range []string{".codex/agents/spec-proof-dp-judge.toml", ".github/agents/spec-proof-dp-decider.agent.md", ".github/hooks/spec-proof-dp.json"} {
			if read(t, root, path) == "<absent>" {
				t.Errorf("%s was not written", path)
			}
		}
		if read(t, root, "shallnot.yaml") != traceabilityOnly || read(t, root, "AGENTS.md") != shallnotAgents {
			t.Error("init changed Shallnot's files")
		}
		again, err := hostinstall.BuildInit(hostinstall.InitOptions{Root: root, Harnesses: all, Seed: hostinstall.ConfigSeed(seedYAML)})
		if err != nil || again.Pending() {
			t.Fatalf("pending = %v, %v", again.Pending(), err)
		}
	})

	t.Run("hooks selection resolves auto, none and a list on the disk [verifies SP-243~2]", func(t *testing.T) {
		root := t.TempDir()
		if harnesses, err := hostinstall.SelectHarnesses(root, "auto"); err != nil || len(harnesses) != 1 {
			t.Fatalf("auto on an empty project: %v, %v", harnesses, err)
		}
		write(t, root, ".codex/config.toml", "")
		if harnesses, err := hostinstall.SelectHarnesses(root, "auto"); err != nil || len(harnesses) != 2 {
			t.Fatalf("auto with a Codex directory: %v, %v", harnesses, err)
		}
		if harnesses, err := hostinstall.SelectHarnesses(root, "none"); err != nil || len(harnesses) != 0 {
			t.Fatalf("none: %v, %v", harnesses, err)
		}
		if _, err := hostinstall.SelectHarnesses(root, "claude,cursor"); !errors.Is(err, hostinstall.ErrUnknownHookHarness) {
			t.Fatalf("an unknown harness: %v", err)
		}
	})

	t.Run("enable adds discriminating-power to proofs.require and sets the local trust, and disable removes the entry and keeps the trust [verifies SP-244~2]", func(t *testing.T) {
		root := repository(t)
		plan, err := hostinstall.BuildEnable(root)
		apply(t, plan, err)
		if got := read(t, root, "shallnot.yaml"); got != traceabilityOnly+"proofs:\n  require:\n    - discriminating-power\n  trust: local\n" {
			t.Fatalf("shallnot.yaml = %q", got)
		}
		if agents := read(t, root, "AGENTS.md"); !strings.HasPrefix(agents, shallnotAgents) || !strings.Contains(agents, "spec-proof-dp prove --changed") {
			t.Fatalf("AGENTS.md = %q", agents)
		}
		plan, err = hostinstall.BuildDisable(root)
		apply(t, plan, err)
		if read(t, root, "shallnot.yaml") != traceabilityOnly+"proofs:\n  trust: local\n" || read(t, root, "AGENTS.md") != shallnotAgents {
			t.Fatal("disable did not take the require entry and the block away and keep the trust")
		}
	})

	t.Run("a project with no shallnot.yaml cannot enable the proof and a flow-style proofs block is refused [verifies SP-244~2]", func(t *testing.T) {
		if _, err := hostinstall.BuildEnable(t.TempDir()); !errors.Is(err, hostinstall.ErrNoShallnotConfig) {
			t.Fatalf("error = %v", err)
		}
		root := repository(t)
		write(t, root, "shallnot.yaml", "proofs: {trust: local}\n")
		if _, err := hostinstall.BuildEnable(root); !errors.Is(err, hostinstall.ErrUnsupportedYAML) {
			t.Fatalf("error = %v", err)
		}
	})
}
