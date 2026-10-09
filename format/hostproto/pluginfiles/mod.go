package pluginfiles

import (
	_ "embed"
)

// The Claude Code mod of a module: a hooks module that carries the classification
// plan of the module's binary to the session's model and the answers back. The
// source is one template shared by every module; only the binary name differs.
const (
	// ClaudeModPath is where the plugin ships the mod's source, beside its hooks file.
	ClaudeModPath = "hooks/register.ts"
	// claudeModEntry names the mod's source in the hooks file, relative to that file.
	claudeModEntry = "./register.ts"
)

//go:embed mod/register.ts.tmpl
var claudeModTemplate string

// claudeHooks is the hooks file of the Claude plugin: the command hooks of the
// module and the hooks modules the plugin loads.
type claudeHooks struct {
	snakeHooks
	Modules []string `json:"modules"`
}

// modData fills the template of the mod.
type modData struct {
	Binary string
}

// claudeModFile renders the mod of the module.
func (m Module) claudeModFile() (File, error) {
	content, err := render("claude-mod", claudeModTemplate, modData{Binary: m.Binary})
	if err != nil {
		return File{}, err
	}
	return File{Path: ClaudeModPath, Content: content}, nil
}
