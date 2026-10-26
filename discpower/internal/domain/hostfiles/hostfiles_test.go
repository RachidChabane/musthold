package hostfiles_test

import (
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/domain/hostfiles"
	"github.com/RachidChabane/musthold/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format/hostproto"
)

// update rewrites the committed trees from the generator.
var update = flag.Bool("update", false, "rewrite the committed plugin trees from the generator")

const pluginDirectory = "../../../plugin"

func generated(t *testing.T, harness hostharness.Harness) map[string]string {
	t.Helper()
	files, err := hostfiles.For(harness)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		if _, duplicate := byPath[file.Path]; duplicate {
			t.Fatalf("%s is generated twice", file.Path)
		}
		byPath[file.Path] = string(file.Content)
	}
	return byPath
}

// committed reads the tree committed for a harness.
func committed(t *testing.T, harness hostharness.Harness) map[string]string {
	t.Helper()
	root := filepath.Join(pluginDirectory, harness.Flag)
	byPath := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		byPath[filepath.ToSlash(relative)] = string(content)
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return byPath
}

func TestCommittedFilesAreGenerated(t *testing.T) {
	for _, harness := range hostharness.All() {
		if harness.Flag == hostharness.FlagCopilot {
			continue // Copilot loads the portable plugin, whose tree TestPortablePlugin compares.
		}
		t.Run(harness.Flag+" tree is what the generator produces, byte for byte [verifies SP-241~1]", func(t *testing.T) {
			want := generated(t, harness)
			if *update {
				root := filepath.Join(pluginDirectory, harness.Flag)
				if err := os.RemoveAll(root); err != nil {
					t.Fatal(err)
				}
				for path, content := range want {
					target := filepath.Join(root, filepath.FromSlash(path))
					if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
						t.Fatal(err)
					}
				}
			}
			got := committed(t, harness)
			for path, content := range want {
				if got[path] != content {
					t.Errorf("%s differs from the generator: run script/plugin-gen", path)
				}
			}
			for path := range got {
				if _, expected := want[path]; !expected {
					t.Errorf("%s is committed but not generated", path)
				}
			}
		})
	}

	t.Run("generating twice gives the same bytes [verifies SP-241~1]", func(t *testing.T) {
		for _, harness := range hostharness.All() {
			first, second := generated(t, harness), generated(t, harness)
			for path, content := range first {
				if second[path] != content {
					t.Errorf("%s: %s differs between two runs", harness.Flag, path)
				}
			}
		}
	})
}

func TestTheModulesOwnFiles(t *testing.T) {
	t.Run("the plugin is musthold-dp with its three executor agents, its server and its skill [verifies SP-242~1]", func(t *testing.T) {
		files := generated(t, hostharness.Claude())
		var paths []string
		for path := range files {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		want := []string{
			".claude-plugin/plugin.json", ".mcp.json", "agents/decider.md", "agents/generator.md", "agents/judge.md",
			"hooks/hooks.json", "hooks/register.ts", "skills/musthold-dp-fulfil/SKILL.md",
		}
		if !slices.Equal(paths, want) {
			t.Fatalf("files = %v", paths)
		}
		for _, agent := range hostproto.Agents() {
			agentFile := files["agents/"+string(agent)+".md"]
			if !strings.Contains(agentFile, "tools: mcp__plugin_musthold-dp_exec__fetch_request, mcp__plugin_musthold-dp_exec__submit_response\n") {
				t.Errorf("%s: %s", agent, agentFile)
			}
		}
		if !strings.Contains(files[".mcp.json"], `"command": "musthold-dp"`) || !strings.Contains(files["skills/musthold-dp-fulfil/SKILL.md"], "musthold-s1 drain --module dp") {
			t.Fatalf("server or skill: %s %s", files[".mcp.json"], files["skills/musthold-dp-fulfil/SKILL.md"])
		}
	})

	t.Run("Codex and Copilot ship an agent file per executor agent of the module [verifies SP-242~1]", func(t *testing.T) {
		for flag, prefix := range map[string]string{hostharness.FlagCodex: ".codex/agents/musthold-dp-", hostharness.FlagCopilot: ".github/agents/musthold-dp-"} {
			harness, err := hostharness.ByFlag(flag)
			if err != nil {
				t.Fatal(err)
			}
			files := generated(t, harness)
			for _, agent := range hostproto.Agents() {
				found := false
				for path := range files {
					found = found || strings.HasPrefix(path, prefix+string(agent)+".")
				}
				if !found {
					t.Errorf("%s: no file for the %s agent", flag, agent)
				}
			}
		}
	})
}
