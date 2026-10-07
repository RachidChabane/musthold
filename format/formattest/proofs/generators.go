package proofs

import (
	"cmp"
	"slices"
	"strings"

	"pgregory.net/rapid"

	"github.com/RachidChabane/spec-proof/format"
)

// Bounds of the generated fixtures.
const (
	maxGeneratedInputs         = 6
	maxGeneratedTests          = 4
	maxGeneratedMutants        = 4
	maxGeneratedExamples       = 4
	maxGeneratedQuestions      = 4
	maxGeneratedCorroborations = 3
	maxGeneratedRevision       = 9
	digestSeedBytes            = 32
	identifierPattern          = `[a-z][a-z0-9_]{2,12}`
	requirementPattern         = `[A-Z]{2,4}-[1-9][0-9]{0,2}`
	pathPattern                = `[a-z]{1,6}(/[a-z]{1,6}){0,2}`
)

// Digests generates SHA-256 hex digests.
func Digests() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		return format.SHA256Hex(rapid.SliceOfN(rapid.Byte(), digestSeedBytes, digestSeedBytes).Draw(t, "seed"))
	})
}

// Inputs generates a sorted, duplicate-free input list whose descriptors have the
// shape their role requires.
func Inputs() *rapid.Generator[[]format.Input] {
	input := rapid.Custom(func(t *rapid.T) format.Input {
		role := rapid.SampledFrom(format.InputRoles()).Draw(t, "role")
		name := rapid.StringMatching(pathPattern).Draw(t, "name")
		digest := Digests().Draw(t, "digest")
		if role == format.RoleProof {
			return format.Input{
				Role: role, Name: string(rapid.SampledFrom(format.Kinds()).Draw(t, "kind")), Digest: format.Digest{SHA256: digest},
				Annotations: &format.Annotations{Criterion: rapid.StringMatching(requirementPattern).Draw(t, "criterionID") + format.RevisionSeparator + "1"},
			}
		}
		return format.Input{Role: role, Name: name, Digest: format.Digest{SHA256: digest}}
	})
	return rapid.Custom(func(t *rapid.T) []format.Input {
		drawn := rapid.SliceOfN(input, 1, maxGeneratedInputs).Draw(t, "inputs")
		sorted := SortInputs(drawn)
		return slices.CompactFunc(sorted, func(a, b format.Input) bool { return format.CompareInputs(a, b) == 0 })
	})
}

// InputsAbout generates an input list for a proof about one criterion: its
// `spec` input is the given one, and no other.
func InputsAbout(spec format.Input) *rapid.Generator[[]format.Input] {
	return rapid.Custom(func(t *rapid.T) []format.Input {
		drawn := Inputs().Draw(t, "inputs")
		kept := slices.DeleteFunc(slices.Clone(drawn), func(input format.Input) bool { return input.Role == format.RoleSpec })
		return SortInputs(append(kept, spec))
	})
}

// Confidences generates confidences inside [0,1].
func Confidences() *rapid.Generator[float64] {
	return rapid.Float64Range(format.MinConfidence, format.MaxConfidence)
}

// Corroborations generates a non-empty, strictly ascending corroboration list.
func Corroborations() *rapid.Generator[[]format.Corroboration] {
	corroboration := rapid.Custom(func(t *rapid.T) format.Corroboration {
		return format.Corroboration{
			Kind:   rapid.SampledFrom(format.CorroborationKinds()).Draw(t, "kind"),
			Digest: Digests().Draw(t, "evidence"),
		}
	})
	return rapid.Custom(func(t *rapid.T) []format.Corroboration {
		drawn := rapid.SliceOfN(corroboration, 1, maxGeneratedCorroborations).Draw(t, "corroborations")
		return sortedUnique(drawn, func(a, b format.Corroboration) int {
			return cmp.Or(strings.Compare(string(a.Kind), string(b.Kind)), strings.Compare(a.Digest, b.Digest))
		})
	})
}

// Executors generates valid executors of every channel and isolation level.
func Executors() *rapid.Generator[format.Executor] {
	return rapid.Custom(func(t *rapid.T) format.Executor {
		isolation := rapid.SampledFrom(format.Isolations()).Draw(t, "isolation")
		source := rapid.SampledFrom(format.ModelSources()).Draw(t, "modelSource")
		executor := Executor(rapid.StringMatching(identifierPattern).Draw(t, "model"), Digests().Draw(t, "promptDigest"))
		executor.Channel = rapid.SampledFrom(format.Channels()).Draw(t, "channel")
		executor.Isolation = isolation
		executor.Model.Source = source
		executor.Blind = rapid.Bool().Draw(t, "blind")
		if isolation != format.IsolationRestrictedAttested {
			executor.Attestation = format.Attestation{}
		}
		return executor
	})
}

func sortedUnique[T any](items []T, compare func(a, b T) int) []T {
	sorted := slices.Clone(items)
	slices.SortFunc(sorted, compare)
	return slices.CompactFunc(sorted, func(a, b T) bool { return compare(a, b) == 0 })
}

func sortedStrings(items []string) []string {
	return sortedUnique(items, strings.Compare)
}

func judgedClaim[V comparable](t *rapid.T, label string, values []V) format.Claim[V] {
	return mustJudged(rapid.SampledFrom(values).Draw(t, label), Confidences().Draw(t, label+"Confidence"))
}

// Alignments generates valid alignment statements.
func Alignments() *rapid.Generator[format.Statement[format.Alignment]] {
	return rapid.Custom(func(t *rapid.T) format.Statement[format.Alignment] {
		requirement := format.Requirement{
			ID:                        rapid.StringMatching(requirementPattern).Draw(t, "requirementID"),
			Revision:                  rapid.IntRange(format.MinimumRevision, maxGeneratedRevision).Draw(t, "revision"),
			StatementCanonicalization: format.StatementCanonicalizationC1,
		}
		statementDigest := Digests().Draw(t, "statementDigest")
		return Alignment(func(s *format.Statement[format.Alignment]) {
			s.Subject = []format.Subject{format.RequirementSubject(requirement.ID, requirement.Revision, statementDigest)}
			s.Predicate.Requirement = requirement
			s.Predicate.Inputs = InputsAbout(format.SpecInput(requirement.ID, requirement.Revision, statementDigest)).Draw(t, "inputs")
			s.Predicate.ProducedBy = rapid.SampledFrom([]format.ProducedBy{format.ProducedByLocal, format.ProducedByCI}).Draw(t, "producedBy")
			s.Predicate.Test = rapid.StringMatching(identifierPattern).Draw(t, "test")
			s.Predicate.Verdict = judgedClaim(t, "verdict", format.Verdicts())
			s.Predicate.Executor = Executors().Draw(t, "executor")
			s.Predicate.Executor.Blind = true
		})
	})
}

// DiscriminatingPowers generates valid discriminating-power statements.
func DiscriminatingPowers() *rapid.Generator[format.Statement[format.DiscriminatingPower]] {
	return rapid.Custom(func(t *rapid.T) format.Statement[format.DiscriminatingPower] {
		tests := sortedStrings(rapid.SliceOfN(rapid.StringMatching(identifierPattern), 1, maxGeneratedTests).Draw(t, "tests"))
		statementDigest := Digests().Draw(t, "statementDigest")
		return DiscriminatingPower(func(s *format.Statement[format.DiscriminatingPower]) {
			s.Subject = []format.Subject{format.RequirementSubject(RequirementID, RequirementRevision, statementDigest)}
			s.Predicate.Inputs = InputsAbout(format.SpecInput(RequirementID, RequirementRevision, statementDigest)).Draw(t, "inputs")
			s.Predicate.Tests = tests
			s.Predicate.Executor = Executors().Draw(t, "executor")
			s.Predicate.CorrectCodePasses = format.Executed(rapid.Bool().Draw(t, "correctCodePasses"))
			s.Predicate.Mutants = generatedMutants(t, tests)
			s.Predicate.Examples = generatedExamples(t)
		})
	})
}

func generatedMutants(t *rapid.T, tests []string) []format.Mutant {
	mutant := rapid.Custom(func(t *rapid.T) format.Mutant {
		results := make([]format.TestResult, len(tests))
		for index, test := range tests {
			results[index] = format.TestResult{Test: test, Outcome: format.Executed(rapid.SampledFrom(format.Outcomes()).Draw(t, "outcome"))}
		}
		sentinel := rapid.Bool().Draw(t, "sentinel")
		validity := judgedClaim(t, "validity", format.Validities())
		if sentinel {
			validity = mustJudged(format.ValidityEquivalent, Confidences().Draw(t, "sentinelConfidence"))
		}
		if validity.Value.CarriesPassEffect() {
			validity = validity.Corroborated(Corroborations().Draw(t, "corroboration")...)
		}
		return format.Mutant{Digest: Digests().Draw(t, "mutantDigest"), Validity: validity, Sentinel: sentinel, Results: results}
	})
	drawn := rapid.SliceOfN(mutant, 0, maxGeneratedMutants).Draw(t, "mutants")
	return sortedUnique(drawn, func(a, b format.Mutant) int { return strings.Compare(a.Digest, b.Digest) })
}

func generatedExamples(t *rapid.T) []format.ExampleRun {
	count := rapid.IntRange(0, maxGeneratedExamples).Draw(t, "exampleCount")
	examples := make([]format.ExampleRun, count)
	for index := range examples {
		examples[index] = format.ExampleRun{
			Index:  index,
			Result: format.Executed(rapid.SampledFrom([]format.ExampleResult{format.ExamplePassed, format.ExampleFailed}).Draw(t, "exampleResult")),
		}
	}
	return examples
}

// Completenesses generates valid completeness statements.
func Completenesses() *rapid.Generator[format.Statement[format.Completeness]] {
	question := rapid.Custom(func(t *rapid.T) format.Question {
		return format.Question{
			Dimension:      rapid.StringMatching(identifierPattern).Draw(t, "dimension"),
			Question:       rapid.StringMatching(identifierPattern).Draw(t, "text"),
			RequirementIDs: sortedStrings(rapid.SliceOfN(rapid.StringMatching(requirementPattern), 0, maxGeneratedTests).Draw(t, "requirementIds")),
		}
	})
	return rapid.Custom(func(t *rapid.T) format.Statement[format.Completeness] {
		return Completeness(func(s *format.Statement[format.Completeness]) {
			path := rapid.StringMatching(pathPattern).Draw(t, "specPath")
			s.Subject = []format.Subject{format.SpecSubject(path, Digests().Draw(t, "fileDigest"))}
			s.Predicate.Inputs = Inputs().Draw(t, "inputs")
			s.Predicate.Questions = sortedUnique(rapid.SliceOfN(question, 0, maxGeneratedQuestions).Draw(t, "questions"), func(a, b format.Question) int {
				return cmp.Or(strings.Compare(a.Dimension, b.Dimension), strings.Compare(a.Question, b.Question))
			})
		})
	})
}
