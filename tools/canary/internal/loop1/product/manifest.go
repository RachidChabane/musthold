// Package product describes the Linux product the loop-1 study builds once and
// every container mounts read-only: the binaries under bin/, the Claude Code
// plugin trees under plugins/, and the images they were built and run in. The
// manifest at the root of the product directory records the SHA-256 of every
// binary, the digest of every plugin tree and the digests of the images, so that
// a later reader can tell exactly what was built.
package product

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"slices"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/plan"
	"github.com/RachidChabane/musthold/tools/canary/internal/domain/modules"
)

// Locations in the product directory.
const (
	ManifestFile = "manifest.json"
	BinDir       = plan.ProductBinDir
	PluginsDir   = plan.ProductPluginsDir
)

// Binaries the product holds besides the proof modules'.
const (
	UmbrellaBinary = "musthold"
	RunnerBinary   = "musthold-demo-loop"
)

// Host architectures the product is compiled for.
const (
	ArchARM64 = "arm64"
	ArchAMD64 = "amd64"
)

var (
	// ErrManifest refuses a manifest that is not valid JSON of the expected shape.
	ErrManifest = errors.New("invalid product manifest")
	// ErrIncomplete refuses a manifest that lacks a binary or a plugin tree the study needs.
	ErrIncomplete = errors.New("incomplete product manifest")
	// ErrDigest refuses a value that is not a digest of the expected form.
	ErrDigest = errors.New("malformed digest")
)

var (
	commitPattern      = regexp.MustCompile(`^[0-9a-f]{40}$`)
	sha256Pattern      = regexp.MustCompile(`^[0-9a-f]{64}$`)
	imageDigestPattern = regexp.MustCompile(`@sha256:[0-9a-f]{64}$`)
	imageIDPattern     = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// Images are the digests of the images the product was built with and runs in.
type Images struct {
	// Build is the Go image the binaries are compiled in, by digest.
	Build string `json:"build"`
	// Base is the Node.js image the session image starts from, by digest.
	Base string `json:"base"`
	// UV is the image the uv binary is copied from, by digest.
	UV string `json:"uv"`
	// Built is the id of the session image the engine built.
	Built string `json:"built"`
}

// Manifest is the record of one build of the product.
type Manifest struct {
	// Commit is the commit of the repository the product was built from.
	Commit string `json:"commit"`
	// Dirty is set when the working tree differed from that commit at build time.
	Dirty bool `json:"dirty"`
	// ShallnotPin is the commit of Shallnot the binary was built from.
	ShallnotPin string `json:"shallnot_pin"`
	// Arch is the architecture the binaries are compiled for.
	Arch string `json:"arch"`
	// Binaries maps each file of bin/ to its SHA-256.
	Binaries map[string]string `json:"binaries"`
	// Plugins maps each plugin tree, by its path under plugins/, to its tree digest.
	Plugins map[string]string `json:"plugins"`
	Images  Images            `json:"images"`
}

// RequiredBinaries are the files of bin/ every product holds, sorted.
func RequiredBinaries() []string {
	names := []string{plan.ShallnotBinary, plan.VerifyBinary, UmbrellaBinary, RunnerBinary}
	for _, module := range modules.All() {
		names = append(names, module.Binary)
	}
	slices.Sort(names)
	return names
}

// PluginPaths are the plugin trees every product holds, each by its path under
// plugins/: the ones a session of the four-proofs arm loads. Sorted.
func PluginPaths() []string {
	const root = "/product"
	var paths []string
	for _, plugin := range plan.Plugins(plan.Layout{Product: root}, config.ArmFourProofs) {
		relative, err := filepath.Rel(filepath.Join(root, PluginsDir), plugin.Dir)
		if err != nil {
			panic(err)
		}
		paths = append(paths, filepath.ToSlash(relative))
	}
	slices.Sort(paths)
	return paths
}

// Parse reads a manifest and refuses one with an unknown member, a malformed
// digest, or a missing binary or plugin tree.
func Parse(data []byte) (Manifest, error) {
	var manifest Manifest
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return Manifest{}, fmt.Errorf("%w: %w", ErrManifest, err)
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return Manifest{}, fmt.Errorf("%w: data after the manifest", ErrManifest)
	}
	if err := manifest.Validate(); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// Marshal renders a manifest as indented JSON ending in a newline, after
// validating it, so that no invalid manifest is written.
func (m Manifest) Marshal() ([]byte, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Validate checks the manifest's shape.
func (m Manifest) Validate() error {
	if !commitPattern.MatchString(m.Commit) {
		return fmt.Errorf("%w: commit %q", ErrDigest, m.Commit)
	}
	if !commitPattern.MatchString(m.ShallnotPin) {
		return fmt.Errorf("%w: shallnot_pin %q", ErrDigest, m.ShallnotPin)
	}
	if m.Arch != ArchARM64 && m.Arch != ArchAMD64 {
		return fmt.Errorf("%w: architecture %q", ErrManifest, m.Arch)
	}
	if err := checkSet("binary", m.Binaries, RequiredBinaries()); err != nil {
		return err
	}
	if err := checkSet("plugin tree", m.Plugins, PluginPaths()); err != nil {
		return err
	}
	return m.Images.validate()
}

func checkSet(kind string, digests map[string]string, required []string) error {
	for _, name := range required {
		if _, ok := digests[name]; !ok {
			return fmt.Errorf("%w: no %s %q", ErrIncomplete, kind, name)
		}
	}
	for name, digest := range digests {
		if !sha256Pattern.MatchString(digest) {
			return fmt.Errorf("%w: %s %q has %q", ErrDigest, kind, name, digest)
		}
	}
	return nil
}

func (i Images) validate() error {
	for name, reference := range map[string]string{"build": i.Build, "base": i.Base, "uv": i.UV} {
		if !imageDigestPattern.MatchString(reference) {
			return fmt.Errorf("%w: image %s %q is not named by digest", ErrDigest, name, reference)
		}
	}
	if !imageIDPattern.MatchString(i.Built) {
		return fmt.Errorf("%w: built image %q", ErrDigest, i.Built)
	}
	return nil
}
