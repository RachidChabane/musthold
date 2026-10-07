package matrix_test

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/alignment/internal/domain/matrix"
)

var (
	declared = criterion.Ref{ID: "PAR-1", Revision: 1}
	another  = criterion.Ref{ID: "PAR-2", Revision: 1}

	thresholds = matrix.Thresholds{Own: 0.5, Other: 0.2, None: 0.2}
)

const subject = "tests.test_parcel::test_line_total"

// killedBy builds a mutant killed by the named tests.
func killedBy(tests ...string) matrix.Mutant {
	killed := map[string]bool{}
	for _, test := range tests {
		killed[test] = true
	}
	return matrix.Mutant{KilledBy: killed}
}

func survivor() matrix.Mutant { return killedBy() }

func pool(killers ...bool) []matrix.Mutant {
	mutants := make([]matrix.Mutant, len(killers))
	for index, kills := range killers {
		if kills {
			mutants[index] = killedBy(subject)
		} else {
			mutants[index] = survivor()
		}
	}
	return mutants
}

func TestClassify(t *testing.T) {
	cases := []struct {
		name  string
		facts matrix.Facts
		want  matrix.Signature
	}{
		{"a criterion without a VALID mutant leaves its own share undefined",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: nil, another: pool(true)}}, matrix.UndefinedOwn},
		{"a test that kills nothing is inert",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(false, false)}}, matrix.Inert},
		{"a test that kills only another criterion's mutants is off target",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(false, false), another: pool(true)}}, matrix.OffTarget},
		{"a test that kills only an unattributed mutant is off target",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(false)}, Unattributed: pool(true)}, matrix.OffTarget},
		{"a test that kills under theta_own of its mutants is partial",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(true, false, false)}}, matrix.Partial},
		{"a test that kills its mutants and few of the others' is specific",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(true, true, false), another: pool(false, false)}}, matrix.Specific},
		{"a test that kills its mutants and many of the others' is overbroad",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(true, true), another: pool(true, false)}}, matrix.Overbroad},
		{"exactly theta_own and theta_other is specific",
			matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(true, false), another: pool(true, false, false, false, false)}}, matrix.Specific},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-304~1]", func(t *testing.T) {
			if got := matrix.Classify(c.facts, subject, declared, thresholds); got.Signature != c.want {
				t.Fatalf("signature %s, want %s: %+v", got.Signature, c.want, got)
			}
		})
	}

	t.Run("the shares, the criteria killed and DC are reported [verifies SP-304~1]", func(t *testing.T) {
		facts := matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: pool(true, true, false, false), another: pool(true, false)}}
		got := matrix.Classify(facts, subject, declared, thresholds)
		if got.Own == nil || *got.Own != 0.5 || got.Other != 0.5 || got.DC == nil || *got.DC != 0 {
			t.Fatalf("result %+v", got)
		}
		if len(got.CriteriaKilled) != 2 || got.CriteriaKilled[0] != "PAR-1~1" || got.CriteriaKilled[1] != "PAR-2~1" {
			t.Fatalf("criteria killed %v", got.CriteriaKilled)
		}
	})

	t.Run("another test's kills never count for the subject [verifies SP-304~1]", func(t *testing.T) {
		facts := matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: {killedBy("tests::other"), killedBy("tests::other")}}}
		if got := matrix.Classify(facts, subject, declared, thresholds); got.Signature != matrix.Inert {
			t.Fatalf("signature %s", got.Signature)
		}
	})
}

func TestContaminationAndOffspec(t *testing.T) {
	base := map[criterion.Ref][]matrix.Mutant{declared: pool(true, true)}

	t.Run("a test that kills a sentinel is contaminated and never eligible [verifies SP-305~1]", func(t *testing.T) {
		got := matrix.Classify(matrix.Facts{Pools: base, Sentinels: []matrix.Mutant{killedBy(subject)}}, subject, declared, thresholds)
		if !got.Contaminated || !got.Sentinel || got.Eligible() {
			t.Fatalf("result %+v", got)
		}
	})

	t.Run("a sentinel that survives leaves the test uncontaminated [verifies SP-305~1]", func(t *testing.T) {
		got := matrix.Classify(matrix.Facts{Pools: base, Sentinels: []matrix.Mutant{survivor()}}, subject, declared, thresholds)
		if got.Contaminated || !got.Eligible() {
			t.Fatalf("result %+v", got)
		}
	})

	t.Run("a test that kills more than theta_none of the unattributed mutants pins off-spec behaviour [verifies SP-305~1]", func(t *testing.T) {
		got := matrix.Classify(matrix.Facts{Pools: base, Unattributed: pool(true, false, false, false)}, subject, declared, thresholds)
		if !got.PinsOffspec || got.Eligible() || !got.Negative() {
			t.Fatalf("result %+v", got)
		}
		exactly := matrix.Classify(matrix.Facts{Pools: base, Unattributed: pool(true, false, false, false, false)}, subject, declared, thresholds)
		if exactly.PinsOffspec {
			t.Fatalf("exactly theta_none pins off-spec: %+v", exactly)
		}
	})

	t.Run("an absent matrix is silent: neither eligible nor negative [verifies SP-305~1]", func(t *testing.T) {
		absent := matrix.Absence()
		if absent.Eligible() || absent.Negative() || absent.Signature != matrix.Absent {
			t.Fatalf("absence %+v", absent)
		}
	})
}

func TestClassificationProperties(t *testing.T) {
	t.Run("a test that kills more of its criterion's mutants never classifies lower [verifies SP-304~1]", func(t *testing.T) {
		rank := map[matrix.Signature]int{matrix.Inert: 0, matrix.OffTarget: 0, matrix.Partial: 1, matrix.Specific: 2, matrix.Overbroad: 2}
		rapid.Check(t, func(t *rapid.T) {
			size := rapid.IntRange(1, 12).Draw(t, "size")
			killed := rapid.IntRange(0, size-1).Draw(t, "killed")
			build := func(kills int) matrix.Facts {
				mutants := make([]matrix.Mutant, size)
				for index := range mutants {
					mutants[index] = survivor()
					if index < kills {
						mutants[index] = killedBy(subject)
					}
				}
				return matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: mutants}}
			}
			fewer := matrix.Classify(build(killed), subject, declared, thresholds)
			more := matrix.Classify(build(killed+1), subject, declared, thresholds)
			if rank[more.Signature] < rank[fewer.Signature] {
				t.Fatalf("%d kills gave %s, %d kills gave %s", killed, fewer.Signature, killed+1, more.Signature)
			}
		})
	})

	t.Run("a sentinel kill makes any signature ineligible [verifies SP-305~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			size := rapid.IntRange(1, 10).Draw(t, "size")
			mutants := make([]matrix.Mutant, size)
			for index := range mutants {
				mutants[index] = killedBy(subject)
			}
			facts := matrix.Facts{Pools: map[criterion.Ref][]matrix.Mutant{declared: mutants}, Sentinels: []matrix.Mutant{killedBy(subject)}}
			if matrix.Classify(facts, subject, declared, thresholds).Eligible() {
				t.Fatal("a contaminated test was eligible")
			}
		})
	})
}
