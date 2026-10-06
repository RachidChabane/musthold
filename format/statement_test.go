package format_test

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

type (
	alignment = format.Statement[format.Alignment]
	power     = format.Statement[format.DiscriminatingPower]
	spec      = format.Statement[format.Completeness]
)

func TestEnvelope(t *testing.T) {
	t.Run("a valid statement of each predicate encodes, decodes and validates [verifies SP-1~1]", func(t *testing.T) {
		cases := []struct {
			name      string
			proof     format.Proof
			kind      format.Kind
			predicate string
		}{
			{"alignment", proofs.Alignment(), format.KindAlignment, format.PredicateTypeAlignment},
			{"discriminating power", proofs.DiscriminatingPower(), format.KindDiscriminatingPower, format.PredicateTypeDiscriminatingPower},
			{"completeness", proofs.Completeness(), format.KindCompleteness, format.PredicateTypeCompleteness},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				encoded, err := c.proof.Encode()
				if err != nil {
					t.Fatal(err)
				}
				decoded, err := format.Decode(encoded)
				if err != nil {
					t.Fatal(err)
				}
				if decoded.Kind() != c.kind || c.kind.PredicateType() != c.predicate {
					t.Fatalf("kind = %s (%s), want %s (%s)", decoded.Kind(), decoded.Kind().PredicateType(), c.kind, c.predicate)
				}
				if err := decoded.Validate(); err != nil {
					t.Fatalf("decoded statement is invalid: %v", err)
				}
				if !reflect.DeepEqual(decoded, c.proof) {
					t.Fatalf("round trip changed the statement:\n%+v\n%+v", decoded, c.proof)
				}
				reencoded, err := decoded.Encode()
				if err != nil || string(reencoded) != string(encoded) {
					t.Fatalf("re-encoding differs (err %v)", err)
				}
			})
		}
	})

	t.Run("the predicate type URIs are the ones of ADR 0001 [verifies SP-1~1]", func(t *testing.T) {
		want := map[format.Kind]string{
			format.KindAlignment:           "https://rachid-chabane.com/attestations/alignment/v1",
			format.KindDiscriminatingPower: "https://rachid-chabane.com/attestations/discriminating-power/v1",
			format.KindCompleteness:        "https://rachid-chabane.com/attestations/completeness/v1",
		}
		for kind, uri := range want {
			if kind.PredicateType() != uri {
				t.Errorf("%s: %s, want %s", kind, kind.PredicateType(), uri)
			}
			if resolved, ok := format.KindOfPredicateType(uri); !ok || resolved != kind {
				t.Errorf("%s does not resolve back to %s", uri, kind)
			}
		}
		if format.StatementType != "https://in-toto.io/Statement/v1" {
			t.Errorf("statement type = %s", format.StatementType)
		}
	})

	t.Run("a wrong statement type or predicate type is a violation [verifies SP-1~1]", func(t *testing.T) {
		wrongType := proofs.Alignment(func(s *alignment) { s.Type = "https://in-toto.io/Statement/v0.1" })
		assertCodes(t, wrongType.Validate(), format.CodeStatementType)
		wrongPredicate := proofs.Alignment(func(s *alignment) { s.PredicateType = format.PredicateTypeCompleteness })
		assertCodes(t, wrongPredicate.Validate(), format.CodePredicateType)
	})

	t.Run("a predicate type outside the format is refused by Decode [verifies SP-1~1]", func(t *testing.T) {
		document := strings.Replace(string(mustEncode(t, proofs.Alignment())), format.PredicateTypeAlignment, "https://rachid-chabane.com/attestations/future/v1", 1)
		_, err := format.Decode([]byte(document))
		if !errors.Is(err, format.ErrUnknownPredicateType) {
			t.Fatalf("error = %v, want ErrUnknownPredicateType", err)
		}
	})

	t.Run("a document that is not JSON is refused by Decode [verifies SP-1~1]", func(t *testing.T) {
		if _, err := format.Decode([]byte(`{"predicateType":`)); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("Encode refuses an invalid statement [verifies SP-1~1]", func(t *testing.T) {
		invalid := proofs.Alignment(func(s *alignment) { s.Predicate.Test = "" })
		if _, err := invalid.Encode(); err == nil {
			t.Fatal("expected an error")
		}
	})

	t.Run("a member the format does not define is ignored at every level [verifies SP-2~1]", func(t *testing.T) {
		var generic map[string]any
		if err := json.Unmarshal(mustEncode(t, proofs.DiscriminatingPower()), &generic); err != nil {
			t.Fatal(err)
		}
		generic["later"] = "top"
		predicate := generic["predicate"].(map[string]any)
		predicate["later"] = map[string]any{"nested": true}
		predicate["generator"].(map[string]any)["later"] = 1
		predicate["mutants"].([]any)[0].(map[string]any)["later"] = []any{"x"}
		document, err := json.Marshal(generic)
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := format.Decode(document)
		if err != nil {
			t.Fatal(err)
		}
		if err := decoded.Validate(); err != nil {
			t.Fatalf("unknown members made the statement invalid: %v", err)
		}
	})
}

func mustEncode(t *testing.T, proof format.Proof) []byte {
	t.Helper()
	encoded, err := proof.Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func TestSubjectRules(t *testing.T) {
	t.Run("alignment and discriminating power are about the criterion and its statement digest [verifies SP-10~1]", func(t *testing.T) {
		wrongName := func(name string) func(*alignment) {
			return func(s *alignment) { s.Subject[0].Name = name }
		}
		cases := []struct {
			name string
			edit func(*alignment)
			want []format.ViolationCode
		}{
			{"the right subject", func(*alignment) {}, nil},
			{"another requirement", wrongName("PARK-3~1"), []format.ViolationCode{format.CodeSubject}},
			{"another revision", wrongName("PARK-2~2"), []format.ViolationCode{format.CodeSubject}},
			{"no revision", wrongName("PARK-2"), []format.ViolationCode{format.CodeSubject}},
			{"no subject", func(s *alignment) { s.Subject = nil }, []format.ViolationCode{format.CodeSubject}},
			{"two subjects", func(s *alignment) { s.Subject = append(s.Subject, s.Subject[0]) }, []format.ViolationCode{format.CodeSubject}},
			{"a digest that is not SHA-256", func(s *alignment) { s.Subject[0].Digest.SHA256 = "abc" }, []format.ViolationCode{format.CodeBadDigest}},
			{"an uppercase digest", func(s *alignment) { s.Subject[0].Digest.SHA256 = strings.ToUpper(s.Subject[0].Digest.SHA256) }, []format.ViolationCode{format.CodeBadDigest}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.Alignment(c.edit).Validate(), c.want...)
			})
		}
		t.Run("discriminating power follows the same rule", func(t *testing.T) {
			statement := proofs.DiscriminatingPower(func(s *power) { s.Subject[0].Name = "PARK-2~9" })
			assertCodes(t, statement.Validate(), format.CodeSubject)
		})
		t.Run("a requirement ID containing the revision separator is refused", func(t *testing.T) {
			statement := proofs.Alignment(func(s *alignment) { s.Predicate.Requirement.ID = "PARK~2" })
			assertCodes(t, statement.Validate(), format.CodeRequirement, format.CodeSubject)
		})
		t.Run("a revision below one and another canonicalization are refused", func(t *testing.T) {
			statement := proofs.Alignment(func(s *alignment) {
				s.Predicate.Requirement.Revision = 0
				s.Predicate.Requirement.StatementCanonicalization = "c2"
			})
			assertCodes(t, statement.Validate(), format.CodeRequirement, format.CodeSubject)
		})
	})

	t.Run("completeness is about a spec path and the hash of the file [verifies SP-11~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*spec)
			want []format.ViolationCode
		}{
			{"a relative path", func(s *spec) { s.Subject[0].Name = "specs/nested/a.md" }, nil},
			{"an absolute path", func(s *spec) { s.Subject[0].Name = "/specs/a.md" }, []format.ViolationCode{format.CodeSubject}},
			{"a parent segment", func(s *spec) { s.Subject[0].Name = "../a.md" }, []format.ViolationCode{format.CodeSubject}},
			{"a backslash", func(s *spec) { s.Subject[0].Name = `specs\a.md` }, []format.ViolationCode{format.CodeSubject}},
			{"a requirement name", func(s *spec) { s.Subject[0].Name = "" }, []format.ViolationCode{format.CodeSubject}},
			{"no subject", func(s *spec) { s.Subject = nil }, []format.ViolationCode{format.CodeSubject}},
			{"a bad digest", func(s *spec) { s.Subject[0].Digest.SHA256 = "zz" }, []format.ViolationCode{format.CodeBadDigest}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.Completeness(c.edit).Validate(), c.want...)
			})
		}
	})

	t.Run("the subject constructors build the names of the rules [verifies SP-10~1]", func(t *testing.T) {
		subject := format.RequirementSubject("PARK-2", 3, proofs.Digest("x"))
		if subject.Name != "PARK-2~3" || subject.Digest.SHA256 != proofs.Digest("x") {
			t.Fatalf("subject = %+v", subject)
		}
		if got := format.SpecSubject("specs/a.md", proofs.Digest("y")); got.Name != "specs/a.md" {
			t.Fatalf("subject = %+v", got)
		}
	})
}

func TestOrdering(t *testing.T) {
	reversed := func(t *testing.T, s []string) []string {
		t.Helper()
		out := slices.Clone(s)
		slices.Reverse(out)
		return out
	}
	cases := []struct {
		name string
		edit func(*testing.T, *power)
	}{
		{"tests", func(t *testing.T, s *power) { s.Predicate.Tests = reversed(t, s.Predicate.Tests) }},
		{"duplicate tests", func(t *testing.T, s *power) { s.Predicate.Tests = []string{"a", "a"} }},
		{"mutants", func(t *testing.T, s *power) { slices.Reverse(s.Predicate.Mutants) }},
		{"mutant results", func(t *testing.T, s *power) {
			slices.Reverse(s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Results)
		}},
		{"examples", func(t *testing.T, s *power) { slices.Reverse(s.Predicate.Examples) }},
		{"inputs", func(t *testing.T, s *power) { slices.Reverse(s.Predicate.Inputs) }},
		{"decision deciders", func(t *testing.T, s *power) {
			regularMutant(s).Extensions.Decision.Deciders = []string{"b", "a"}
		}},
		{"annotation globs", func(t *testing.T, s *power) {
			s.Predicate.Inputs[0].Annotations = &format.Annotations{Globs: []string{"src/**", "lib/**"}}
		}},
	}
	for _, c := range cases {
		t.Run("an unsorted array is reported: "+c.name+" [verifies SP-12~1]", func(t *testing.T) {
			statement := proofs.DiscriminatingPower(func(s *power) { c.edit(t, s) })
			assertCodes(t, statement.Validate(), format.CodeUnsorted)
		})
	}

	t.Run("completeness questions, requirement IDs and out-of-scope entries are sorted [verifies SP-12~1]", func(t *testing.T) {
		questions := proofs.Completeness(func(s *spec) { slices.Reverse(s.Predicate.Questions) })
		assertCodes(t, questions.Validate(), format.CodeUnsorted)
		ids := proofs.Completeness(func(s *spec) { s.Predicate.Questions[0].RequirementIDs = []string{"PARK-2", "PARK-10"} })
		assertCodes(t, ids.Validate(), format.CodeUnsorted)
		outOfScope := proofs.Completeness(func(s *spec) {
			s.Predicate.OutOfScopeDeclared = []format.OutOfScope{{Dimension: "b", Reason: "x"}, {Dimension: "a", Reason: "x"}}
		})
		assertCodes(t, outOfScope.Validate(), format.CodeUnsorted)
	})

	t.Run("the order is byte-wise so PARK-10 precedes PARK-2 [verifies SP-12~1]", func(t *testing.T) {
		statement := proofs.Completeness(func(s *spec) { s.Predicate.Questions[0].RequirementIDs = []string{"PARK-10", "PARK-2"} })
		if err := statement.Validate(); err != nil {
			t.Fatalf("byte-wise order was rejected: %v", err)
		}
	})
}

func TestClosedSets(t *testing.T) {
	t.Run("a value outside a closed set is reported [verifies SP-13~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*power)
			want format.ViolationCode
		}{
			{"input role", func(s *power) { s.Predicate.Inputs[0].Role = "weird" }, format.CodeUnknownValue},
			{"producer", func(s *power) { s.Predicate.ProducedBy = "laptop" }, format.CodeUnknownValue},
			{"verdict-like validity", func(s *power) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Validity.Value = "MAYBE"
			}, format.CodeUnknownValue},
			{"outcome", func(s *power) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Results[0].Outcome.Value = "crashed"
			}, format.CodeUnknownValue},
			{"example result", func(s *power) { s.Predicate.Examples[0].Result.Value = "unknown" }, format.CodeUnknownValue},
			{"decision basis", func(s *power) {
				regularMutant(s).Extensions.Decision.Basis = "vibes"
			}, format.CodeUnknownValue},
			{"input digest", func(s *power) { s.Predicate.Inputs[0].Digest.SHA256 = "short" }, format.CodeBadDigest},
			{"mutant digest", func(s *power) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Digest = "short"
			}, format.CodeBadDigest},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				statement := proofs.DiscriminatingPower(c.edit)
				codes := violationCodes(t, statement.Validate())
				if !slices.Contains(codes, c.want) {
					t.Fatalf("codes = %v, want %s among them", codes, c.want)
				}
			})
		}
	})

	t.Run("an alignment verdict outside the closed set is reported [verifies SP-13~1]", func(t *testing.T) {
		statement := proofs.Alignment(func(s *alignment) {
			s.Predicate.Verdict = format.Claim[format.Verdict]{Value: "ALMOST", Class: format.ClassJudged, Confidence: floatPointer(0.5)}
		})
		assertCodes(t, statement.Validate(), format.CodeUnknownValue)
	})

	t.Run("a cache key that is not a SHA-256 digest is reported [verifies SP-13~1]", func(t *testing.T) {
		statement := proofs.Alignment()
		statement.Predicate.CacheKey = "short"
		assertCodes(t, statement.Validate(), format.CodeBadDigest)
	})

	t.Run("every input role of ADR 0001 and ADR 0003 is accepted [verifies SP-13~1]", func(t *testing.T) {
		roles := format.InputRoles()
		if len(roles) != 9 {
			t.Fatalf("roles = %v", roles)
		}
		for _, role := range []format.InputRole{format.RoleDecisionPolicy, format.RoleExampleBinding} {
			if !slices.Contains(roles, role) {
				t.Errorf("role %s is missing", role)
			}
		}
		statement := proofs.DiscriminatingPower()
		if err := statement.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestCacheKeyInvariant(t *testing.T) {
	t.Run("a recorded cache key that differs from the digest of subject and inputs is reported [verifies SP-14~1]", func(t *testing.T) {
		statement := proofs.Alignment()
		statement.Predicate.CacheKey = proofs.Digest("another key")
		assertCodes(t, statement.Validate(), format.CodeCacheKeyMismatch)
	})

	t.Run("editing an input without recomputing the key is reported [verifies SP-14~1]", func(t *testing.T) {
		statement := proofs.Alignment()
		statement.Predicate.Inputs[0].Digest.SHA256 = proofs.Digest("edited")
		assertCodes(t, statement.Validate(), format.CodeCacheKeyMismatch)
	})

	t.Run("the key still matches after the inputs are shuffled and resorted [verifies SP-14~1]", func(t *testing.T) {
		statement := proofs.Alignment()
		key := statement.Predicate.CacheKey
		statement.Predicate.Inputs = proofs.SortInputs(statement.Predicate.Inputs)
		if statement.Predicate.CacheKey != key {
			t.Fatal("key changed")
		}
		if err := statement.Validate(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPredicateInvariants(t *testing.T) {
	t.Run("an alignment needs a judged verdict and a blind judge [verifies SP-15~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*alignment)
			want []format.ViolationCode
		}{
			{"valid", func(*alignment) {}, nil},
			{"judge not blind", func(s *alignment) { s.Predicate.Judge.Blind = false }, []format.ViolationCode{format.CodeNotBlind}},
			{"executed verdict", func(s *alignment) { s.Predicate.Verdict = format.Executed(format.VerdictAligned) }, []format.ViolationCode{format.CodeWrongClass}},
			{"no judge model", func(s *alignment) { s.Predicate.Judge.Model = "" }, []format.ViolationCode{format.CodeMissingField}},
			{"bad prompt digest", func(s *alignment) { s.Predicate.Judge.PromptDigest = "x" }, []format.ViolationCode{format.CodeBadDigest}},
			{"no test", func(s *alignment) { s.Predicate.Test = "" }, []format.ViolationCode{format.CodeMissingField}},
			{"no module", func(s *alignment) { s.Predicate.Module = format.Module{} }, []format.ViolationCode{format.CodeMissingField}},
			{"no inputs", func(s *alignment) { s.Predicate.Inputs = nil }, []format.ViolationCode{format.CodeMissingField}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.Alignment(c.edit).Validate(), c.want...)
			})
		}
	})

	t.Run("discriminating power needs executed passes, judged validities and executed outcomes [verifies SP-16~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*power)
			want []format.ViolationCode
		}{
			{"valid", func(*power) {}, nil},
			{"no tests", func(s *power) { s.Predicate.Tests = nil }, []format.ViolationCode{format.CodeMissingField}},
			{"no generator model", func(s *power) { s.Predicate.Generator.Model = "" }, []format.ViolationCode{format.CodeMissingField}},
			{"a mutant without results", func(s *power) {
				s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)].Results = nil
			}, []format.ViolationCode{format.CodeMissingField}},
			{"a judged correctCodePasses", func(s *power) {
				s.Predicate.CorrectCodePasses = format.Claim[bool]{Value: false, Class: format.ClassJudged, Confidence: floatPointer(0.6)}
			}, []format.ViolationCode{format.CodeWrongClass}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				assertCodes(t, proofs.DiscriminatingPower(c.edit).Validate(), c.want...)
			})
		}
	})

	t.Run("a sentinel mutant must be EQUIVALENT [verifies SP-17~1]", func(t *testing.T) {
		for _, validity := range format.Validities() {
			t.Run(string(validity), func(t *testing.T) {
				statement := proofs.DiscriminatingPower(func(s *power) {
					index := slices.IndexFunc(s.Predicate.Mutants, func(m format.Mutant) bool { return m.Sentinel })
					claim, err := format.Judged(validity, 1)
					if err != nil {
						t.Fatal(err)
					}
					s.Predicate.Mutants[index].Validity = claim
				})
				if validity == format.ValidityEquivalent {
					assertCodes(t, statement.Validate())
					return
				}
				assertCodes(t, statement.Validate(), format.CodeSentinelNotEquivalent)
			})
		}
	})

	t.Run("example results are executed passed or failed [verifies SP-18~1]", func(t *testing.T) {
		failing := proofs.DiscriminatingPower(func(s *power) { s.Predicate.Examples[1].Result = format.Executed(format.ExampleFailed) })
		if err := failing.Validate(); err != nil {
			t.Fatalf("a failed example is valid evidence: %v", err)
		}
		judged := proofs.DiscriminatingPower(func(s *power) {
			s.Predicate.Examples[0].Result = format.Claim[format.ExampleResult]{Value: format.ExamplePassed, Class: format.ClassJudged, Confidence: floatPointer(0.5)}
		})
		assertCodes(t, judged.Validate(), format.CodeWrongClass)
		unknown := proofs.DiscriminatingPower(func(s *power) { s.Predicate.Examples[0].Result.Value = "skipped" })
		assertCodes(t, unknown.Validate(), format.CodeUnknownValue)
	})

	t.Run("a completeness predicate is advisory and carries no verdict [verifies SP-19~1]", func(t *testing.T) {
		notAdvisory := proofs.Completeness(func(s *spec) { s.Predicate.Advisory = false })
		assertCodes(t, notAdvisory.Validate(), format.CodeNotAdvisory)
		encoded := mustEncode(t, proofs.Completeness())
		var generic map[string]any
		if err := json.Unmarshal(encoded, &generic); err != nil {
			t.Fatal(err)
		}
		if _, hasVerdict := generic["predicate"].(map[string]any)["verdict"]; hasVerdict {
			t.Fatal("a completeness predicate must not have a verdict field")
		}
		missing := proofs.Completeness(func(s *spec) { s.Predicate.Questions[0].Question = "" })
		assertCodes(t, missing.Validate(), format.CodeMissingField)
	})
}

func TestExtensions(t *testing.T) {
	t.Run("decision and operator screen survive an encode and decode [verifies SP-20~1]", func(t *testing.T) {
		original := proofs.DiscriminatingPower(func(s *power) {
			s.Predicate.Extensions.Decision = &format.Decision{
				Basis:                format.BasisSystemOne,
				Deciders:             []string{"nimble:9b"},
				RawConfidence:        0.91,
				CalibratedConfidence: floatPointer(0.8),
				Threshold:            0.7,
				EscalationReason:     "below threshold",
			}
		})
		decoded, err := format.DecodeAs[format.DiscriminatingPower](mustEncode(t, original))
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(decoded.Predicate.Extensions, original.Predicate.Extensions) {
			t.Fatalf("extensions changed:\n%+v\n%+v", decoded.Predicate.Extensions, original.Predicate.Extensions)
		}
		if decoded.Predicate.Extensions.OperatorScreen.ScreenedOut == 0 {
			t.Fatal("operator screen was lost")
		}
	})

	t.Run("an invalid decision is reported [verifies SP-20~1]", func(t *testing.T) {
		cases := []struct {
			name string
			edit func(*format.Decision)
			want format.ViolationCode
		}{
			{"unknown basis", func(d *format.Decision) { d.Basis = "guess" }, format.CodeUnknownValue},
			{"raw confidence above one", func(d *format.Decision) { d.RawConfidence = 1.2 }, format.CodeConfidenceRange},
			{"calibrated confidence below zero", func(d *format.Decision) { d.CalibratedConfidence = floatPointer(-0.2) }, format.CodeConfidenceRange},
			{"threshold above one", func(d *format.Decision) { d.Threshold = 2 }, format.CodeConfidenceRange},
			{"no deciders", func(d *format.Decision) { d.Deciders = nil }, format.CodeMissingField},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				statement := proofs.Alignment(func(s *alignment) { c.edit(s.Predicate.Extensions.Decision) })
				assertCodes(t, statement.Validate(), c.want)
			})
		}
	})

	t.Run("an absent extensions member decodes to an empty one [verifies SP-20~1]", func(t *testing.T) {
		var generic map[string]any
		if err := json.Unmarshal(mustEncode(t, proofs.Alignment()), &generic); err != nil {
			t.Fatal(err)
		}
		delete(generic["predicate"].(map[string]any), "extensions")
		document, _ := json.Marshal(generic)
		decoded, err := format.DecodeAs[format.Alignment](document)
		if err != nil {
			t.Fatal(err)
		}
		if decoded.Predicate.Extensions.Decision != nil || decoded.Predicate.Extensions.OperatorScreen != nil {
			t.Fatalf("extensions = %+v", decoded.Predicate.Extensions)
		}
	})
}
