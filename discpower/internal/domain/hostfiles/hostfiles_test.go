package hostfiles_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostfiles"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/format/hostproto"
)

// update rewrites the committed trees from the generator.
var update = flag.Bool("update", false, "rewrite the committed plugin trees from the generator")

const (
	pluginDirectory  = "../../../plugin"
	frontMatterFence = "---\n"
)

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

	t.Run("a harness without files is refused [verifies SP-241~1]", func(t *testing.T) {
		if _, err := hostfiles.For(hostharness.Harness{Flag: "vim"}); err == nil {
			t.Fatal("files were generated for an unknown harness")
		}
	})
}

// frontMatter splits a Markdown file into its YAML front matter and its body.
func frontMatter(t *testing.T, text string) (map[string]any, string) {
	t.Helper()
	rest, found := strings.CutPrefix(text, frontMatterFence)
	if !found {
		t.Fatalf("no front matter in %q", text)
	}
	header, body, found := strings.Cut(rest, "\n"+frontMatterFence)
	if !found {
		t.Fatalf("front matter is not closed in %q", text)
	}
	var members map[string]any
	if err := yaml.Unmarshal([]byte(header), &members); err != nil {
		t.Fatalf("front matter: %v\n%s", err, header)
	}
	return members, body
}

func decodeJSON(t *testing.T, text string, into any) {
	t.Helper()
	decoder := json.NewDecoder(strings.NewReader(text))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(into); err != nil {
		t.Fatalf("%v\n%s", err, text)
	}
}

func stringList(t *testing.T, value any) []string {
	t.Helper()
	items, ok := value.([]any)
	if !ok {
		t.Fatalf("%v is not a list", value)
	}
	var list []string
	for _, item := range items {
		text, ok := item.(string)
		if !ok {
			t.Fatalf("%v is not a string", item)
		}
		list = append(list, text)
	}
	return list
}

const executorInstructions = "Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt."

func TestClaudePlugin(t *testing.T) {
	claude := hostharness.Claude()
	files := generated(t, claude)

	t.Run("the plugin lists exactly the files of the documented layout [verifies SP-242~1]", func(t *testing.T) {
		want := []string{
			".claude-plugin/plugin.json", ".mcp.json", "agents/decider.md", "agents/generator.md", "agents/judge.md",
			"hooks/hooks.json", "skills/spec-proof-dp-fulfil/SKILL.md",
		}
		var got []string
		for path := range files {
			got = append(got, path)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("files = %v", got)
		}
	})

	t.Run("the manifest names the plugin and the server file starts its own executor server [verifies SP-242~1]", func(t *testing.T) {
		var manifest struct {
			Name, Version, Description, License string
			Author                              map[string]string
		}
		decodeJSON(t, files[".claude-plugin/plugin.json"], &manifest)
		if manifest.Name != hostharness.PluginName || !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(manifest.Version) || manifest.Description == "" {
			t.Fatalf("manifest = %+v", manifest)
		}
		var servers struct {
			Servers map[string]struct {
				Command string   `json:"command"`
				Args    []string `json:"args"`
			} `json:"mcpServers"`
		}
		decodeJSON(t, files[".mcp.json"], &servers)
		server, found := servers.Servers[hostharness.ExecutorServer]
		if !found || len(servers.Servers) != 1 || server.Command != hostfiles.Binary ||
			!slices.Equal(server.Args, []string{"mcp", "--role", "executor", "--harness", "claude"}) {
			t.Fatalf("servers = %+v", servers)
		}
	})

	t.Run("each agent holds exactly the two executor tools, inherits the model and keeps its context blind [verifies SP-242~1]", func(t *testing.T) {
		wantTools := "mcp__plugin_spec-proof-dp_exec__fetch_request, mcp__plugin_spec-proof-dp_exec__submit_response"
		for _, agent := range hostproto.Agents() {
			members, body := frontMatter(t, files["agents/"+string(agent)+".md"])
			if members["name"] != string(agent) || members["tools"] != wantTools || members["model"] != "inherit" ||
				members["omitClaudeMd"] != true || members["maxTurns"] != 4 {
				t.Errorf("%s: members = %v", agent, members)
			}
			if description, _ := members["description"].(string); description == "" {
				t.Errorf("%s has no description", agent)
			}
			allowed := []string{"name", "description", "tools", "model", "omitClaudeMd", "maxTurns"}
			for key := range members {
				if !slices.Contains(allowed, key) {
					t.Errorf("%s: unexpected member %q", agent, key)
				}
			}
			if strings.TrimSpace(body) != executorInstructions {
				t.Errorf("%s: body = %q", agent, body)
			}
		}
	})

	t.Run("the tool names an agent holds are the names the hooks recognize [verifies SP-242~1]", func(t *testing.T) {
		for _, name := range claude.ToolNames() {
			if _, ok := claude.ExecutorTool(name); !ok {
				t.Errorf("%s is not recognized", name)
			}
			parsed, ok := hostproto.ParseMCPToolName(name)
			if !ok {
				t.Fatalf("%s is not an MCP tool name", name)
			}
			plugin, server, ok := hostproto.ParsePluginServer(parsed.Server)
			if !ok || plugin != hostharness.PluginName || server != hostharness.ExecutorServer {
				t.Errorf("%s: server %q parses as %q, %q", name, parsed.Server, plugin, server)
			}
		}
	})

	t.Run("the hooks file runs the four hooks of the module under matchers that select the executor alone [verifies SP-242~1]", func(t *testing.T) {
		var parsed struct {
			Hooks map[string][]struct {
				Matcher string `json:"matcher"`
				Hooks   []struct {
					Type    string `json:"type"`
					Command string `json:"command"`
					Timeout int    `json:"timeout"`
				} `json:"hooks"`
			} `json:"hooks"`
		}
		decodeJSON(t, files["hooks/hooks.json"], &parsed)
		wantCommands := map[string]hostproto.HookKind{
			"PreToolUse": hostproto.HookPreTool, "SubagentStart": hostproto.HookSubagentStart,
			"SubagentStop": hostproto.HookSubagentStop, "PostToolUse": hostproto.HookPostTool,
		}
		if len(parsed.Hooks) != len(wantCommands) {
			t.Fatalf("events = %v", parsed.Hooks)
		}
		for event, kind := range wantCommands {
			groups := parsed.Hooks[event]
			if len(groups) != 1 || len(groups[0].Hooks) != 1 {
				t.Fatalf("%s: groups = %+v", event, groups)
			}
			hook := groups[0].Hooks[0]
			if hook.Type != "command" || hook.Command != "spec-proof-dp hook "+string(kind)+" --harness claude" || hook.Timeout <= 0 {
				t.Errorf("%s: hook = %+v", event, hook)
			}
		}
		matches := func(event, subject string) bool {
			return regexp.MustCompile(parsed.Hooks[event][0].Matcher).MatchString(subject)
		}
		for _, subject := range []string{"Agent", "mcp__plugin_spec-proof-dp_exec__fetch_request", "mcp__plugin_spec-proof-dp_exec__submit_response"} {
			if !matches("PreToolUse", subject) {
				t.Errorf("PreToolUse does not match %s", subject)
			}
		}
		if matches("PreToolUse", "Bash") || matches("PreToolUse", "mcp__plugin_other_exec__fetch_request") {
			t.Error("PreToolUse matches a tool that is not the module's")
		}
		for _, event := range []string{"SubagentStart", "SubagentStop"} {
			for _, role := range []string{"spec-proof-dp:generator", "spec-proof-dp:judge", "spec-proof-dp:decider"} {
				if !matches(event, role) {
					t.Errorf("%s does not match %s", event, role)
				}
			}
			for _, other := range []string{"spec-proof-dp:boss", "xspec-proof-dp:judge", "spec-proof-dp:judgey", "general-purpose", "other:judge"} {
				if matches(event, other) {
					t.Errorf("%s matches %s", event, other)
				}
			}
		}
		if parsed.Hooks["PostToolUse"][0].Matcher != "Agent" {
			t.Errorf("PostToolUse matcher = %q", parsed.Hooks["PostToolUse"][0].Matcher)
		}
	})

	t.Run("the skill pre-approves what its loop runs and states the loop [verifies SP-242~1]", func(t *testing.T) {
		members, body := frontMatter(t, files["skills/spec-proof-dp-fulfil/SKILL.md"])
		allowed, _ := members["allowed-tools"].(string)
		for _, rule := range []string{"Bash(spec-proof-dp prove *)", "Bash(spec-proof-dp request list)", "Bash(spec-proof-s1 drain *)"} {
			if !strings.Contains(allowed, rule) {
				t.Errorf("allowed-tools lacks %s: %q", rule, allowed)
			}
		}
		if members["name"] != "spec-proof-dp-fulfil" || members["description"] == "" {
			t.Errorf("members = %v", members)
		}
		for _, step := range []string{"spec-proof-s1 drain --module dp", "spec-proof-dp request list", "`Agent`", "ticket <ticket>", "spec-proof-dp prove --resume", "exit code 3", "host_unresponsive", "at most 4"} {
			if !strings.Contains(body, step) {
				t.Errorf("the skill does not mention %q", step)
			}
		}
	})
}

// tomlTable parses the subset of TOML the Codex agent files use: string and
// string-array values, and one level of tables.
func tomlTable(t *testing.T, text string) map[string]map[string]any {
	t.Helper()
	tables := map[string]map[string]any{"": {}}
	current := ""
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]"):
			current = strings.TrimSuffix(strings.TrimPrefix(line, "["), "]")
			tables[current] = map[string]any{}
		default:
			key, value, found := strings.Cut(line, " = ")
			if !found {
				t.Fatalf("not a key and value: %q", line)
			}
			var decoded any
			if err := json.Unmarshal([]byte(value), &decoded); err != nil {
				t.Fatalf("value of %s is not a string or an array of strings: %s", key, value)
			}
			tables[current][key] = decoded
		}
	}
	return tables
}

func TestCodexFiles(t *testing.T) {
	codex := hostharness.Codex()
	files := generated(t, codex)

	t.Run("each agent is a read-only custom agent that carries only the executor server [verifies SP-242~1]", func(t *testing.T) {
		for _, agent := range hostproto.Agents() {
			name := "spec-proof-dp-" + string(agent)
			tables := tomlTable(t, files[".codex/agents/"+name+".toml"])
			root, server := tables[""], tables["mcp_servers.spec-proof-dp-exec"]
			if root["name"] != name || root["sandbox_mode"] != "read-only" || root["developer_instructions"] != executorInstructions || root["description"] == "" {
				t.Errorf("%s: root = %v", agent, root)
			}
			if server["command"] != "spec-proof-dp" || !slices.Equal(stringList(t, server["args"]), []string{"mcp", "--role", "executor", "--harness", "codex"}) || len(tables) != 2 {
				t.Errorf("%s: tables = %v", agent, tables)
			}
		}
	})

	t.Run("the hook entries are the three hooks Codex can serve, under matchers on its executor server [verifies SP-242~1]", func(t *testing.T) {
		var parsed snakeHooksFile
		decodeJSON(t, files[".codex/hooks.json"], &parsed)
		want := map[string]string{
			"PreToolUse": "spec-proof-dp hook pre-tool --harness codex", "SubagentStart": "spec-proof-dp hook subagent-start --harness codex",
			"SubagentStop": "spec-proof-dp hook subagent-stop --harness codex",
		}
		if len(parsed.Hooks) != len(want) {
			t.Fatalf("events = %v", parsed.Hooks)
		}
		for event, command := range want {
			if groups := parsed.Hooks[event]; len(groups) != 1 || len(groups[0].Hooks) != 1 || groups[0].Hooks[0].Command != command {
				t.Errorf("%s: %+v", event, groups)
			}
		}
		matcher := regexp.MustCompile(parsed.Hooks["PreToolUse"][0].Matcher)
		for _, name := range codex.ToolNames() {
			if !matcher.MatchString(name) {
				t.Errorf("the matcher does not select %s", name)
			}
		}
		if matcher.MatchString("Bash") {
			t.Error("the matcher selects Bash")
		}
	})

	t.Run("the skill is a project skill that delegates by agent name [verifies SP-242~1]", func(t *testing.T) {
		members, body := frontMatter(t, files[".agents/skills/spec-proof-dp-fulfil/SKILL.md"])
		if members["name"] != "spec-proof-dp-fulfil" || members["allowed-tools"] != nil {
			t.Errorf("members = %v", members)
		}
		if !strings.Contains(body, "`spec-proof-dp-judge`") || !strings.Contains(body, "spec-proof-dp prove --resume") {
			t.Errorf("body = %s", body)
		}
	})
}

type snakeHooksFile struct {
	Hooks map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	} `json:"hooks"`
}

func TestCopilotFiles(t *testing.T) {
	copilot := hostharness.Copilot()
	files := generated(t, copilot)

	t.Run("each agent names the server and tool pairs and starts its own server [verifies SP-242~1]", func(t *testing.T) {
		for _, agent := range hostproto.Agents() {
			members, body := frontMatter(t, files[".github/agents/spec-proof-dp-"+string(agent)+".agent.md"])
			tools := stringList(t, members["tools"])
			if !slices.Equal(tools, []string{"spec-proof-dp-exec/fetch_request", "spec-proof-dp-exec/submit_response"}) {
				t.Errorf("%s: tools = %v", agent, tools)
			}
			servers, _ := members["mcp-servers"].(map[string]any)
			server, _ := servers["spec-proof-dp-exec"].(map[string]any)
			if len(servers) != 1 || server["command"] != "spec-proof-dp" || server["type"] != "local" ||
				!slices.Equal(stringList(t, server["args"]), []string{"mcp", "--role", "executor", "--harness", "copilot"}) {
				t.Errorf("%s: mcp-servers = %v", agent, servers)
			}
			if strings.TrimSpace(body) != executorInstructions {
				t.Errorf("%s: body = %q", agent, body)
			}
		}
	})

	t.Run("the hooks file serves the pre-tool and the stop checks in the Copilot layout [verifies SP-242~1]", func(t *testing.T) {
		var parsed struct {
			Version int `json:"version"`
			Hooks   map[string][]struct {
				Type       string `json:"type"`
				Bash       string `json:"bash"`
				PowerShell string `json:"powershell"`
				TimeoutSec int    `json:"timeoutSec"`
			} `json:"hooks"`
		}
		decodeJSON(t, files[".github/hooks/spec-proof-dp.json"], &parsed)
		want := map[string]string{"preToolUse": "spec-proof-dp hook pre-tool --harness copilot", "subagentStop": "spec-proof-dp hook subagent-stop --harness copilot"}
		if parsed.Version != 1 || len(parsed.Hooks) != len(want) {
			t.Fatalf("hooks = %+v", parsed)
		}
		for event, command := range want {
			entries := parsed.Hooks[event]
			if len(entries) != 1 || entries[0].Bash != command || entries[0].PowerShell != command || entries[0].Type != "command" || entries[0].TimeoutSec <= 0 {
				t.Errorf("%s: %+v", event, entries)
			}
		}
	})

	t.Run("the skill is a project skill that delegates by agent name [verifies SP-242~1]", func(t *testing.T) {
		members, body := frontMatter(t, files[".github/skills/spec-proof-dp-fulfil/SKILL.md"])
		if members["name"] != "spec-proof-dp-fulfil" || !strings.Contains(body, "`spec-proof-dp-judge`") {
			t.Errorf("members = %v, body = %s", members, body)
		}
	})
}

func TestGeneratedTextIsPlainShippedContent(t *testing.T) {
	t.Run("no generated file talks about its own authoring, its verification state or its history [verifies SP-242~1]", func(t *testing.T) {
		forbidden := regexp.MustCompile(`(?i)\b(unverified|canary|ADR|previously|no longer|currently|this version|TODO)\b`)
		for _, harness := range hostharness.All() {
			for path, content := range generated(t, harness) {
				if found := forbidden.FindString(content); found != "" {
					t.Errorf("%s/%s says %q", harness.Flag, path, found)
				}
			}
		}
	})

	t.Run("every generated file ends with a newline and has no trailing blanks [verifies SP-242~1]", func(t *testing.T) {
		for _, harness := range hostharness.All() {
			for path, content := range generated(t, harness) {
				if !strings.HasSuffix(content, "\n") || bytes.Contains([]byte(content), []byte(" \n")) {
					t.Errorf("%s/%s is not tidy", harness.Flag, path)
				}
			}
		}
	})

	t.Run("a hook command names the marker the installer recognizes it by [verifies SP-242~1]", func(t *testing.T) {
		for _, harness := range hostharness.All() {
			for _, kind := range hostproto.HookKinds() {
				if command := hostfiles.HookCommand(kind, harness); !strings.HasPrefix(command, hostfiles.HookCommandMarker) {
					t.Errorf("%s does not start with %q", command, hostfiles.HookCommandMarker)
				}
			}
		}
	})
}
