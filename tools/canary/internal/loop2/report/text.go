package report

// The conclusion templates of tools/canary/specs/loop-2-analysis.md; a
// placeholder <name> is filled from the summary.
const (
	Outcome           = "On <T> tasks (<n_c> control and <n_t> treatment runs, one coder, pytest), an accepted change was oracle-defective in <k_t>/<n_t> treatment runs against <k_c>/<n_c> control runs (exact stratified test, one-sided p_less = <p_less>, p_more = <p_more>; 0.90 score interval of the difference [<low>, <high>]); the outcome is <outcome>: <reading>."
	CapInconclusive   = "The study cannot decide the outcome (<reason>). This is not evidence of no difference."
	SpecWitnessGain   = "The gain comes mainly from the spec-witness check: <s>/<c> catches were attributed to spec_witness_failed."
	CorrectionAfterH2 = "After a finding block, the agent corrected its change and the gate then accepted it in <c>/<b> treatment runs (exact 95 % interval [<lo>, <hi>]); <decision>."
)

// Templates lists every conclusion template.
func Templates() []string {
	return []string{Outcome, CapInconclusive, SpecWitnessGain, CorrectionAfterH2}
}

// Limits is the limits ADR 0017 states in advance, verbatim.
const Limits = "The tasks are synthetic, and the oracle is model-written and checked by a second oracle and a citation pass. The arms differ by the whole product, not by the requirement alone. The result covers one ecosystem and one coder."

// Regime is the regime a negative result names (ADR 0017, "Consequences").
const Regime = "fully specified business-rule pytest tasks of 250–600 lines, on the chosen coder"
