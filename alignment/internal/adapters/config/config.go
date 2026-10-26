// Package config reads the module's own configuration, `.musthold/alignment.yaml`:
// the decision parameters a project sets. An absent member keeps its default.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/format"
)

// FileName is the configuration file inside the project's `.musthold` directory.
const FileName = "alignment.yaml"

// document is the YAML shape of the configuration file.
type document struct {
	Policy policyDocument `yaml:"policy"`
}

type policyDocument struct {
	TauRoute          *format.ThresholdOverride `yaml:"tau_route"`
	AlignK            *int                      `yaml:"align_k"`
	ModalMin          *format.ThresholdOverride `yaml:"modal_min"`
	ThetaOwn          *float64                  `yaml:"theta_own"`
	ThetaOther        *float64                  `yaml:"theta_other"`
	ThetaNone         *float64                  `yaml:"theta_none"`
	ShadowRate        *float64                  `yaml:"shadow_rate"`
	MaxReasoningCalls *int                      `yaml:"max_reasoning_calls"`
	DecideK           *int                      `yaml:"decide_k"`
	TwoSignal         *bool                     `yaml:"two_signal"`
	JudgeAlone        *bool                     `yaml:"judge_alone"`
	MinIsolation      *string                   `yaml:"min_isolation"`
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
	setThreshold(&result.TauRoute, doc.Policy.TauRoute)
	setThreshold(&result.ModalMin, doc.Policy.ModalMin)
	setFloat(&result.ThetaOwn, doc.Policy.ThetaOwn)
	setFloat(&result.ThetaOther, doc.Policy.ThetaOther)
	setFloat(&result.ThetaNone, doc.Policy.ThetaNone)
	setFloat(&result.ShadowRate, doc.Policy.ShadowRate)
	setInt(&result.AlignK, doc.Policy.AlignK)
	setInt(&result.MaxReasoningCalls, doc.Policy.MaxReasoningCalls)
	setInt(&result.DecideK, doc.Policy.DecideK)
	setBool(&result.TwoSignal, doc.Policy.TwoSignal)
	setBool(&result.JudgeAlone, doc.Policy.JudgeAlone)
	if doc.Policy.MinIsolation != nil {
		result.MinIsolation = format.Isolation(*doc.Policy.MinIsolation)
	}
	if err := result.Validate(); err != nil {
		return policy.Policy{}, err
	}
	return result, nil
}

func setFloat(target *float64, value *float64) {
	if value != nil {
		*target = *value
	}
}

func setThreshold(target *format.Threshold, override *format.ThresholdOverride) {
	if override != nil {
		*target = override.ApplyTo(*target)
	}
}

func setInt(target *int, value *int) {
	if value != nil {
		*target = *value
	}
}

func setBool(target *bool, value *bool) {
	if value != nil {
		*target = *value
	}
}
