package format_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/formattest/proofs"
)

const verifiesSP2700 = " [verifies SP-2700~1]"

// Parts of the failed witnesses of these tests.
const (
	failedWitnessName   = "free_from_fifty"
	failedWitnessClause = "reaches 50"
	failedWitnessCode   = "def free_from_fifty():\n    assert shipping_fee(50) == 0"
)

// failedWitnessStatement is a statement, produced by the producer, that lists the
// failed witnesses.
func failedWitnessStatement(producedBy format.ProducedBy, witnesses ...format.FailedWitness) format.Statement[format.DiscriminatingPower] {
	return proofs.DiscriminatingPower(func(s *format.Statement[format.DiscriminatingPower]) {
		s.Predicate.ProducedBy = producedBy
		s.Predicate.FailedWitnesses = witnesses
	})
}

func failedWitness(label string) format.FailedWitness {
	return format.FailedWitness{ID: proofs.Digest(label), Name: failedWitnessName, Clause: failedWitnessClause}
}

func TestFailedWitnesses(t *testing.T) {
	t.Run("a local statement shows the code of its failed witnesses and a CI statement names them"+verifiesSP2700, func(t *testing.T) {
		statements := []format.Statement[format.DiscriminatingPower]{
			failedWitnessStatement(format.ProducedByLocal, failedWitness("a").WithCode(failedWitnessCode)),
			failedWitnessStatement(format.ProducedByCI, failedWitness("a")),
		}
		for _, statement := range statements {
			if err := statement.Validate(); err != nil {
				t.Errorf("%s: %v", statement.Predicate.ProducedBy, err)
			}
		}
	})

	t.Run("the failed witnesses survive encoding and decoding"+verifiesSP2700, func(t *testing.T) {
		statement := failedWitnessStatement(format.ProducedByLocal, failedWitness("a").WithCode(failedWitnessCode))
		encoded, err := statement.Encode()
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := format.DecodeAs[format.DiscriminatingPower](encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(decoded.Predicate.FailedWitnesses, statement.Predicate.FailedWitnesses) {
			t.Errorf("decoded %+v", decoded.Predicate.FailedWitnesses)
		}
	})

	t.Run("code in a CI statement, oversized code, a bad id, no name and an unsorted list are refused"+verifiesSP2700, func(t *testing.T) {
		oversized := failedWitness("a")
		oversized.Code = strings.Repeat("x", format.SourceViewCap+1)
		cut := failedWitness("a")
		cut.Truncated = true
		badID := failedWitness("a")
		badID.ID = failedWitnessName
		unnamed := failedWitness("a")
		unnamed.Name = ""
		ascending := []format.FailedWitness{failedWitness("a"), failedWitness("b")}
		slices.SortFunc(ascending, func(x, y format.FailedWitness) int { return strings.Compare(x.ID, y.ID) })
		cases := []struct {
			want      format.ViolationCode
			statement format.Statement[format.DiscriminatingPower]
		}{
			{format.CodeCounterexampleNotLocal, failedWitnessStatement(format.ProducedByCI, failedWitness("a").WithCode(failedWitnessCode))},
			{format.CodeCounterexampleNotLocal, failedWitnessStatement(format.ProducedByCI, cut)},
			{format.CodeCounterexampleSize, failedWitnessStatement(format.ProducedByLocal, oversized)},
			{format.CodeBadDigest, failedWitnessStatement(format.ProducedByLocal, badID)},
			{format.CodeMissingField, failedWitnessStatement(format.ProducedByLocal, unnamed)},
			{format.CodeUnsorted, failedWitnessStatement(format.ProducedByLocal, ascending[1], ascending[0])},
			{format.CodeUnsorted, failedWitnessStatement(format.ProducedByLocal, ascending[0], ascending[0])},
		}
		for _, c := range cases {
			if codes := violationCodes(t, c.statement.Validate()); !slices.Contains(codes, c.want) {
				t.Errorf("want %s, got %v", c.want, codes)
			}
		}
	})

	t.Run("the schema bounds a failed witness's code by the bound of a host view"+verifiesSP2700, func(t *testing.T) {
		schema, err := format.PredicateSchema(format.KindDiscriminatingPower.PredicateType())
		if err != nil {
			t.Fatal(err)
		}
		var document struct {
			Defs struct {
				FailedWitness struct {
					Properties struct {
						Code struct {
							MaxLength int `json:"maxLength"`
						} `json:"code"`
					} `json:"properties"`
				} `json:"failedWitness"`
			} `json:"$defs"`
		}
		if err := json.Unmarshal(schema.Bytes, &document); err != nil {
			t.Fatal(err)
		}
		if got := document.Defs.FailedWitness.Properties.Code.MaxLength; got != format.SourceViewCap {
			t.Errorf("maxLength %d, SourceViewCap %d", got, format.SourceViewCap)
		}
	})

	t.Run("a longer code is cut by the bound of a host view and says so"+verifiesSP2700, func(t *testing.T) {
		shown := failedWitness("a").WithCode(strings.Repeat("x", format.SourceViewCap+5))
		if !shown.Truncated || len(shown.Code) != format.SourceViewCap {
			t.Errorf("truncated %t, %d bytes", shown.Truncated, len(shown.Code))
		}
	})
}
