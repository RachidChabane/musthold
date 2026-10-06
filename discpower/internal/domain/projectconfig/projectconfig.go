// Package projectconfig defines the settings `spec-proof init` and `enable` keep
// in `.spec-proof/config.yaml`.
package projectconfig

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/format"
)

// Directory and file names of the project configuration.
const (
	// Directory is the project directory that holds spec-proof state.
	Directory = ".spec-proof"
	// FileName is the configuration file inside Directory.
	FileName = "config.yaml"
)

// Settings is the content of the configuration file.
type Settings struct {
	// Proofs maps each proof kind to whether it is enabled.
	Proofs map[format.Kind]bool
	// Ecosystems lists the ecosystems detected in the project.
	Ecosystems []ecosystem.Name
}

// Default returns the settings `spec-proof init` writes: every proof disabled and
// the given ecosystems recorded.
func Default(detected []ecosystem.Name) Settings {
	proofs := make(map[format.Kind]bool, len(format.Kinds()))
	for _, kind := range format.Kinds() {
		proofs[kind] = false
	}
	return Settings{Proofs: proofs, Ecosystems: detected}
}

// ErrInvalidSettings is returned for settings naming an unknown proof or ecosystem.
var ErrInvalidSettings = errors.New("invalid settings")

// Validate rejects unknown proof kinds and unknown ecosystems.
func (s Settings) Validate() error {
	for kind := range s.Proofs {
		if kind.PredicateType() == "" {
			return fmt.Errorf("%w: unknown proof %q", ErrInvalidSettings, kind)
		}
	}
	for _, name := range s.Ecosystems {
		if !name.Valid() {
			return fmt.Errorf("%w: unknown ecosystem %q", ErrInvalidSettings, name)
		}
	}
	return nil
}

// Enabled returns the enabled proof kinds in the format's stable order.
func (s Settings) Enabled() []format.Kind {
	var enabled []format.Kind
	for _, kind := range format.Kinds() {
		if s.Proofs[kind] {
			enabled = append(enabled, kind)
		}
	}
	return enabled
}
