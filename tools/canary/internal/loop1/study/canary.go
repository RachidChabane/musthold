package study

// CanaryOutputFile is what the canary writes into each of its numbered
// directories under the work directory's canary/.
const CanaryOutputFile = "output.json"

// CanaryCheck is whether one canary session held, and why not.
type CanaryCheck struct {
	Held     bool     `json:"held"`
	Problems []string `json:"problems,omitempty"`
}

// CanaryOutput is what the canary gives the inputs freeze (PREREG §13.3, §13.4).
type CanaryOutput struct {
	C1 CanaryCheck `json:"c1"`
	C2 CanaryCheck `json:"c2"`
	C3 CanaryCheck `json:"c3"`
	// EgressObserved is every host C1 saw the CLI contact; EgressAllowlist those
	// within the model's domains.
	EgressObserved  []string `json:"egress_observed"`
	EgressAllowlist []string `json:"egress_allowlist"`
	// CLIAuxiliaryModels are the models C3's usage names beside the session model.
	CLIAuxiliaryModels []string `json:"cli_auxiliary_models"`
	// QuotaPatterns are the patterns that recognise a quota stop, the runner's
	// fixed list.
	QuotaPatterns []string `json:"quota_patterns"`
	// Records are the records of every session that ran, each with its digest and
	// the start of its session.
	Records []CanaryRecord `json:"records"`
}

// Held reports whether C1 to C3 all held.
func (o CanaryOutput) Held() bool { return o.C1.Held && o.C2.Held && o.C3.Held }
