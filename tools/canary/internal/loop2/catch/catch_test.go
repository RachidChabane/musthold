package catch

import (
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/analyze/facts"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/decision"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/record"
	rt "github.com/RachidChabane/musthold/tools/canary/internal/loop2/record/recordtest"
)

const (
	moduleDP = "musthold-dp"
	broken   = "FARE-7"
	other    = "FARE-2"
)

var (
	witness   = rt.Finding(string(SpecWitness), broken, moduleDP)
	example   = rt.Finding(string(SpecExample), broken, moduleDP)
	mutant    = rt.Finding(string(SurvivingMutant), broken, moduleDP)
	misalign  = rt.Finding("misaligned_test", broken, AlignmentModule)
	sensitive = rt.Finding("mutation_sensitive_test", broken, moduleDP)
)

func TestOf(t *testing.T) {
	t.Run("the catches of the treatment arm, by their two kinds [verifies SP-2813~1]", func(t *testing.T) {
		cases := []struct {
			name string
			run  record.Run
			kind Kind
			ok   bool
		}{
			{"defective at the first stop, clean and accepted at the end",
				rt.Run("fare", rt.Treatment, 1, rt.FirstStop(true, broken), rt.FindingBlock(true, mutant)), KindCorrected, true},
			{"the same run in the control arm is no catch",
				rt.Run("fare", rt.Control, 1, rt.FirstStop(true, broken)), "", false},
			{"clean at the first stop",
				rt.Run("fare", rt.Treatment, 1, rt.FirstStop(false)), "", false},
			{"no first-stop grade, clean and accepted",
				rt.Run("fare", rt.Treatment, 1), "", false},
			{"defective at the first stop, clean but refused",
				rt.Run("fare", rt.Treatment, 1, rt.FirstStop(true, broken), rt.Refused(record.CausePending)), "", false},
			{"defective, refused by a finding on the violated criterion",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CauseFinding, mutant)), KindRefused, true},
			{"defective, refused without a first-stop grade still catches",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CauseFinding, witness)), KindRefused, true},
			{"defective, refused by a finding on another criterion",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CauseFinding, rt.Finding("surviving_valid_mutant", other, moduleDP))), "", false},
			{"defective, refused for stale or pending proofs",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CausePending, rt.Finding("stale_proof", broken, moduleDP))), "", false},
			{"defective, refused at a cap with a finding on the violated criterion",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CauseCap, mutant)), "", false},
			{"defective and accepted is an escape",
				rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.FirstStop(true, broken)), "", false},
		}
		for _, c := range cases {
			got, ok := Of(c.run)
			if ok != c.ok || (ok && got.Kind != c.kind) {
				t.Errorf("%s: %+v, %v", c.name, got, ok)
			}
		}
	})
}

func TestAttribute(t *testing.T) {
	t.Run("the first class in the order of precedence, none without a finding [verifies SP-2814~1]", func(t *testing.T) {
		cases := []struct {
			findings []facts.Finding
			want     Attribution
		}{
			{[]facts.Finding{mutant, witness}, SpecWitness},
			{[]facts.Finding{misalign, example, mutant}, SpecExample},
			{[]facts.Finding{misalign, mutant}, SurvivingMutant},
			{[]facts.Finding{sensitive, misalign}, Alignment},
			{[]facts.Finding{sensitive}, Other},
			{nil, None},
		}
		for _, c := range cases {
			if got := Attribute(c.findings); got != c.want {
				t.Errorf("%+v: %s, want %s", c.findings, got, c.want)
			}
		}
	})
	t.Run("a corrected catch is owed to its last finding block, a refused one to the findings naming the violated criterion [verifies SP-2814~1]", func(t *testing.T) {
		corrected, _ := Of(rt.Run("fare", rt.Treatment, 1, rt.FirstStop(true, broken), rt.FindingBlock(false, witness), rt.FindingBlock(true, mutant)))
		if corrected.Attribution != SurvivingMutant {
			t.Errorf("corrected after a mutant: %s", corrected.Attribution)
		}
		unaided, _ := Of(rt.Run("fare", rt.Treatment, 1, rt.FirstStop(true, broken)))
		if unaided.Attribution != None {
			t.Errorf("corrected without a finding block: %s", unaided.Attribution)
		}
		refused, _ := Of(rt.Run("fare", rt.Treatment, 1, rt.Defective(broken), rt.Refused(record.CauseFinding, rt.Finding(string(SpecWitness), other, moduleDP), misalign)))
		if refused.Attribution != Alignment || len(refused.Findings) != 1 {
			t.Errorf("a witness on another criterion is not the refusal's: %+v", refused)
		}
	})
	t.Run("the attribution never depends on the order of the findings [verifies SP-2814~1]", rapid.MakeCheck(func(t *rapid.T) {
		pool := []facts.Finding{witness, example, mutant, misalign, sensitive}
		findings := rapid.SliceOfN(rapid.SampledFrom(pool), 0, 6).Draw(t, "findings")
		reversed := make([]facts.Finding, len(findings))
		for index, finding := range findings {
			reversed[len(findings)-1-index] = finding
		}
		if Attribute(findings) != Attribute(reversed) {
			t.Fatalf("%+v", findings)
		}
	}))
}

func catches(witnesses, others int) []Catch {
	var list []Catch
	for range witnesses {
		list = append(list, Catch{Attribution: SpecWitness})
	}
	for range others {
		list = append(list, Catch{Attribution: SurvivingMutant})
	}
	return list
}

func TestSummarise(t *testing.T) {
	t.Run("the gain comes mainly from the spec-witness check when it shows a gain and holds two thirds of at least one catch [verifies SP-2815~1]", func(t *testing.T) {
		cases := []struct {
			witnesses, others int
			outcome           decision.Outcome
			want              bool
		}{
			{2, 1, decision.Gain, true},
			{2, 1, decision.UsefulGain, true},
			{1, 1, decision.Gain, false},
			{6, 4, decision.Gain, false},
			{7, 3, decision.Gain, true},
			{2, 1, decision.GainBelowUseful, false},
			{2, 1, decision.RejectionOnly, false},
			{2, 1, decision.Inconclusive, false},
			{0, 0, decision.Gain, false},
		}
		for _, c := range cases {
			got := Summarise(catches(c.witnesses, c.others), c.outcome)
			if got.MainlySpecWitness != c.want || got.SpecWitness != (facts.Fraction{K: c.witnesses, N: c.witnesses + c.others}) {
				t.Errorf("%d of %d under %s: %+v", c.witnesses, c.witnesses+c.others, c.outcome, got)
			}
		}
		counts := Summarise(catches(1, 2), decision.Gain).ByAttribution
		if counts[SpecWitness] != 1 || counts[SurvivingMutant] != 2 || len(counts) != len(Attributions()) {
			t.Fatalf("counts %v", counts)
		}
	})
}

func TestMutationOnly(t *testing.T) {
	t.Run("a spec-witness catch becomes an accepted defective change and nothing else moves [verifies SP-2816~1]", func(t *testing.T) {
		runs := []record.Run{
			rt.Run("fare", rt.Treatment, 1, rt.FirstStop(true, broken), rt.FindingBlock(true, witness)),
			rt.Run("fare", rt.Treatment, 2, rt.Defective(broken), rt.Refused(record.CauseFinding, witness)),
			rt.Run("fare", rt.Treatment, 3, rt.FirstStop(true, broken), rt.FindingBlock(true, mutant)),
			rt.Run("fare", rt.Control, 1, rt.FirstStop(true, broken), rt.FindingBlock(true, witness)),
		}
		got := MutationOnly(runs)
		for index, want := range []bool{true, true, false, false} {
			if got[index].Escaped != want {
				t.Errorf("run %d: escaped %v, want %v", index, got[index].Escaped, want)
			}
		}
		if !got[1].Accepted || got[1].Refusal.Cause != record.CauseNone || runs[1].Accepted {
			t.Fatalf("the refused catch: %+v; the input changed: %v", got[1], runs[1].Accepted)
		}
	})
}
