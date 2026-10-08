// Package umbrella is the single source of the files of the spec-proof umbrella:
// the marketplace that lists Shallnot's plugin, the three proof-module plugins and
// the relay beside the thin plugin, and the thin plugin itself. The thin plugin
// declares its dependencies and holds one skill, `prove-all`, that runs the fulfil
// loop of each module that is present; it holds no logic of its own. The committed
// files under `umbrella/` are generated here, and so is the marketplace at the
// repository root, which is the one `/plugin marketplace add RachidChabane/spec-proof`
// reads. The package knows no I/O.
package umbrella

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

// Identity of the umbrella.
const (
	// MarketplaceName is the name of the marketplace, and of the thin plugin's
	// own marketplace in the `<plugin>@<marketplace>` keys `init` enables.
	MarketplaceName = "spec-proof"
	// PluginName is the name of the thin plugin.
	PluginName = "spec-proof"
	// SkillName is the name of the thin plugin's only skill.
	SkillName = "prove-all"
	// Version is the version of the shipped files.
	Version = "0.1.0"

	// MarketplacePath is where the marketplace file lives, relative to `umbrella/`.
	MarketplacePath = ".claude-plugin/marketplace.json"
	// RootMarketplacePath is where the marketplace file of the repository root
	// lives, relative to the repository root.
	RootMarketplacePath = ".claude-plugin/marketplace.json"
	// UmbrellaDirectory is the directory of the umbrella in the repository.
	UmbrellaDirectory = "umbrella"
	// PluginDirectory is the thin plugin's tree, relative to `umbrella/`.
	PluginDirectory = "plugins/" + PluginName

	ownerName    = "Rachid Chabane"
	ownerURL     = "https://github.com/RachidChabane"
	license      = "Apache-2.0"
	indent       = "  "
	repositoryID = "RachidChabane/spec-proof"
)

// Module is one proof module of the umbrella, in the order its proofs are made.
type Module struct {
	// Plugin is the name of the module's plugin.
	Plugin string
	// Binary is the executable of the module.
	Binary string
	// Directory is the module's directory in the repository.
	Directory string
	// Summary describes the plugin in the marketplace.
	Summary string
}

// Modules lists the proof modules in the order `prove-all` runs them:
// discriminating power, alignment, completeness.
var Modules = []Module{
	{Plugin: "spec-proof-dp", Binary: "spec-proof-dp", Directory: "discpower", Summary: "Discriminating power: do the tests of a criterion catch a violation of it."},
	{Plugin: "spec-proof-alignment", Binary: "spec-proof-alignment", Directory: "alignment", Summary: "Alignment: does each test verify what its criterion states."},
	{Plugin: "spec-proof-completeness", Binary: "spec-proof-completeness", Directory: "completeness", Summary: "Completeness: does the spec cover what a change does (advisory)."},
}

// Relay is the System One relay, whose plugin the marketplace lists.
var Relay = Module{Plugin: "spec-proof-s1", Binary: "spec-proof-s1", Directory: "s1relay", Summary: "The relay of closed questions to a System One endpoint."}

// PluginPath is where the plugin tree of a module lives in the repository.
func (m Module) PluginPath() string { return m.Directory + "/plugin/claude" }

// FulfilSkill is the name of the skill that answers the module's requests.
func (m Module) FulfilSkill() string { return m.Binary + "-fulfil" }

// Shallnot describes Shallnot's own plugin by its public marketplace source: the
// plugin lives in the `plugin` directory of the `RachidChabane/shallnot`
// repository, which is also its own marketplace.
var Shallnot = struct{ Plugin, Repository, Path, Summary string }{
	Plugin: "shallnot", Repository: "RachidChabane/shallnot", Path: "plugin",
	Summary: "Requirement-to-test traceability for coding agents and the end-of-turn gate the proofs plug into.",
}

// DependencyNames lists what the thin plugin depends on: Shallnot, the three
// modules and the relay, each by the name of its entry in the marketplace.
func DependencyNames() []string {
	names := []string{Shallnot.Plugin}
	for _, module := range Modules {
		names = append(names, module.Plugin)
	}
	return append(names, Relay.Plugin)
}

// File is one generated file: its path relative to `umbrella/` and its bytes.
type File struct {
	Path    string
	Content []byte
}

func encodeJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

type owner struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type gitSubdir struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	Path   string `json:"path"`
	Ref    string `json:"ref,omitempty"`
}

// marketplaceEntry is one plugin of the marketplace; its source is a relative
// path inside the marketplace or a git-subdir object.
type marketplaceEntry struct {
	Name        string `json:"name"`
	Source      any    `json:"source"`
	Description string `json:"description"`
}

type marketplace struct {
	Name     string `json:"name"`
	Owner    owner  `json:"owner"`
	Metadata struct {
		Description string `json:"description"`
	} `json:"metadata"`
	Plugins []marketplaceEntry `json:"plugins"`
}

// SubdirSource is the source of a plugin that lives in a subdirectory of a
// public GitHub repository, pinned to ref when ref is not empty.
func SubdirSource(repository, path, ref string) any {
	return gitSubdir{Source: "git-subdir", URL: repository, Path: path, Ref: ref}
}

// Versions maps the directory of a module to the version its plugin trees carry.
// The marketplace pins each module's plugin to the release tag of that version.
type Versions map[string]string

// ReleaseTag is the tag that releases a module at a version: `<module>/v<version>`.
func ReleaseTag(directory, version string) string {
	return directory + "/v" + version
}

func (v Versions) refOf(directory string) (string, error) {
	version, found := v[directory]
	if !found || version == "" {
		return "", fmt.Errorf("no plugin version for the module %s", directory)
	}
	return ReleaseTag(directory, version), nil
}

// marketplaceFile builds the marketplace at path, whose relative plugin sources
// are resolved against the directory that holds the marketplace's `.claude-plugin`
// directory: thinPluginSource is where the thin plugin's tree is from there.
func marketplaceFile(path, thinPluginSource string, versions Versions) (File, error) {
	listing := marketplace{Name: MarketplaceName, Owner: owner{Name: ownerName, URL: ownerURL}}
	listing.Metadata.Description = "spec-proof: proofs that the tests of a project check what its requirements say, for coding agents."
	listing.Plugins = append(listing.Plugins, marketplaceEntry{
		Name: Shallnot.Plugin, Source: SubdirSource(Shallnot.Repository, Shallnot.Path, ""), Description: Shallnot.Summary,
	})
	for _, module := range append(append([]Module{}, Modules...), Relay) {
		ref, err := versions.refOf(module.Directory)
		if err != nil {
			return File{}, err
		}
		listing.Plugins = append(listing.Plugins, marketplaceEntry{
			Name: module.Plugin, Source: SubdirSource(repositoryID, module.PluginPath(), ref), Description: module.Summary,
		})
	}
	listing.Plugins = append(listing.Plugins, marketplaceEntry{
		Name: PluginName, Source: thinPluginSource,
		Description: "Installs Shallnot and every spec-proof module and runs them together with the prove-all skill.",
	})
	content, err := encodeJSON(listing)
	return File{Path: path, Content: content}, err
}

type pluginManifest struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Description  string            `json:"description"`
	Author       map[string]string `json:"author"`
	License      string            `json:"license"`
	Dependencies []string          `json:"dependencies"`
}

func pluginManifestFile() (File, error) {
	content, err := encodeJSON(pluginManifest{
		Name: PluginName, Version: Version,
		Description:  "Installs Shallnot and the spec-proof modules and runs their proofs together.",
		Author:       map[string]string{"name": ownerName},
		License:      license,
		Dependencies: DependencyNames(),
	})
	return File{Path: PluginDirectory + "/.claude-plugin/plugin.json", Content: content}, err
}

// skillData fills the prove-all template.
type skillData struct {
	Name    string
	Modules []Module
	Allowed string
	Relay   string
	Names   string
	Verify  string
}

const skillTemplate = `---
name: {{.Name}}
description: Prove a change with every spec-proof module that is installed, in the order discriminating power, alignment, completeness, answering each module's requests {{if .Allowed}}with its blind executors{{else}}as its fulfil skill gives{{end}}. Use before finishing a change to code, tests or specs, and when Shallnot reports awaiting_host, awaiting_proof or awaiting_host_judgment.
{{- if .Allowed}}
allowed-tools: {{.Allowed}}
{{- end}}
---

# Prove with every module

This skill holds no logic of its own. Each module runs its own loop and names its own steps in its fulfil skill; this skill only orders the modules and interleaves their waves.{{if .Allowed}} Never answer a request yourself and never read what a request asks.{{end}}

Modules, in this order, each with the skill that holds its loop:
{{range .Modules}}
- ` + "`{{.Binary}}`" + `, whose loop is the skill ` + "`{{.FulfilSkill}}`" + `.{{end}}

1. Run ` + "{{.Names}}" + `. A module whose binary is not on ` + "`PATH`" + ` is missing: name every missing module to the user and go on with the others; a missing module never counts as proved. If a module that is present fails its ` + "`doctor`" + `, report it and leave that module out. If no module is present, stop.
2. For each present module, in the order above, run ` + "`<binary> prove --changed`" + ` once.
3. While any module exited with code 3, run one wave for each module that exited with code 3, in the order above, before any module starts its next wave: a wave is the steps its fulfil skill gives for answering the tickets, which is the relay when ` + "`{{.Relay}}`" + ` is on ` + "`PATH`" + ` and an endpoint is configured, then {{if .Allowed}}one foreground blind executor per ticket{{else}}the answer to each ticket, as that skill gives it{{end}}, then ` + "`<binary> prove --resume`" + `. Alignment's judge tickets do not wait for the discriminating-power proof, so no module's wave waits for another's.
4. A module whose ` + "`prove`" + ` exits with code 0 or 1 is done: report its verdict lines. For exit code 2, or a ` + "`host_unresponsive`" + ` finding, report it to the user and stop that module. Do not loop.
5. When every present module is done, publish the local proofs once, where Shallnot's gate reads them: run ` + "`{{.Verify}}`" + `. When ` + "`" + pluginfiles.VerifyBinary + "`" + ` is not on ` + "`PATH`" + `, tell the user: the proofs stay unpublished and the gate reports them stale. Report a rejection by the verifier with its message.
6. Report, per module, its last exit code and verdict lines, the modules that were missing and whether the proofs were published.
`

func skillFile() (File, error) {
	content, err := renderSkill(true)
	return File{Path: PluginDirectory + "/skills/" + SkillName + "/SKILL.md", Content: content}, err
}

// PortableDirectory is the portable plugin's tree, relative to `umbrella/`.
const PortableDirectory = "plugin/portable"

// PortableFiles returns the files of the portable plugin of the Agent Plugins
// standard, relative to `umbrella/plugin/portable`. The standard has no
// dependency mechanism and each module plugin ships its own server and skill, so
// the umbrella declares no MCP server and its skill pre-approves no tool, which
// the Agent Skills format leaves experimental.
func PortableFiles() ([]File, error) {
	manifest, err := pluginfiles.PortableManifest{
		Name: PluginName, Version: Version,
		Description: "Runs the proofs of the installed spec-proof modules together.",
	}.ManifestFile()
	if err != nil {
		return nil, err
	}
	servers, err := pluginfiles.MCPFile(nil)
	if err != nil {
		return nil, err
	}
	skill, err := renderSkill(false)
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: manifest.Path, Content: manifest.Content},
		{Path: servers.Path, Content: servers.Content},
		{Path: pluginfiles.PortableSkillPath(SkillName), Content: skill},
	}, nil
}

func renderSkill(restricted bool) ([]byte, error) {
	allowed := make([]string, 0, 3*len(Modules)+1)
	names := make([]string, 0, len(Modules))
	for _, module := range Modules {
		allowed = append(allowed, "Bash("+module.Binary+" doctor)", "Bash("+module.Binary+" prove *)", "Bash("+module.Binary+" request list)")
		names = append(names, "`"+module.Binary+" doctor`")
	}
	allowed = append(allowed, "Bash("+Relay.Binary+" drain *)", "Bash("+pluginfiles.VerifyLocalCommand+")")
	parsed, err := template.New("skill").Parse(skillTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse the skill template: %w", err)
	}
	data := skillData{
		Name: SkillName, Modules: Modules, Relay: Relay.Binary,
		Names: joinWithAnd(names), Verify: pluginfiles.VerifyLocalCommand,
	}
	if restricted {
		data.Allowed = strings.Join(allowed, ", ")
	}
	var buffer bytes.Buffer
	if err := parsed.Execute(&buffer, data); err != nil {
		return nil, fmt.Errorf("render the skill: %w", err)
	}
	return buffer.Bytes(), nil
}

// joinWithAnd spells a list as `a, b and c`.
func joinWithAnd(items []string) string {
	last := len(items) - 1
	return strings.Join(items[:last], ", ") + " and " + items[last]
}

func umbrellaMarketplaceFile(versions Versions) (File, error) {
	return marketplaceFile(MarketplacePath, "./"+PluginDirectory, versions)
}

func rootMarketplaceFile(versions Versions) (File, error) {
	return marketplaceFile(RootMarketplacePath, "./"+UmbrellaDirectory+"/"+PluginDirectory, versions)
}

// RootFiles returns every generated file of the repository root, relative to the
// root: the marketplace that lists the same plugins as the umbrella's own, with the
// thin plugin's path taken from the root.
func RootFiles(versions Versions) ([]File, error) {
	file, err := rootMarketplaceFile(versions)
	if err != nil {
		return nil, err
	}
	return []File{file}, nil
}

// Files returns every generated file of the umbrella, relative to `umbrella/`.
func Files(versions Versions) ([]File, error) {
	var files []File
	builds := []func() (File, error){
		func() (File, error) { return umbrellaMarketplaceFile(versions) }, pluginManifestFile, skillFile,
	}
	for _, build := range builds {
		file, err := build()
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
