package hostfiles_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/alignment/internal/domain/hostfiles"
	"github.com/RachidChabane/musthold/alignment/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

const (
	portableDirectory = "../../../plugin/portable"
	skillPath         = "skills/musthold-alignment-fulfil/SKILL.md"
)

func portableGenerated(t *testing.T) map[string]string {
	t.Helper()
	files, err := hostfiles.Portable()
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]string{}
	for _, file := range files {
		byPath[file.Path] = string(file.Content)
	}
	return byPath
}

func portableCommitted(t *testing.T) map[string]string {
	t.Helper()
	byPath := map[string]string{}
	err := filepath.WalkDir(portableDirectory, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(portableDirectory, path)
		byPath[filepath.ToSlash(relative)] = string(content)
		return err
	})
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return byPath
}

func TestPortablePlugin(t *testing.T) {
	t.Run("the committed portable tree is what the generator produces, byte for byte [verifies SP-1711~1]", func(t *testing.T) {
		want := portableGenerated(t)
		if *update {
			if err := os.RemoveAll(portableDirectory); err != nil {
				t.Fatal(err)
			}
			for path, content := range want {
				target := filepath.Join(portableDirectory, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
		got := portableCommitted(t)
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

	t.Run("the portable plugin holds the manifest, the server, the skill and the Copilot namespace [verifies SP-1711~1]", func(t *testing.T) {
		files := portableGenerated(t)
		var paths []string
		for path := range files {
			paths = append(paths, path)
		}
		slices.Sort(paths)
		want := []string{pluginfiles.PortableManifestPath, pluginfiles.PortableMCPPath, skillPath, pluginfiles.CopilotNamespace + "/hooks/hooks.json"}
		for _, agent := range hostharness.Identity.Agents {
			want = append(want, pluginfiles.CopilotNamespace+"/agents/musthold-alignment-"+string(agent)+".agent.md")
		}
		slices.Sort(want)
		if !slices.Equal(paths, want) {
			t.Fatalf("files = %v, want %v", paths, want)
		}
	})

	t.Run("the root server launches the module with no harness, so its answers are inline [verifies SP-1711~1]", func(t *testing.T) {
		servers := portableGenerated(t)[pluginfiles.PortableMCPPath]
		for _, fragment := range []string{
			`"$schema": "` + pluginfiles.MCPSchemaURL + `"`, `"musthold-alignment": {`, `"type": "stdio"`, `"command": "musthold-alignment"`,
			"\"mcp\",\n        \"--role\",\n        \"executor\"\n      ]",
		} {
			if !strings.Contains(servers, fragment) {
				t.Errorf("mcp.json lacks %q:\n%s", fragment, servers)
			}
		}
		if strings.Contains(servers, "--harness") {
			t.Errorf("the portable server names a harness:\n%s", servers)
		}
	})

	t.Run("the Copilot agents hold their own restricted server, named apart from the root server [verifies SP-1711~1]", func(t *testing.T) {
		files := portableGenerated(t)
		for _, agent := range hostharness.Identity.Agents {
			content := files[pluginfiles.CopilotNamespace+"/agents/musthold-alignment-"+string(agent)+".agent.md"]
			for _, fragment := range []string{"musthold-alignment-exec:", "--harness", "tools: [\"musthold-alignment-exec/fetch_request\", \"musthold-alignment-exec/submit_response\"]"} {
				if !strings.Contains(content, fragment) {
					t.Errorf("%s agent lacks %q:\n%s", agent, fragment, content)
				}
			}
		}
	})

	t.Run("the skill is an Agent Skill that answers through the subagents when they exist and inline otherwise [verifies SP-1711~1]", func(t *testing.T) {
		skill := portableGenerated(t)[skillPath]
		if strings.Contains(skill, "allowed-tools") || !strings.HasPrefix(skill, "---\nname: musthold-alignment-fulfil\n") {
			t.Errorf("front matter:\n%s", skill)
		}
		for _, fragment := range []string{"`fetch_request`", "`submit_response`", "block power only", "musthold-s1 drain --module alignment"} {
			if !strings.Contains(skill, fragment) {
				t.Errorf("skill lacks %q", fragment)
			}
		}
	})
}
