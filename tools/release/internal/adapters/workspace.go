package adapters

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"

	"github.com/RachidChabane/spec-proof/tools/release/internal/domain"
)

const (
	goModFile        = "go.mod"
	manifestFileName = "plugin.json"
	directoryMode    = 0o755
)

// Disk reads and writes files under a root. A relative path is relative to the
// root; an absolute path is used as it is.
type Disk struct{ root string }

// NewDisk returns a file store rooted at root.
func NewDisk(root string) Disk { return Disk{root: root} }

func (d Disk) resolve(name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(d.root, name)
}

// ReadFile reads a file.
func (d Disk) ReadFile(name string) ([]byte, error) { return os.ReadFile(d.resolve(name)) }

// WriteFile writes a file with a mode, creating its directory.
func (d Disk) WriteFile(name string, data []byte, mode int64) error {
	target := d.resolve(name)
	if err := os.MkdirAll(filepath.Dir(target), directoryMode); err != nil {
		return err
	}
	if err := os.WriteFile(target, data, fs.FileMode(mode)); err != nil {
		return err
	}
	return os.Chmod(target, fs.FileMode(mode))
}

// RemoveAll removes a path and what it holds.
func (d Disk) RemoveAll(name string) error { return os.RemoveAll(d.resolve(name)) }

// Workspace reads the committed files the pre-flight rules inspect.
type Workspace struct{ disk Disk }

// NewWorkspace returns a reader of the repository rooted at root.
func NewWorkspace(root string) Workspace { return Workspace{disk: NewDisk(root)} }

// GoMod reads the replace and exclude rules of a module's go.mod and the version
// of the format library it requires.
func (w Workspace) GoMod(module string) (domain.GoModFacts, error) {
	path := filepath.Join(module, goModFile)
	data, err := w.disk.ReadFile(path)
	if err != nil {
		return domain.GoModFacts{}, err
	}
	parsed, err := modfile.Parse(path, data, nil)
	if err != nil {
		return domain.GoModFacts{}, err
	}
	var facts domain.GoModFacts
	for _, replace := range parsed.Replace {
		facts.Replaces = append(facts.Replaces, fmt.Sprintf("%s => %s", moduleVersion(replace.Old.Path, replace.Old.Version), moduleVersion(replace.New.Path, replace.New.Version)))
	}
	for _, exclude := range parsed.Exclude {
		facts.Excludes = append(facts.Excludes, moduleVersion(exclude.Mod.Path, exclude.Mod.Version))
	}
	for _, requirement := range parsed.Require {
		if requirement.Mod.Path == domain.FormatModulePath {
			facts.FormatRequirement = requirement.Mod.Version
		}
	}
	return facts, nil
}

func moduleVersion(path, version string) string {
	if version == "" {
		return path
	}
	return path + " " + version
}

// Manifests reads the version of every plugin.json under the roots.
func (w Workspace) Manifests(roots []string) ([]domain.ManifestFact, error) {
	var manifests []domain.ManifestFact
	for _, root := range roots {
		base := w.disk.resolve(root)
		err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || entry.Name() != manifestFileName {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			version, err := domain.ParseManifestVersion(data)
			if err != nil {
				return fmt.Errorf("%s: %w", path, err)
			}
			relative, err := filepath.Rel(w.disk.root, path)
			if err != nil {
				return err
			}
			manifests = append(manifests, domain.ManifestFact{Path: filepath.ToSlash(relative), Version: version})
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
	}
	slices.SortFunc(manifests, func(a, b domain.ManifestFact) int { return strings.Compare(a.Path, b.Path) })
	return manifests, nil
}

// Marketplace reads the plugins of the root marketplace.
func (w Workspace) Marketplace() ([]domain.MarketplaceEntry, error) {
	data, err := w.disk.ReadFile(domain.MarketplaceFile)
	if err != nil {
		return nil, err
	}
	return domain.ParseMarketplace(data)
}
