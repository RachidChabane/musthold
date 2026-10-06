package schemacheck

import (
	"errors"
	"strings"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/golden"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

func mustChecker(t testing.TB) *Checker {
	t.Helper()
	checker, err := New()
	if err != nil {
		t.Fatalf("compile schemas: %v", err)
	}
	return checker
}

func TestCompilation(t *testing.T) {
	t.Run("every embedded schema compiles from its embedded bytes [verifies SP-110~1]", func(t *testing.T) {
		mustChecker(t)
	})

	t.Run("a schema that references a resource that is not embedded fails to compile [verifies SP-27~1]", func(t *testing.T) {
		const remote = "https://example.com/remote.json"
		schemas := []format.Schema{{
			ID:    "https://rachid-chabane.com/attestations/remote-test/v1",
			Bytes: []byte(`{"$schema":"https://json-schema.org/draft/2020-12/schema","$ref":"` + remote + `"}`),
		}}
		_, err := compileAll(schemas)
		if err == nil {
			t.Fatal("expected a compile error")
		}
		if !strings.Contains(err.Error(), ErrRemoteReference.Error()) || !strings.Contains(err.Error(), remote) {
			t.Fatalf("error = %v, want the refusal naming %s", err, remote)
		}
	})
}

func TestGoldenSchemaResults(t *testing.T) {
	checker := mustChecker(t)
	for _, vector := range golden.Cases() {
		t.Run(vector.FileName()+" has the schema result its manifest states [verifies SP-25~1]", func(t *testing.T) {
			err := checker.Validate(vector.Document)
			switch vector.Schema {
			case golden.SchemaValid:
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
			case golden.SchemaInvalid:
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("expected ErrInvalid, got %v", err)
				}
			case golden.SchemaUnknownPredicateType:
				if !errors.Is(err, format.ErrUnknownPredicateType) {
					t.Fatalf("expected ErrUnknownPredicateType, got %v", err)
				}
			default:
				t.Fatalf("unknown schema expectation %q", vector.Schema)
			}
		})
	}
}

func TestRefusals(t *testing.T) {
	checker := mustChecker(t)
	cases := []struct {
		name     string
		document string
		want     error
	}{
		{"not JSON", `{"predicateType":`, ErrMalformed},
		{"not an object", `[1,2]`, ErrMalformed},
		{"no predicate type", `{"_type":"x"}`, format.ErrUnknownPredicateType},
		{"a predicate type outside the format", `{"predicateType":"https://example.com/p/v1"}`, format.ErrUnknownPredicateType},
	}
	for _, c := range cases {
		t.Run(c.name+" is refused before schema validation [verifies SP-28~1]", func(t *testing.T) {
			if err := checker.Validate([]byte(c.document)); !errors.Is(err, c.want) {
				t.Fatalf("error = %v, want %v", err, c.want)
			}
		})
	}

	t.Run("ValidateValue serializes a statement and validates it [verifies SP-28~1]", func(t *testing.T) {
		if err := checker.ValidateValue(proofs.Alignment()); err != nil {
			t.Fatal(err)
		}
		if err := checker.ValidateValue(make(chan int)); !errors.Is(err, ErrMalformed) {
			t.Fatalf("error = %v, want ErrMalformed", err)
		}
	})
}

func TestClaimRulesInSchemas(t *testing.T) {
	checker := mustChecker(t)
	confidence := 0.4
	cases := []struct {
		name  string
		claim format.Claim[format.Verdict]
		valid bool
	}{
		{"judged with confidence", format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: format.ClassJudged, Confidence: &confidence}, true},
		{"judged without confidence", format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: format.ClassJudged}, false},
		{"executed where judged is required", format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: format.ClassExecuted}, false},
		{"executed with confidence", format.Claim[format.Verdict]{Value: format.VerdictAligned, Class: format.ClassExecuted, Confidence: &confidence}, false},
	}
	for _, c := range cases {
		t.Run("the alignment verdict schema: "+c.name+" [verifies SP-23~1]", func(t *testing.T) {
			statement := proofs.Alignment(func(s *format.Statement[format.Alignment]) { s.Predicate.Verdict = c.claim })
			if err := checker.ValidateValue(statement); (err == nil) != c.valid {
				t.Fatalf("schema error = %v, want valid=%v", err, c.valid)
			}
		})
	}

	t.Run("the executed fields of discriminating power forbid a confidence [verifies SP-23~1]", func(t *testing.T) {
		statement := proofs.DiscriminatingPower(func(s *format.Statement[format.DiscriminatingPower]) {
			s.Predicate.CorrectCodePasses = format.Claim[bool]{Value: true, Class: format.ClassExecuted, Confidence: &confidence}
		})
		if err := checker.ValidateValue(statement); !errors.Is(err, ErrInvalid) {
			t.Fatalf("error = %v, want ErrInvalid", err)
		}
	})
}

type corruption[S any] struct {
	name  string
	apply func(*S)
}

func alignmentCorruptions() []corruption[format.Statement[format.Alignment]] {
	type statement = format.Statement[format.Alignment]
	tooHigh, negative := 1.5, -0.1
	return []corruption[statement]{
		{"judged without confidence", func(s *statement) { s.Predicate.Verdict.Confidence = nil }},
		{"executed class", func(s *statement) { s.Predicate.Verdict.Class = format.ClassExecuted }},
		{"confidence above one", func(s *statement) { s.Predicate.Verdict.Confidence = &tooHigh }},
		{"confidence below zero", func(s *statement) { s.Predicate.Verdict.Confidence = &negative }},
		{"unknown verdict", func(s *statement) { s.Predicate.Verdict.Value = "ALMOST" }},
		{"executor not blind", func(s *statement) { s.Predicate.Executor.Blind = false }},
		{"bad prompt digest", func(s *statement) { s.Predicate.Executor.PromptDigest = "short" }},
		{"empty test", func(s *statement) { s.Predicate.Test = "" }},
		{"unknown channel", func(s *statement) { s.Predicate.Executor.Channel = "carrier-pigeon" }},
		{"attested without cap digest", func(s *statement) { s.Predicate.Executor.Attestation.CapDigest = "" }},
		{"unnamed known model", func(s *statement) { s.Predicate.Executor.Model.Name = "" }},
		{"unknown producer", func(s *statement) { s.Predicate.ProducedBy = "laptop" }},
		{"bad cache key", func(s *statement) { s.Predicate.CacheKey = "short" }},
		{"empty module name", func(s *statement) { s.Predicate.Module.Name = "" }},
		{"wrong canonicalization", func(s *statement) { s.Predicate.Requirement.StatementCanonicalization = "c9" }},
		{"wrong statement type", func(s *statement) { s.Type = "https://in-toto.io/Statement/v0.1" }},
	}
}

func powerCorruptions() []corruption[format.Statement[format.DiscriminatingPower]] {
	type statement = format.Statement[format.DiscriminatingPower]
	confidence := 0.5
	onFirstMutant := func(edit func(*format.Mutant)) func(*statement) {
		return func(s *statement) {
			if len(s.Predicate.Mutants) > 0 {
				edit(&s.Predicate.Mutants[0])
			}
		}
	}
	return []corruption[statement]{
		{"correctCodePasses with confidence", func(s *statement) { s.Predicate.CorrectCodePasses.Confidence = &confidence }},
		{"correctCodePasses judged", func(s *statement) {
			s.Predicate.CorrectCodePasses.Class = format.ClassJudged
			s.Predicate.CorrectCodePasses.Confidence = &confidence
		}},
		{"validity executed", onFirstMutant(func(m *format.Mutant) { m.Validity.Class = format.ClassExecuted })},
		{"validity without confidence", onFirstMutant(func(m *format.Mutant) { m.Validity.Confidence = nil })},
		{"unknown validity", onFirstMutant(func(m *format.Mutant) { m.Validity.Value = "MAYBE" })},
		{"bad mutant digest", onFirstMutant(func(m *format.Mutant) { m.Digest = "short" })},
		{"outcome judged", onFirstMutant(func(m *format.Mutant) {
			m.Results[0].Outcome.Class = format.ClassJudged
			m.Results[0].Outcome.Confidence = &confidence
		})},
		{"unknown outcome", onFirstMutant(func(m *format.Mutant) { m.Results[0].Outcome.Value = "crashed" })},
		{"sentinel not equivalent", onFirstMutant(func(m *format.Mutant) {
			m.Sentinel = true
			m.Validity.Value = format.ValidityValid
		})},
		{"example judged", func(s *statement) {
			if len(s.Predicate.Examples) > 0 {
				s.Predicate.Examples[0].Result.Class = format.ClassJudged
				s.Predicate.Examples[0].Result.Confidence = &confidence
			}
		}},
		{"no tests", func(s *statement) { s.Predicate.Tests = nil }},
		{"valid mutant without corroboration", onFirstMutant(func(m *format.Mutant) {
			m.Validity.Value = format.ValidityValid
			m.Validity.CorroboratedBy = nil
		})},
		{"corroboration on an executed outcome", onFirstMutant(func(m *format.Mutant) {
			m.Results[0].Outcome.CorroboratedBy = []format.Corroboration{{Kind: format.CorroborationBuilds, Digest: strings.Repeat("a", format.Sha256HexLength)}}
		})},
		{"unattributed valid mutant", onFirstMutant(func(m *format.Mutant) {
			m.Validity.Value = format.ValidityValid
			m.Extensions = &format.MutantExtensions{Unattributed: true}
		})},
		{"no generator model", func(s *statement) { s.Predicate.Executor.Model.Name = "" }},
	}
}

func completenessCorruptions() []corruption[format.Statement[format.Completeness]] {
	type statement = format.Statement[format.Completeness]
	return []corruption[statement]{
		{"not advisory", func(s *statement) { s.Predicate.Advisory = false }},
		{"no inputs", func(s *statement) { s.Predicate.Inputs = nil }},
		{"bad subject digest", func(s *statement) { s.Subject[0].Digest.SHA256 = "short" }},
		{"empty question", func(s *statement) {
			if len(s.Predicate.Questions) > 0 {
				s.Predicate.Questions[0].Question = ""
			}
		}},
	}
}

// assertAgreement checks that the schemas accept a statement exactly when the
// domain invariants hold.
func assertAgreement(t *rapid.T, checker *Checker, proof format.Proof) {
	schemaErr := checker.ValidateValue(proof)
	domainErr := proof.Validate()
	if (schemaErr == nil) != (domainErr == nil) {
		t.Fatalf("schema error: %v\ndomain error: %v", schemaErr, domainErr)
	}
}

func applyCorruption[S any](t *rapid.T, statement *S, corruptions []corruption[S]) {
	choice := rapid.IntRange(-1, len(corruptions)-1).Draw(t, "corruption")
	if choice >= 0 {
		corruptions[choice].apply(statement)
	}
}

func TestSchemaAgreesWithInvariants(t *testing.T) {
	checker := mustChecker(t)

	t.Run("generated alignments pass the schema exactly when the invariants hold [verifies SP-26~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := proofs.Alignments().Draw(t, "statement")
			applyCorruption(t, &statement, alignmentCorruptions())
			assertAgreement(t, checker, statement)
		})
	})

	t.Run("generated discriminating-power statements pass the schema exactly when the invariants hold [verifies SP-26~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := proofs.DiscriminatingPowers().Draw(t, "statement")
			applyCorruption(t, &statement, powerCorruptions())
			assertAgreement(t, checker, statement)
		})
	})

	t.Run("generated completeness statements pass the schema exactly when the invariants hold [verifies SP-26~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := proofs.Completenesses().Draw(t, "statement")
			applyCorruption(t, &statement, completenessCorruptions())
			assertAgreement(t, checker, statement)
		})
	})
}

func TestSpecPathAgreement(t *testing.T) {
	checker := mustChecker(t)
	t.Run("the Go path check and the schema pattern agree on every path [verifies SP-29~1]", func(t *testing.T) {
		alphabet := []rune{'a', 'b', '.', '/', '\\', '\x00'}
		rapid.Check(t, func(t *rapid.T) {
			path := string(rapid.SliceOfN(rapid.SampledFrom(alphabet), 1, 8).Draw(t, "path"))
			statement := proofs.Completeness(func(s *format.Statement[format.Completeness]) {
				s.Subject[0].Name = path
			})
			schemaErr := checker.ValidateValue(statement)
			if goAccepts := format.IsRelativePOSIXPath(path); goAccepts != (schemaErr == nil) {
				t.Fatalf("path %q: Go accepts %v, schema error %v", path, goAccepts, schemaErr)
			}
		})
	})
}
