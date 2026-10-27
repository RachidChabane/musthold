package product

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
)

// Where the plugin trees come from, relative to the staged source tree or to the
// tree of the pinned Shallnot commit.
const (
	umbrellaPluginSource = "umbrella/plugins/musthold"
	shallnotPluginSource = "plugin"
	dirMode              = 0o755
)

// ErrNoBinaries refuses a product directory whose bin/ holds no file.
var ErrNoBinaries = errors.New("the product has no binary")

// Sources are the trees the plugin trees are copied from.
type Sources struct {
	// Repository is the staged tree of the repository at the built commit.
	Repository string
	// Shallnot is the tree of the pinned Shallnot commit.
	Shallnot string
}

// pluginSource is the directory a plugin tree, named by its path under
// plugins/, is copied from.
func (s Sources) pluginSource(path string) string {
	switch path {
	case plan.ProductShallnotPlugin:
		return filepath.Join(s.Shallnot, shallnotPluginSource)
	case plan.ProductUmbrella:
		return filepath.Join(s.Repository, umbrellaPluginSource)
	default:
		return filepath.Join(s.Repository, filepath.FromSlash(path))
	}
}

// StagePlugins replaces the plugin trees of a product directory with fresh copies
// of the trees every product holds.
func StagePlugins(productDir string, sources Sources) error {
	root := filepath.Join(productDir, PluginsDir)
	if err := os.RemoveAll(root); err != nil {
		return err
	}
	for _, path := range PluginPaths() {
		target := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(target), dirMode); err != nil {
			return err
		}
		if err := os.CopyFS(target, os.DirFS(sources.pluginSource(path))); err != nil {
			return fmt.Errorf("copy plugin tree %s: %w", path, err)
		}
	}
	return nil
}

// Digests are what a product directory holds, digested.
type Digests struct {
	Binaries map[string]string
	Plugins  map[string]string
}

// Describe digests every binary under bin/ and every plugin tree of a product
// directory.
func Describe(productDir string) (Digests, error) {
	binaries, err := digestBinaries(filepath.Join(productDir, BinDir))
	if err != nil {
		return Digests{}, err
	}
	plugins := map[string]string{}
	for _, path := range PluginPaths() {
		digest, err := TreeDigest(os.DirFS(filepath.Join(productDir, PluginsDir, filepath.FromSlash(path))))
		if err != nil {
			return Digests{}, fmt.Errorf("digest plugin tree %s: %w", path, err)
		}
		plugins[path] = digest
	}
	return Digests{Binaries: binaries, Plugins: plugins}, nil
}

func digestBinaries(binDir string) (map[string]string, error) {
	entries, err := os.ReadDir(binDir)
	if err != nil {
		return nil, err
	}
	digests := map[string]string{}
	fsys := os.DirFS(binDir)
	for _, entry := range entries {
		if !entry.Type().IsRegular() {
			return nil, fmt.Errorf("%w: %s", ErrNotRegular, entry.Name())
		}
		digest, err := FileDigest(fsys, entry.Name())
		if err != nil {
			return nil, err
		}
		digests[entry.Name()] = digest
	}
	if len(digests) == 0 {
		return nil, ErrNoBinaries
	}
	return digests, nil
}
