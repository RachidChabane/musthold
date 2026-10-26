package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/format/hostproto"
)

// CopilotListing is what `copilot plugin list --json` and `copilot skill list
// --json` printed with the session's plugin directories mounted, before the
// session. Both make no model call.
type CopilotListing struct {
	Attempted     bool   `json:"attempted"`
	PluginsExit   int    `json:"pluginsExit"`
	PluginsOutput string `json:"pluginsOutput"`
	SkillsExit    int    `json:"skillsExit"`
	SkillsOutput  string `json:"skillsOutput"`
}

// ListedPlugin is a member of `copilot plugin list --json`. Only the members read
// here are decoded; Enabled is nil when the listing does not carry it.
type ListedPlugin struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Source  string `json:"source"`
	Enabled *bool  `json:"enabled"`
}

// ListedSkill is a member of `copilot skill list --json`.
type ListedSkill struct {
	Name    string `json:"name"`
	Source  string `json:"source"`
	Path    string `json:"path"`
	Enabled *bool  `json:"enabled"`
}

// Plugins decodes the plugin listing; false when the output is not a JSON list.
func (l CopilotListing) Plugins() ([]ListedPlugin, bool) {
	var plugins []ListedPlugin
	err := json.Unmarshal([]byte(strings.TrimSpace(l.PluginsOutput)), &plugins)
	return plugins, err == nil
}

// Skills decodes the skill listing; false when the output is not a JSON list.
func (l CopilotListing) Skills() ([]ListedSkill, bool) {
	var skills []ListedSkill
	err := json.Unmarshal([]byte(strings.TrimSpace(l.SkillsOutput)), &skills)
	return skills, err == nil
}

// PluginTree is what the canary found in one generated Agent Plugins tree.
type PluginTree struct {
	Dir string `json:"dir"`
	// Name is the manifest's name.
	Name string `json:"name"`
	// Servers are the names of the servers of the root mcp.json.
	Servers []string `json:"servers"`
	// Skills are the skills under skills/, with the directory each lives in.
	Skills []TreeSkill `json:"skills"`
	// Namespace lists the files under the Copilot namespace directory, relative to
	// the root of the tree.
	Namespace []string `json:"namespace"`
	// Problems are the shapes the tree has that the standard does not allow.
	Problems []string `json:"problems"`
}

// TreeSkill is one skill of a tree.
type TreeSkill struct {
	Dir  string `json:"dir"`
	Name string `json:"name"`
}

// TreeFiles holds what was read of a tree; a file that is absent is empty.
type TreeFiles struct {
	Manifest []byte
	MCP      []byte
	// Skills maps the directory of each skill to its SKILL.md.
	Skills    map[string][]byte
	Namespace []string
}

const frontmatterFence = "---"

// InspectPluginTree reads the root files of a tree for the shape ADR 0009 gives
// them: a manifest with a name, an mcp.json with an `mcpServers` table, and skills
// whose directory carries the name of their frontmatter.
func InspectPluginTree(dir string, files TreeFiles) PluginTree {
	tree := PluginTree{Dir: dir, Namespace: files.Namespace}
	problem := func(format string, args ...any) { tree.Problems = append(tree.Problems, fmt.Sprintf(format, args...)) }

	if manifest, ok := decodeRoot(files.Manifest, "plugin.json", problem); ok {
		_ = json.Unmarshal(manifest["name"], &tree.Name)
		if tree.Name == "" {
			problem("plugin.json has no name")
		}
	}
	if mcp, ok := decodeRoot(files.MCP, "mcp.json", problem); ok {
		var servers map[string]json.RawMessage
		if err := json.Unmarshal(mcp["mcpServers"], &servers); err != nil {
			problem("mcp.json has no mcpServers table")
		}
		for name := range servers {
			tree.Servers = append(tree.Servers, name)
		}
		sort.Strings(tree.Servers)
	}

	skillDirs := make([]string, 0, len(files.Skills))
	for skillDir := range files.Skills {
		skillDirs = append(skillDirs, skillDir)
	}
	sort.Strings(skillDirs)
	for _, skillDir := range skillDirs {
		name := frontmatterName(files.Skills[skillDir])
		tree.Skills = append(tree.Skills, TreeSkill{Dir: skillDir, Name: name})
		if name != skillDir {
			problem("skill directory %q holds a skill named %q", skillDir, name)
		}
	}
	return tree
}

// decodeRoot decodes a root file of a tree as a JSON object, reporting a file that
// is missing or is not one.
func decodeRoot(data []byte, name string, problem func(string, ...any)) (map[string]json.RawMessage, bool) {
	if len(bytes.TrimSpace(data)) == 0 {
		problem("%s is missing", name)
		return nil, false
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(data, &object); err != nil {
		problem("%s is not a JSON object: %v", name, err)
		return nil, false
	}
	return object, true
}

// frontmatterName reads the name member of a SKILL.md frontmatter, or "".
func frontmatterName(skill []byte) string {
	lines := strings.Split(string(skill), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != frontmatterFence {
		return ""
	}
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == frontmatterFence {
			return ""
		}
		if value, found := strings.CutPrefix(line, "name:"); found {
			return strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	return ""
}

// CopilotHook is the payload of a Copilot hook, decoded by the module's own
// decoder so that the canary reads the payload the way the module does. A payload
// the decoder rejects still yields the members it found.
func (c HookCall) CopilotHook() hostproto.HookEvent {
	decoder, err := hostproto.DecoderFor(hostproto.HarnessCopilot)
	if err != nil {
		return hostproto.HookEvent{}
	}
	event, _ := decoder.Decode(hostproto.HookKind(c.Kind()), c.Stdin)
	return event
}
