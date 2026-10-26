package format_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/formattest/proofs"
)

// counterexampleStatement is a statement whose regular mutant carries the
// counterexample, produced by the producer.
func counterexampleStatement(producedBy format.ProducedBy, counterexample format.Counterexample) format.Statement[format.DiscriminatingPower] {
	return proofs.DiscriminatingPower(func(s *format.Statement[format.DiscriminatingPower]) {
		s.Predicate.ProducedBy = producedBy
		mutant := &s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)]
		if mutant.Extensions == nil {
			mutant.Extensions = &format.MutantExtensions{}
		}
		mutant.Extensions.Counterexample = &counterexample
	})
}

func TestCounterexample(t *testing.T) {
	t.Run("the schema bounds a witness's code by the bound of a host view [verifies SP-2420~1]", func(t *testing.T) {
		schema, err := format.PredicateSchema(format.KindDiscriminatingPower.PredicateType())
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Defs struct {
				Counterexample struct {
					Properties struct {
						Witness struct {
							Properties struct {
								Code struct {
									MaxLength int `json:"maxLength"`
								} `json:"code"`
							} `json:"properties"`
						} `json:"witness"`
					} `json:"properties"`
				} `json:"counterexample"`
			} `json:"$defs"`
		}
		if err := json.Unmarshal(schema.Bytes, &document); err != nil {
			t.Fatal(err)
		}
		if got := document.Defs.Counterexample.Properties.Witness.Properties.Code.MaxLength; got != format.SourceViewCap {
			t.Errorf("maxLength %d, SourceViewCap %d", got, format.SourceViewCap)
		}
	})

	example := 0
	t.Run("a local statement carries a counterexample of a targeted or an operator mutant [verifies SP-2420~1]", func(t *testing.T) {
		witness := format.BoundedWitness("w", "clause", "def w():\n    assert f(1) == 2")
		for _, counterexample := range []format.Counterexample{
			{Origin: format.OriginTargeted, Slot: format.SlotBoundary, Example: &example},
			{Origin: format.OriginOperator, Family: "relational", Witness: &witness},
		} {
			statement := counterexampleStatement(format.ProducedByLocal, counterexample)
			if err := statement.Validate(); err != nil {
				t.Errorf("%+v: %v", counterexample, err)
			}
		}
	})

	t.Run("a statement produced by CI and a malformed counterexample are refused [verifies SP-2420~1]", func(t *testing.T) {
		witness := format.WitnessExample{Name: "w", Clause: "c", Code: strings.Repeat("x", format.SourceViewCap+1)}
		for want, statement := range map[format.ViolationCode]format.Statement[format.DiscriminatingPower]{
			format.CodeCounterexampleNotLocal: counterexampleStatement(format.ProducedByCI, format.Counterexample{Origin: format.OriginTargeted, Slot: format.SlotDirect, Example: &example}),
			format.CodeCounterexampleSize:     counterexampleStatement(format.ProducedByLocal, format.Counterexample{Origin: format.OriginOperator, Family: "relational", Witness: &witness}),
			format.CodeMissingField:           counterexampleStatement(format.ProducedByLocal, format.Counterexample{Origin: format.OriginOperator, Family: "relational"}),
			format.CodeUnknownValue:           counterexampleStatement(format.ProducedByLocal, format.Counterexample{Origin: format.OriginTargeted, Family: "relational", Example: &example}),
		} {
			if codes := violationCodes(t, statement.Validate()); !slices.Contains(codes, want) {
				t.Errorf("want %s, got %v", want, codes)
			}
		}
	})

	t.Run("a witness within the bound is kept whole [verifies SP-2420~1]", func(t *testing.T) {
		witness := format.BoundedWitness("w", "clause", "def w():\n    assert f(1) == 2")
		if witness.Code != "def w():\n    assert f(1) == 2" || witness.Truncated || witness.Name != "w" || witness.Clause != "clause" {
			t.Errorf("witness = %+v", witness)
		}
	})

	t.Run("a longer witness is cut at the last line break within the bound and says so [verifies SP-2420~1]", func(t *testing.T) {
		line := strings.Repeat("x", 99) + "\n"
		code := strings.Repeat(line, format.SourceViewCap/len(line)+10)
		witness := format.BoundedWitness("w", "clause", code)
		if !witness.Truncated || len(witness.Code) > format.SourceViewCap || !strings.HasSuffix(witness.Code, "\n") || !strings.HasPrefix(code, witness.Code) {
			t.Errorf("truncated %t, %d bytes", witness.Truncated, len(witness.Code))
		}
		if len(witness.Code) < format.SourceViewCap-len(line) {
			t.Errorf("the cut kept %d bytes, more than a line short of the bound", len(witness.Code))
		}
	})

	t.Run("a longer witness without a line break is cut at the bound [verifies SP-2420~1]", func(t *testing.T) {
		witness := format.BoundedWitness("w", "clause", strings.Repeat("x", format.SourceViewCap+5))
		if !witness.Truncated || len(witness.Code) != format.SourceViewCap {
			t.Errorf("truncated %t, %d bytes", witness.Truncated, len(witness.Code))
		}
	})
}
