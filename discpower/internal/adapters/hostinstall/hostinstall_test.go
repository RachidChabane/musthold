package hostinstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
)

const (
	traceabilityOnly = `# Shallnot configuration of the parcel service
version: 1
specs:
  - specs

tests:
  - tests   # the test roots
results:
  - build/test-results/*.xml
`
	shallnotAgents     = "<!-- shallnot:begin -->\n## Tracing requirements\n\nRun the gate.\n<!-- shallnot:end -->\n"
	claudeSettingsFile = `{
  "hooks": {
    "Stop": [
      {
        "hooks": [
          {
            "command": "shallnot hook claude-stop",
            "timeout": 600,
            "type": "command"
          }
        ]
      }
    ]
  }
}
`
	seedYAML = "proofs:\n  alignment: false\n  discriminating-power: false\n  completeness: false\necosystems:\n  - pytest\n"
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

// repository is a project Shallnot's `init` equipped: traceability only.
func repository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	write(t, root, "shallnot.yaml", traceabilityOnly)
	write(t, root, "AGENTS.md", shallnotAgents)
	write(t, root, ".claude/settings.json", claudeSettingsFile)
	write(t, root, ".gitignore", "build/\n")
	return root
}

// apply returns a function that applies a plan the test just built.
func apply(t *testing.T) func(Plan, error) {
	return func(plan Plan, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		if err := plan.Apply(); err != nil {
			t.Fatal(err)
		}
	}
}

func initPlan(t *testing.T, root string, harnesses ...hostharness.Harness) (Plan, error) {
	t.Helper()
	return BuildInit(InitOptions{Root: root, Harnesses: harnesses, Seed: ConfigSeed(seedYAML)})
}

func TestInit(t *testing.T) {
	t.Run("init leaves a traceability-only repository's Shallnot files and AGENTS.md as they were [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		apply(t)(initPlan(t, root, hostharness.Claude()))
		if got := read(t, root, "shallnot.yaml"); got != traceabilityOnly {
			t.Errorf("shallnot.yaml changed:\n%s", got)
		}
		if got := read(t, root, "AGENTS.md"); got != shallnotAgents {
			t.Errorf("AGENTS.md changed:\n%s", got)
		}
		if got := read(t, root, ".spec-proof/config.yaml"); got != seedYAML {
			t.Errorf("config.yaml = %q", got)
		}
		if got := read(t, root, ".gitignore"); got != "build/\n.spec-proof/cache/\n.spec-proof/verified/\n" {
			t.Errorf(".gitignore = %q", got)
		}
	})

	t.Run("init enables the plugin in the project settings and keeps Shallnot's stop hook [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		apply(t)(initPlan(t, root, hostharness.Claude()))
		settings := read(t, root, ".claude/settings.json")
		if !strings.Contains(settings, `"spec-proof-dp@spec-proof": true`) || !strings.Contains(settings, "shallnot hook claude-stop") {
			t.Fatalf("settings = %s", settings)
		}
		if strings.Contains(settings, "spec-proof-dp hook") {
			t.Fatal("init installed a module hook in the project settings, which the plugin carries")
		}
	})

	t.Run("a choice the user made about the plugin is kept [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		write(t, root, ".claude/settings.json", `{"enabledPlugins":{"spec-proof-dp@spec-proof":false}}`)
		apply(t)(initPlan(t, root, hostharness.Claude()))
		if got := read(t, root, ".claude/settings.json"); got != `{"enabledPlugins":{"spec-proof-dp@spec-proof":false}}` {
			t.Fatalf("settings = %s", got)
		}
	})

	t.Run("init is idempotent and a second run plans no change [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		all := []hostharness.Harness{hostharness.Claude(), hostharness.Codex(), hostharness.Copilot()}
		apply(t)(initPlan(t, root, all...))
		again, err := initPlan(t, root, all...)
		if err != nil || again.Pending() {
			t.Fatalf("pending = %v, error = %v, actions = %+v", again.Pending(), err, again.Actions)
		}
	})

	t.Run("Codex gets read-only agents, a skill and hook entries merged into its hooks file [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		write(t, root, ".codex/hooks.json", `{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"shallnot hook stop","timeout":600}]}]}}`)
		apply(t)(initPlan(t, root, hostharness.Codex()))
		hooks := read(t, root, ".codex/hooks.json")
		for _, want := range []string{"shallnot hook stop", "spec-proof-dp hook pre-tool --harness codex", "spec-proof-dp hook subagent-start --harness codex", "spec-proof-dp hook subagent-stop --harness codex"} {
			if strings.Count(hooks, want) != 1 {
				t.Errorf("hooks file holds %q %d times", want, strings.Count(hooks, want))
			}
		}
		for _, path := range []string{".codex/agents/spec-proof-dp-judge.toml", ".agents/skills/spec-proof-dp-fulfil/SKILL.md"} {
			if got := read(t, root, path); got == "<absent>" {
				t.Errorf("%s was not written", path)
			}
		}
	})

	t.Run("Codex hook entries are created when the project has no hooks file [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		apply(t)(initPlan(t, root, hostharness.Codex()))
		if got := read(t, root, ".codex/hooks.json"); !strings.Contains(got, "spec-proof-dp hook pre-tool --harness codex") {
			t.Fatalf("hooks = %s", got)
		}
	})

	t.Run("Copilot gets its agents, its skill and a hooks file of its own beside Shallnot's [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		write(t, root, ".github/hooks/shallnot.json", `{"version":1}`)
		apply(t)(initPlan(t, root, hostharness.Copilot()))
		for _, path := range []string{".github/agents/spec-proof-dp-decider.agent.md", ".github/skills/spec-proof-dp-fulfil/SKILL.md", ".github/hooks/spec-proof-dp.json"} {
			if read(t, root, path) == "<absent>" {
				t.Errorf("%s was not written", path)
			}
		}
		if got := read(t, root, ".github/hooks/shallnot.json"); got != `{"version":1}` {
			t.Errorf("Shallnot's hooks file changed: %s", got)
		}
	})

	t.Run("an existing configuration gets the proof's entry and nothing else changes [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		original := "# kept\nproofs:\n  alignment: true\necosystems:\n  - go\nalignment:\n  anything: 1\n"
		write(t, root, ".spec-proof/config.yaml", original)
		apply(t)(initPlan(t, root))
		want := "# kept\nproofs:\n  alignment: true\n  discriminating-power: false\necosystems:\n  - go\nalignment:\n  anything: 1\n"
		if got := read(t, root, ".spec-proof/config.yaml"); got != want {
			t.Fatalf("config = %q", got)
		}
	})

	t.Run("hooks none installs no harness file [verifies SP-243~1]", func(t *testing.T) {
		harnesses, err := SelectHarnesses(t.TempDir(), "none")
		if err != nil || len(harnesses) != 0 {
			t.Fatalf("harnesses = %v, %v", harnesses, err)
		}
		root := repository(t)
		apply(t)(initPlan(t, root))
		if read(t, root, ".claude/settings.json") != claudeSettingsFile {
			t.Fatal("the Claude Code settings changed")
		}
	})

	t.Run("hooks auto selects Claude Code and the harnesses the project shows signs of [verifies SP-243~1]", func(t *testing.T) {
		root := t.TempDir()
		names := func() []string {
			harnesses, err := SelectHarnesses(root, "auto")
			if err != nil {
				t.Fatal(err)
			}
			var flags []string
			for _, harness := range harnesses {
				flags = append(flags, harness.Flag)
			}
			return flags
		}
		if got := strings.Join(names(), ","); got != "claude" {
			t.Errorf("empty project: %s", got)
		}
		write(t, root, ".codex/config.toml", "")
		write(t, root, ".github/copilot-instructions.md", "")
		if got := strings.Join(names(), ","); got != "claude,codex,copilot" {
			t.Errorf("project with signs: %s", got)
		}
	})

	t.Run("hooks takes a list and refuses a harness the module has no files for [verifies SP-243~1]", func(t *testing.T) {
		harnesses, err := SelectHarnesses(t.TempDir(), "codex, copilot")
		if err != nil || len(harnesses) != 2 {
			t.Fatalf("harnesses = %v, %v", harnesses, err)
		}
		if _, err := SelectHarnesses(t.TempDir(), "claude,cursor"); !errors.Is(err, ErrUnknownHookHarness) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("a hooks file that is not a JSON object is refused rather than rewritten [verifies SP-243~1]", func(t *testing.T) {
		root := repository(t)
		write(t, root, ".codex/hooks.json", "[1, 2")
		if _, err := initPlan(t, root, hostharness.Codex()); err == nil {
			t.Fatal("a broken hooks file was planned over")
		}
	})
}

func TestEnableAndDisable(t *testing.T) {
	prepared := func(t *testing.T, shallnot string) string {
		t.Helper()
		root := repository(t)
		write(t, root, "shallnot.yaml", shallnot)
		apply(t)(initPlan(t, root, hostharness.Claude()))
		return root
	}
	enable := func(t *testing.T, root string) {
		t.Helper()
		apply(t)(BuildEnable(root))
	}
	disable := func(t *testing.T, root string) {
		t.Helper()
		apply(t)(BuildDisable(root))
	}

	t.Run("enable adds the proof to proofs.require at the end of a file that has no proofs block [verifies SP-244~1]", func(t *testing.T) {
		root := prepared(t, traceabilityOnly)
		enable(t, root)
		want := traceabilityOnly + "proofs:\n  require:\n    - discriminating-power\n"
		if got := read(t, root, "shallnot.yaml"); got != want {
			t.Fatalf("shallnot.yaml = %q", got)
		}
	})

	t.Run("enable turns the proof on in the configuration and writes its block of AGENTS.md beside Shallnot's [verifies SP-244~1]", func(t *testing.T) {
		root := prepared(t, traceabilityOnly)
		enable(t, root)
		if got := read(t, root, ".spec-proof/config.yaml"); !strings.Contains(got, "discriminating-power: true") || strings.Contains(got, "alignment: true") {
			t.Errorf("config = %q", got)
		}
		agents := read(t, root, "AGENTS.md")
		if !strings.HasPrefix(agents, shallnotAgents+"\n") || !strings.Contains(agents, "spec-proof-dp prove --changed") ||
			strings.Count(agents, "<!-- spec-proof-dp:begin -->") != 1 {
			t.Errorf("AGENTS.md = %q", agents)
		}
	})

	t.Run("enable extends an existing proofs block in each layout and disable restores every file byte for byte [verifies SP-244~1]", func(t *testing.T) {
		layouts := map[string]struct{ before, enabled string }{
			"a block without require": {
				"version: 1 # keep\nproofs:\n  trust: local\n  code:\n    - parcel/**\ntests:\n  - tests\n",
				"version: 1 # keep\nproofs:\n  trust: local\n  code:\n    - parcel/**\n  require:\n    - discriminating-power\ntests:\n  - tests\n",
			},
			"a block list": {
				"proofs:\n  require:\n    - alignment # first\n  trust: local\n",
				"proofs:\n  require:\n    - alignment # first\n    - discriminating-power\n  trust: local\n",
			},
			"a flow list": {
				"proofs:\n  require: [alignment]\n  trust: local\n",
				"proofs:\n  require: [alignment, discriminating-power]\n  trust: local\n",
			},
			"an empty value": {
				"proofs:\n  require:\n  trust: local\n",
				"proofs:\n  require:\n    - discriminating-power\n  trust: local\n",
			},
			"four-space indentation": {
				"proofs:\n    trust: local\n",
				"proofs:\n    trust: local\n    require:\n      - discriminating-power\n",
			},
			"no trailing newline": {
				"version: 1\nspecs: [specs]",
				"version: 1\nspecs: [specs]\nproofs:\n  require:\n    - discriminating-power\n",
			},
		}
		for name, layout := range layouts {
			root := prepared(t, layout.before)
			agentsBefore, configBefore := read(t, root, "AGENTS.md"), read(t, root, ".spec-proof/config.yaml")
			enable(t, root)
			if got := read(t, root, "shallnot.yaml"); got != layout.enabled {
				t.Errorf("%s: enabled = %q, want %q", name, got, layout.enabled)
			}
			disable(t, root)
			shallnotAfter := read(t, root, "shallnot.yaml")
			wantAfter := layout.before
			switch name {
			case "no trailing newline":
				wantAfter = "version: 1\nspecs: [specs]\n"
			case "an empty value":
				wantAfter = "proofs:\n  trust: local\n"
			}
			if shallnotAfter != wantAfter {
				t.Errorf("%s: disabled = %q, want %q", name, shallnotAfter, wantAfter)
			}
			if got := read(t, root, "AGENTS.md"); got != agentsBefore {
				t.Errorf("%s: AGENTS.md = %q, want %q", name, got, agentsBefore)
			}
			if got := read(t, root, ".spec-proof/config.yaml"); got != configBefore {
				t.Errorf("%s: config = %q, want %q", name, got, configBefore)
			}
		}
	})

	t.Run("enable and disable are idempotent [verifies SP-244~1]", func(t *testing.T) {
		root := prepared(t, traceabilityOnly)
		enable(t, root)
		if again, err := BuildEnable(root); err != nil || again.Pending() {
			t.Fatalf("second enable pending = %v, %v", again.Pending(), err)
		}
		disable(t, root)
		if again, err := BuildDisable(root); err != nil || again.Pending() {
			t.Fatalf("second disable pending = %v, %v", again.Pending(), err)
		}
	})

	t.Run("a project that has no shallnot.yaml cannot enable the proof [verifies SP-244~1]", func(t *testing.T) {
		if _, err := BuildEnable(t.TempDir()); !errors.Is(err, ErrNoShallnotConfig) {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("disable in a project that never enabled anything changes nothing [verifies SP-244~1]", func(t *testing.T) {
		root := repository(t)
		plan, err := BuildDisable(root)
		if err != nil || plan.Pending() {
			t.Fatalf("pending = %v, %v", plan.Pending(), err)
		}
	})

	t.Run("an AGENTS.md that held only the module's block is removed on disable and a file with other text keeps it [verifies SP-244~1]", func(t *testing.T) {
		root := t.TempDir()
		write(t, root, "shallnot.yaml", traceabilityOnly)
		apply(t)(BuildEnable(root))
		if read(t, root, "AGENTS.md") == "<absent>" {
			t.Fatal("AGENTS.md was not created")
		}
		apply(t)(BuildDisable(root))
		if got := read(t, root, "AGENTS.md"); got != "<absent>" {
			t.Fatalf("AGENTS.md = %q", got)
		}
	})

	t.Run("unbalanced markers, a proofs block in flow style and a non-boolean flag are refused [verifies SP-244~1]", func(t *testing.T) {
		root := prepared(t, traceabilityOnly)
		write(t, root, "AGENTS.md", "<!-- spec-proof-dp:begin -->\ntext\n")
		if _, err := BuildEnable(root); err == nil {
			t.Error("unbalanced markers were accepted")
		}
		root = prepared(t, "proofs: {trust: local}\n")
		if _, err := BuildEnable(root); !errors.Is(err, ErrUnsupportedYAML) {
			t.Errorf("flow mapping: %v", err)
		}
		root = prepared(t, traceabilityOnly)
		write(t, root, ".spec-proof/config.yaml", "proofs:\n  discriminating-power: maybe\n")
		if _, err := BuildEnable(root); !errors.Is(err, ErrUnsupportedYAML) {
			t.Errorf("non-boolean flag: %v", err)
		}
	})

	t.Run("the replacement of the module's block keeps the text around it [verifies SP-244~1]", func(t *testing.T) {
		current := []byte("before\n\n<!-- spec-proof-dp:begin -->\nold\n<!-- spec-proof-dp:end -->\nafter\n")
		updated, err := setAgentsBlock(current)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(string(updated), "before\n\n") || !strings.HasSuffix(string(updated), "\nafter\n") || strings.Contains(string(updated), "old") {
			t.Fatalf("updated = %q", updated)
		}
		removed, err := removeAgentsBlock(updated)
		if err != nil || string(removed) != "before\n\nafter\n" {
			t.Fatalf("removed = %q, %v", removed, err)
		}
	})
}
