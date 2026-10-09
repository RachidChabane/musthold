package format

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ThresholdDefaultKey is the key of a threshold mapping that holds the threshold
// of every confidence source the mapping does not name.
const ThresholdDefaultKey = "default"

// ErrThreshold is returned for a threshold that is not a number or a mapping of
// numbers, that keys a source the format does not know, or that lies outside
// [0,1].
var ErrThreshold = errors.New("invalid threshold")

// Threshold is the confidence at which a decision has power, per confidence
// source (ADR 0005, "System One chain"). It is written as a bare number, which
// applies to every source, or as a mapping of `default` and of sources to
// numbers; a source is a kind (`host-votes`) or one exact source (`host-votes(2)`).
// This is the syntax and the resolution order of Shallnot's `proofs.thresholds`.
type Threshold struct {
	// Default applies to every confidence source that has no entry of its own.
	Default float64
	// BySource maps a source kind or an exact source to its threshold.
	BySource map[ConfidenceSource]float64
}

// UniformThreshold is the threshold that applies to every confidence source.
func UniformThreshold(value float64) Threshold { return Threshold{Default: value} }

// For returns the threshold of a source: the entry of the exact source, else the
// entry of its kind, else the default. A source the format does not know takes
// the default.
func (t Threshold) For(source ConfidenceSource) float64 {
	if value, keyed := t.BySource[source]; keyed {
		return value
	}
	if kind, known := source.Kind(); known {
		if value, keyed := t.BySource[kind]; keyed {
			return value
		}
	}
	return t.Default
}

// HasExact reports whether the threshold has an entry for exactly this source,
// which neither its kind nor the default can stand in for.
func (t Threshold) HasExact(source ConfidenceSource) bool {
	_, keyed := t.BySource[source]
	return keyed
}

// Validate rejects a value outside [0,1] and a key that is neither a source kind
// nor an exact source.
func (t Threshold) Validate() error {
	if !confidenceInRange(t.Default) {
		return fmt.Errorf("%w: %v is outside [%v,%v]", ErrThreshold, t.Default, MinConfidence, MaxConfidence)
	}
	return validateEntries(t.BySource)
}

func validateEntries(entries map[ConfidenceSource]float64) error {
	for _, source := range slices.Sorted(maps.Keys(entries)) {
		if !source.IsKnown() && !slices.Contains(SourceKinds(), source) {
			return fmt.Errorf("%w: %q is not one of the source kinds %v, nor an exact source such as host-votes(2) or system-one:<model>@<digest>", ErrThreshold, source, SourceKinds())
		}
		if value := entries[source]; !confidenceInRange(value) {
			return fmt.Errorf("%w: %v for %q is outside [%v,%v]", ErrThreshold, value, source, MinConfidence, MaxConfidence)
		}
	}
	return nil
}

// MarshalJSON writes a threshold without entries as a bare number and any other
// as a mapping, so that a policy without per-source thresholds keeps its digest.
func (t Threshold) MarshalJSON() ([]byte, error) {
	if len(t.BySource) == 0 {
		return json.Marshal(t.Default)
	}
	entries := make(map[string]float64, len(t.BySource)+1)
	for source, value := range t.BySource {
		entries[string(source)] = value
	}
	entries[ThresholdDefaultKey] = t.Default
	return json.Marshal(entries)
}

// UnmarshalJSON reads a number or a mapping of numbers.
func (t *Threshold) UnmarshalJSON(data []byte) error {
	var value any
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&value); err != nil {
		return fmt.Errorf("%w: %v", ErrThreshold, err)
	}
	override, err := thresholdOverrideOf(value)
	if err != nil {
		return err
	}
	*t = override.ApplyTo(Threshold{})
	return nil
}

// ThresholdOverride is a threshold as a configuration file writes it: a bare
// number, which sets the default, or a mapping, which sets the default when it
// has one and the sources it names. A member the file leaves out keeps the value
// of the threshold the override is applied to, as in Shallnot's configuration.
type ThresholdOverride struct {
	// Default is the new default, nil when the file does not set one.
	Default *float64
	// BySource holds the entries the file sets.
	BySource map[ConfidenceSource]float64
}

// ApplyTo returns the base threshold with the override laid over it.
func (o ThresholdOverride) ApplyTo(base Threshold) Threshold {
	result := Threshold{Default: base.Default, BySource: maps.Clone(base.BySource)}
	if o.Default != nil {
		result.Default = *o.Default
	}
	for source, value := range o.BySource {
		if result.BySource == nil {
			result.BySource = map[ConfidenceSource]float64{}
		}
		result.BySource[source] = value
	}
	return result
}

// MarshalYAML writes the override as a bare number when it sets nothing else and
// as a mapping otherwise.
func (o ThresholdOverride) MarshalYAML() (any, error) {
	if len(o.BySource) == 0 && o.Default != nil {
		return *o.Default, nil
	}
	entries := make(map[string]float64, len(o.BySource)+1)
	for source, value := range o.BySource {
		entries[string(source)] = value
	}
	if o.Default != nil {
		entries[ThresholdDefaultKey] = *o.Default
	}
	return entries, nil
}

// UnmarshalYAML reads a number or a mapping of numbers, and refuses a value
// outside [0,1] and a key that names no source.
func (o *ThresholdOverride) UnmarshalYAML(unmarshal func(any) error) error {
	var value any
	if err := unmarshal(&value); err != nil {
		return err
	}
	parsed, err := thresholdOverrideOf(value)
	if err != nil {
		return err
	}
	*o = parsed
	return nil
}

func thresholdOverrideOf(value any) (ThresholdOverride, error) {
	if number, ok := numberOf(value); ok {
		override := ThresholdOverride{Default: &number}
		return override, override.validate()
	}
	entries, ok := stringKeyed(value)
	if !ok {
		return ThresholdOverride{}, fmt.Errorf("%w: a threshold is a number or a mapping of numbers", ErrThreshold)
	}
	override := ThresholdOverride{}
	for key, raw := range entries {
		number, isNumber := numberOf(raw)
		if !isNumber {
			return ThresholdOverride{}, fmt.Errorf("%w: the entry %q is not a number", ErrThreshold, key)
		}
		if key == ThresholdDefaultKey {
			override.Default = &number
			continue
		}
		if override.BySource == nil {
			override.BySource = map[ConfidenceSource]float64{}
		}
		override.BySource[ConfidenceSource(key)] = number
	}
	return override, override.validate()
}

func (o ThresholdOverride) validate() error {
	if o.Default != nil && !confidenceInRange(*o.Default) {
		return fmt.Errorf("%w: %v is outside [%v,%v]", ErrThreshold, *o.Default, MinConfidence, MaxConfidence)
	}
	return validateEntries(o.BySource)
}

func stringKeyed(value any) (map[string]any, bool) {
	switch entries := value.(type) {
	case map[string]any:
		return entries, true
	case map[any]any:
		converted := make(map[string]any, len(entries))
		for key, item := range entries {
			text, ok := key.(string)
			if !ok {
				return nil, false
			}
			converted[text] = item
		}
		return converted, true
	}
	return nil, false
}

func numberOf(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	}
	return 0, false
}
