package pluginfiles_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

func portableOf(t *testing.T, module pluginfiles.Module) map[string]string {
	t.Helper()
	files, err := module.Portable()
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

func TestPortablePlugin(t *testing.T) {
	pluginSchema, mcpSchema := schemaOf(t, "plugin.schema.json"), schemaOf(t, "mcp.schema.json")
	for _, sample := range samples() {
		module := sample.module
		files := portableOf(t, module)

		t.Run(sample.name+": the generated manifest and server file are valid against the official schemas [verifies SP-1702~1]", func(t *testing.T) {
			if err := validate(t, pluginSchema, []byte(files[pluginfiles.PortableManifestPath])); err != nil {
				t.Errorf("plugin.json: %v", err)
			}
			if err := validate(t, mcpSchema, []byte(files[pluginfiles.PortableMCPPath])); err != nil {
				t.Errorf("mcp.json: %v", err)
			}
		})

		t.Run(sample.name+": the generated skill is an Agent Skill whose name is its directory [verifies SP-1702~1]", func(t *testing.T) {
			path := pluginfiles.PortableSkillPath(module.SkillName())
			if problems := skillProblems(files[path], module.SkillName()); len(problems) != 0 {
				t.Errorf("%s: %v", path, problems)
			}
		})

		t.Run(sample.name+": the skill starts the executor through the subagent tool with the ticket as the whole prompt and keeps the inline branch [verifies SP-1702~1]", func(t *testing.T) {
			body := files[pluginfiles.PortableSkillPath(module.SkillName())]
			for _, want := range []string{"`task` tool with `agent_type`", "`ticket <ticket>` as the whole prompt and no conversation history", "Otherwise answer inline", "`fetch_request`", "`submit_response`"} {
				if !strings.Contains(body, want) {
					t.Errorf("the skill lacks %q:\n%s", want, body)
				}
			}
		})

		t.Run(sample.name+": the server carries no harness, the Copilot agents carry theirs [verifies SP-1702~1]", func(t *testing.T) {
			if strings.Contains(files[pluginfiles.PortableMCPPath], "--harness") {
				t.Errorf("the portable server names a harness:\n%s", files[pluginfiles.PortableMCPPath])
			}
			var agents []string
			for path, content := range files {
				if strings.HasPrefix(path, pluginfiles.CopilotNamespace+"/agents/") {
					agents = append(agents, path)
					if !strings.Contains(content, "--harness") || !strings.Contains(content, harness.FlagCopilot) {
						t.Errorf("%s does not launch its server for Copilot:\n%s", path, content)
					}
				}
			}
			if len(agents) != len(module.Identity.Agents) {
				t.Errorf("agents = %v", agents)
			}
			if !slices.Contains(slicesOfKeys(files), pluginfiles.CopilotNamespace+"/hooks/hooks.json") {
				t.Errorf("no Copilot hooks file")
			}
		})

		t.Run(sample.name+": generating twice gives the same bytes [verifies SP-1702~1]", func(t *testing.T) {
			again := portableOf(t, module)
			for path, content := range files {
				if again[path] != content {
					t.Errorf("%s differs between two runs", path)
				}
			}
		})
	}

	t.Run("a server list of the standard may be empty and then holds no server [verifies SP-1702~1]", func(t *testing.T) {
		file, err := pluginfiles.MCPFile(nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := validate(t, mcpSchema, file.Content); err != nil || !strings.Contains(string(file.Content), `"mcpServers": {}`) {
			t.Errorf("%s: %v", file.Content, err)
		}
	})
}

func slicesOfKeys(files map[string]string) []string {
	keys := make([]string, 0, len(files))
	for path := range files {
		keys = append(keys, path)
	}
	return keys
}
