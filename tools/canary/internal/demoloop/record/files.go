package record

import (
	"encoding/json"
	"regexp"
	"slices"
)

// Directories of a recording.
const (
	// CounterexamplesDir holds, for each mutant a gate block named, the unified
	// diff of the mutant against the project's source.
	CounterexamplesDir = "counterexamples"
	// DiffExtension ends a counterexample file.
	DiffExtension = ".diff"
	// MutantExtension ends a vault file.
	MutantExtension = ".json"
)

// mutantMention is how the gate names a mutant in a finding: its id cut to the
// length Shallnot prints.
var mutantMention = regexp.MustCompile(`\bmutant ([0-9a-f]{12})\b`)

// MentionedMutants lists the mutant ids a text names, in first-seen order.
func MentionedMutants(text string) []string {
	var found []string
	for _, match := range mutantMention.FindAllStringSubmatch(text, -1) {
		if !slices.Contains(found, match[1]) {
			found = append(found, match[1])
		}
	}
	return found
}

// GateReport is the part of Shallnot's JSON report a recording reads.
type GateReport struct {
	Verdict  string `json:"verdict"`
	ExitCode int    `json:"exit_code"`
	Summary  struct {
		Requirements json.RawMessage `json:"requirements"`
	} `json:"summary"`
	Findings []GateFinding `json:"findings"`
}

// GateFinding is one finding of the report.
type GateFinding struct {
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	Blocking      bool   `json:"blocking"`
	Message       string `json:"message"`
	RequirementID string `json:"requirement_id"`
	Module        string `json:"module"`
}

// DecodeGateReport reads a report; an empty file is no report.
func DecodeGateReport(data []byte) (*GateReport, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var report GateReport
	if err := json.Unmarshal(data, &report); err != nil {
		return nil, err
	}
	return &report, nil
}
