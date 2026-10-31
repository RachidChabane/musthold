package analyze

import "github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"

// Kept is the secondary endpoints of PREREG §10.4 and the sign tests over tasks
// of §10.5, computed by this study's rules, for a later study that keeps them
// unchanged.
type Kept struct {
	S1 Permutation `json:"s1"`
	S2 S2          `json:"s2"`
	Secondary
	// SignH1 and SignS1 test the per-task difference of escape and defect shares,
	// control minus treatment; SignH2 the per-task majority of corrected runs.
	SignH1 Sign `json:"signH1"`
	SignS1 Sign `json:"signS1"`
	SignH2 Sign `json:"signH2"`
}

// KeptEndpoints computes S1 to S8 and the sign tests over tasks on counted runs.
func KeptEndpoints(runs []facts.Facts) Kept {
	kept := Kept{S1: defectTest(runs), S2: DecideS2(runs), Secondary: secondary(runs)}
	kept.SignH1, kept.SignS1, kept.SignH2 = signTests(runs)
	return kept
}
