package pluginfiles_test

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

const (
	frontMatterFence = "---\n"
	otherPlugin      = "other"
)

const executorInstructions = "Fetch the ticket in your task, follow its instructions, submit JSON matching its schema, reply only with the receipt."

// codexInstructions is the text a Codex executor reads: it names both tools as the
// harness spells them and tells the executor to use no skill and no shell.
func codexInstructions(codex harness.Harness) string {
	fetch, submit := codex.ToolName(hostproto.ToolFetchRequest), codex.ToolName(hostproto.ToolSubmitResponse)
	return "Call `" + fetch + "` with the ticket from your task and an empty `cap`. Follow the instructions it returns. " +
		"Call `" + submit + "` with the ticket, the nonce from the fetch, an empty `cap` and the JSON output matching the schema it returned. " +
		"Then reply only with the receipt. Use no skill and no shell command for this."
}

// sample is one module the generator is exercised with.
type sample struct {
	name   string
	module pluginfiles.Module
}

func samples() []sample {
	return []sample{
		{name: "full", module: pluginfiles.Module{
			Identity: harness.Identity{Plugin: "musthold-sample", Agents: hostproto.Agents()},
			Binary:   "musthold-sample", Version: "0.1.0", Subject: "sample proof", ProofPhrase: "the sample proof",
		}},
		{name: "judge-and-decider", module: pluginfiles.Module{
			Identity: harness.Identity{Plugin: "musthold-pair", Agents: []hostproto.Agent{hostproto.AgentJudge, hostproto.AgentDecider}},
			Binary:   "musthold-pair", Version: "1.2.3", Subject: "pair proof", ProofPhrase: "the pair proof",
		}},
		{name: "decider-only", module: pluginfiles.Module{
			Identity: harness.Identity{Plugin: "musthold-solo", Agents: []hostproto.Agent{hostproto.AgentDecider}},
			Binary:   "musthold-solo", Version: "2.0.1", Subject: "solo proof", ProofPhrase: "the solo proof",
		}},
	}
}

// harnesses lists the harnesses of a module.
func harnesses(module pluginfiles.Module) []harness.Harness {
	return harness.All(module.Identity)
}

// delegationExample is the agent the generated skills use to show how a role maps
// to the name of its agent: the judge when the module ships one, its first agent
// otherwise.
func delegationExample(module pluginfiles.Module) hostproto.Agent {
	if slices.Contains(module.Identity.Agents, hostproto.AgentJudge) {
		return hostproto.AgentJudge
	}
	return module.Identity.Agents[0]
}

// missingAgents lists the executor agents a module does not ship.
func missingAgents(module pluginfiles.Module) []hostproto.Agent {
	var missing []hostproto.Agent
	for _, agent := range hostproto.Agents() {
		if !slices.Contains(module.Identity.Agents, agent) {
			missing = append(missing, agent)
		}
	}
	return missing
}

// agentPath is the path of the file that declares an agent on a harness.
func agentPath(flag string, module pluginfiles.Module, agent hostproto.Agent) string {
	switch flag {
	case harness.FlagClaude:
		return "agents/" + string(agent) + ".md"
	case harness.FlagCodex:
		return ".codex/agents/" + module.Plugin() + "-" + string(agent) + ".toml"
	default:
		return ".github/agents/" + module.Plugin() + "-" + string(agent) + ".agent.md"
	}
}

func skillPath(flag string, module pluginfiles.Module) string {
	switch flag {
	case harness.FlagClaude:
		return "skills/" + module.SkillName() + "/SKILL.md"
	case harness.FlagCodex:
		return ".agents/skills/" + module.SkillName() + "/SKILL.md"
	default:
		return ".github/skills/" + module.SkillName() + "/SKILL.md"
	}
}

func generated(t *testing.T, module pluginfiles.Module, host harness.Harness) map[string]string {
	t.Helper()
	files, err := module.For(host)
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

func sortedPaths(files map[string]string) []string {
	paths := make([]string, 0, len(files))
	for path := range files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	return paths
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

// tomlTable parses the subset of TOML the Codex agent files use: string and
// string-array values, and one level of tables.
func tomlTable(t *testing.T, text string) map[string]map[string]any {
	t.Helper()
	tables := map[string]map[string]any{"": {}}
	current := ""
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "" || strings.HasPrefix(line, "#"):
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

// snakeHooksFile is the hooks file of Claude Code and Codex; Modules is the list of
// hooks modules a Claude plugin loads.
type snakeHooksFile struct {
	Modules []string `json:"modules"`
	Hooks   map[string][]struct {
		Matcher string `json:"matcher"`
		Hooks   []struct {
			Type    string `json:"type"`
			Command string `json:"command"`
			Timeout int    `json:"timeout"`
		} `json:"hooks"`
	} `json:"hooks"`
}

// hookCommand is the command line the module installs for a hook of a harness.
func hookCommand(module pluginfiles.Module, kind hostproto.HookKind, flag string) string {
	return fmt.Sprintf("%s hook %s --harness %s", module.Binary, kind, flag)
}

// agentTypes lists the type each harness gives the generated executor agents of
// a module: the name of the agent as its file declares it, joined with the plugin
// where the harness scopes agents by plugin.
func agentTypes(t *testing.T, module pluginfiles.Module, host harness.Harness, files map[string]string) []string {
	t.Helper()
	var types []string
	for _, agent := range module.Identity.Agents {
		content := files[agentPath(host.Flag, module, agent)]
		switch host.Flag {
		case harness.FlagClaude:
			members, _ := frontMatter(t, content)
			types = append(types, module.Plugin()+":"+members["name"].(string))
		case harness.FlagCodex:
			types = append(types, tomlTable(t, content)[""]["name"].(string))
		case harness.FlagCopilot:
			members, _ := frontMatter(t, content)
			types = append(types, members["name"].(string))
		}
	}
	return types
}
