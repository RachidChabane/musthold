// Package config reads the module's own configuration,
// `.musthold/completeness.yaml`: the decision parameters a project sets. An
// absent member keeps its default.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/completeness/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
)

// FileName is the configuration file inside the project's `.musthold` directory.
const FileName = "completeness.yaml"

// document is the YAML shape of the configuration file.
type document struct {
	Policy policyDocument `yaml:"policy"`
}

type policyDocument struct {
	TauAdv       *format.ThresholdOverride `yaml:"tau_adv"`
	DecideK      *int                      `yaml:"decide_k"`
	MaxHunkBytes *int                      `yaml:"max_hunk_bytes"`
	MaxHunks     *int                      `yaml:"max_hunks"`
}

// Decode parses the YAML configuration into the decision policy. An unknown key
// or an invalid parameter is an error; an empty document gives the default policy.
func Decode(data []byte) (policy.Policy, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var doc document
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, format.ErrThreshold) {
			return policy.Policy{}, fmt.Errorf("%w: %v", policy.ErrInvalid, err)
		}
		return policy.Policy{}, fmt.Errorf("decode configuration: %w", err)
	}
	result := policy.Default()
	if doc.Policy.TauAdv != nil {
		result.TauAdv = doc.Policy.TauAdv.ApplyTo(result.TauAdv)
	}
	if doc.Policy.DecideK != nil {
		result.DecideK = *doc.Policy.DecideK
	}
	if doc.Policy.MaxHunkBytes != nil {
		result.MaxHunkBytes = *doc.Policy.MaxHunkBytes
	}
	if doc.Policy.MaxHunks != nil {
		result.MaxHunks = *doc.Policy.MaxHunks
	}
	if err := result.Validate(); err != nil {
		return policy.Policy{}, err
	}
	return result, nil
}
