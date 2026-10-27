package study

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// InputsFile is the inputs freeze, written by resolve under experiments/loop-1/freeze
// (PREREG §13.4).
const InputsFile = "inputs.json"

// Commits are the commits of the two tags and the product commits between them.
type Commits struct {
	Prereg  string   `json:"prereg"`
	Inputs  string   `json:"inputs"`
	Between []string `json:"between"`
}

// Shallnot is the pinned Shallnot.
type Shallnot struct {
	Pin     string `json:"pin"`
	Version string `json:"version"`
	// Release is the first public release tag that contains the pin, or "none".
	Release string `json:"release"`
}

// ClaudeCode is the pinned CLI.
type ClaudeCode struct {
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
}

// Images are the digests of the images the study builds and runs.
type Images struct {
	Build string `json:"build"`
	Base  string `json:"base"`
	UV    string `json:"uv"`
	Built string `json:"built"`
	Proxy string `json:"proxy"`
}

// ArmInstructions are the digests of what one arm's session reads (PREREG §5).
type ArmInstructions struct {
	Prompt string `json:"prompt"`
	// Files maps every file the equipped commit adds or changes relative to the
	// base commit to its SHA-256.
	Files           map[string]string `json:"files"`
	SessionSettings string            `json:"session_settings"`
	// Plugins maps each plugin tree to its tree digest.
	Plugins map[string]string `json:"plugins"`
}

// TaskInputs is one task's entry of inputs.json.
type TaskInputs struct {
	Name         string `json:"name"`
	FixtureTree  string `json:"fixture_tree"`
	Criterion    string `json:"criterion"`
	PromptSHA256 string `json:"prompt_sha256"`
	BaseCommit   string `json:"base_commit"`
	// EquippedTree and Instructions are keyed by arm.
	EquippedTree map[string]string          `json:"equipped_tree"`
	Instructions map[string]ArmInstructions `json:"instructions"`
	// Truth maps oracle, reference, violations and refactors to their tree digests.
	Truth map[string]string `json:"truth"`
}

// Inputs is inputs.json (PREREG §13.4). Construction holds the construction
// records as construct wrote them.
type Inputs struct {
	Commits            Commits                      `json:"commits"`
	Binaries           map[string]string            `json:"binaries"`
	Shallnot           Shallnot                     `json:"shallnot"`
	ClaudeCode         ClaudeCode                   `json:"claude_code"`
	Codex              []string                     `json:"codex"`
	CLIAuxiliaryModels []string                     `json:"cli_auxiliary_models"`
	Images             Images                       `json:"images"`
	Toolchains         map[string]string            `json:"toolchains"`
	EgressObserved     []string                     `json:"egress_observed"`
	EgressAllowlist    []string                     `json:"egress_allowlist"`
	QuotaPatterns      []string                     `json:"quota_patterns"`
	Policy             map[string]map[string]string `json:"policy"`
	Tasks              []TaskInputs                 `json:"tasks"`
	Construction       json.RawMessage              `json:"construction"`
	Order              []string                     `json:"order"`
}

// ErrMalformedInputs is returned for an inputs freeze of another shape.
var ErrMalformedInputs = errors.New("the inputs freeze is not a JSON document of the expected shape")

// ParseInputs reads inputs.json, refusing unknown members and a second document.
func ParseInputs(data []byte) (Inputs, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var inputs Inputs
	if err := decoder.Decode(&inputs); err != nil {
		return Inputs{}, fmt.Errorf("%w: %v", ErrMalformedInputs, err)
	}
	if decoder.More() {
		return Inputs{}, fmt.Errorf("%w: more than one document", ErrMalformedInputs)
	}
	return inputs, nil
}

// Task returns the entry of the named task.
func (in Inputs) Task(name string) (TaskInputs, bool) {
	for _, task := range in.Tasks {
		if task.Name == name {
			return task, true
		}
	}
	return TaskInputs{}, false
}
