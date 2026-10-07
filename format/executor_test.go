package format_test

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
	"github.com/RachidChabane/spec-proof/format/formattest/proofs"
)

type (
	alignmentStatement = format.Statement[format.Alignment]
	powerStatement     = format.Statement[format.DiscriminatingPower]
)

func TestExecutorRules(t *testing.T) {
	cases := []struct {
		name string
		edit func(*alignmentStatement)
		want []format.ViolationCode
	}{
		{"an attested executor", func(*alignmentStatement) {}, nil},
		{"an unattested restricted executor", func(s *alignmentStatement) {
			s.Predicate.Executor.Isolation = format.IsolationRestricted
			s.Predicate.Executor.Attestation = format.Attestation{}
		}, nil},
		{"an unknown model needs no name", func(s *alignmentStatement) {
			s.Predicate.Executor.Model = format.ExecutorModel{Source: format.ModelSourceUnknown}
		}, nil},
		{"a declared model needs a name", func(s *alignmentStatement) {
			s.Predicate.Executor.Model = format.ExecutorModel{Source: format.ModelSourceDeclared}
		}, []format.ViolationCode{format.CodeMissingField}},
		{"an unknown channel", func(s *alignmentStatement) { s.Predicate.Executor.Channel = "carrier-pigeon" }, []format.ViolationCode{format.CodeUnknownValue}},
		{"an unknown isolation", func(s *alignmentStatement) { s.Predicate.Executor.Isolation = "sandboxed" }, []format.ViolationCode{format.CodeUnknownValue}},
		{"an unknown model source", func(s *alignmentStatement) { s.Predicate.Executor.Model.Source = "guessed" }, []format.ViolationCode{format.CodeUnknownValue}},
		{"an unnamed harness", func(s *alignmentStatement) { s.Predicate.Executor.Harness.Name = "" }, []format.ViolationCode{format.CodeMissingField}},
		{"a bad prompt digest", func(s *alignmentStatement) { s.Predicate.Executor.PromptDigest = "x" }, []format.ViolationCode{format.CodeBadDigest}},
		{"an attested executor without an agent", func(s *alignmentStatement) { s.Predicate.Executor.Attestation.AgentID = "" }, []format.ViolationCode{format.CodeMissingField}},
		{"an attested executor without a cap digest", func(s *alignmentStatement) { s.Predicate.Executor.Attestation.CapDigest = "" }, []format.ViolationCode{format.CodeBadDigest}},
		{"an attested executor without a transcript digest", func(s *alignmentStatement) { s.Predicate.Executor.Attestation.TranscriptDigest = "" }, []format.ViolationCode{format.CodeBadDigest}},
		{"a malformed cap digest on an unattested executor", func(s *alignmentStatement) {
			s.Predicate.Executor.Isolation = format.IsolationUnrestricted
			s.Predicate.Executor.Attestation.CapDigest = "x"
		}, []format.ViolationCode{format.CodeBadDigest}},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-111~1]", func(t *testing.T) {
			assertCodes(t, proofs.Alignment(c.edit).Validate(), c.want...)
		})
	}

	t.Run("a discriminating-power executor is checked like an alignment one [verifies SP-111~1]", func(t *testing.T) {
		broken := proofs.DiscriminatingPower(func(s *powerStatement) { s.Predicate.Executor.Channel = "" })
		assertCodes(t, broken.Validate(), format.CodeUnknownValue)
	})

	t.Run("a claim executor is checked and an executed claim forbids one [verifies SP-111~1]", func(t *testing.T) {
		withExecutor := proofs.Alignment(func(s *alignmentStatement) {
			s.Predicate.Verdict = s.Predicate.Verdict.ProducedByExecutor(s.Predicate.Executor)
		})
		assertCodes(t, withExecutor.Validate())
		broken := proofs.Alignment(func(s *alignmentStatement) {
			executor := s.Predicate.Executor
			executor.Isolation = "sandboxed"
			s.Predicate.Verdict = s.Predicate.Verdict.ProducedByExecutor(executor)
		})
		assertCodes(t, broken.Validate(), format.CodeUnknownValue)
		executed := proofs.DiscriminatingPower(func(s *powerStatement) {
			s.Predicate.CorrectCodePasses = format.Executed(true).ProducedByExecutor(s.Predicate.Executor)
		})
		assertCodes(t, executed.Validate(), format.CodeExecutedWithExecutor)
	})

	t.Run("every generated executor is valid [verifies SP-111~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			executor := proofs.Executors().Draw(t, "executor")
			if err := executor.ExecutorProfile.Validate(); err != nil {
				t.Fatalf("generated executor is invalid: %v", err)
			}
		})
	})

	t.Run("the executor round-trips through JSON [verifies SP-111~1]", func(t *testing.T) {
		executor := proofs.Executor("claude-sonnet-5-5", proofs.Digest("prompt"))
		encoded, err := format.Canonicalize(executor)
		if err != nil {
			t.Fatal(err)
		}
		var decoded format.Executor
		if err := json.Unmarshal(encoded, &decoded); err != nil {
			t.Fatal(err)
		}
		if decoded != executor {
			t.Fatalf("decoded %+v, want %+v", decoded, executor)
		}
	})
}

func TestIsolationOrder(t *testing.T) {
	t.Run("the levels are listed from the strongest to the weakest [verifies SP-116~1]", func(t *testing.T) {
		levels := format.Isolations()
		for index := 1; index < len(levels); index++ {
			if levels[index-1].Strength() <= levels[index].Strength() {
				t.Errorf("%s is not stronger than %s", levels[index-1], levels[index])
			}
		}
		if levels[0] != format.IsolationRestrictedAttested || levels[len(levels)-1] != format.IsolationInline {
			t.Errorf("levels = %v", levels)
		}
	})

	t.Run("a level is at least every floor that is not stronger [verifies SP-116~1]", func(t *testing.T) {
		for _, level := range format.Isolations() {
			for _, floor := range format.Isolations() {
				if got, want := level.AtLeast(floor), level.Strength() >= floor.Strength(); got != want {
					t.Errorf("%s.AtLeast(%s) = %v, want %v", level, floor, got, want)
				}
			}
		}
	})

	t.Run("an unknown level is below every floor [verifies SP-116~1]", func(t *testing.T) {
		const unknown = format.Isolation("sandboxed")
		for _, floor := range format.Isolations() {
			if unknown.AtLeast(floor) {
				t.Errorf("an unknown level reached the floor %s", floor)
			}
		}
		if unknown.Strength() != 0 {
			t.Errorf("strength = %d", unknown.Strength())
		}
	})
}

func TestCorroborationInvariant(t *testing.T) {
	regular := func(s *powerStatement) *format.Mutant {
		return &s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)]
	}
	judgedValidity := func(value format.Validity) format.Claim[format.Validity] {
		claim, err := format.Judged(value, 1)
		if err != nil {
			t.Fatal(err)
		}
		return claim
	}

	t.Run("a judged VALID mutant without corroboration is malformed [verifies SP-112~1]", func(t *testing.T) {
		statement := proofs.DiscriminatingPower(func(s *powerStatement) { regular(s).Validity = judgedValidity(format.ValidityValid) })
		assertCodes(t, statement.Validate(), format.CodeUncorroboratedClaim)
	})

	t.Run("a judged VALID mutant with corroboration is well formed [verifies SP-112~1]", func(t *testing.T) {
		statement := proofs.DiscriminatingPower(func(s *powerStatement) {
			regular(s).Validity = judgedValidity(format.ValidityValid).Corroborated(corroborationFixture())
		})
		assertCodes(t, statement.Validate())
	})

	t.Run("a judgment without the power to block or credit needs no corroboration [verifies SP-112~1]", func(t *testing.T) {
		for _, validity := range []format.Validity{format.ValidityEquivalent, format.ValidityNotViolating, format.ValidityUndetermined} {
			statement := proofs.DiscriminatingPower(func(s *powerStatement) { regular(s).Validity = judgedValidity(validity) })
			assertCodes(t, statement.Validate())
		}
		for _, validity := range format.Validities() {
			if got, want := validity.CarriesPassEffect(), validity == format.ValidityValid; got != want {
				t.Errorf("%s.CarriesPassEffect() = %v", validity, got)
			}
		}
	})

	t.Run("an alignment verdict may name its corroboration [verifies SP-112~1]", func(t *testing.T) {
		statement := proofs.Alignment(func(s *alignmentStatement) {
			s.Predicate.Verdict = s.Predicate.Verdict.Corroborated(format.Corroboration{Kind: format.CorroborationSpecificMatrix, Digest: proofs.Digest("matrix")})
		})
		assertCodes(t, statement.Validate())
	})

	t.Run("corroboration entries are known, digested and strictly ascending [verifies SP-112~1]", func(t *testing.T) {
		digest := proofs.Digest("evidence")
		low, high := proofs.Digest("a"), proofs.Digest("b")
		if low > high {
			low, high = high, low
		}
		cases := []struct {
			name string
			by   []format.Corroboration
			want []format.ViolationCode
		}{
			{"ascending", []format.Corroboration{{Kind: format.CorroborationBuilds, Digest: digest}, {Kind: format.CorroborationWitness, Digest: digest}}, nil},
			{"same kind, ascending digests", []format.Corroboration{{Kind: format.CorroborationBuilds, Digest: low}, {Kind: format.CorroborationBuilds, Digest: high}}, nil},
			{"descending", []format.Corroboration{{Kind: format.CorroborationWitness, Digest: digest}, {Kind: format.CorroborationBuilds, Digest: digest}}, []format.ViolationCode{format.CodeUnsorted}},
			{"duplicated", []format.Corroboration{{Kind: format.CorroborationBuilds, Digest: digest}, {Kind: format.CorroborationBuilds, Digest: digest}}, []format.ViolationCode{format.CodeUnsorted}},
			{"unknown kind", []format.Corroboration{{Kind: "gut_feeling", Digest: digest}}, []format.ViolationCode{format.CodeUnknownValue}},
			{"a judgment is not executed evidence", []format.Corroboration{{Kind: "judge_quorum", Digest: digest}}, []format.ViolationCode{format.CodeUnknownValue}},
			{"bad digest", []format.Corroboration{{Kind: format.CorroborationBuilds, Digest: "x"}}, []format.ViolationCode{format.CodeBadDigest}},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				statement := proofs.DiscriminatingPower(func(s *powerStatement) {
					regular(s).Validity = judgedValidity(format.ValidityValid).Corroborated(c.by...)
				})
				assertCodes(t, statement.Validate(), c.want...)
			})
		}
	})

	t.Run("an executed claim forbids corroboration [verifies SP-112~1]", func(t *testing.T) {
		statement := proofs.DiscriminatingPower(func(s *powerStatement) {
			s.Predicate.CorrectCodePasses = format.Executed(true).Corroborated(corroborationFixture())
		})
		assertCodes(t, statement.Validate(), format.CodeExecutedWithCorroboration)
	})

	t.Run("a judged VALID mutant never validates without corroboration [verifies SP-112~1]", func(t *testing.T) {
		rapid.Check(t, func(t *rapid.T) {
			statement := proofs.DiscriminatingPowers().Draw(t, "statement")
			if err := statement.Validate(); err != nil {
				t.Fatalf("generated statement is invalid: %v", err)
			}
			stripped := false
			for index := range statement.Predicate.Mutants {
				if mutant := &statement.Predicate.Mutants[index]; mutant.Validity.Value.CarriesPassEffect() {
					mutant.Validity.CorroboratedBy = nil
					stripped = true
				}
			}
			if !stripped {
				t.Skip("no VALID mutant drawn")
			}
			var validation *format.ValidationError
			if !errors.As(statement.Validate(), &validation) || !slices.Contains(validation.Codes(), format.CodeUncorroboratedClaim) {
				t.Fatal("a VALID mutant without corroboration was accepted")
			}
		})
	})

	t.Run("the corroboration kinds are a closed set [verifies SP-112~1]", func(t *testing.T) {
		kinds := format.CorroborationKinds()
		if len(kinds) == 0 || len(slices.Compact(slices.Sorted(slices.Values(kinds)))) != len(kinds) {
			t.Fatalf("kinds = %v", kinds)
		}
	})
}

func TestMutantAttribution(t *testing.T) {
	regular := func(s *powerStatement) *format.Mutant {
		return &s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)]
	}
	judged := func(value format.Validity) format.Claim[format.Validity] {
		claim, err := format.Judged(value, 1)
		if err != nil {
			t.Fatal(err)
		}
		return claim
	}
	cases := []struct {
		name       string
		validity   format.Validity
		extensions format.MutantExtensions
		want       []format.ViolationCode
	}{
		{"an unattributed NOT_VIOLATING mutant", format.ValidityNotViolating, format.MutantExtensions{Unattributed: true}, nil},
		{"an off-target NOT_VIOLATING mutant", format.ValidityNotViolating, format.MutantExtensions{OffTarget: "PARK-9"}, nil},
		{"an unattributed EQUIVALENT mutant", format.ValidityEquivalent, format.MutantExtensions{Unattributed: true}, []format.ViolationCode{format.CodeInconsistentAttribution}},
		{"an off-target UNDETERMINED mutant", format.ValidityUndetermined, format.MutantExtensions{OffTarget: "PARK-9"}, []format.ViolationCode{format.CodeInconsistentAttribution}},
		{"a mutant that is both", format.ValidityNotViolating, format.MutantExtensions{Unattributed: true, OffTarget: "PARK-9"}, []format.ViolationCode{format.CodeInconsistentAttribution}},
	}
	for _, c := range cases {
		t.Run(c.name+" [verifies SP-113~1]", func(t *testing.T) {
			statement := proofs.DiscriminatingPower(func(s *powerStatement) {
				regular(s).Validity = judged(c.validity)
				extensions := c.extensions
				regular(s).Extensions = &extensions
			})
			assertCodes(t, statement.Validate(), c.want...)
		})
	}

	t.Run("the unattributed flag survives decoding and encoding [verifies SP-113~1]", func(t *testing.T) {
		statement := proofs.DiscriminatingPower(func(s *powerStatement) {
			regular(s).Validity = judged(format.ValidityNotViolating)
			regular(s).Extensions = &format.MutantExtensions{Unattributed: true}
		})
		encoded := mustEncode(t, statement)
		decoded, err := format.DecodeAs[format.DiscriminatingPower](encoded)
		if err != nil {
			t.Fatal(err)
		}
		if !decoded.Predicate.Mutants[proofs.RegularMutantIndex(decoded.Predicate.Mutants)].Extensions.Unattributed {
			t.Fatal("the unattributed flag was lost")
		}
	})
}

func TestAdditiveRolesAndMinorVersions(t *testing.T) {
	t.Run("a host response and a proof are input roles [verifies SP-114~1]", func(t *testing.T) {
		for _, role := range []format.InputRole{format.RoleHostResponse, format.RoleProof} {
			statement := proofs.Alignment(func(s *alignmentStatement) {
				s.Predicate.Inputs = proofs.SortInputs(append(s.Predicate.Inputs, proofs.Input(role, "extra")))
			})
			assertCodes(t, statement.Validate())
		}
	})

	t.Run("each predicate schema states the minor version of its kind [verifies SP-115~1]", func(t *testing.T) {
		for _, kind := range format.Kinds() {
			schema, err := format.PredicateSchema(kind.PredicateType())
			if err != nil {
				t.Fatal(err)
			}
			var document struct {
				Minor int `json:"x-minorVersion"`
			}
			if err := json.Unmarshal(schema.Bytes, &document); err != nil {
				t.Fatal(err)
			}
			if document.Minor != kind.MinorVersion() || document.Minor < 1 {
				t.Errorf("%s: schema minor %d, kind minor %d", kind, document.Minor, kind.MinorVersion())
			}
		}
		if format.Kind("unknown").MinorVersion() != 0 {
			t.Error("an unknown kind has no minor version")
		}
	})
}
