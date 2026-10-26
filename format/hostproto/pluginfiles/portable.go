package pluginfiles

import (
	"github.com/RachidChabane/musthold/format/hostproto"
	"github.com/RachidChabane/musthold/format/hostproto/harness"
)

// The portable plugin: the files of the Agent Plugins standard, which every
// client of the standard loads the same way. Its components are a manifest, skills
// and MCP servers; what the standard leaves to a client, the agents and hooks of
// Copilot CLI, lives in the client's own namespace directory.
const (
	// PluginSchemaURL is the canonical `$schema` of a manifest of Agent Plugins 1.0.0.
	PluginSchemaURL = "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json"
	// MCPSchemaURL is the canonical `$schema` of an `mcp.json` of Agent Plugins 1.0.0.
	MCPSchemaURL = "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json"

	// PortableManifestPath, PortableMCPPath and PortableSkillsDirectory are the
	// fixed locations of the standard, relative to the root of the plugin.
	PortableManifestPath    = "plugin.json"
	PortableMCPPath         = "mcp.json"
	PortableSkillsDirectory = "skills"

	// CopilotNamespace is the extension namespace of Copilot CLI.
	CopilotNamespace = "com.github.copilot"

	pluginRepository = "https://github.com/RachidChabane/musthold"
	transportStdio   = "stdio"
)

// PortableKeywords describe every plugin of the repository to a plugin directory.
var PortableKeywords = []string{"musthold", "proof", "tests", "requirements"}

// PortableManifest is the identity a plugin of the repository gives its
// `plugin.json`.
type PortableManifest struct {
	Name        string
	Version     string
	Description string
}

type manifestAuthor struct {
	Name string `json:"name"`
}

type portableManifestFile struct {
	Schema      string         `json:"$schema"`
	Name        string         `json:"name"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Author      manifestAuthor `json:"author"`
	Repository  string         `json:"repository"`
	License     string         `json:"license"`
	Keywords    []string       `json:"keywords"`
}

// ManifestFile renders the root `plugin.json` of a portable plugin.
func (p PortableManifest) ManifestFile() (File, error) {
	content, err := encodeJSON(portableManifestFile{
		Schema: PluginSchemaURL, Name: p.Name, Version: p.Version, Description: p.Description,
		Author: manifestAuthor{Name: pluginAuthor}, Repository: pluginRepository, License: pluginLicense,
		Keywords: PortableKeywords,
	})
	return File{Path: PortableManifestPath, Content: content}, err
}

// PortableServer is one stdio server of a portable `mcp.json`.
type PortableServer struct {
	Command string
	Args    []string
}

type portableServerEntry struct {
	Type    string   `json:"type"`
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type portableMCPFile struct {
	Schema  string                         `json:"$schema"`
	Servers map[string]portableServerEntry `json:"mcpServers"`
}

// MCPFile renders the root `mcp.json` of a portable plugin from its servers by name;
// no server is an empty `mcpServers`.
func MCPFile(servers map[string]PortableServer) (File, error) {
	entries := make(map[string]portableServerEntry, len(servers))
	for name, server := range servers {
		entries[name] = portableServerEntry{Type: transportStdio, Command: server.Command, Args: server.Args}
	}
	content, err := encodeJSON(portableMCPFile{Schema: MCPSchemaURL, Servers: entries})
	return File{Path: PortableMCPPath, Content: content}, err
}

// PortableSkillPath is where the skill of a portable plugin lives.
func PortableSkillPath(skill string) string {
	return PortableSkillsDirectory + "/" + skill + "/SKILL.md"
}

// portableServerArgs are the arguments of the root server: no `--harness`, so the
// server is the portable harness and its answers are the main agent's.
var portableServerArgs = []string{"mcp", "--role", "executor"}

// Portable returns the files of the module's portable plugin: the manifest, the
// fulfil skill, the executor server that answers inline, and the agents and hooks
// of Copilot CLI in its namespace.
func (m Module) Portable() ([]File, error) {
	manifest, err := PortableManifest{
		Name: m.Plugin(), Version: m.Version,
		Description: "Fulfil skill and executor server that let a client of the Agent Plugins standard answer the model requests of " +
			m.Plugin() + ", " + m.ProofPhrase + ".",
	}.ManifestFile()
	if err != nil {
		return nil, err
	}
	servers, err := MCPFile(map[string]PortableServer{m.Plugin(): {Command: m.Binary, Args: portableServerArgs}})
	if err != nil {
		return nil, err
	}
	example := m.exampleAgent()
	skill, err := render("portable-skill", portableSkillTemplate, skillData{
		Binary: m.Binary, Name: m.SkillName(), Plugin: m.Plugin(), Subject: m.Subject, Relay: m.RelayName(), Verify: VerifyLocalCommand,
		RoleExample: hostproto.RoleFor(m.Plugin(), example), AgentExample: m.agentName(example), PromptExample: promptExample,
	})
	if err != nil {
		return nil, err
	}
	files := []File{manifest, servers, {Path: PortableSkillPath(m.SkillName()), Content: skill}}
	copilot, err := m.copilotNamespaceFiles(harness.Copilot(m.Identity))
	if err != nil {
		return nil, err
	}
	return append(files, copilot...), nil
}

// copilotNamespaceFiles are the agents and hooks of Copilot CLI inside the plugin:
// the same definitions `init` writes into a project, at the places Copilot reads
// them from a plugin.
func (m Module) copilotNamespaceFiles(h harness.Harness) ([]File, error) {
	var files []File
	agents, err := m.copilotAgents(h)
	if err != nil {
		return nil, err
	}
	for _, agent := range agents {
		files = append(files, File{Path: CopilotNamespace + "/agents/" + agent.name + copilotAgentSuffix, Content: agent.content})
	}
	hooks, err := m.copilotHooksFile(h)
	if err != nil {
		return nil, err
	}
	return append(files, File{Path: CopilotNamespace + "/hooks/hooks.json", Content: hooks}), nil
}
