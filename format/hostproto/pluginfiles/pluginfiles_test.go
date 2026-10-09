package pluginfiles_test

import (
	"bytes"
	"errors"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

const (
	verifiesShipped    = " [verifies SP-711~3]"
	verifiesGeneration = " [verifies SP-712~1]"
	versionPattern     = `^\d+\.\d+\.\d+$`
	hookTypeCommand    = "command"
)

func TestModuleNames(t *testing.T) {
	for _, s := range samples() {
		t.Run(s.name+": the names derive from the binary and the plugin"+verifiesShipped, func(t *testing.T) {
			module := s.module
			relay := strings.TrimPrefix(module.Binary, "spec-proof-")
			if module.Plugin() != module.Identity.Plugin || module.SkillName() != module.Plugin()+"-fulfil" ||
				module.RelayName() != relay || module.HookCommandMarker() != module.Binary+" hook " {
				t.Errorf("names = %q, %q, %q, %q", module.Plugin(), module.SkillName(), module.RelayName(), module.HookCommandMarker())
			}
		})
	}
}

func TestClaudePlugin(t *testing.T) {
	for _, s := range samples() {
		module := s.module
		claude := harness.Claude(module.Identity)
		files := generated(t, module, claude)

		t.Run(s.name+": the plugin lists exactly the files of the documented layout"+verifiesShipped, func(t *testing.T) {
			want := []string{".claude-plugin/plugin.json", ".mcp.json", "hooks/hooks.json", pluginfiles.ClaudeModPath, skillPath(claude.Flag, module)}
			for _, agent := range module.Identity.Agents {
				want = append(want, agentPath(claude.Flag, module, agent))
			}
			slices.Sort(want)
			if got := sortedPaths(files); !slices.Equal(got, want) {
				t.Fatalf("files = %v, want %v", got, want)
			}
		})

		t.Run(s.name+": the manifest names the plugin and the server file starts its own executor server"+verifiesShipped, func(t *testing.T) {
			var manifest struct {
				Name, Version, Description, License string
				Author                              map[string]string
			}
			decodeJSON(t, files[".claude-plugin/plugin.json"], &manifest)
			if manifest.Name != module.Plugin() || manifest.Version != module.Version ||
				!regexp.MustCompile(versionPattern).MatchString(manifest.Version) ||
				!strings.Contains(manifest.Description, module.ProofPhrase) || !strings.Contains(manifest.Description, module.Plugin()) {
				t.Fatalf("manifest = %+v", manifest)
			}
			var servers struct {
				Servers map[string]struct {
					Command string   `json:"command"`
					Args    []string `json:"args"`
				} `json:"mcpServers"`
			}
			decodeJSON(t, files[".mcp.json"], &servers)
			server, found := servers.Servers[harness.ExecutorServer]
			if !found || len(servers.Servers) != 1 || server.Command != module.Binary ||
				!slices.Equal(server.Args, []string{"mcp", "--role", "executor", "--harness", "claude"}) {
				t.Fatalf("servers = %+v", servers)
			}
		})

		t.Run(s.name+": each agent holds exactly the two executor tools, inherits the model and keeps its context blind"+verifiesShipped, func(t *testing.T) {
			wantTools := strings.Join(claude.ToolNames(), ", ")
			for _, agent := range module.Identity.Agents {
				members, body := frontMatter(t, files[agentPath(claude.Flag, module, agent)])
				if members["name"] != string(agent) || members["tools"] != wantTools || members["model"] != "inherit" ||
					members["omitClaudeMd"] != true || members["maxTurns"] != 4 {
					t.Errorf("%s: members = %v", agent, members)
				}
				if description, _ := members["description"].(string); !strings.Contains(description, module.Plugin()) || !strings.Contains(description, module.SkillName()) {
					t.Errorf("%s: description = %q", agent, members["description"])
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

		t.Run(s.name+": the tool names an agent holds are the names the hooks recognize"+verifiesShipped, func(t *testing.T) {
			for _, name := range claude.ToolNames() {
				if _, ok := claude.ExecutorTool(name); !ok {
					t.Errorf("%s is not recognized", name)
				}
				parsed, ok := hostproto.ParseMCPToolName(name)
				if !ok {
					t.Fatalf("%s is not an MCP tool name", name)
				}
				plugin, server, ok := hostproto.ParsePluginServer(parsed.Server)
				if !ok || plugin != module.Plugin() || server != harness.ExecutorServer {
					t.Errorf("%s: server %q parses as %q, %q", name, parsed.Server, plugin, server)
				}
			}
		})

		t.Run(s.name+": the hooks file runs the four hooks of the module under matchers that select the executor alone"+verifiesShipped, func(t *testing.T) {
			var parsed snakeHooksFile
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
				if hook.Type != hookTypeCommand || hook.Command != hookCommand(module, kind, claude.Flag) || hook.Timeout <= 0 {
					t.Errorf("%s: hook = %+v", event, hook)
				}
			}
			matches := func(event, subject string) bool {
				return regexp.MustCompile(parsed.Hooks[event][0].Matcher).MatchString(subject)
			}
			subjects := append([]string{hostproto.ToolAgent}, claude.ToolNames()...)
			for _, subject := range subjects {
				if !matches("PreToolUse", subject) {
					t.Errorf("PreToolUse does not match %s", subject)
				}
			}
			otherTools := harness.Claude(harness.Identity{Plugin: otherPlugin, Agents: module.Identity.Agents}).ToolNames()
			if matches("PreToolUse", "Bash") || matches("PreToolUse", otherTools[0]) {
				t.Error("PreToolUse matches a tool that is not the module's")
			}
			for _, event := range []string{"SubagentStart", "SubagentStop"} {
				for _, agentType := range agentTypes(t, module, claude, files) {
					if !matches(event, agentType) {
						t.Errorf("%s does not match %s", event, agentType)
					}
				}
				for _, other := range []string{
					module.Plugin() + ":boss", "x" + module.Plugin() + ":" + string(delegationExample(module)),
					module.Plugin() + ":" + string(delegationExample(module)) + "y", "general-purpose", otherPlugin + ":" + string(delegationExample(module)),
				} {
					if matches(event, other) {
						t.Errorf("%s matches %s", event, other)
					}
				}
			}
			if parsed.Hooks["PostToolUse"][0].Matcher != hostproto.ToolAgent {
				t.Errorf("PostToolUse matcher = %q", parsed.Hooks["PostToolUse"][0].Matcher)
			}
		})

		t.Run(s.name+": the skill pre-approves what its loop runs and states the loop"+verifiesShipped, func(t *testing.T) {
			members, body := frontMatter(t, files[skillPath(claude.Flag, module)])
			allowed, _ := members["allowed-tools"].(string)
			for _, rule := range []string{
				"Bash(" + module.Binary + " prove *)", "Bash(" + module.Binary + " request list)", "Bash(" + pluginfiles.RelayBinary + " drain *)",
				"Bash(spec-proof-verify --accept-local .spec-proof/cache --out .spec-proof/verified)",
			} {
				if !strings.Contains(allowed, rule) {
					t.Errorf("allowed-tools lacks %s: %q", rule, allowed)
				}
			}
			description, _ := members["description"].(string)
			if members["name"] != module.SkillName() || !strings.Contains(description, module.Subject) || !strings.Contains(description, module.Plugin()) {
				t.Errorf("members = %v", members)
			}
			for _, step := range []string{
				pluginfiles.RelayBinary + " drain --module " + module.RelayName(), module.Binary + " request list", "`Agent`", "ticket <ticket>",
				module.Binary + " prove --resume", "exit code 3", "host_unresponsive", "at most 4", "`" + pluginfiles.VerifyLocalCommand + "`",
			} {
				if !strings.Contains(body, step) {
					t.Errorf("the skill does not mention %q", step)
				}
			}
		})
	}
}

func TestCodexFiles(t *testing.T) {
	for _, s := range samples() {
		module := s.module
		codex := harness.Codex(module.Identity)
		files := generated(t, module, codex)

		t.Run(s.name+": the files are the agents, the skill, the hooks and the server of the module"+verifiesShipped, func(t *testing.T) {
			want := []string{skillPath(codex.Flag, module), pluginfiles.CodexHooksPath, pluginfiles.CodexConfigPath}
			for _, agent := range module.Identity.Agents {
				want = append(want, agentPath(codex.Flag, module, agent))
			}
			slices.Sort(want)
			if got := sortedPaths(files); !slices.Equal(got, want) {
				t.Fatalf("files = %v, want %v", got, want)
			}
		})

		t.Run(s.name+": each agent is a read-only custom agent that carries no server"+verifiesShipped, func(t *testing.T) {
			for _, agent := range module.Identity.Agents {
				name := module.Plugin() + "-" + string(agent)
				tables := tomlTable(t, files[agentPath(codex.Flag, module, agent)])
				root := tables[""]
				if root["name"] != name || root["sandbox_mode"] != "read-only" || root["developer_instructions"] != codexInstructions(codex) || root["description"] == "" {
					t.Errorf("%s: root = %v", agent, root)
				}
				if len(tables) != 1 {
					t.Errorf("%s: tables = %v", agent, tables)
				}
			}
		})

		t.Run(s.name+": the instructions of a Codex executor name its two tools as Codex spells them for the module and forbid skills and the shell [verifies SP-723~1]", func(t *testing.T) {
			for _, agent := range module.Identity.Agents {
				instructions := tomlTable(t, files[agentPath(codex.Flag, module, agent)])[""]["developer_instructions"]
				text, _ := instructions.(string)
				fetch, submit := codex.ToolName(hostproto.ToolFetchRequest), codex.ToolName(hostproto.ToolSubmitResponse)
				fetchAt, submitAt := strings.Index(text, "`"+fetch+"`"), strings.Index(text, "`"+submit+"`")
				if fetchAt < 0 || submitAt < fetchAt {
					t.Errorf("%s: the instructions do not name %s then %s: %q", agent, fetch, submit, text)
				}
				for _, phrase := range []string{"the ticket from your task", "an empty `cap`", "the nonce from the fetch", "reply only with the receipt", "Use no skill and no shell command"} {
					if !strings.Contains(text, phrase) {
						t.Errorf("%s: the instructions lack %q: %q", agent, phrase, text)
					}
				}
			}
		})

		t.Run(s.name+": the project's config.toml starts the executor server of the module between its markers, with its tools approved"+verifiesShipped, func(t *testing.T) {
			text := files[pluginfiles.CodexConfigPath]
			if !strings.HasPrefix(text, module.CodexConfigBegin()+"\n") || !strings.HasSuffix(text, module.CodexConfigEnd()+"\n") {
				t.Fatalf("the markers do not delimit the file:\n%s", text)
			}
			tables := tomlTable(t, text)
			server := tables["mcp_servers."+codex.Server()]
			if server["command"] != module.Binary || !slices.Equal(stringList(t, server["args"]), []string{"mcp", "--role", "executor", "--harness", "codex"}) ||
				server["default_tools_approval_mode"] != "approve" || len(tables) != 2 {
				t.Errorf("tables = %v", tables)
			}
		})

		t.Run(s.name+": the hook entries are the three hooks Codex can serve, under matchers on its executor server"+verifiesShipped, func(t *testing.T) {
			var parsed snakeHooksFile
			decodeJSON(t, files[pluginfiles.CodexHooksPath], &parsed)
			want := map[string]string{
				"PreToolUse":    hookCommand(module, hostproto.HookPreTool, codex.Flag),
				"SubagentStart": hookCommand(module, hostproto.HookSubagentStart, codex.Flag),
				"SubagentStop":  hookCommand(module, hostproto.HookSubagentStop, codex.Flag),
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
			if want := "|mcp__" + strings.NewReplacer("-", "_").Replace(module.Plugin()) + "_exec__.*"; !strings.HasSuffix(parsed.Hooks["PreToolUse"][0].Matcher, want) {
				t.Errorf("matcher = %q, want the suffix %q", parsed.Hooks["PreToolUse"][0].Matcher, want)
			}
			for _, name := range codex.ToolNames() {
				if !matcher.MatchString(name) {
					t.Errorf("the matcher does not select %s", name)
				}
			}
			otherTools := harness.Codex(harness.Identity{Plugin: otherPlugin, Agents: module.Identity.Agents}).ToolNames()
			if matcher.MatchString("Bash") || matcher.MatchString(otherTools[0]) {
				t.Error("the matcher selects a tool that is not the module's")
			}
		})

		t.Run(s.name+": the pre-tool hook entry selects the spawn tool under each of its names, so that the hook sees every spawn [verifies SP-1937~1]", func(t *testing.T) {
			var parsed snakeHooksFile
			decodeJSON(t, files[pluginfiles.CodexHooksPath], &parsed)
			matcher := regexp.MustCompile(parsed.Hooks["PreToolUse"][0].Matcher)
			for _, name := range hostproto.CodexSpawnToolNames() {
				if !matcher.MatchString(name) {
					t.Errorf("the matcher does not select %s", name)
				}
			}
			for _, name := range []string{"wait_agent", "multi_agent_v1wait_agent", "spawn_agents"} {
				if matcher.MatchString(name) {
					t.Errorf("the matcher selects %s", name)
				}
			}
		})

		t.Run(s.name+": the skill is a project skill that delegates by agent name"+verifiesShipped, func(t *testing.T) {
			members, body := frontMatter(t, files[skillPath(codex.Flag, module)])
			if members["name"] != module.SkillName() || members["allowed-tools"] != nil {
				t.Errorf("members = %v", members)
			}
			example := delegationExample(module)
			wantExample := "`" + hostproto.RoleFor(module.Plugin(), example) + "` is `" + module.Plugin() + "-" + string(example) + "`"
			for _, want := range []string{"`spawn_agent`", "`agent_type` set to the role on its line", "`fork_turns` set to `\"none\"`", "`message` set to `ticket <ticket>` and nothing else", "`wait_agent`", "at most 3", wantExample, module.Binary + " prove --resume", pluginfiles.RelayBinary + " drain --module " + module.RelayName(), "`" + pluginfiles.VerifyLocalCommand + "`"} {
				if !strings.Contains(body, want) {
					t.Errorf("body lacks %q: %s", want, body)
				}
			}
		})
	}
}

func TestCopilotFiles(t *testing.T) {
	for _, s := range samples() {
		module := s.module
		copilot := harness.Copilot(module.Identity)
		files := generated(t, module, copilot)
		hooksPath := ".github/hooks/" + module.Plugin() + ".json"

		t.Run(s.name+": the files are the agents, the skill and the hooks of the module"+verifiesShipped, func(t *testing.T) {
			want := []string{skillPath(copilot.Flag, module), hooksPath}
			for _, agent := range module.Identity.Agents {
				want = append(want, agentPath(copilot.Flag, module, agent))
			}
			slices.Sort(want)
			if got := sortedPaths(files); !slices.Equal(got, want) {
				t.Fatalf("files = %v, want %v", got, want)
			}
		})

		t.Run(s.name+": each agent names the server and tool pairs and starts its own server"+verifiesShipped, func(t *testing.T) {
			for _, agent := range module.Identity.Agents {
				members, body := frontMatter(t, files[agentPath(copilot.Flag, module, agent)])
				tools := stringList(t, members["tools"])
				if !slices.Equal(tools, copilot.ToolNames()) {
					t.Errorf("%s: tools = %v, want %v", agent, tools, copilot.ToolNames())
				}
				servers, _ := members["mcp-servers"].(map[string]any)
				server, _ := servers[copilot.Server()].(map[string]any)
				if members["name"] != module.Plugin()+"-"+string(agent) || len(servers) != 1 || server["command"] != module.Binary || server["type"] != "local" ||
					!slices.Equal(stringList(t, server["args"]), []string{"mcp", "--role", "executor", "--harness", "copilot"}) ||
					!slices.Equal(stringList(t, server["tools"]), []string{hostproto.ToolFetchRequest, hostproto.ToolSubmitResponse}) {
					t.Errorf("%s: mcp-servers = %v", agent, servers)
				}
				if strings.TrimSpace(body) != executorInstructions {
					t.Errorf("%s: body = %q", agent, body)
				}
			}
		})

		t.Run(s.name+": the hooks file serves the pre-tool and the stop checks in the Copilot layout"+verifiesShipped, func(t *testing.T) {
			var parsed struct {
				Version int `json:"version"`
				Hooks   map[string][]struct {
					Type       string `json:"type"`
					Bash       string `json:"bash"`
					PowerShell string `json:"powershell"`
					TimeoutSec int    `json:"timeoutSec"`
				} `json:"hooks"`
			}
			decodeJSON(t, files[hooksPath], &parsed)
			want := map[string]string{
				"preToolUse":   hookCommand(module, hostproto.HookPreTool, copilot.Flag),
				"subagentStop": hookCommand(module, hostproto.HookSubagentStop, copilot.Flag),
			}
			if parsed.Version != 1 || len(parsed.Hooks) != len(want) {
				t.Fatalf("hooks = %+v", parsed)
			}
			for event, command := range want {
				entries := parsed.Hooks[event]
				if len(entries) != 1 || entries[0].Bash != command || entries[0].PowerShell != command || entries[0].Type != hookTypeCommand || entries[0].TimeoutSec <= 0 {
					t.Errorf("%s: %+v", event, entries)
				}
			}
		})

		t.Run(s.name+": the skill is a project skill that delegates by agent name"+verifiesShipped, func(t *testing.T) {
			members, body := frontMatter(t, files[skillPath(copilot.Flag, module)])
			example := delegationExample(module)
			wantExample := "`" + module.Plugin() + "-" + string(example) + "`"
			if members["name"] != module.SkillName() || !strings.Contains(body, wantExample) || !strings.Contains(body, "`"+pluginfiles.VerifyLocalCommand+"`") {
				t.Errorf("members = %v, body = %s", members, body)
			}
		})
	}
}

func TestAgentNamesMeetTheHooksAndTheRoles(t *testing.T) {
	for _, s := range samples() {
		module := s.module

		t.Run(s.name+": the type of each generated agent passes the matchers of the generated hooks and maps to the role of its requests"+verifiesShipped, func(t *testing.T) {
			hooksFiles := map[string]string{harness.FlagClaude: "hooks/hooks.json", harness.FlagCodex: pluginfiles.CodexHooksPath}
			for _, host := range harnesses(module) {
				files := generated(t, module, host)
				for index, agentType := range agentTypes(t, module, host, files) {
					role, ok := host.RoleOf(agentType)
					if want := hostproto.RoleFor(module.Plugin(), module.Identity.Agents[index]); !ok || role != want {
						t.Errorf("%s: %q maps to %q, %v, want %q", host.Flag, agentType, role, ok, want)
					}
					hooksFile, hasMatchers := hooksFiles[host.Flag]
					if !hasMatchers {
						continue
					}
					var parsed snakeHooksFile
					decodeJSON(t, files[hooksFile], &parsed)
					for _, event := range []string{"SubagentStart", "SubagentStop"} {
						if !regexp.MustCompile(parsed.Hooks[event][0].Matcher).MatchString(agentType) {
							t.Errorf("%s: the %s matcher %q does not select the agent type %q", host.Flag, event, parsed.Hooks[event][0].Matcher, agentType)
						}
					}
				}
			}
		})

		t.Run(s.name+": the matcher of a harness selects its own agent types and not the form of another"+verifiesShipped, func(t *testing.T) {
			claude := regexp.MustCompile(module.ExecutorRoleMatcher(harness.Claude(module.Identity)))
			codex := regexp.MustCompile(module.ExecutorRoleMatcher(harness.Codex(module.Identity)))
			example := string(delegationExample(module))
			scoped, dashed := module.Plugin()+":"+example, module.Plugin()+"-"+example
			if claude.MatchString(dashed) || codex.MatchString(scoped) {
				t.Error("a matcher selects the form of another harness")
			}
			if !claude.MatchString(scoped) || !codex.MatchString(dashed) || codex.MatchString(dashed+"y") {
				t.Error("a matcher misses its own form")
			}
		})
	}
}

func TestGeneratedTextIsPlainShippedContent(t *testing.T) {
	for _, s := range samples() {
		module := s.module

		t.Run(s.name+": no generated file talks about its own authoring, its verification state or its history"+verifiesShipped, func(t *testing.T) {
			forbidden := regexp.MustCompile(`(?i)\b(unverified|canary|ADR|previously|no longer|currently|this version|TODO)\b`)
			for _, host := range harnesses(module) {
				for path, content := range generated(t, module, host) {
					if found := forbidden.FindString(content); found != "" {
						t.Errorf("%s/%s says %q", host.Flag, path, found)
					}
				}
			}
		})

		t.Run(s.name+": every generated file ends with a newline and has no trailing blanks"+verifiesShipped, func(t *testing.T) {
			for _, host := range harnesses(module) {
				for path, content := range generated(t, module, host) {
					if !strings.HasSuffix(content, "\n") || bytes.Contains([]byte(content), []byte(" \n")) {
						t.Errorf("%s/%s is not tidy", host.Flag, path)
					}
				}
			}
		})

		t.Run(s.name+": a hook command names the marker the installer recognizes it by"+verifiesShipped, func(t *testing.T) {
			for _, host := range harnesses(module) {
				for _, kind := range hostproto.HookKinds() {
					if command := module.HookCommand(kind, host); !strings.HasPrefix(command, module.HookCommandMarker()) {
						t.Errorf("%s does not start with %q", command, module.HookCommandMarker())
					}
				}
			}
		})
	}
}

func TestGenerationIsDeterministicAndBounded(t *testing.T) {
	for _, s := range samples() {
		module := s.module

		t.Run(s.name+": generating twice gives the same bytes"+verifiesGeneration, func(t *testing.T) {
			for _, host := range harnesses(module) {
				first, second := generated(t, module, host), generated(t, module, host)
				if !slices.Equal(sortedPaths(first), sortedPaths(second)) {
					t.Errorf("%s: the files differ between two runs", host.Flag)
				}
				for path, content := range first {
					if second[path] != content {
						t.Errorf("%s: %s differs between two runs", host.Flag, path)
					}
				}
			}
		})

		t.Run(s.name+": a harness without files is refused"+verifiesGeneration, func(t *testing.T) {
			_, err := module.For(harness.Harness{Flag: "vim"})
			if !errors.Is(err, harness.ErrUnknownHarness) {
				t.Fatalf("err = %v, want an unknown harness", err)
			}
		})

		t.Run(s.name+": a module ships exactly the agent files of its own agents"+verifiesGeneration, func(t *testing.T) {
			for _, host := range harnesses(module) {
				files := generated(t, module, host)
				for _, agent := range module.Identity.Agents {
					if _, shipped := files[agentPath(host.Flag, module, agent)]; !shipped {
						t.Errorf("%s: no file for the agent %s", host.Flag, agent)
					}
				}
				for _, agent := range missingAgents(module) {
					if _, shipped := files[agentPath(host.Flag, module, agent)]; shipped {
						t.Errorf("%s: a file for the agent %s the module lacks", host.Flag, agent)
					}
				}
				agentFiles := 0
				for path := range files {
					if strings.Contains(path, "agents/") && !strings.Contains(path, "skills/") {
						agentFiles++
					}
				}
				if agentFiles != len(module.Identity.Agents) {
					t.Errorf("%s: %d agent files for %d agents", host.Flag, agentFiles, len(module.Identity.Agents))
				}
			}
		})

		t.Run(s.name+": the matchers select only the agent types of the module"+verifiesGeneration, func(t *testing.T) {
			for _, host := range harnesses(module) {
				matcher := regexp.MustCompile(module.ExecutorRoleMatcher(host))
				for _, agent := range module.Identity.Agents {
					if !matcher.MatchString(host.AgentTypeOf(agent)) {
						t.Errorf("%s: the matcher misses %s", host.Flag, agent)
					}
				}
				for _, agent := range missingAgents(module) {
					agentType := host.AgentTypeOf(agent)
					if matcher.MatchString(agentType) {
						t.Errorf("%s: the matcher selects %s, an agent the module lacks", host.Flag, agentType)
					}
					if _, ok := host.RoleOf(agentType); ok {
						t.Errorf("%s: %s maps to a role although the module lacks the agent", host.Flag, agentType)
					}
				}
				for _, other := range samples() {
					if other.module.Plugin() == module.Plugin() {
						continue
					}
					otherHost, err := harness.ByFlag(other.module.Identity, host.Flag)
					if err != nil {
						t.Fatal(err)
					}
					for _, agent := range other.module.Identity.Agents {
						if matcher.MatchString(otherHost.AgentTypeOf(agent)) {
							t.Errorf("%s: the matcher selects an agent of %s", host.Flag, other.module.Plugin())
						}
					}
				}
			}
		})
	}
}
