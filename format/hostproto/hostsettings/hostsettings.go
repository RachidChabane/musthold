// Package hostsettings is the `host` section of `.spec-proof/config.yaml`, the
// settings of the host harness a module hands its model work to. Every module
// reads the same section the same way: the file is shared, so a module reads its
// own sections and tolerates the others'. It reads no file; the bytes are given.
package hostsettings

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"
)

// DefaultRequireAttestation is the default of `host.require_attestation`: an
// executor answer without a capability is refused.
const DefaultRequireAttestation = true

// Settings are the settings of the host harness.
type Settings struct {
	// Sampling is `host.sampling: on`: the executor server offers `drain` when
	// the client declares sampling.
	Sampling bool
	// RequireAttestation is `host.require_attestation`; nil keeps the default.
	RequireAttestation *bool
}

// AttestationRequired reports whether an answer needs a capability issued by the
// harness's hooks.
func (s Settings) AttestationRequired() bool {
	if s.RequireAttestation == nil {
		return DefaultRequireAttestation
	}
	return *s.RequireAttestation
}

// IsDefault reports whether the settings equal the defaults, so that an encoder
// can leave them out.
func (s Settings) IsDefault() bool {
	return !s.Sampling && s.RequireAttestation == nil
}

// Document is the YAML shape of the section. A member the section does not name
// is refused wherever the section is decoded strictly.
type Document struct {
	Sampling           bool  `yaml:"sampling"`
	RequireAttestation *bool `yaml:"require_attestation"`
}

// Settings returns the settings the document holds.
func (d Document) Settings() Settings {
	return Settings{Sampling: d.Sampling, RequireAttestation: d.RequireAttestation}
}

// ErrInvalidSection is returned for a host section that names an unknown member
// or gives a member the wrong type.
var ErrInvalidSection = errors.New("invalid host section")

// Parse reads the host section of a configuration file. The other top-level
// sections of the file belong to the modules and are not read; an absent section,
// or an empty file, gives the defaults. A member the section does not define, or
// one of the wrong type, is an error.
func Parse(config []byte) (Settings, error) {
	var file struct {
		Host yaml.Node `yaml:"host"`
	}
	if err := yaml.Unmarshal(config, &file); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	if file.Host.Kind == 0 {
		return Settings{}, nil
	}
	section, err := yaml.Marshal(&file.Host)
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(section))
	decoder.KnownFields(true)
	var document Document
	if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	return document.Settings(), nil
}
