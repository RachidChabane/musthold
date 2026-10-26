// Package hostsettings is the `host` section of `.musthold/config.yaml`, the
// settings of the host harness a module hands its model work to. Every module
// reads the same section the same way: the file is shared, so a module reads its
// own sections and tolerates the others'. It reads no file; the bytes are given.
package hostsettings

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/hostproto"
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
	// Classify is `systemone.classify` when it is one value, or its `default`
	// member when it is a mapping; empty keeps the default.
	Classify hostproto.ClassifyMode
	// ClassifyByKind holds the members of a `systemone.classify` mapping that name
	// a proof kind.
	ClassifyByKind map[format.Kind]hostproto.ClassifyMode
	// ClassifyWitnessDir is `host.classify.witness_dir`: the directory of the
	// user's own raw API log, empty when the check is not opted into.
	ClassifyWitnessDir string
}

// ClassifyModeFor is the value of `systemone.classify` for the module of a proof
// kind: its own member of the mapping, else the default of the mapping or the
// value, else auto.
func (s Settings) ClassifyModeFor(kind format.Kind) hostproto.ClassifyMode {
	if mode, named := s.ClassifyByKind[kind]; named {
		return mode
	}
	if s.Classify == "" {
		return hostproto.DefaultClassifyMode
	}
	return s.Classify
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
	return !s.Sampling && s.RequireAttestation == nil && s.Classify == "" && len(s.ClassifyByKind) == 0 && s.ClassifyWitnessDir == ""
}

// Document is the YAML shape of the section. A member the section does not name
// is refused wherever the section is decoded strictly.
type Document struct {
	Sampling           bool             `yaml:"sampling"`
	RequireAttestation *bool            `yaml:"require_attestation"`
	Classify           classifyDocument `yaml:"classify"`
}

// classifyDocument is the `host.classify` mapping.
type classifyDocument struct {
	WitnessDir string `yaml:"witness_dir"`
}

// systemOneDocument is the part of the `systemone` section this package reads;
// the relay reads the rest.
type systemOneDocument struct {
	Classify yaml.Node `yaml:"classify"`
}

// classifyDefaultKey names the member of a `systemone.classify` mapping that
// holds the value of the modules it does not name.
const classifyDefaultKey = "default"

// Settings returns the settings the document holds.
func (d Document) Settings() Settings {
	return Settings{Sampling: d.Sampling, RequireAttestation: d.RequireAttestation, ClassifyWitnessDir: d.Classify.WitnessDir}
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
		Host      yaml.Node `yaml:"host"`
		SystemOne yaml.Node `yaml:"systemone"`
	}
	if err := yaml.Unmarshal(config, &file); err != nil {
		return Settings{}, fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	settings, err := hostSection(file.Host)
	if err != nil {
		return Settings{}, err
	}
	if err := readClassifySwitch(file.SystemOne, &settings); err != nil {
		return Settings{}, err
	}
	return settings, nil
}

func hostSection(node yaml.Node) (Settings, error) {
	if node.Kind == 0 {
		return Settings{}, nil
	}
	section, err := yaml.Marshal(&node)
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

// readClassifySwitch reads `systemone.classify`: one value for every module, or a
// mapping from proof kinds, and `default`, to values. The other members of the
// section are the relay's and are not read here.
func readClassifySwitch(node yaml.Node, settings *Settings) error {
	if node.Kind == 0 {
		return nil
	}
	var section systemOneDocument
	if err := node.Decode(&section); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	switch section.Classify.Kind {
	case 0:
		return nil
	case yaml.ScalarNode:
		mode, err := hostproto.ParseClassifyMode(section.Classify.Value)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSection, err)
		}
		settings.Classify = mode
		return nil
	case yaml.MappingNode:
		return readClassifyMapping(section.Classify, settings)
	}
	return fmt.Errorf("%w: systemone.classify is a value or a mapping", ErrInvalidSection)
}

func readClassifyMapping(node yaml.Node, settings *Settings) error {
	var entries map[string]string
	if err := node.Decode(&entries); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSection, err)
	}
	for key, text := range entries {
		mode, err := hostproto.ParseClassifyMode(text)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidSection, err)
		}
		switch kind := format.Kind(key); {
		case key == classifyDefaultKey:
			settings.Classify = mode
		case slices.Contains(format.Kinds(), kind):
			if settings.ClassifyByKind == nil {
				settings.ClassifyByKind = map[format.Kind]hostproto.ClassifyMode{}
			}
			settings.ClassifyByKind[kind] = mode
		default:
			return fmt.Errorf("%w: %q is neither %q nor one of the proof kinds %v", ErrInvalidSection, key, classifyDefaultKey, format.Kinds())
		}
	}
	return nil
}
