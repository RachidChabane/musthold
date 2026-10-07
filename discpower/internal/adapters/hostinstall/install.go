package hostinstall

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostfiles"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/hostharness"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

// Paths and names of what the installer reads and writes.
const (
	configPath        = projectconfig.Directory + "/" + projectconfig.FileName
	shallnotConfig    = "shallnot.yaml"
	agentsPath        = "AGENTS.md"
	gitignorePath     = ".gitignore"
	claudeSettings    = ".claude/settings.json"
	codexHooksPath    = ".codex/hooks.json"
	keyProofs         = "proofs"
	keyRequire        = "require"
	pluginKey         = hostfiles.Binary + "@spec-proof"
	proofKind         = format.KindDiscriminatingPower
	hookSelectionAuto = "auto"
	hookSelectionNone = "none"
)

// ignoredPaths are the module's local state, kept out of version control.
var ignoredPaths = []string{projectconfig.Directory + "/cache/", projectconfig.Directory + "/verified/"}

// Errors of the installer.
var (
	// ErrNoShallnotConfig is returned when enabling in a project that has no shallnot.yaml.
	ErrNoShallnotConfig = errors.New("the project has no shallnot.yaml: run `shallnot init` first")
	// ErrUnknownHookHarness is returned for a harness `--hooks` names that the module has no files for.
	ErrUnknownHookHarness = errors.New("unknown hook harness")
)

// ConfigSeed is the content of a new `.spec-proof/config.yaml`.
type ConfigSeed []byte

// InitOptions select what init installs.
type InitOptions struct {
	Root      string
	Harnesses []hostharness.Harness
	Seed      ConfigSeed
}

// SelectHarnesses resolves `--hooks`: `auto` is Claude Code and every harness
// whose configuration the project holds, `none` is none, and a comma-separated
// list names them.
func SelectHarnesses(root, selection string) ([]hostharness.Harness, error) {
	switch selection {
	case hookSelectionNone:
		return nil, nil
	case hookSelectionAuto:
		return detect(root), nil
	}
	var chosen []hostharness.Harness
	for _, name := range strings.Split(selection, ",") {
		harness, err := hostharness.ByFlag(strings.TrimSpace(name))
		if err != nil {
			return nil, fmt.Errorf("%w %q (expected %s)", ErrUnknownHookHarness, name, strings.Join(hostharness.Flags(), ", "))
		}
		chosen = append(chosen, harness)
	}
	return chosen, nil
}

// markers are the paths whose presence says a project uses a harness; Claude
// Code is always selected.
var markers = map[string][]string{
	hostharness.FlagCodex:   {".codex"},
	hostharness.FlagCopilot: {".github/copilot-instructions.md", ".github/hooks", ".github/skills", ".github/instructions", ".github/agents"},
}

func detect(root string) []hostharness.Harness {
	var found []hostharness.Harness
	for _, harness := range hostharness.All() {
		if harness.Flag == hostharness.FlagClaude || anyExists(root, markers[harness.Flag]) {
			found = append(found, harness)
		}
	}
	return found
}

func anyExists(root string, paths []string) bool {
	for _, path := range paths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(path))); err == nil {
			return true
		}
	}
	return false
}

// BuildInit plans the install of the module's files: its configuration, its local
// state ignored, and the files of each harness. It enables nothing.
func BuildInit(options InitOptions) (Plan, error) {
	provisioners := []provisioner{
		{path: configPath, desired: seedConfig(options.Seed)},
		{path: gitignorePath, desired: ignoreLines(ignoredPaths)},
	}
	var steps []string
	for _, harness := range options.Harnesses {
		more, step, err := harnessProvisioners(harness)
		if err != nil {
			return Plan{}, err
		}
		provisioners = append(provisioners, more...)
		steps = append(steps, step...)
	}
	return buildPlan(options.Root, provisioners, steps)
}

func harnessProvisioners(harness hostharness.Harness) ([]provisioner, []string, error) {
	files, err := hostfiles.For(harness)
	if err != nil {
		return nil, nil, err
	}
	var provisioners []provisioner
	var steps []string
	switch harness.Flag {
	case hostharness.FlagClaude:
		provisioners = append(provisioners, provisioner{path: claudeSettings, desired: enablePlugin(pluginKey)})
		steps = append(steps, "Claude Code: install the spec-proof-dp plugin from the spec-proof marketplace; the project settings enable it.")
	default:
		for _, file := range files {
			provisioners = append(provisioners, filePath(file))
		}
	}
	if harness.Flag == hostharness.FlagCodex {
		steps = append(steps, "Codex runs project hooks once `hooks = true` is set under `[features]` in its config.toml and the project is trusted.")
	}
	return provisioners, steps, nil
}

// filePath is the strategy for one generated file: hook files shared with other
// tools are merged into, every other file is owned.
func filePath(file hostfiles.File) provisioner {
	if file.Path == codexHooksPath {
		return provisioner{path: file.Path, desired: func(current []byte) ([]byte, error) {
			if current == nil {
				return file.Content, nil
			}
			return mergeHookEntries(current, file.Content)
		}}
	}
	return provisioner{path: file.Path, desired: fixed(file.Content)}
}

// seedConfig writes the configuration of a project that has none, and makes sure
// the module's proof has an entry, disabled, in one that has.
func seedConfig(seed ConfigSeed) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		if current == nil {
			return seed, nil
		}
		text, err := parseYAML(current)
		if err != nil {
			return nil, err
		}
		if _, present := text.boolValue(keyProofs, string(proofKind)); present {
			return current, nil
		}
		return edited(current, text, func() (bool, error) { return text.setBool(keyProofs, string(proofKind), false) })
	}
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

// BuildEnable plans enabling the proof: its flag in the module's configuration, its
// entry in Shallnot's `proofs.require` and its block of AGENTS.md.
func BuildEnable(root string) (Plan, error) {
	if _, err := os.Stat(filepath.Join(root, shallnotConfig)); err != nil {
		return Plan{}, ErrNoShallnotConfig
	}
	return buildPlan(root, []provisioner{
		{path: configPath, desired: setProofFlag(true)},
		{path: shallnotConfig, desired: requireProof(true)},
		{path: agentsPath, desired: setAgentsBlock},
	}, nil)
}

// BuildDisable plans the reverse of BuildEnable, leaving the files of the
// harnesses in place.
func BuildDisable(root string) (Plan, error) {
	return buildPlan(root, []provisioner{
		{path: configPath, desired: disableFlag},
		{path: shallnotConfig, desired: requireProof(false)},
		{path: agentsPath, desired: removeAgentsBlock},
	}, nil)
}

func setProofFlag(enabled bool) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		text, err := parseYAML(current)
		if err != nil {
			return nil, err
		}
		return edited(current, text, func() (bool, error) { return text.setBool(keyProofs, string(proofKind), enabled) })
	}
}

// disableFlag turns the proof off in a configuration that exists.
func disableFlag(current []byte) ([]byte, error) {
	if current == nil {
		return nil, nil
	}
	return setProofFlag(false)(current)
}

// requireProof adds or removes the proof's kind in `proofs.require` of shallnot.yaml.
func requireProof(add bool) func([]byte) ([]byte, error) {
	return func(current []byte) ([]byte, error) {
		if current == nil {
			return nil, nil
		}
		text, err := parseYAML(current)
		if err != nil {
			return nil, err
		}
		return edited(current, text, func() (bool, error) {
			if add {
				return text.addToList(keyProofs, keyRequire, string(proofKind))
			}
			return text.removeFromList(keyProofs, keyRequire, string(proofKind))
		})
	}
}
