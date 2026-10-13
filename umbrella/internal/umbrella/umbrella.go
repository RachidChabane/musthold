// Package umbrella is the single source of the files of the spec-proof umbrella:
// the marketplace that lists Shallnot's plugin, the three proof-module plugins and
// the relay beside the thin plugin, and the thin plugin itself. The thin plugin
// declares its dependencies and holds two skills: `prove-all`, which runs the fulfil
// loop of each module that is present and holds no logic of its own, and `scan`,
// which drives the host loop of the discriminating-power scan. The committed
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

	"github.com/RachidChabane/spec-proof/format/hostproto"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

// Identity of the umbrella.
const (
	// MarketplaceName is the name of the marketplace, and of the thin plugin's
	// own marketplace in the `<plugin>@<marketplace>` keys `init` enables.
	MarketplaceName = "spec-proof"
	// PluginName is the name of the thin plugin.
	PluginName = "spec-proof"
	// SkillName is the name of the skill that proves a change with every module.
	SkillName = "prove-all"
	// ScanSkillName is the name of the skill that runs a scan with a host.
	ScanSkillName = "scan"
	// DispatcherBinary is the command that runs the binary of the module that owns
	// a command.
	DispatcherBinary = "spec-proof"
	// scanModule is the name the relay gives the module that owns the scan.
	scanModule = "dp"
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
// public GitHub repository.
func SubdirSource(repository, path string) any {
	return gitSubdir{Source: "git-subdir", URL: repository, Path: path}
}

// marketplaceFile builds the marketplace at path, whose relative plugin sources
// are resolved against the directory that holds the marketplace's `.claude-plugin`
// directory: thinPluginSource is where the thin plugin's tree is from there.
func marketplaceFile(path, thinPluginSource string) (File, error) {
	listing := marketplace{Name: MarketplaceName, Owner: owner{Name: ownerName, URL: ownerURL}}
	listing.Metadata.Description = "spec-proof: proofs that the tests of a project check what its requirements say, for coding agents."
	listing.Plugins = append(listing.Plugins, marketplaceEntry{
		Name: Shallnot.Plugin, Source: SubdirSource(Shallnot.Repository, Shallnot.Path), Description: Shallnot.Summary,
	})
	for _, module := range append(append([]Module{}, Modules...), Relay) {
		listing.Plugins = append(listing.Plugins, marketplaceEntry{
			Name: module.Plugin, Source: SubdirSource(repositoryID, module.PluginPath()), Description: module.Summary,
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

func skillFile(name string, render func(restricted bool) ([]byte, error)) (File, error) {
	content, err := render(true)
	return File{Path: skillPath(name), Content: content}, err
}

func skillPath(name string) string { return PluginDirectory + "/skills/" + name + "/SKILL.md" }

func proveAllSkillFile() (File, error) { return skillFile(SkillName, renderSkill) }

func scanSkillFile() (File, error) { return skillFile(ScanSkillName, renderScanSkill) }

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
	scanSkill, err := renderScanSkill(false)
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: manifest.Path, Content: manifest.Content},
		{Path: servers.Path, Content: servers.Content},
		{Path: pluginfiles.PortableSkillPath(SkillName), Content: skill},
		{Path: pluginfiles.PortableSkillPath(ScanSkillName), Content: scanSkill},
	}, nil
}

func renderSkill(restricted bool) ([]byte, error) {
	allowed := make([]string, 0, 3*len(Modules)+1)
	names := make([]string, 0, len(Modules))
	for _, module := range Modules {
		allowed = append(allowed, "Bash("+module.Binary+" doctor)", "Bash("+module.Binary+" prove *)", "Bash("+module.Binary+" "+hostproto.ProveListCommand+")")
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

// scanSkillData fills the scan template.
type scanSkillData struct {
	Name       string
	Allowed    string
	Dispatcher string
	Binary     string
	Relay      string
	Module     string
	Fulfil     string
	Listing    string
}

const scanSkillTemplate = `---
name: {{.Name}}
description: Scan the code, or a pull request, for the promises its sources make and the tests that do not hold them, answering the model work of the scan {{if .Allowed}}with blind executors{{else}}as the fulfil skill of ` + "`{{.Binary}}`" + ` gives{{end}}. Use when asked to scan a repository or a pull request, and when ` + "`{{.Dispatcher}} scan --host`" + ` exits with code 3.
{{- if .Allowed}}
allowed-tools: {{.Allowed}}
{{- end}}
---

# Scan with a host

` + "`{{.Dispatcher}} scan --host`" + ` stops with exit code 3 while it waits for model work. Each request is a ticket of the scan's own session. The tickets of any other session are not yours.{{if .Allowed}} Never answer a ticket yourself and never read what a ticket asks.{{end}}

1. Run ` + "`{{.Dispatcher}} scan --host [<PR>]`" + `, with the pull request number when the user gave one, and the flags the user gave.
2. Exit code 0: the scan finished. Show its report and stop. Exit code 2: show the refusal to the user and stop. Do not loop.
3. Exit code 3: read the line ` + "`session scan:<key>`" + ` of the output and keep that session. Answer its tickets with steps 4 to 6.
4. If ` + "`{{.Relay}}`" + ` is on ` + "`PATH`" + ` and ` + "`.spec-proof/config.yaml`" + ` sets ` + "`systemone.endpoint`" + `, run ` + "`{{.Relay}} drain --module {{.Module}} --session scan:<key>`" + `. A failure leaves the tickets for the next step.
5. Run ` + "`{{.Binary}} {{.Listing}} scan:<key>`" + `. Each line is ` + "`<ticket> <role>`" + `. List no other session.
6. {{if .Allowed}}For each ticket, launch one foreground ` + "`Agent`" + ` with ` + "`subagent_type`" + ` set to the role on its line and ` + "`prompt`" + ` set to ` + "`ticket <ticket>`" + `. Launch at most 4 in one message and wait for them. Set no model and add nothing to the prompt. The reply of an executor is a receipt: do not act on it.{{else}}Answer each ticket as the skill ` + "`{{.Fulfil}}`" + ` gives for a ticket, with the session ` + "`scan:<key>`" + ` in place of ` + "`prove`" + `.{{end}}
7. Run ` + "`{{.Dispatcher}} scan --host --resume [<PR>]`" + `, with the same pull request number and flags as in step 1. Go back to step 2 with its exit code.
`

func renderScanSkill(restricted bool) ([]byte, error) {
	parsed, err := template.New("scan-skill").Parse(scanSkillTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse the scan skill template: %w", err)
	}
	module := Modules[0]
	data := scanSkillData{
		Name: ScanSkillName, Dispatcher: DispatcherBinary, Binary: module.Binary, Relay: Relay.Binary,
		Module: scanModule, Fulfil: module.FulfilSkill(),
		Listing: hostproto.CommandRequest + " " + hostproto.SubcommandList + " " + hostproto.FlagSession,
	}
	if restricted {
		data.Allowed = strings.Join(scanAllowedTools(module), ", ")
	}
	var buffer bytes.Buffer
	if err := parsed.Execute(&buffer, data); err != nil {
		return nil, fmt.Errorf("render the scan skill: %w", err)
	}
	return buffer.Bytes(), nil
}

// scanAllowedTools lists the commands the scan skill pre-approves: the scan, the
// listing of tickets and the relay's drain, and no command that answers one.
func scanAllowedTools(module Module) []string {
	return []string{
		"Bash(" + DispatcherBinary + " scan *)",
		"Bash(" + module.Binary + " " + hostproto.CommandRequest + " " + hostproto.SubcommandList + " *)",
		"Bash(" + Relay.Binary + " drain *)",
	}
}

// joinWithAnd spells a list as `a, b and c`.
func joinWithAnd(items []string) string {
	last := len(items) - 1
	return strings.Join(items[:last], ", ") + " and " + items[last]
}

func umbrellaMarketplaceFile() (File, error) {
	return marketplaceFile(MarketplacePath, "./"+PluginDirectory)
}

func rootMarketplaceFile() (File, error) {
	return marketplaceFile(RootMarketplacePath, "./"+UmbrellaDirectory+"/"+PluginDirectory)
}

// RootFiles returns every generated file of the repository root, relative to the
// root: the marketplace that lists the same plugins as the umbrella's own, with the
// thin plugin's path taken from the root.
func RootFiles() ([]File, error) {
	file, err := rootMarketplaceFile()
	if err != nil {
		return nil, err
	}
	return []File{file}, nil
}

// Files returns every generated file of the umbrella, relative to `umbrella/`.
func Files() ([]File, error) {
	var files []File
	for _, build := range []func() (File, error){umbrellaMarketplaceFile, pluginManifestFile, proveAllSkillFile, scanSkillFile} {
		file, err := build()
		if err != nil {
			return nil, err
		}
		files = append(files, file)
	}
	return files, nil
}
