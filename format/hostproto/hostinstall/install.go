package hostinstall

import (
	"errors"
	"fmt"
	"strings"

	"github.com/RachidChabane/spec-proof/format/hostproto/harness"
	"github.com/RachidChabane/spec-proof/format/hostproto/pluginfiles"
)

// Paths and names of what the installer reads and writes.
const (
	// StateDirectory is the project directory that holds spec-proof state.
	StateDirectory = pluginfiles.StateDirectory
	// SharedConfigFile is the configuration file the modules share, inside StateDirectory.
	SharedConfigFile = "config.yaml"
	// Marketplace is the marketplace the module plugins are published in.
	Marketplace = "spec-proof"

	shallnotConfig     = "shallnot.yaml"
	agentsPath         = "AGENTS.md"
	gitignorePath      = ".gitignore"
	claudeSettings     = ".claude/settings.json"
	keyProofs          = "proofs"
	keyRequire         = "require"
	keyTrust           = "trust"
	keyCode            = "code"
	keyOperations      = "operations"
	trustLocal         = "local"
	hookSelectionAuto  = "auto"
	hookSelectionNone  = "none"
	pluginKeySeparator = "@"
)

// SharedConfigPath is the path of the configuration file the modules share.
const SharedConfigPath = StateDirectory + "/" + SharedConfigFile

// LocalStatePaths are the directories of local state every module keeps out of
// version control: the statements of the proof cache, the verified set built from
// them and the pending files of the modules that wait on an answer.
var LocalStatePaths = []string{pluginfiles.CacheDirectory + "/", pluginfiles.VerifiedDirectory + "/", pluginfiles.PendingDirectory + "/"}

// OperationsExample is the commented block of the shared configuration that shows
// how an operation named by the examples of a spec is bound to code. It is a
// comment because a binding is only valid once it names a callable that exists.
const OperationsExample = `# Bind the operation names of the examples in your specs to the code they call.
# A target names an exported or public callable that exists.
# operations:
#   line_total:
#     python: package.module:function
#     go: import/path:Function
#     typescript: path/file.ts:function
#     java: package.Class:method
`

// shallnotNextSteps tell what the gate configuration needs besides what enable
// writes.
const (
	trustNextStep = "shallnot.yaml: proofs.trust is local, so Shallnot reads the proofs made on this machine. The CI configuration of the gate sets proofs.trust to signed."
	codeNextStep  = "shallnot.yaml: no source root was detected for proofs.code; list the files, directories or globs a proof may name as code, or a proof that names a source file is stale."
)

// Errors of the installer.
var (
	// ErrNoShallnotConfig is returned when enabling in a project that has no shallnot.yaml.
	ErrNoShallnotConfig = errors.New("the project has no shallnot.yaml: run `shallnot init` first")
	// ErrUnknownHookHarness is returned for a harness `--hooks` names that the module has no files for.
	ErrUnknownHookHarness = errors.New("unknown hook harness")
)

// Module describes one proof module to the installer.
type Module struct {
	// Files generates the module's plugin and scaffold files.
	Files pluginfiles.Module
	// ProofKind is the module's key under `proofs` in the configuration, and the
	// kind it adds to Shallnot's `proofs.require`.
	ProofKind string
	// ConfigPath is the configuration file the proof's flag lives in.
	ConfigPath string
	// Required says whether Shallnot accepts the proof in `proofs.require`. An
	// advisory proof is refused there, so enabling it writes no entry.
	Required bool
}

// ConfigSeed is the content of a new configuration file.
type ConfigSeed []byte

// InitOptions select what init installs.
type InitOptions struct {
	Harnesses []harness.Harness
	Seed      ConfigSeed
}

func (m Module) pluginKey() string {
	return m.Files.Plugin() + pluginKeySeparator + Marketplace
}

// SelectHarnesses resolves `--hooks`: `auto` is Claude Code and every harness
// whose configuration the project holds, `none` is none, and a comma-separated
// list names them.
func (m Module) SelectHarnesses(files Files, selection string) ([]harness.Harness, error) {
	switch selection {
	case hookSelectionNone:
		return nil, nil
	case hookSelectionAuto:
		return m.detect(files), nil
	}
	var chosen []harness.Harness
	for _, name := range strings.Split(selection, ",") {
		selected, err := harness.ByFlag(m.Files.Identity, strings.TrimSpace(name))
		if err != nil {
			return nil, fmt.Errorf("%w %q (expected %s)", ErrUnknownHookHarness, name, strings.Join(harness.Flags(), ", "))
		}
		chosen = append(chosen, selected)
	}
	return chosen, nil
}

// markers are the paths whose presence says a project uses a harness; Claude
// Code is always selected.
var markers = map[string][]string{
	harness.FlagCodex:   {".codex"},
	harness.FlagCopilot: {".github/copilot-instructions.md", ".github/hooks", ".github/skills", ".github/instructions", ".github/agents"},
}

func (m Module) detect(files Files) []harness.Harness {
	var found []harness.Harness
	for _, candidate := range harness.All(m.Files.Identity) {
		if candidate.Flag == harness.FlagClaude || anyExists(files, markers[candidate.Flag]) {
			found = append(found, candidate)
		}
	}
	return found
}

func anyExists(files Files, paths []string) bool {
	for _, path := range paths {
		if files.Exists(path) {
			return true
		}
	}
	return false
}

// Installed lists the flags of the harnesses whose files of the module the
// project holds: the Claude Code project settings that enable the plugin, the
// Codex hooks file that runs the module's hooks, and the Copilot hooks file of
// the module. It reads and changes nothing.
func (m Module) Installed(files Files) []string {
	var installed []string
	if fileMentions(files, claudeSettings, m.pluginKey()) {
		installed = append(installed, harness.FlagClaude)
	}
	if m.codexInstalled(files) {
		installed = append(installed, harness.FlagCodex)
	}
	if files.Exists(m.Files.CopilotHooksPath()) {
		installed = append(installed, harness.FlagCopilot)
	}
	return installed
}

// fileMentions reports whether a file exists and holds the text.
func fileMentions(files Files, path, text string) bool {
	content, found, err := files.Read(path)
	return err == nil && found && strings.Contains(string(content), text)
}

// BuildInit plans the install of the module's files: its configuration, its local
// state ignored, and the files of each harness. It enables nothing.
func (m Module) BuildInit(files Files, options InitOptions) (Plan, error) {
	provisioners := []provisioner{
		{path: m.ConfigPath, desired: m.seedConfig(options.Seed)},
		{path: gitignorePath, desired: ignoreLines(LocalStatePaths)},
	}
	var steps []string
	for _, selected := range options.Harnesses {
		more, step, err := m.harnessProvisioners(selected)
		if err != nil {
			return Plan{}, err
		}
		provisioners = append(provisioners, more...)
		steps = append(steps, step...)
	}
	return buildPlan(files, provisioners, steps)
}

func (m Module) harnessProvisioners(selected harness.Harness) ([]provisioner, []string, error) {
	generated, err := m.Files.For(selected)
	if err != nil {
		return nil, nil, err
	}
	var provisioners []provisioner
	var steps []string
	switch selected.Flag {
	case harness.FlagClaude:
		provisioners = append(provisioners, provisioner{path: claudeSettings, desired: enablePlugin(m.pluginKey())})
		steps = append(steps, fmt.Sprintf("Claude Code: install the %s plugin from the %s marketplace; the project settings enable it.", m.Files.Plugin(), Marketplace))
	default:
		for _, file := range generated {
			provisioners = append(provisioners, m.filePath(file))
		}
	}
	if selected.Flag == harness.FlagCodex {
		steps = append(steps, "Codex runs project hooks once `hooks = true` is set under `[features]` in its config.toml and the project is trusted.")
	}
	return provisioners, steps, nil
}

// filePath is the strategy for one generated file: hook files shared with other
// tools are merged into, every other file is owned.
func (m Module) filePath(file pluginfiles.File) provisioner {
	switch file.Path {
	case pluginfiles.CodexHooksPath:
		return provisioner{path: file.Path, desired: func(current []byte) ([]byte, error) {
			if current == nil {
				return file.Content, nil
			}
			return mergeHookEntries(current, file.Content)
		}}
	case pluginfiles.CodexConfigPath:
		return provisioner{path: file.Path, desired: m.codexConfigBlock(file).set}
	}
	return provisioner{path: file.Path, desired: fixed(file.Content)}
}

// codexConfigBlock is the module's table of the project's Codex config.toml.
func (m Module) codexConfigBlock(file pluginfiles.File) markedBlock {
	return markedBlock{begin: m.Files.CodexConfigBegin(), end: m.Files.CodexConfigEnd(), text: string(file.Content)}
}

// codexConfigProvisioner is the strategy for the module's table of the Codex
// config.toml: put it in, or take it out.
func (m Module) codexConfigProvisioner(present bool) (provisioner, error) {
	files, err := m.Files.For(harness.Codex(m.Files.Identity))
	if err != nil {
		return provisioner{}, err
	}
	for _, file := range files {
		if file.Path != pluginfiles.CodexConfigPath {
			continue
		}
		block := m.codexConfigBlock(file)
		if present {
			return provisioner{path: file.Path, desired: block.set}, nil
		}
		return provisioner{path: file.Path, desired: block.remove}, nil
	}
	return provisioner{}, fmt.Errorf("the Codex files of %s have no %s", m.Files.Plugin(), pluginfiles.CodexConfigPath)
}

// seedConfig writes the configuration of a project that has none, and makes sure
// the module's proof has an entry, disabled, in one that has. A configuration the
// installer writes or extends ends with the commented example of the operations
// block unless it binds operations or already holds the example; one that has the
// proof's entry already is left as it is.
func (m Module) seedConfig(seed ConfigSeed) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		if current == nil {
			return withOperationsExample(seed)
		}
		text, err := parseYAML(current)
		if err != nil {
			return nil, err
		}
		if _, present := text.boolValue(keyProofs, m.ProofKind); present {
			return current, nil
		}
		extended, err := edited(current, text, func() (bool, error) { return text.setBool(keyProofs, m.ProofKind, false) })
		if err != nil {
			return nil, err
		}
		return withOperationsExample(extended)
	}
}

// withOperationsExample appends the commented example of the operations block to a
// configuration that neither binds operations nor holds the example.
func withOperationsExample(content []byte) ([]byte, error) {
	text, err := parseYAML(content)
	if err != nil {
		return nil, err
	}
	if text.hasKey(keyOperations) || strings.Contains(string(content), OperationsExample) {
		return content, nil
	}
	body := string(content)
	if body != "" && !strings.HasSuffix(body, newline) {
		body += newline
	}
	if body != "" {
		body += newline
	}
	return []byte(body + OperationsExample), nil
}

// edited runs an edit of a parsed document and returns the document's bytes, or the
// current ones when the edit changed nothing.
func edited(current []byte, text *yamlText, edit func() (bool, error)) ([]byte, error) {
	changed, err := edit()
	if err != nil {
		return nil, err
	}
	if !changed {
		return current, nil
	}
	return text.bytes(), nil
}

// ignoreLines appends the lines a .gitignore does not hold yet.
func ignoreLines(lines []string) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		text := string(current)
		held := map[string]bool{}
		for _, line := range strings.Split(text, "\n") {
			held[strings.TrimSpace(line)] = true
		}
		var missing []string
		for _, line := range lines {
			if !held[line] {
				missing = append(missing, line)
			}
		}
		if len(missing) == 0 {
			return current, nil
		}
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return []byte(text + strings.Join(missing, "\n") + "\n"), nil
	}
}

// advisoryNote is what `enable` tells for a proof Shallnot cannot require.
func (m Module) advisoryNote() string {
	return fmt.Sprintf("%s is advisory: Shallnot refuses it in `proofs.require`, so enabling it writes no require entry and the gate never blocks on it.", m.ProofKind)
}

// BuildEnable plans enabling the proof: its flag in the module's configuration, its
// entry in Shallnot's `proofs.require` when Shallnot accepts the proof there, the
// `proofs.trust` and `proofs.code` the local loop needs when the gate configuration
// sets none, and its block of AGENTS.md.
func (m Module) BuildEnable(files Files) (Plan, error) {
	provisioners := []provisioner{{path: m.ConfigPath, desired: m.setProofFlag(true)}}
	var steps []string
	if m.Required {
		if !files.Exists(shallnotConfig) {
			return Plan{}, ErrNoShallnotConfig
		}
		roots := SourceRoots(files)
		provisioners = append(provisioners, provisioner{path: shallnotConfig, desired: m.enableInGate(roots)})
		steps = append(steps, trustNextStep)
		if len(roots) == 0 {
			steps = append(steps, codeNextStep)
		}
	} else {
		steps = append(steps, m.advisoryNote())
	}
	provisioners = append(provisioners, provisioner{path: agentsPath, desired: m.setAgentsBlock})
	if m.codexInstalled(files) {
		codex, err := m.codexConfigProvisioner(true)
		if err != nil {
			return Plan{}, err
		}
		provisioners = append(provisioners, codex)
	}
	return buildPlan(files, provisioners, steps)
}

// codexInstalled reports whether the project runs the module's hooks on Codex.
func (m Module) codexInstalled(files Files) bool {
	return fileMentions(files, pluginfiles.CodexHooksPath, m.Files.HookCommandMarker())
}

// BuildDisable plans the reverse of BuildEnable, leaving the files of the
// harnesses in place except the module's table of the Codex config.toml, which
// starts the executor server.
func (m Module) BuildDisable(files Files) (Plan, error) {
	provisioners := []provisioner{{path: m.ConfigPath, desired: m.disableFlag}}
	if m.Required {
		provisioners = append(provisioners, provisioner{path: shallnotConfig, desired: m.dropFromGate})
	}
	provisioners = append(provisioners, provisioner{path: agentsPath, desired: m.removeAgentsBlock})
	codex, err := m.codexConfigProvisioner(false)
	if err != nil {
		return Plan{}, err
	}
	return buildPlan(files, append(provisioners, codex), nil)
}

func (m Module) setProofFlag(enabled bool) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		text, err := parseYAML(current)
		if err != nil {
			return nil, err
		}
		return edited(current, text, func() (bool, error) { return text.setBool(keyProofs, m.ProofKind, enabled) })
	}
}

// disableFlag turns the proof off in a configuration that exists.
func (m Module) disableFlag(current []byte) ([]byte, error) {
	if current == nil {
		return nil, nil
	}
	return m.setProofFlag(false)(current)
}

// enableInGate adds the proof's kind to `proofs.require` of shallnot.yaml, sets
// `proofs.trust` to local when it is not set and fills `proofs.code` with the
// source roots when it lists none. The settings other modules share are never
// changed once they are set.
func (m Module) enableInGate(roots []string) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		if current == nil {
			return nil, nil
		}
		return editChain(current,
			func(text *yamlText) (bool, error) { return text.addToList(keyProofs, keyRequire, m.ProofKind) },
			func(text *yamlText) (bool, error) { return text.setStringIfAbsent(keyProofs, keyTrust, trustLocal) },
			func(text *yamlText) (bool, error) { return text.fillList(keyProofs, keyCode, roots) },
		)
	}
}

// dropFromGate removes the proof's kind from `proofs.require` of shallnot.yaml and
// leaves `proofs.trust` and `proofs.code`, which the other modules rely on too.
func (m Module) dropFromGate(current []byte) ([]byte, error) {
	if current == nil {
		return nil, nil
	}
	return editChain(current, func(text *yamlText) (bool, error) {
		return text.removeFromList(keyProofs, keyRequire, m.ProofKind)
	})
}

// editChain applies edits to a YAML document one after the other, reading the
// document again after each, since an edit moves the lines the next one locates.
func editChain(current []byte, edits ...func(*yamlText) (bool, error)) ([]byte, error) {
	content := current
	for _, edit := range edits {
		text, err := parseYAML(content)
		if err != nil {
			return nil, err
		}
		changed, err := edit(text)
		if err != nil {
			return nil, err
		}
		if changed {
			content = text.bytes()
		}
	}
	return content, nil
}
