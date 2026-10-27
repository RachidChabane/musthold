// Package conclusion holds the texts that PREREG §11.2 and §11.3 fix for the
// loop-1 report: the conclusion templates, with their placeholders in angle
// brackets, and the limits paragraph. The report fills the templates; the
// report lint exempts a filled template from its word rule.
package conclusion

// The conclusion templates of PREREG §11.2, verbatim.
const (
	H1Supported     = "On <T> fixture tasks (<n_c> control and <n_t> treatment runs, one agent, one model, pytest), an accepted change was oracle-defective in <k_t>/<n_t> runs under the gate that requires discriminating power and alignment, against <k_c>/<n_c> under traceability alone (exact stratified test, one-sided p = <p>); the treatment gate rejected <r>/<m> of its oracle-clean changes. The tasks were built for this study; this does not establish the rate on other code."
	H1RejectionOnly = "An accepted change was oracle-defective in <k_t>/<n_t> treatment runs against <k_c>/<n_c> control runs (one-sided p = <p>), but the gate rejected <r>/<m> of its oracle-clean changes, above the pre-registered bound of 1/3."
	H1Contradicted  = "An accepted change was oracle-defective in <k_t>/<n_t> treatment runs against <k_c>/<n_c> under traceability alone (one-sided p = <p> for more defects)."
	H1Inconclusive  = "The study cannot decide H1 (<reason>): <k_t>/<n_t> treatment and <k_c>/<n_c> control runs escaped. This is not evidence of no difference."
	H2              = "After a finding block, the agent changed its tests or code and the gate then passed in <c>/<b> treatment runs (exact 95 % interval [<lo>, <hi>]); <decision>."
)

// Limits is the limits paragraph of PREREG §11.3, verbatim.
const Limits = "- the tasks are small fixtures built for this study, each with a test that leaves an\n  implemented criterion partly unchecked by construction, so block rates are not\n  those of natural code;\n- `boxoffice` was built for the demonstration of the loop and is in-sample;\n- the truth is model-written (`gpt-5.6-terra`) from the spec alone; no human labels\n  any item;\n- one coding agent and one model; the gate's executors run on the coder's model, in\n  its session;\n- the control arm holds the same tools and instructions, so it measures the gate's\n  requirement, not the presence of the modules;\n- the escape test treats a task's runs as exchangeable; the sign test over tasks\n  does not, and has less power;\n- intervals and secondary tests are unadjusted."
