// Package plugin is the single source of the Claude Code plugin of the relay: the
// committed tree under `plugin/claude` is generated here. The plugin holds one
// skill that runs the relay for each proof module; it holds no logic, since every
// step goes through the `musthold-s1` binary. It knows no I/O.
package plugin

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"github.com/RachidChabane/musthold/format/hostproto/pluginfiles"
)

// Identity of the shipped plugin.
const (
	// Name is the name of the plugin.
	Name = "musthold-s1"
	// Version is the version of the shipped files.
	Version = "0.1.0"
	// Binary is the executable the skill runs.
	Binary = "musthold-s1"
	// SkillName is the name of the skill that relays the closed requests.
	SkillName = "musthold-s1-drain"

	author  = "Rachid Chabane"
	license = "Apache-2.0"
	indent  = "  "
)

// Modules lists the proof modules the relay serves, by the name `--module` takes.
var Modules = []string{"dp", "alignment", "completeness"}

// File is one generated file: its path relative to the root of the tree and its
// bytes.
type File struct {
	Path    string
	Content []byte
}

type manifest struct {
	Name        string            `json:"name"`
	Version     string            `json:"version"`
	Description string            `json:"description"`
	Author      map[string]string `json:"author"`
	License     string            `json:"license"`
}

const skillTemplate = `---
name: {{.Name}}
description: Relay the closed questions of the musthold modules to the System One endpoint of the project. Use when a module has closed requests waiting and ` + "`.musthold/config.yaml`" + ` sets ` + "`systemone.endpoint`" + `.
{{- if .Allowed}}
allowed-tools: Bash({{.Binary}} drain *)
{{- end}}
---

# Relay the closed requests

` + "`{{.Binary}} drain`" + ` posts the closed questions a module exported to the System One endpoint of the project and hands the answers back to the module. It is the only code of musthold that opens a connection to a model service.

1. If ` + "`.musthold/config.yaml`" + ` sets no ` + "`systemone.endpoint`" + `, stop: the modules answer their closed requests with host votes.
2. For each module that is installed, run ` + "`{{.Binary}} drain --module <module>`" + ` with the name of the module: {{.Modules}}. A failure leaves the requests of that module for its own fulfil skill.
3. Report how many requests were answered and how many remain.
`

// For returns the files of the plugin, relative to `plugin/claude`.
func For() ([]File, error) { return files(true) }

// Portable returns the files of the portable plugin of the Agent Plugins standard,
// relative to `plugin/portable`: the manifest and the skill. The relay has no MCP
// server, so the plugin ships no `mcp.json`, and the skill pre-approves no tool,
// which the Agent Skills format leaves experimental.
func Portable() ([]File, error) {
	manifest, err := pluginfiles.PortableManifest{Name: Name, Version: Version, Description: description}.ManifestFile()
	if err != nil {
		return nil, err
	}
	skill, err := skillFile(false)
	if err != nil {
		return nil, err
	}
	return []File{{Path: manifest.Path, Content: manifest.Content}, {Path: pluginfiles.PortableSkillPath(SkillName), Content: skill}}, nil
}

const description = "Relays the closed questions of the musthold modules to a System One endpoint."

func skillFile(allowed bool) ([]byte, error) {
	parsed, err := template.New("skill").Parse(skillTemplate)
	if err != nil {
		return nil, fmt.Errorf("parse the skill template: %w", err)
	}
	var skill bytes.Buffer
	if err := parsed.Execute(&skill, struct {
		Name, Binary, Modules string
		Allowed               bool
	}{SkillName, Binary, moduleNames(), allowed}); err != nil {
		return nil, fmt.Errorf("render the skill: %w", err)
	}
	return skill.Bytes(), nil
}

func files(allowed bool) ([]File, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", indent)
	if err := encoder.Encode(manifest{
		Name: Name, Version: Version,
		Description: description,
		Author:      map[string]string{"name": author}, License: license,
	}); err != nil {
		return nil, err
	}
	skill, err := skillFile(allowed)
	if err != nil {
		return nil, err
	}
	return []File{
		{Path: ".claude-plugin/plugin.json", Content: buffer.Bytes()},
		{Path: "skills/" + SkillName + "/SKILL.md", Content: skill},
	}, nil
}

// moduleNames spells the module names of the relay as a sentence: `a`, `b` or `c`.
func moduleNames() string {
	quoted := make([]string, len(Modules))
	for index, name := range Modules {
		quoted[index] = "`" + name + "`"
	}
	last := len(quoted) - 1
	return strings.Join(quoted[:last], ", ") + " or " + quoted[last]
}
