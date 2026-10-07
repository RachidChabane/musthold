// Package verdict combines the signals about one test and its declared
// criterion into a verdict, following the table of ADR 0003 ("Deciders and
// verdicts") and the claim-power table of ADR 0005. It asks for the next signal
// it needs, so the caller emits only the requests that can change the outcome.
package verdict

import (
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/matrix"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/policy"
	"github.com/RachidChabane/spec-proof/format"
)

// Need names the signal a decision still waits for.
type Need string

// Signals a decision can wait for.
const (
	NeedNone      Need = ""
	NeedSystemOne Need = "system_one"
	NeedJudge     Need = "judge"
	NeedTiebreak  Need = "tiebreak"
)

// Reasons recorded as the escalation reason of a decision.
const (
	ReasonContaminated        = "mutation_sensitive_test"
	ReasonUncorroborated      = "uncorroborated"
	ReasonModalShareBelow     = "modal_share_below_min"
	ReasonMatrixDisagrees     = "judge_matrix_disagreement"
	ReasonMatrixRequired      = "matrix_required"
	ReasonBudgetExhausted     = "budget_exhausted"
	ReasonHostUnresponsive    = "host_unresponsive"
	ReasonMaskedInputLeaks    = "masked_input_leaks"
	ReasonUnreadableSource    = "unreadable_test_source"
	ReasonIsolationBelowFloor = "isolation_below_floor"
)

// Deciders named in a decision.
const (
	DeciderJudge       = "judge"
	DeciderSystemOne   = "system_one"
	DeciderShadowAudit = "shadow_audit"
	DeciderMatrix      = "matrix"
	DeciderModule      = "module"
)

// SystemOneInput is the two-signal check: whether System One named the declared
// criterion with a supporting coverage, and how confident its answers were.
type SystemOneInput struct {
	// Settled is true once the closed questions were answered, or given up on.
	Settled    bool
	Supports   bool
	Confidence float64
	// Source says where Confidence came from: the endpoint's model, or the host
	// votes that stood in for it.
	Source format.ConfidenceSource
}

// JudgeInput is the judge's aggregated votes.
type JudgeInput struct {
	Settled bool
	Result  aggregate.Result
	Stance  aggregate.Stance
	// IndeterminateReason names why the stance is indeterminate, when it is not
	// because the modal share is too low.
	IndeterminateReason string
}

// TiebreakInput is the single extra judge vote asked when the judge supports
// alignment against a negative matrix.
type TiebreakInput struct {
	Settled  bool
	Supports bool
}

// Inputs are the signals available so far.
type Inputs struct {
	Matrix    matrix.Result
	SystemOne SystemOneInput
	Judge     JudgeInput
	Tiebreak  TiebreakInput
	// Audited is true when the shadow audit selected this verdict.
	Audited bool
}

// Decision is the verdict, or the next signal the verdict waits for.
type Decision struct {
	Done     bool
	Need     Need
	Verdict  format.Verdict
	Basis    format.DecisionBasis
	Deciders []string
	// Reason is the escalation reason; empty for a clean ALIGNED or MISALIGNED.
	Reason string
	// Corroborated is true when the matrix, SPECIFIC or OVERBROAD and neither
	// contaminated nor pinning off-spec behaviour, backs an ALIGNED verdict.
	Corroborated bool
	// Confidence is the raw confidence of the deciding signal.
	Confidence float64
}

func waiting(need Need) Decision { return Decision{Need: need} }

func finished(value format.Verdict, basis format.DecisionBasis, confidence float64, reason string, deciders ...string) Decision {
	return Decision{Done: true, Verdict: value, Basis: basis, Deciders: deciders, Reason: reason, Confidence: confidence}
}

// Decide applies the table. A contaminated test is UNDETERMINED whatever else
// was said. When the matrix is eligible and the policy allows it, System One is
// asked first, and a System One answer that supports alignment with a raw
// confidence at or above the route threshold issues ALIGNED alone, unless the
// shadow audit sends the verdict to the judge, whose outcome then replaces it.
// Every other case goes to the judge.
func Decide(in Inputs, p policy.Policy) Decision {
	if in.Matrix.Contaminated {
		return finished(format.VerdictUndetermined, format.BasisExecution, format.MaxConfidence, ReasonContaminated, DeciderMatrix)
	}
	if p.TwoSignal && in.Matrix.Eligible() {
		if !in.SystemOne.Settled {
			return waiting(NeedSystemOne)
		}
		if in.SystemOne.Supports && in.SystemOne.Confidence >= p.TauRoute.For(in.SystemOne.Source) {
			if !in.Audited {
				decision := finished(format.VerdictAligned, format.BasisSystemOne, in.SystemOne.Confidence, "", DeciderSystemOne, DeciderMatrix)
				decision.Corroborated = true
				return decision
			}
			return judged(in, p, true)
		}
	}
	return judged(in, p, false)
}

func judged(in Inputs, p policy.Policy, audited bool) Decision {
	if !in.Judge.Settled {
		return waiting(NeedJudge)
	}
	deciders := []string{DeciderJudge}
	if audited {
		deciders = append(deciders, DeciderShadowAudit)
	}
	share := in.Judge.Result.ModalShare
	switch in.Judge.Stance {
	case aggregate.Rejects:
		return finished(format.VerdictMisaligned, format.BasisJudges, share, "", deciders...)
	case aggregate.Supports:
		return supported(in, p, deciders)
	}
	reason := ReasonModalShareBelow
	if in.Judge.IndeterminateReason != "" {
		reason = in.Judge.IndeterminateReason
	}
	return finished(format.VerdictUndetermined, format.BasisJudges, share, reason, deciders...)
}

func supported(in Inputs, p policy.Policy, deciders []string) Decision {
	share := in.Judge.Result.ModalShare
	if in.Matrix.Negative() {
		if !in.Tiebreak.Settled {
			return waiting(NeedTiebreak)
		}
		if in.Tiebreak.Supports {
			return finished(format.VerdictUndetermined, format.BasisJudges, share, ReasonMatrixDisagrees, append(deciders, DeciderMatrix)...)
		}
		return finished(format.VerdictMisaligned, format.BasisJudges, share, "", append(deciders, DeciderMatrix)...)
	}
	if !in.Matrix.Eligible() && !p.JudgeAlone {
		return finished(format.VerdictUndetermined, format.BasisJudges, share, ReasonMatrixRequired, deciders...)
	}
	decision := finished(format.VerdictAligned, format.BasisJudges, share, "", deciders...)
	decision.Corroborated = in.Matrix.Eligible()
	if decision.Corroborated {
		decision.Deciders = append(decision.Deciders, DeciderMatrix)
	} else {
		decision.Reason = ReasonUncorroborated
	}
	return decision
}
