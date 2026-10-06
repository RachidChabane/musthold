// Package config reads and writes `.spec-proof/config.yaml`.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

// Top-level keys of the configuration file.
const (
	keyProofs     = "proofs"
	keyEcosystems = "ecosystems"
)

// indentWidth is the number of spaces per YAML indentation level.
const indentWidth = 2

// document is the YAML shape of the configuration file.
type document struct {
	Proofs     map[string]bool `yaml:"proofs"`
	Ecosystems []string        `yaml:"ecosystems"`
}

// Decode parses the YAML configuration and validates it. An unknown top-level
// key, proof or ecosystem is an error. An empty document yields empty settings.
func Decode(data []byte) (projectconfig.Settings, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var doc document
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		return projectconfig.Settings{}, fmt.Errorf("decode configuration: %w", err)
	}

	settings := projectconfig.Settings{Proofs: make(map[format.Kind]bool, len(doc.Proofs))}
	for name, enabled := range doc.Proofs {
		settings.Proofs[format.Kind(name)] = enabled
	}
	for _, name := range doc.Ecosystems {
		settings.Ecosystems = append(settings.Ecosystems, ecosystem.Name(name))
	}
	if err := settings.Validate(); err != nil {
		return projectconfig.Settings{}, err
	}
	return settings, nil
}

// Encode renders the settings as YAML with a fixed key order: the proofs in the
// order of format.Kinds, then the ecosystems in the order given.
func Encode(settings projectconfig.Settings) ([]byte, error) {
	if err := settings.Validate(); err != nil {
		return nil, err
	}

	proofs := &yaml.Node{Kind: yaml.MappingNode}
	for _, kind := range format.Kinds() {
		if enabled, present := settings.Proofs[kind]; present {
			proofs.Content = append(proofs.Content, scalar(string(kind)), scalar(strconv.FormatBool(enabled)))
		}
	}
	ecosystems := &yaml.Node{Kind: yaml.SequenceNode}
	for _, name := range settings.Ecosystems {
		ecosystems.Content = append(ecosystems.Content, scalar(string(name)))
	}
	root := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
		scalar(keyProofs), proofs,
		scalar(keyEcosystems), ecosystems,
	}}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(indentWidth)
	if err := encoder.Encode(root); err != nil {
		return nil, fmt.Errorf("encode configuration: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("encode configuration: %w", err)
	}
	return out.Bytes(), nil
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}
