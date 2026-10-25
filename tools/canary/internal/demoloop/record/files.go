package record

import "encoding/json"

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
