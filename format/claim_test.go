package format_test

import (
	"math"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

func TestClaimConstructors(t *testing.T) {
	judgedCases := []struct {
		name       string
		confidence float64
		wantErr    bool
	}{
		{"lower bound", 0, false},
		{"middle", 0.5, false},
		{"upper bound", 1, false},
		{"below the range", -0.1, true},
		{"above the range", 1.1, true},
		{"not a number", math.NaN(), true},
		{"infinity", math.Inf(1), true},
	}
	for _, c := range judgedCases {
		t.Run("a judged claim with confidence "+c.name+" [verifies SP-3~1]", func(t *testing.T) {
			claim, err := format.Judged(format.VerdictAligned, c.confidence)
			if (err != nil) != c.wantErr {
				t.Fatalf("Judged error = %v, wantErr %v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if claim.Class != format.ClassJudged || claim.Confidence == nil || *claim.Confidence != c.confidence {
				t.Fatalf("claim = %+v", claim)
			}
		})
	}

	t.Run("an executed claim carries no confidence [verifies SP-3~1]", func(t *testing.T) {
		claim := format.Executed(format.OutcomeKilled)
		if claim.Class != format.ClassExecuted || claim.Confidence != nil {
			t.Fatalf("claim = %+v", claim)
		}
	})

	t.Run("a constructed judged claim keeps a confidence of zero on the wire [verifies SP-3~1]", func(t *testing.T) {
		claim, err := format.Judged(format.VerdictUndetermined, 0)
		if err != nil {
			t.Fatal(err)
		}
		encoded, err := format.Canonicalize(claim)
		if err != nil {
			t.Fatal(err)
		}
		const want = `{"class":"judged","confidence":0,"value":"UNDETERMINED"}`
		if string(encoded) != want {
			t.Fatalf("encoded = %s, want %s", encoded, want)
		}
	})

	t.Run("every claim built by a constructor passes validation inside a statement [verifies SP-3~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := proofs.Alignments().Draw(t, "statement")
			if err := statement.Validate(); err != nil {
				t.Fatalf("generated statement is invalid: %v", err)
			}
		})
	})
}

func TestClaimRules(t *testing.T) {
	t.Run("a decoded claim that breaks the confidence rule is reported [verifies SP-4~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*format.Statement[format.DiscriminatingPower])
			want format.ViolationCode
		}{
			{"judged without confidence", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity = format.Claim[format.Validity]{Value: format.ValidityEquivalent, Class: format.ClassJudged}
			}, format.CodeJudgedWithoutConfidence},
			{"executed with confidence", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.CorrectCodePasses = format.Claim[bool]{Value: true, Class: format.ClassExecuted, Confidence: floatPointer(0.5)}
			}, format.CodeExecutedWithConfidence},
			{"confidence above one", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity = format.Claim[format.Validity]{Value: format.ValidityEquivalent, Class: format.ClassJudged, Confidence: floatPointer(1.01)}
			}, format.CodeConfidenceRange},
			{"negative confidence", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity = format.Claim[format.Validity]{Value: format.ValidityEquivalent, Class: format.ClassJudged, Confidence: floatPointer(-0.01)}
			}, format.CodeConfidenceRange},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.DiscriminatingPower(c.edit).Validate(), c.want)
			})
		}
	})

	t.Run("a claim of the wrong class for its field is reported [verifies SP-5~1]", func(t *testing.T) {
		judged := floatPointer(0.8)
		cases := []struct {
			name string
			edit func(*format.Statement[format.DiscriminatingPower])
		}{
			{"correctCodePasses judged", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.CorrectCodePasses = format.Claim[bool]{Value: true, Class: format.ClassJudged, Confidence: judged}
			}},
			{"validity executed", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity = format.Executed(format.ValidityValid)
			}},
			{"outcome judged", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Results[0].Outcome = format.Claim[format.Outcome]{Value: format.OutcomeKilled, Class: format.ClassJudged, Confidence: judged}
			}},
			{"example result judged", func(s *format.Statement[format.DiscriminatingPower]) {
				s.Predicate.Examples[0].Result = format.Claim[format.ExampleResult]{Value: format.ExamplePassed, Class: format.ClassJudged, Confidence: judged}
			}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.DiscriminatingPower(c.edit).Validate(), format.CodeWrongClass)
			})
		}
		t.Run("alignment verdict executed", func(t *testing.T) {
			statement := proofs.Alignment(func(s *format.Statement[format.Alignment]) {
				s.Predicate.Verdict = format.Executed(format.VerdictAligned)
			})
			assertCodes(t, statement.Validate(), format.CodeWrongClass)
		})
		t.Run("an unknown class is not a class", func(t *testing.T) {
			statement := proofs.Alignment(func(s *format.Statement[format.Alignment]) {
				s.Predicate.Verdict = format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: "inferred"}
			})
			assertCodes(t, statement.Validate(), format.CodeUnknownValue)
		})
	})
}
