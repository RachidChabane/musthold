// Package hostinstall installs and removes the module's files in a project on
// disk. What the files hold and how they merge into the files other tools share
// is the shared installer of the host protocol; this package is the module's
// definition of it and the disk it reads and writes.
package hostinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/completeness/internal/domain/hostfiles"
	"github.com/RachidChabane/musthold/completeness/internal/domain/hostharness"
	"github.com/RachidChabane/musthold/format"
	shared "github.com/RachidChabane/musthold/format/hostproto/hostinstall"
)

// Permissions of what the installer writes.
const (
	dirMode  os.FileMode = 0o755
	fileMode os.FileMode = 0o644
)

// Re-exported names of the shared installer.
type (
	// Change says what the installer does to one file.
	Change = shared.Change
	// Action is one file in the state the installer wants.
	Action = shared.Action
	// ConfigSeed is the content of a new `.musthold/config.yaml`.
	ConfigSeed = shared.ConfigSeed
)

// OperationsExample is the commented example of the operations block that a
// configuration the installer writes ends with.
const OperationsExample = shared.OperationsExample

// Errors of the installer.
var (
	// ErrNoShallnotConfig is returned when enabling in a project that has no shallnot.yaml.
	ErrNoShallnotConfig = shared.ErrNoShallnotConfig
	// ErrUnknownHookHarness is returned for a harness `--hooks` names that the module has no files for.
	ErrUnknownHookHarness = shared.ErrUnknownHookHarness
	// ErrUnsupportedYAML is returned for a document the line-level edits cannot extend.
	ErrUnsupportedYAML = shared.ErrUnsupportedYAML
)

// proofKind is the proof the module produces: its key under `proofs`.
const proofKind = format.KindCompleteness

// module is the module as the installer sees it. Completeness is advisory:
// Shallnot refuses it in `proofs.require`, so enabling it writes no require entry.
var module = shared.Module{
	Files:      hostfiles.Module,
	ProofKind:  string(proofKind),
	ConfigPath: shared.SharedConfigPath,
	Required:   false,
}

// Seed is the content of a configuration file the module creates: its proof,
// disabled.
func Seed() ConfigSeed { return ConfigSeed("proofs:\n  " + string(proofKind) + ": false\n") }

// InitOptions select what init installs.
type InitOptions struct {
	Root      string
	Harnesses []hostharness.Harness
	Seed      ConfigSeed
}

// Plan is everything an install, an enable or a disable wants for a project root.
type Plan struct {
	shared.Plan
	Root string
}

// Apply writes the files that differ from the plan.
func (p Plan) Apply() error { return p.Plan.Apply(disk{root: p.Root}) }

// SelectHarnesses resolves `--hooks`: `auto` is Claude Code and every harness
// whose configuration the project holds, `none` is none, and a comma-separated
// list names them.
func SelectHarnesses(root, selection string) ([]hostharness.Harness, error) {
	return module.SelectHarnesses(disk{root: root}, selection)
}

// Installed lists the flags of the harnesses whose files of the module the
// project holds.
func Installed(root string) []string { return module.Installed(disk{root: root}) }

// BuildInit plans the install of the module's files: its configuration, its local
// state ignored, and the files of each harness. It enables nothing.
func BuildInit(options InitOptions) (Plan, error) {
	plan, err := module.BuildInit(disk{root: options.Root}, shared.InitOptions{Harnesses: options.Harnesses, Seed: options.Seed})
	return Plan{Plan: plan, Root: options.Root}, err
}

// BuildEnable plans enabling the proof: its flag in the shared configuration, its
// entry in Shallnot's `proofs.require` and its block of AGENTS.md.
func BuildEnable(root string) (Plan, error) {
	plan, err := module.BuildEnable(disk{root: root})
	return Plan{Plan: plan, Root: root}, err
}

// BuildDisable plans the reverse of BuildEnable, leaving the files of the
// harnesses in place.
func BuildDisable(root string) (Plan, error) {
	plan, err := module.BuildDisable(disk{root: root})
	return Plan{Plan: plan, Root: root}, err
}

// disk is the project tree on the file system.
type disk struct{ root string }

var _ shared.Files = disk{}

func (d disk) path(relative string) string {
	return filepath.Join(d.root, filepath.FromSlash(relative))
}

func (d disk) Read(path string) ([]byte, bool, error) {
	content, err := os.ReadFile(d.path(path))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return content, err == nil, err
}

func (d disk) Directories(path string) []string {
	entries, err := os.ReadDir(d.path(path))
	if err != nil {
		return nil
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			names = append(names, entry.Name())
		}
	}
	return names
}

func (d disk) Exists(path string) bool {
	_, err := os.Stat(d.path(path))
	return err == nil
}

func (d disk) Write(path string, content []byte) error {
	target := d.path(path)
	if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
		return err
	}
	return os.WriteFile(target, content, fileMode)
}

func (d disk) Remove(path string) error {
	if err := os.Remove(d.path(path)); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
