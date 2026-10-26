// Package projectconfig defines the settings `musthold init` and `enable` keep
// in `.musthold/config.yaml`.
package projectconfig

import (
	"errors"
	"fmt"

	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/format"
)

// Directory and file names of the project configuration.
const (
	// Directory is the project directory that holds musthold state.
	Directory = ".musthold"
	// FileName is the configuration file inside Directory.
	FileName = "config.yaml"
)

// Settings is the content of the configuration file.
type Settings struct {
	// Proofs maps each proof kind to whether it is enabled.
	Proofs map[format.Kind]bool
	// Ecosystems lists the ecosystems detected in the project.
	Ecosystems []ecosystem.Name
	// Operations binds the operation names of spec examples to targets, per
	// ecosystem or language: `{quote: {python: "parkfee:quote"}}`.
	Operations map[string]map[string]string
	// PolicyOverrides are the decision parameters the project sets.
	PolicyOverrides PolicyOverrides
	// Host holds the settings of the host harness.
	Host HostSettings
	// Admission holds the inputs that take part in admitting a witness beyond
	// the reference. Neither `init` nor `enable` writes it.
	Admission AdmissionSettings
}

// AdmissionSettings names the planted refactors every witness must pass to be
// admitted: a directory of edit sets, one JSON file per refactor, each a change
// that preserves every criterion. An empty directory name admits every witness
// that passes the reference.
type AdmissionSettings struct {
	RefactorsDir string
}

// Default returns the settings `musthold init` writes: every proof disabled and
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
