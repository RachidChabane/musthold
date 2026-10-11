// Package config reads and writes `.spec-proof/config.yaml`.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strconv"

	"gopkg.in/yaml.v3"

	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/projectconfig"
	"github.com/RachidChabane/spec-proof/format"
)

// Top-level keys of the configuration file.
const (
	keyProofs     = "proofs"
	keyEcosystems = "ecosystems"
	keyOperations = "operations"
	keyPolicy     = "policy"
	keyHost       = "host"

	keySampling           = "sampling"
	keyRequireAttestation = "require_attestation"
	keyClassify           = "classify"
	keyWitnessDir         = "witness_dir"
	samplingOn            = "on"

	keyAdmission = "admission"
	keyRefactors = "refactors"
)

// indentWidth is the number of spaces per YAML indentation level.
const indentWidth = 2

// document is the YAML shape of the configuration file.
type document struct {
	Proofs     map[string]bool              `yaml:"proofs"`
	Ecosystems []string                     `yaml:"ecosystems"`
	Operations map[string]map[string]string `yaml:"operations"`
	Policy     policyDocument               `yaml:"policy"`
	Host       hostDocument                 `yaml:"host"`
	Admission  admissionDocument            `yaml:"admission"`

	// Sections of the other modules share the file; this module reads none of them.
	Alignment    yaml.Node `yaml:"alignment"`
	Completeness yaml.Node `yaml:"completeness"`
	SystemOne    yaml.Node `yaml:"systemone"`
}

// hostDocument is the YAML shape of the host settings.
type hostDocument struct {
	Sampling           bool             `yaml:"sampling"`
	RequireAttestation *bool            `yaml:"require_attestation"`
	Classify           classifyDocument `yaml:"classify"`
}

// admissionDocument is the YAML shape of `admission`.
type admissionDocument struct {
	Refactors string `yaml:"refactors"`
}

// classifyDocument is the YAML shape of `host.classify`.
type classifyDocument struct {
	WitnessDir string `yaml:"witness_dir"`
}

// policyDocument is the YAML shape of the decision parameters; an absent member
// keeps the default.
type policyDocument struct {
	TauJudge        *format.ThresholdOverride `yaml:"tau_judge"`
	JudgeK          *int                      `yaml:"judge_k"`
	TauScreen       *format.ThresholdOverride `yaml:"tau_screen"`
	DecideK         *int                      `yaml:"decide_k"`
	MinValid        *int                      `yaml:"min_valid"`
	MinSites        *int                      `yaml:"min_sites"`
	MaxRounds       *int                      `yaml:"max_rounds"`
	KOp             *int                      `yaml:"k_op"`
	MaxHostTickets  *int                      `yaml:"max_host_tickets"`
	SignedPassBasis *string                   `yaml:"signed_pass_basis"`
	MinIsolation    *string                   `yaml:"min_isolation"`
}

func (d policyDocument) overrides() projectconfig.PolicyOverrides {
	overrides := projectconfig.PolicyOverrides{
		TauJudge: d.TauJudge, JudgeK: d.JudgeK, TauScreen: d.TauScreen, DecideK: d.DecideK,
		MinValid: d.MinValid, MinSites: d.MinSites, MaxRounds: d.MaxRounds, KOp: d.KOp, MaxHostTickets: d.MaxHostTickets,
	}
	if d.SignedPassBasis != nil {
		basis := policy.SignedPassBasis(*d.SignedPassBasis)
		overrides.SignedPassBasis = &basis
	}
	if d.MinIsolation != nil {
		level := format.Isolation(*d.MinIsolation)
		overrides.MinIsolation = &level
	}
	return overrides
}

// Decode parses the YAML configuration and validates it. An unknown top-level
// key, proof or ecosystem is an error. An empty document yields empty settings.
func Decode(data []byte) (projectconfig.Settings, error) {
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var doc document
	if err := decoder.Decode(&doc); err != nil && !errors.Is(err, io.EOF) {
		if errors.Is(err, format.ErrThreshold) {
			return projectconfig.Settings{}, fmt.Errorf("%w: %v", projectconfig.ErrInvalidSettings, err)
		}
		return projectconfig.Settings{}, fmt.Errorf("decode configuration: %w", err)
	}

	settings := projectconfig.Settings{Proofs: make(map[format.Kind]bool, len(doc.Proofs))}
	for name, enabled := range doc.Proofs {
		settings.Proofs[format.Kind(name)] = enabled
	}
	for _, name := range doc.Ecosystems {
		settings.Ecosystems = append(settings.Ecosystems, ecosystem.Name(name))
	}
	settings.Operations = doc.Operations
	settings.PolicyOverrides = doc.Policy.overrides()
	settings.Host = projectconfig.HostSettings{Sampling: doc.Host.Sampling, RequireAttestation: doc.Host.RequireAttestation, ClassifyWitnessDir: doc.Host.Classify.WitnessDir}
	settings.Admission = projectconfig.AdmissionSettings{RefactorsDir: doc.Admission.Refactors}
	if err := settings.Validate(); err != nil {
		return projectconfig.Settings{}, err
	}
	if err := settings.Policy().Validate(); err != nil {
		return projectconfig.Settings{}, fmt.Errorf("%w: %v", projectconfig.ErrInvalidSettings, err)
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
	if len(settings.Operations) > 0 {
		root.Content = append(root.Content, scalar(keyOperations), operationsNode(settings.Operations))
	}
	if node := policyNode(settings.PolicyOverrides); len(node.Content) > 0 {
		root.Content = append(root.Content, scalar(keyPolicy), node)
	}

	if !settings.Host.IsDefault() {
		root.Content = append(root.Content, scalar(keyHost), hostNode(settings.Host))
	}
	if settings.Admission.RefactorsDir != "" {
		admission := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar(keyRefactors), scalar(settings.Admission.RefactorsDir)}}
		root.Content = append(root.Content, scalar(keyAdmission), admission)
	}

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

// hostNode renders the host settings that differ from the defaults.
func hostNode(host projectconfig.HostSettings) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	if host.Sampling {
		node.Content = append(node.Content, scalar(keySampling), scalar(samplingOn))
	}
	if host.RequireAttestation != nil {
		node.Content = append(node.Content, scalar(keyRequireAttestation), scalar(strconv.FormatBool(*host.RequireAttestation)))
	}
	if host.ClassifyWitnessDir != "" {
		classify := &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{scalar(keyWitnessDir), scalar(host.ClassifyWitnessDir)}}
		node.Content = append(node.Content, scalar(keyClassify), classify)
	}
	return node
}

func scalar(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Value: value}
}

func operationsNode(operations map[string]map[string]string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	for _, operation := range sortedKeys(operations) {
		targets := &yaml.Node{Kind: yaml.MappingNode}
		for _, key := range sortedKeys(operations[operation]) {
			targets.Content = append(targets.Content, scalar(key), scalar(operations[operation][key]))
		}
		node.Content = append(node.Content, scalar(operation), targets)
	}
	return node
}

func sortedKeys[V any](values map[string]V) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// policyNode renders the overrides that are set, in a fixed order.
func policyNode(o projectconfig.PolicyOverrides) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}
	add := func(key string, value any, set bool) {
		if set {
			node.Content = append(node.Content, scalar(key), scalar(fmt.Sprint(value)))
		}
	}
	addThreshold := func(key string, override *format.ThresholdOverride) {
		if override != nil {
			node.Content = append(node.Content, scalar(key), thresholdNode(*override))
		}
	}
	addThreshold("tau_judge", o.TauJudge)
	add("judge_k", deref(o.JudgeK), o.JudgeK != nil)
	addThreshold("tau_screen", o.TauScreen)
	add("decide_k", deref(o.DecideK), o.DecideK != nil)
	add("min_valid", deref(o.MinValid), o.MinValid != nil)
	add("min_sites", deref(o.MinSites), o.MinSites != nil)
	add("max_rounds", deref(o.MaxRounds), o.MaxRounds != nil)
	add("k_op", deref(o.KOp), o.KOp != nil)
	add("max_host_tickets", deref(o.MaxHostTickets), o.MaxHostTickets != nil)
	add("signed_pass_basis", deref(o.SignedPassBasis), o.SignedPassBasis != nil)
	add("min_isolation", deref(o.MinIsolation), o.MinIsolation != nil)
	return node
}

// thresholdNode renders a threshold as a bare number or as a mapping with the
// default first and the sources in order.
func thresholdNode(override format.ThresholdOverride) *yaml.Node {
	if len(override.BySource) == 0 && override.Default != nil {
		return scalar(formatNumber(*override.Default))
	}
	node := &yaml.Node{Kind: yaml.MappingNode}
	if override.Default != nil {
		node.Content = append(node.Content, scalar(format.ThresholdDefaultKey), scalar(formatNumber(*override.Default)))
	}
	for _, source := range slices.Sorted(maps.Keys(override.BySource)) {
		node.Content = append(node.Content, scalar(string(source)), scalar(formatNumber(override.BySource[source])))
	}
	return node
}

func formatNumber(value float64) string { return strconv.FormatFloat(value, 'g', -1, 64) }

func deref[V any](value *V) V {
	var zero V
	if value == nil {
		return zero
	}
	return *value
}
