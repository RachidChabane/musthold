package golden

import (
	"github.com/RachidChabane/musthold/format"
	"github.com/RachidChabane/musthold/format/formattest/proofs"
)

const (
	unknownChannel   = "carrier-pigeon"
	unknownIsolation = "sandboxed"
	unknownEvidence  = "gut_feeling"
	offTargetCrit    = "PARK-9"
)

func someCorroboration() format.Corroboration {
	return format.Corroboration{Kind: format.CorroborationSpecificKill, Digest: proofs.Digest("evidence/specific-kill")}
}

func otherCorroboration() format.Corroboration {
	return format.Corroboration{Kind: format.CorroborationWitness, Digest: proofs.Digest("evidence/witness")}
}

// executorCases returns the vectors about the executor of a predicate and of a
// claim.
func executorCases() []Case {
	type alignment = format.Statement[format.Alignment]
	type power = format.Statement[format.DiscriminatingPower]
	build := func(name string, expectation Expectation, mutate func(*alignment)) Case {
		return Case{Name: name, Document: encode(proofs.Alignment(mutate)), Expectation: expectation}
	}
	buildPower := func(name string, expectation Expectation, mutate func(*power)) Case {
		return Case{Name: name, Document: encode(proofs.DiscriminatingPower(mutate)), Expectation: expectation}
	}
	return []Case{
		build("alignment-executor-model-unknown", valid(), func(s *alignment) {
			s.Predicate.Executor.Model = format.ExecutorModel{Source: format.ModelSourceUnknown}
		}),
		build("alignment-executor-unattested", valid(), func(s *alignment) {
			s.Predicate.Executor.Isolation = format.IsolationRestricted
			s.Predicate.Executor.Attestation = format.Attestation{}
		}),
		build("alignment-executor-inline", valid(), func(s *alignment) {
			s.Predicate.Executor.Channel = format.ChannelHostInline
			s.Predicate.Executor.Isolation = format.IsolationInline
			s.Predicate.Executor.Attestation = format.Attestation{}
		}),
		build("alignment-claim-executor-and-corroboration", valid(), func(s *alignment) {
			s.Predicate.Verdict = s.Predicate.Verdict.
				Corroborated(format.Corroboration{Kind: format.CorroborationSpecificMatrix, Digest: proofs.Digest("evidence/matrix")}).
				ProducedByExecutor(s.Predicate.Executor)
		}),
		build("alignment-executor-unknown-channel", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *alignment) {
			s.Predicate.Executor.Channel = unknownChannel
		}),
		build("alignment-executor-unknown-isolation", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *alignment) {
			s.Predicate.Executor.Isolation = unknownIsolation
		}),
		build("alignment-executor-unknown-model-source", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *alignment) {
			s.Predicate.Executor.Model.Source = "guessed"
		}),
		build("alignment-executor-model-unnamed", breaks(SchemaInvalid, format.CodeMissingField), func(s *alignment) {
			s.Predicate.Executor.Model.Name = ""
		}),
		build("alignment-executor-attested-without-attestation", breaks(SchemaInvalid, format.CodeMissingField, format.CodeBadDigest), func(s *alignment) {
			s.Predicate.Executor.Attestation = format.Attestation{}
		}),
		build("alignment-executor-bad-cap-digest", breaks(SchemaInvalid, format.CodeBadDigest), func(s *alignment) {
			s.Predicate.Executor.Isolation = format.IsolationRestricted
			s.Predicate.Executor.Attestation.CapDigest = "short"
		}),
		build("alignment-executor-bad-prompt-digest", breaks(SchemaInvalid, format.CodeBadDigest), func(s *alignment) {
			s.Predicate.Executor.PromptDigest = "short"
		}),
		build("alignment-claim-executor-invalid", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *alignment) {
			broken := s.Predicate.Executor
			broken.Channel = unknownChannel
			s.Predicate.Verdict = s.Predicate.Verdict.ProducedByExecutor(broken)
		}),
		buildPower("discriminating-power-executed-with-corroboration", breaks(SchemaInvalid, format.CodeExecutedWithCorroboration), func(s *power) {
			s.Predicate.CorrectCodePasses = format.Executed(true).Corroborated(someCorroboration())
		}),
		buildPower("discriminating-power-executed-with-executor", breaks(SchemaInvalid, format.CodeExecutedWithExecutor), func(s *power) {
			s.Predicate.CorrectCodePasses = format.Executed(true).ProducedByExecutor(s.Predicate.Executor)
		}),
	}
}

// corroborationCases returns the vectors about the corroboration invariant and
// the attribution of mutants.
func corroborationCases() []Case {
	type power = format.Statement[format.DiscriminatingPower]
	build := func(name string, expectation Expectation, mutate func(*power)) Case {
		return Case{Name: name, Document: encode(proofs.DiscriminatingPower(mutate)), Expectation: expectation}
	}
	regular := func(s *power) *format.Mutant {
		return &s.Predicate.Mutants[proofs.RegularMutantIndex(s.Predicate.Mutants)]
	}
	notViolating := func(s *power, extensions format.MutantExtensions) {
		mutant := regular(s)
		mutant.Validity = judgedClaim(format.ValidityNotViolating)
		mutant.Extensions = &extensions
	}
	return []Case{
		build("discriminating-power-valid-mutant-corroborated-twice", valid(), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid).Corroborated(otherCorroboration(), someCorroboration())
		}),
		build("discriminating-power-unattributed-mutant", valid(), func(s *power) {
			notViolating(s, format.MutantExtensions{Unattributed: true})
		}),
		build("discriminating-power-off-target-mutant", valid(), func(s *power) {
			notViolating(s, format.MutantExtensions{OffTarget: offTargetCrit})
		}),
		build("discriminating-power-excluded-mutant-without-corroboration", valid(), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityEquivalent)
		}),
		build("discriminating-power-valid-mutant-uncorroborated", breaks(SchemaInvalid, format.CodeUncorroboratedClaim), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid)
		}),
		build("discriminating-power-corroboration-unknown-kind", breaks(SchemaInvalid, format.CodeUnknownValue), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid).Corroborated(format.Corroboration{Kind: unknownEvidence, Digest: proofs.Digest("evidence/unknown")})
		}),
		build("discriminating-power-corroboration-bad-digest", breaks(SchemaInvalid, format.CodeBadDigest), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid).Corroborated(format.Corroboration{Kind: format.CorroborationBuilds, Digest: "short"})
		}),
		build("discriminating-power-corroboration-unsorted", breaks(SchemaValid, format.CodeUnsorted), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid).Corroborated(someCorroboration(), otherCorroboration())
		}),
		build("discriminating-power-valid-mutant-unattributed", breaks(SchemaInvalid, format.CodeInconsistentAttribution), func(s *power) {
			regular(s).Validity = judgedClaim(format.ValidityValid).Corroborated(someCorroboration())
			regular(s).Extensions = &format.MutantExtensions{Unattributed: true}
		}),
		build("discriminating-power-unattributed-and-off-target", breaks(SchemaInvalid, format.CodeInconsistentAttribution), func(s *power) {
			notViolating(s, format.MutantExtensions{Unattributed: true, OffTarget: offTargetCrit})
		}),
	}
}
