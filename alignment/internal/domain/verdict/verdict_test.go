package verdict_test

import (
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/alignment/internal/domain/aggregate"
	"github.com/RachidChabane/musthold/alignment/internal/domain/matrix"
	"github.com/RachidChabane/musthold/alignment/internal/domain/policy"
	"github.com/RachidChabane/musthold/alignment/internal/domain/verdict"
	"github.com/RachidChabane/musthold/format"
)

func result(signature matrix.Signature) matrix.Result {
	return matrix.Result{Signature: signature, CriteriaKilled: []string{}}
}

func contaminated() matrix.Result {
	r := result(matrix.Specific)
	r.Sentinel, r.Contaminated = true, true
	return r
}

func pinsOffspec() matrix.Result {
	r := result(matrix.Specific)
	r.PinsOffspec = true
	return r
}

func settledJudge(stance aggregate.Stance, share float64) verdict.JudgeInput {
	return verdict.JudgeInput{Settled: true, Stance: stance, Result: aggregate.Result{ModalShare: share, Votes: 5}}
}

var (
	supportingSystemOne = verdict.SystemOneInput{Settled: true, Supports: true, Confidence: 0.95}
	rejectingSystemOne  = verdict.SystemOneInput{Settled: true, Supports: false, Confidence: 0.95}
)

func TestDecide(t *testing.T) {
	p := policy.Default()

	t.Run("a test that kills a sentinel is UNDETERMINED before anything is asked [verifies SP-310~1]", func(t *testing.T) {
		got := verdict.Decide(verdict.Inputs{Matrix: contaminated(), SystemOne: supportingSystemOne, Judge: settledJudge(aggregate.Supports, 1)}, p)
		if !got.Done || got.Verdict != format.VerdictUndetermined || got.Reason != verdict.ReasonContaminated || got.Basis != format.BasisExecution {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("an eligible matrix asks System One first [verifies SP-311~1]", func(t *testing.T) {
		if got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific)}, p); got.Done || got.Need != verdict.NeedSystemOne {
			t.Fatalf("decision %+v", got)
		}
		if got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Overbroad)}, p); got.Need != verdict.NeedSystemOne {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("System One and an eligible matrix issue ALIGNED, corroborated, without the judge [verifies SP-311~1]", func(t *testing.T) {
		got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: supportingSystemOne}, p)
		if !got.Done || got.Verdict != format.VerdictAligned || got.Basis != format.BasisSystemOne || !got.Corroborated || got.Reason != "" {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("a System One confidence below the route threshold goes to the judge [verifies SP-311~1]", func(t *testing.T) {
		weak := verdict.SystemOneInput{Settled: true, Supports: true, Confidence: 0.89}
		if got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: weak}, p); got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
		if got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: rejectingSystemOne}, p); got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("a matrix that is not eligible never asks System One [verifies SP-311~1]", func(t *testing.T) {
		for _, m := range []matrix.Result{result(matrix.Absent), result(matrix.UndefinedOwn), result(matrix.Partial), result(matrix.Inert), result(matrix.OffTarget), pinsOffspec()} {
			if got := verdict.Decide(verdict.Inputs{Matrix: m}, p); got.Need != verdict.NeedJudge {
				t.Errorf("%s: decision %+v", m.Signature, got)
			}
		}
	})

	t.Run("with two-signal ALIGNED disabled the judge decides [verifies SP-311~1]", func(t *testing.T) {
		off := p
		off.TwoSignal = false
		if got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: supportingSystemOne}, off); got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("an audited two-signal verdict is replaced by the judge's outcome [verifies SP-316~1]", func(t *testing.T) {
		audited := verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: supportingSystemOne, Audited: true}
		if got := verdict.Decide(audited, p); got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
		audited.Judge = settledJudge(aggregate.Rejects, 1)
		got := verdict.Decide(audited, p)
		if !got.Done || got.Verdict != format.VerdictMisaligned || !slices.Contains(got.Deciders, verdict.DeciderShadowAudit) {
			t.Fatalf("decision %+v", got)
		}
		audited.Judge = settledJudge(aggregate.Supports, 1)
		if got := verdict.Decide(audited, p); got.Verdict != format.VerdictAligned || got.Basis != format.BasisJudges || !got.Corroborated {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("the audit does not mark a verdict System One would not have issued [verifies SP-316~1]", func(t *testing.T) {
		in := verdict.Inputs{Matrix: result(matrix.Specific), SystemOne: rejectingSystemOne, Audited: true, Judge: settledJudge(aggregate.Supports, 1)}
		if got := verdict.Decide(in, p); slices.Contains(got.Deciders, verdict.DeciderShadowAudit) {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("the judge's stance gives MISALIGNED, UNDETERMINED or ALIGNED [verifies SP-312~1]", func(t *testing.T) {
		in := verdict.Inputs{Matrix: result(matrix.Absent)}
		if got := verdict.Decide(in, p); got.Need != verdict.NeedJudge {
			t.Fatalf("decision %+v", got)
		}
		in.Judge = settledJudge(aggregate.Rejects, 1)
		if got := verdict.Decide(in, p); got.Verdict != format.VerdictMisaligned || got.Reason != "" {
			t.Fatalf("decision %+v", got)
		}
		in.Judge = settledJudge(aggregate.Indeterminate, 0.6)
		if got := verdict.Decide(in, p); got.Verdict != format.VerdictUndetermined || got.Reason != verdict.ReasonModalShareBelow {
			t.Fatalf("decision %+v", got)
		}
		in.Judge = settledJudge(aggregate.Supports, 1)
		if got := verdict.Decide(in, p); got.Verdict != format.VerdictAligned {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("an indeterminate judge says why when it is not for want of agreement [verifies SP-312~1]", func(t *testing.T) {
		in := verdict.Inputs{Matrix: result(matrix.Absent), Judge: settledJudge(aggregate.Indeterminate, 1)}
		in.Judge.IndeterminateReason = verdict.ReasonIsolationBelowFloor
		if got := verdict.Decide(in, p); got.Verdict != format.VerdictUndetermined || got.Reason != verdict.ReasonIsolationBelowFloor {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("a judge that supports alignment against a negative matrix asks one more ticket [verifies SP-313~1]", func(t *testing.T) {
		for _, m := range []matrix.Result{result(matrix.OffTarget), result(matrix.Inert), result(matrix.Partial), pinsOffspec()} {
			in := verdict.Inputs{Matrix: m, Judge: settledJudge(aggregate.Supports, 1)}
			if got := verdict.Decide(in, p); got.Need != verdict.NeedTiebreak {
				t.Fatalf("%s: decision %+v", m.Signature, got)
			}
			in.Tiebreak = verdict.TiebreakInput{Settled: true, Supports: true}
			if got := verdict.Decide(in, p); got.Verdict != format.VerdictUndetermined || got.Reason != verdict.ReasonMatrixDisagrees {
				t.Fatalf("%s: persisting disagreement: %+v", m.Signature, got)
			}
			in.Tiebreak = verdict.TiebreakInput{Settled: true, Supports: false}
			if got := verdict.Decide(in, p); got.Verdict != format.VerdictMisaligned {
				t.Fatalf("%s: resolved disagreement: %+v", m.Signature, got)
			}
		}
	})

	t.Run("a judge verdict without a matrix is ALIGNED but uncorroborated [verifies SP-314~1]", func(t *testing.T) {
		for _, m := range []matrix.Result{matrix.Absence(), result(matrix.UndefinedOwn)} {
			got := verdict.Decide(verdict.Inputs{Matrix: m, Judge: settledJudge(aggregate.Supports, 1)}, p)
			if got.Verdict != format.VerdictAligned || got.Corroborated || got.Reason != verdict.ReasonUncorroborated {
				t.Fatalf("%s: decision %+v", m.Signature, got)
			}
		}
	})

	t.Run("a judge verdict with an eligible matrix is corroborated [verifies SP-314~1]", func(t *testing.T) {
		got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Overbroad), SystemOne: rejectingSystemOne, Judge: settledJudge(aggregate.Supports, 1)}, p)
		if got.Verdict != format.VerdictAligned || !got.Corroborated || got.Reason != "" {
			t.Fatalf("decision %+v", got)
		}
	})

	t.Run("when the judge may not decide alone a matrix is required [verifies SP-314~1]", func(t *testing.T) {
		strict := p
		strict.JudgeAlone = false
		got := verdict.Decide(verdict.Inputs{Matrix: result(matrix.Absent), Judge: settledJudge(aggregate.Supports, 1)}, strict)
		if got.Verdict != format.VerdictUndetermined || got.Reason != verdict.ReasonMatrixRequired {
			t.Fatalf("decision %+v", got)
		}
	})
}

var signatures = []matrix.Signature{matrix.Absent, matrix.UndefinedOwn, matrix.Inert, matrix.OffTarget, matrix.Partial, matrix.Specific, matrix.Overbroad}

func drawMatrix(t *rapid.T) matrix.Result {
	r := result(rapid.SampledFrom(signatures).Draw(t, "signature"))
	r.Contaminated = rapid.Bool().Draw(t, "contaminated")
	r.Sentinel = r.Contaminated
	r.PinsOffspec = rapid.Bool().Draw(t, "pins")
	return r
}

func drawInputs(t *rapid.T) verdict.Inputs {
	stances := []aggregate.Stance{aggregate.Supports, aggregate.Rejects, aggregate.Indeterminate}
	return verdict.Inputs{
		Matrix:    drawMatrix(t),
		SystemOne: verdict.SystemOneInput{Settled: rapid.Bool().Draw(t, "so settled"), Supports: rapid.Bool().Draw(t, "so supports"), Confidence: rapid.Float64Range(0, 1).Draw(t, "so confidence")},
		Judge:     settledJudgeOrNot(t, stances),
		Tiebreak:  verdict.TiebreakInput{Settled: rapid.Bool().Draw(t, "tb settled"), Supports: rapid.Bool().Draw(t, "tb supports")},
		Audited:   rapid.Bool().Draw(t, "audited"),
	}
}

func settledJudgeOrNot(t *rapid.T, stances []aggregate.Stance) verdict.JudgeInput {
	if !rapid.Bool().Draw(t, "judge settled") {
		return verdict.JudgeInput{}
	}
	return settledJudge(rapid.SampledFrom(stances).Draw(t, "stance"), rapid.Float64Range(0, 1).Draw(t, "share"))
}

func TestDecisionProperties(t *testing.T) {
	p := policy.Default()

	t.Run("an ALIGNED verdict is corroborated exactly when the matrix is eligible, and otherwise says so [verifies SP-314~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			in := drawInputs(t)
			got := verdict.Decide(in, p)
			if !got.Done || got.Verdict != format.VerdictAligned {
				return
			}
			if got.Corroborated != in.Matrix.Eligible() {
				t.Fatalf("corroborated %t for matrix %+v", got.Corroborated, in.Matrix)
			}
			if !got.Corroborated && got.Reason != verdict.ReasonUncorroborated {
				t.Fatalf("an uncorroborated ALIGNED verdict says %q", got.Reason)
			}
		})
	})

	t.Run("System One never issues ALIGNED without an eligible matrix [verifies SP-311~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			in := drawInputs(t)
			got := verdict.Decide(in, p)
			if got.Done && got.Verdict == format.VerdictAligned && got.Basis == format.BasisSystemOne && !in.Matrix.Eligible() {
				t.Fatalf("a System One ALIGNED verdict on matrix %+v", in.Matrix)
			}
		})
	})

	t.Run("a contaminated test and a negative matrix never give ALIGNED [verifies SP-310~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			in := drawInputs(t)
			got := verdict.Decide(in, p)
			if got.Done && got.Verdict == format.VerdictAligned && (in.Matrix.Contaminated || in.Matrix.Negative()) {
				t.Fatalf("ALIGNED on matrix %+v", in.Matrix)
			}
		})
	})

	t.Run("worsening the evidence never turns a verdict that is not ALIGNED into ALIGNED [verifies SP-315~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			in := drawInputs(t)
			before := verdict.Decide(in, p)
			worse := in
			switch rapid.IntRange(0, 3).Draw(t, "worsening") {
			case 0:
				worse.Matrix.Contaminated = true
			case 1:
				worse.Matrix.PinsOffspec = true
			case 2:
				worse.Judge.Stance = aggregate.Rejects
			case 3:
				worse.SystemOne.Supports = false
			}
			after := verdict.Decide(worse, p)
			wasAligned := before.Done && before.Verdict == format.VerdictAligned
			isAligned := after.Done && after.Verdict == format.VerdictAligned
			if !wasAligned && isAligned {
				t.Fatalf("%+v became ALIGNED after worsening: %+v -> %+v", in, before, after)
			}
		})
	})

	t.Run("a decision waits for exactly one signal or is final [verifies SP-312~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			got := verdict.Decide(drawInputs(t), p)
			if got.Done == (got.Need != verdict.NeedNone) {
				t.Fatalf("decision %+v", got)
			}
		})
	})
}
