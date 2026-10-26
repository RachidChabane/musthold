package differential_test

import (
	"slices"
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

func callOf(value string) spec.Call {
	return exampleCall(map[string]spec.Literal{"x": integer(value)})
}

func returned(value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: value}
}

func raised(name string) spec.Observation {
	return spec.Observation{Kind: spec.Raises, Value: name}
}

func witnessInputs(outcome power.Differential) []string {
	inputs := make([]string, len(outcome.Witnesses))
	for index, witness := range outcome.Witnesses {
		inputs[index] = witness.Call.Args["x"].Value
	}
	return inputs
}

func TestCompare(t *testing.T) {
	calls := []spec.Call{callOf("1"), callOf("2"), callOf("3")}
	t.Run("agreeing observations give no difference"+verifiesSP171, func(t *testing.T) {
		observations := []spec.Observation{returned("1"), returned("2"), raised("ValueError")}
		got := differential.Compare(calls, observations, slices.Clone(observations))
		if got.Status != power.NoDiffFound || len(got.Witnesses) != 0 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a number spelled differently is no difference"+verifiesSP171, func(t *testing.T) {
		reference := []spec.Observation{returned("2")}
		mutant := []spec.Observation{{Kind: spec.Returns, Type: spec.TypeDecimal, Value: "2.0"}}
		if got := differential.Compare(calls[:1], reference, mutant); got.Status != power.NoDiffFound {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("no input gives no difference"+verifiesSP171, func(t *testing.T) {
		if got := differential.Compare(nil, nil, nil); got.Status != power.NoDiffFound {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a returned value that differs is a witness with both observations"+verifiesSP171, func(t *testing.T) {
		got := differential.Compare(calls, []spec.Observation{returned("1"), returned("5"), returned("3")}, []spec.Observation{returned("1"), returned("6"), returned("3")})
		if got.Status != power.DiffersAt || len(got.Witnesses) != 1 {
			t.Fatalf("got %+v", got)
		}
		witness := got.Witnesses[0]
		if witness.Call.Args["x"].Value != "2" || witness.Reference.Value != "5" || witness.Mutant.Value != "6" {
			t.Errorf("witness %+v", witness)
		}
	})
	t.Run("a mutant that raises where the reference returns is a witness"+verifiesSP171, func(t *testing.T) {
		got := differential.Compare(calls[:1], []spec.Observation{returned("1")}, []spec.Observation{raised("KeyError")})
		if got.Status != power.DiffersAt || len(got.Witnesses) != 1 || got.Witnesses[0].Mutant.Kind != spec.Raises {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("a reference that raises gives a difference without witness"+verifiesSP171, func(t *testing.T) {
		got := differential.Compare(calls[:1], []spec.Observation{raised("KeyError")}, []spec.Observation{returned("1")})
		if got.Status != power.DiffersAt || len(got.Witnesses) != 0 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("two exceptions of different names differ without witness"+verifiesSP171, func(t *testing.T) {
		got := differential.Compare(calls[:1], []spec.Observation{raised("KeyError")}, []spec.Observation{raised("ValueError")})
		if got.Status != power.DiffersAt || len(got.Witnesses) != 0 {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("observations beyond the shorter list are ignored"+verifiesSP171, func(t *testing.T) {
		got := differential.Compare(calls, []spec.Observation{returned("1"), returned("2"), returned("3")}, []spec.Observation{returned("1")})
		if got.Status != power.NoDiffFound {
			t.Errorf("got %+v", got)
		}
	})
	t.Run("the witnesses are sorted by simplicity"+verifiesSP171, func(t *testing.T) {
		inputs := []spec.Call{callOf("123456"), callOf("12"), callOf("1234"), callOf("1")}
		reference := []spec.Observation{returned("0"), returned("0"), returned("0"), returned("0")}
		mutant := []spec.Observation{returned("9"), returned("9"), returned("9"), returned("9")}
		got := differential.Compare(inputs, reference, mutant)
		if want := []string{"1", "12", "1234"}; !slices.Equal(witnessInputs(got), want) {
			t.Errorf("witnesses %v, want %v", witnessInputs(got), want)
		}
	})
	t.Run("simplicity adds up the size of every argument"+verifiesSP171, func(t *testing.T) {
		wide := exampleCall(map[string]spec.Literal{"x": integer("1"), "y": integer("22")})
		narrow := exampleCall(map[string]spec.Literal{"x": integer("33")})
		got := differential.Compare([]spec.Call{wide, narrow}, []spec.Observation{returned("0"), returned("0")}, []spec.Observation{returned("9"), returned("9")})
		if len(got.Witnesses) != 2 || len(got.Witnesses[0].Call.Args) != 1 {
			t.Errorf("witnesses %+v", got.Witnesses)
		}
	})
	t.Run("witnesses of equal simplicity are ordered by their text"+verifiesSP171, func(t *testing.T) {
		inputs := []spec.Call{callOf("3"), callOf("1"), callOf("2")}
		reference := []spec.Observation{returned("0"), returned("0"), returned("0")}
		mutant := []spec.Observation{returned("9"), returned("9"), returned("9")}
		if got := witnessInputs(differential.Compare(inputs, reference, mutant)); !slices.Equal(got, []string{"1", "2", "3"}) {
			t.Errorf("witnesses %v", got)
		}
	})
	t.Run("at most three witnesses are kept"+verifiesSP171, func(t *testing.T) {
		const inputCount = 7
		var inputs []spec.Call
		var reference, mutant []spec.Observation
		for index := range inputCount {
			inputs = append(inputs, callOf(strconv.Itoa(index)))
			reference = append(reference, returned("0"))
			mutant = append(mutant, returned("1"))
		}
		got := differential.Compare(inputs, reference, mutant)
		if len(got.Witnesses) != differential.MaxWitnesses || differential.MaxWitnesses != 3 {
			t.Errorf("%d witnesses", len(got.Witnesses))
		}
		if !slices.Equal(witnessInputs(got), []string{"0", "1", "2"}) {
			t.Errorf("witnesses %v", witnessInputs(got))
		}
	})
	t.Run("a skipped search names its reason"+verifiesSP171, func(t *testing.T) {
		const reason = "the operation cannot be called"
		got := differential.SkippedBecause(reason)
		if got.Status != power.Skipped || got.SkipReason != reason || len(got.Witnesses) != 0 {
			t.Errorf("got %+v", got)
		}
	})
}

func TestCompareProperties(t *testing.T) {
	observation := rapid.Custom(func(t *rapid.T) spec.Observation {
		if rapid.Bool().Draw(t, "raises") {
			return raised(rapid.SampledFrom([]string{"KeyError", "ValueError"}).Draw(t, "name"))
		}
		return returned(strconv.Itoa(rapid.IntRange(0, 3).Draw(t, "value")))
	})
	t.Run("the outcome is consistent with the observations"+verifiesSP171, func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			count := rapid.IntRange(0, 12).Draw(t, "count")
			inputs := make([]spec.Call, count)
			for index := range inputs {
				inputs[index] = callOf(strconv.Itoa(rapid.IntRange(0, 99999).Draw(t, "input")))
			}
			reference := rapid.SliceOfN(observation, count, count).Draw(t, "reference")
			mutant := rapid.SliceOfN(observation, count, count).Draw(t, "mutant")
			got := differential.Compare(inputs, reference, mutant)
			differs := false
			for index := range reference {
				differs = differs || !reference[index].Same(mutant[index])
			}
			if differs != (got.Status == power.DiffersAt) || (!differs && got.Status != power.NoDiffFound) {
				t.Fatalf("status %s for differs=%v", got.Status, differs)
			}
			if len(got.Witnesses) > differential.MaxWitnesses {
				t.Fatalf("%d witnesses", len(got.Witnesses))
			}
			size := func(witness power.Witness) int { return len(witness.Call.Args["x"].Value) }
			if !slices.IsSortedFunc(got.Witnesses, func(a, b power.Witness) int { return size(a) - size(b) }) {
				t.Fatalf("witnesses are not sorted by simplicity: %v", witnessInputs(got))
			}
			for _, witness := range got.Witnesses {
				if witness.Reference.Kind != spec.Returns || witness.Reference.Same(witness.Mutant) {
					t.Fatalf("an invalid witness: %+v", witness)
				}
			}
		})
	})
}
