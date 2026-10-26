// Package proofs builds valid statements of the three predicates for tests. Every
// builder returns a statement that passes format validation; a mutation callback
// edits it before the cache key is recomputed, so a test breaks exactly one rule.
package proofs

import (
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/format"
)

// Names and texts of the fixture criterion.
const (
	RequirementID       = "PARK-2"
	RequirementRevision = 1
	SpecPath            = "specs/parkfee.md"
	TestName            = "test_grace_period_is_free"
	moduleVersion       = "0.1.0"
	moduleAlignment     = "musthold-alignment"
	moduleDiscriminate  = "musthold-discriminating-power"
	moduleCompleteness  = "musthold-completeness"
	judgeModel          = "claude-sonnet-5-5"
	generatorModel      = "claude-opus-5-5"
	harnessName         = "claude-code"
	harnessVersion      = "2.1.292"
	fixtureAgentID      = "agent-0001"
	statementText       = "WHEN a stay lasts 15 minutes or less THE SYSTEM SHALL charge 0.00."
	specText            = "# parkfee\n\n- PARK-2~1: " + statementText + "\n"
	defaultConfidence   = 0.9
	defaultThreshold    = 0.7
	judgeVotes          = 2
)

// Digest returns the SHA-256 hex digest of a label, a stable stand-in for any
// content digest of a fixture.
func Digest(label string) string { return format.SHA256Hex([]byte(label)) }

// StatementDigest is the digest of the fixture criterion's statement.
func StatementDigest() string { return Digest(statementText) }

// Input builds one input descriptor whose digest is derived from its role and name.
func Input(role format.InputRole, name string) format.Input {
	return format.Input{Role: role, Name: name, Digest: format.Digest{SHA256: Digest(string(role) + "/" + name)}}
}

// SortInputs sorts inputs in the order of the format, keeping repeats.
func SortInputs(inputs []format.Input) []format.Input {
	sorted := slices.Clone(inputs)
	slices.SortFunc(sorted, format.CompareInputs)
	return sorted
}

// Executor builds a valid attested executor that ran the given model and
// answered the prompt with the given digest.
func Executor(model, promptDigest string) format.Executor {
	return format.Executor{
		ExecutorProfile: format.ExecutorProfile{
			Channel:   format.ChannelHostSubagent,
			Harness:   format.Harness{Name: harnessName, Version: harnessVersion},
			Isolation: format.IsolationRestrictedAttested,
			Attestation: format.Attestation{
				AgentID:          fixtureAgentID,
				CapDigest:        Digest("cap/" + fixtureAgentID),
				TranscriptDigest: Digest("transcript/" + fixtureAgentID),
			},
			Model: format.ExecutorModel{Name: model, Source: format.ModelSourceResolved},
		},
		PromptDigest: promptDigest,
		Blind:        true,
	}
}

// TestFile and CodeFile are the repository-relative paths of the fixture's test
// and code files.
const (
	TestFile = "tests/test_parkfee.py"
	CodeFile = "src/parkfee.py"
)

// FileInput builds a per-file descriptor whose digest is derived from its path.
func FileInput(role format.InputRole, path string) format.Input {
	return format.FileInput(role, path, Digest("file/"+path))
}

// SpecInput is the `spec` descriptor of the fixture criterion.
func SpecInput() format.Input {
	return format.SpecInput(RequirementID, RequirementRevision, StatementDigest())
}

// ProofInput is the `proof` descriptor of the discriminating-power Statement the
// fixture alignment proof consumes.
func ProofInput() format.Input {
	return format.ProofInput(format.KindDiscriminatingPower, format.SubjectName(RequirementID, RequirementRevision), []byte("discriminating-power statement"))
}

func alignmentInputs() []format.Input {
	return SortInputs([]format.Input{
		SpecInput(),
		FileInput(format.RoleTestSource, TestFile),
		Input(format.RoleModelID, judgeModel),
		Input(format.RolePromptTemplate, "alignment-judge"),
		Input(format.RoleDecisionPolicy, "decision-policy"),
		ProofInput(),
	})
}

func discriminatingPowerInputs() []format.Input {
	return SortInputs([]format.Input{
		SpecInput(),
		FileInput(format.RoleTestSource, TestFile),
		FileInput(format.RoleCodeTree, CodeFile),
		Input(format.RoleJUnitResult, "junit-outcomes"),
		Input(format.RoleMutantSet, "mutant-set"),
		Input(format.RoleModelID, generatorModel),
		Input(format.RolePromptTemplate, "mutant-generator"),
		Input(format.RoleDecisionPolicy, "decision-policy"),
		Input(format.RoleExampleBinding, "example-binding"),
		Input(format.RoleHostResponse, "host-responses"),
	})
}

func completenessInputs() []format.Input {
	return SortInputs([]format.Input{
		Input(format.RoleSpec, SpecPath),
		Input(format.RoleDecisionPolicy, "decision-policy"),
	})
}

func mustJudged[V comparable](value V, confidence float64) format.Claim[V] {
	claim, err := format.Judged(value, confidence)
	if err != nil {
		panic(err)
	}
	return claim
}

func core(name string, inputs []format.Input) format.Core {
	return format.Core{
		Module:     format.Module{Name: name, Version: moduleVersion},
		Inputs:     inputs,
		ProducedBy: format.ProducedByCI,
	}
}

// Alignment builds a valid alignment statement and applies the mutations before
// the cache key is recomputed.
func Alignment(mutations ...func(*format.Statement[format.Alignment])) format.Statement[format.Alignment] {
	decision := &format.Decision{
		Basis:            format.BasisJudges,
		Deciders:         []string{judgeModel},
		ConfidenceSource: format.HostVotesSource(judgeVotes),
		RawConfidence:    defaultConfidence,
		Threshold:        defaultThreshold,
	}
	predicate := format.Alignment{
		Requirement: requirement(),
		Core:        core(moduleAlignment, alignmentInputs()),
		Test:        TestName,
		Verdict:     mustJudged(format.VerdictAligned, defaultConfidence),
		Executor:    Executor(judgeModel, Digest("alignment-judge-prompt")),
	}
	predicate.Extensions.Decision = decision
	statement := format.NewStatement([]format.Subject{requirementSubject()}, predicate)
	for _, mutate := range mutations {
		mutate(&statement)
	}
	statement.Predicate.CacheKey = mustCacheKey(statement.Subject, statement.Predicate.Inputs)
	return statement
}

// DiscriminatingPower builds a valid discriminating-power statement with a
// killed valid mutant, a surviving equivalent one, a sentinel, example results and
// the operator screen counters.
func DiscriminatingPower(mutations ...func(*format.Statement[format.DiscriminatingPower])) format.Statement[format.DiscriminatingPower] {
	predicate := format.DiscriminatingPower{
		Requirement:       requirement(),
		Core:              core(moduleDiscriminate, discriminatingPowerInputs()),
		Tests:             []string{"test_grace_period_boundary", TestName},
		CorrectCodePasses: format.Executed(true),
		Mutants:           mutants(),
		Executor:          Executor(generatorModel, Digest("mutant-generator-prompt")),
		Examples: []format.ExampleRun{
			{Index: 0, Result: format.Executed(format.ExamplePassed)},
			{Index: 1, Result: format.Executed(format.ExamplePassed)},
		},
	}
	predicate.Extensions.OperatorScreen = &format.OperatorScreen{Flagged: 2, ScreenedOut: 37, FloorEntries: 1}
	statement := format.NewStatement([]format.Subject{requirementSubject()}, predicate)
	for _, mutate := range mutations {
		mutate(&statement)
	}
	statement.Predicate.CacheKey = mustCacheKey(statement.Subject, statement.Predicate.Inputs)
	return statement
}

// Completeness builds a valid advisory completeness statement.
func Completeness(mutations ...func(*format.Statement[format.Completeness])) format.Statement[format.Completeness] {
	predicate := format.Completeness{
		Core:     core(moduleCompleteness, completenessInputs()),
		Advisory: true,
		Questions: []format.Question{
			{Dimension: "boundaries", Question: "What is charged at exactly 15 minutes?", RequirementIDs: []string{RequirementID}},
			{Dimension: "time_and_locale", Question: "Which time zone defines the night window?"},
		},
		OutOfScopeDeclared: []format.OutOfScope{{Dimension: "concurrency", Reason: "single-threaded batch tool"}},
	}
	statement := format.NewStatement([]format.Subject{format.SpecSubject(SpecPath, Digest(specText))}, predicate)
	for _, mutate := range mutations {
		mutate(&statement)
	}
	statement.Predicate.CacheKey = mustCacheKey(statement.Subject, statement.Predicate.Inputs)
	return statement
}

func requirement() format.Requirement {
	return format.Requirement{ID: RequirementID, Revision: RequirementRevision, StatementCanonicalization: format.StatementCanonicalizationC1}
}

func requirementSubject() format.Subject {
	return format.RequirementSubject(RequirementID, RequirementRevision, StatementDigest())
}

func mutants() []format.Mutant {
	killedValid := format.Mutant{
		Digest:   Digest("mutant/boundary"),
		Validity: mustJudged(format.ValidityValid, 1.0).Corroborated(format.Corroboration{Kind: format.CorroborationExampleFailed, Digest: Digest("example-0")}),
		Results: []format.TestResult{
			{Test: "test_grace_period_boundary", Outcome: format.Executed(format.OutcomeKilled)},
			{Test: TestName, Outcome: format.Executed(format.OutcomeSurvived)},
		},
		Extensions: &format.MutantExtensions{Decision: &format.Decision{
			Basis:            format.BasisSpecExample,
			Deciders:         []string{"example-0"},
			ConfidenceSource: format.SourceExecution,
			RawConfidence:    1.0,
			Threshold:        defaultThreshold,
		}},
	}
	survivedEquivalent := format.Mutant{
		Digest:   Digest("mutant/rename"),
		Validity: mustJudged(format.ValidityEquivalent, 0.95),
		Results: []format.TestResult{
			{Test: "test_grace_period_boundary", Outcome: format.Executed(format.OutcomeSurvived)},
			{Test: TestName, Outcome: format.Executed(format.OutcomeSurvived)},
		},
	}
	sentinel := format.Mutant{
		Digest:   Digest("mutant/sentinel"),
		Validity: mustJudged(format.ValidityEquivalent, 1.0),
		Sentinel: true,
		Results: []format.TestResult{
			{Test: "test_grace_period_boundary", Outcome: format.Executed(format.OutcomeSurvived)},
			{Test: TestName, Outcome: format.Executed(format.OutcomeSurvived)},
		},
		Extensions: &format.MutantExtensions{Decision: &format.Decision{
			Basis:            format.BasisConstruction,
			Deciders:         []string{"sentinel-catalogue"},
			ConfidenceSource: format.SourceExecution,
			RawConfidence:    1.0,
			Threshold:        defaultThreshold,
		}},
	}
	pool := []format.Mutant{killedValid, survivedEquivalent, sentinel}
	slices.SortFunc(pool, func(a, b format.Mutant) int { return strings.Compare(a.Digest, b.Digest) })
	return pool
}

func mustCacheKey(subject []format.Subject, inputs []format.Input) string {
	key, err := format.CacheKey(subject, inputs)
	if err != nil {
		panic(err)
	}
	return key
}

// RegularMutantIndex returns the index of the first mutant that is not a
// sentinel, or -1 when every mutant is one.
func RegularMutantIndex(pool []format.Mutant) int {
	return slices.IndexFunc(pool, func(m format.Mutant) bool { return !m.Sentinel })
}
