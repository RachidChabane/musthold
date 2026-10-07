// Package testkit holds what the tests of the module share: builders of
// discriminating-power Statements for the fixture projects, and the answer
// sources a fake host plays them with. It is never linked into the binary.
package testkit

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/alignment/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

// MutantKind says how a mutant of a discriminating-power Statement is classed.
type MutantKind int

// Kinds of mutant.
const (
	// ValidJudged: VALID on the judges' word, with a corroborating kill.
	ValidJudged MutantKind = iota
	// ValidExample: VALID because it fails an example of the criterion.
	ValidExample
	// ValidCollateral: VALID but collateral, which the matrix leaves out.
	ValidCollateral
	// Unattributed: NOT_VIOLATING and unattributed.
	Unattributed
	// Sentinel: an AST-level no-op, EQUIVALENT by construction.
	Sentinel
)

// Mutant is a mutant of a Statement and the tests that killed it.
type Mutant struct {
	Digest  string
	Kind    MutantKind
	Killers []string
}

// Power describes a discriminating-power Statement of one criterion.
type Power struct {
	Ref             criterion.Ref
	StatementDigest string
	// Tests are the identities of the bound tests, as `classname::name`.
	Tests   []string
	Mutants []Mutant
	// TestFiles and CodeFiles are the project files whose digests the Statement
	// records, relative to the project root.
	TestFiles []string
	CodeFiles []string
}

const (
	moduleName        = "spec-proof-dp"
	moduleVersion     = "0.1.0"
	harnessName       = "fixture-host"
	harnessVersion    = "1"
	judgeConfidence   = 0.9
	judgeCount        = 2
	decisionThreshold = 0.7
)

func digestOf(text string) string { return format.SHA256Hex([]byte(text)) }

// FileDigest digests a file of the project.
func FileDigest(t testing.TB, root, file string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
	if err != nil {
		t.Fatal(err)
	}
	return format.SHA256Hex(content)
}

// Statement builds the canonical bytes of the discriminating-power Statement for
// the project at root.
func (p Power) Statement(t testing.TB, root string) []byte {
	t.Helper()
	inputs := []format.Input{
		format.SpecInput(p.Ref.ID, p.Ref.Revision, p.StatementDigest),
		{Role: format.RoleDecisionPolicy, Name: "decision-policy", Digest: format.Digest{SHA256: digestOf("policy")}},
		{Role: format.RolePromptTemplate, Name: "judge.validity", Digest: format.Digest{SHA256: digestOf("template")}},
	}
	for _, file := range p.TestFiles {
		inputs = append(inputs, format.FileInput(format.RoleTestSource, file, FileDigest(t, root, file)))
	}
	for _, file := range p.CodeFiles {
		inputs = append(inputs, format.FileInput(format.RoleCodeTree, file, FileDigest(t, root, file)))
	}
	slices.SortFunc(inputs, func(a, b format.Input) int {
		if byRole := strings.Compare(string(a.Role), string(b.Role)); byRole != 0 {
			return byRole
		}
		return strings.Compare(a.Name, b.Name)
	})
	subject := []format.Subject{format.RequirementSubject(p.Ref.ID, p.Ref.Revision, p.StatementDigest)}
	cacheKey, err := format.CacheKey(subject, inputs)
	if err != nil {
		t.Fatal(err)
	}
	tests := slices.Clone(p.Tests)
	slices.Sort(tests)
	mutants := make([]format.Mutant, len(p.Mutants))
	for index, mutant := range p.Mutants {
		mutants[index] = p.mutant(t, mutant, tests)
	}
	slices.SortFunc(mutants, func(a, b format.Mutant) int { return strings.Compare(a.Digest, b.Digest) })
	predicate := format.DiscriminatingPower{
		Requirement: format.Requirement{ID: p.Ref.ID, Revision: p.Ref.Revision, StatementCanonicalization: format.StatementCanonicalizationC1},
		Core: format.Core{
			Module: format.Module{Name: moduleName, Version: moduleVersion}, Inputs: inputs, CacheKey: cacheKey,
			ProducedBy: format.ProducedByLocal,
		},
		Tests:             tests,
		CorrectCodePasses: format.Executed(true),
		Mutants:           mutants,
		Executor: format.Executor{
			ExecutorProfile: format.ExecutorProfile{
				Channel: format.ChannelHostInline, Harness: format.Harness{Name: harnessName, Version: harnessVersion},
				Isolation: format.IsolationInline, Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
			},
			PromptDigest: digestOf("generator"), Blind: true,
		},
	}
	encoded, err := format.NewStatement(subject, predicate).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

func (p Power) mutant(t testing.TB, mutant Mutant, tests []string) format.Mutant {
	t.Helper()
	results := make([]format.TestResult, len(tests))
	for index, test := range tests {
		outcome := format.OutcomeSurvived
		if slices.Contains(mutant.Killers, test) {
			outcome = format.OutcomeKilled
		}
		results[index] = format.TestResult{Test: test, Outcome: format.Executed(outcome)}
	}
	decision := func(basis format.DecisionBasis) *format.Decision {
		source := format.HostVotesSource(judgeCount)
		if basis == format.BasisSpecExample {
			source = format.SourceExecution
		}
		return &format.Decision{Basis: basis, Deciders: []string{"judge-1", "judge-2"}, ConfidenceSource: source, RawConfidence: judgeConfidence, Threshold: decisionThreshold}
	}
	entry := format.Mutant{Digest: digestOf(p.Ref.String() + mutant.Digest), Results: results}
	switch mutant.Kind {
	case Sentinel:
		entry.Sentinel = true
		entry.Validity = judged(t, format.ValidityEquivalent)
		entry.Extensions = &format.MutantExtensions{Decision: &format.Decision{Basis: format.BasisConstruction, Deciders: []string{"sentinel-catalogue"}, ConfidenceSource: format.SourceExecution, RawConfidence: 1, Threshold: decisionThreshold}}
	case Unattributed:
		entry.Validity = judged(t, format.ValidityNotViolating)
		entry.Extensions = &format.MutantExtensions{Decision: decision(format.BasisJudges), Unattributed: true}
	case ValidExample:
		entry.Validity = judged(t, format.ValidityValid).Corroborated(p.corroboration(mutant, format.CorroborationExampleFailed))
		entry.Extensions = &format.MutantExtensions{Decision: decision(format.BasisSpecExample)}
	case ValidCollateral:
		entry.Validity = judged(t, format.ValidityValid).Corroborated(p.corroboration(mutant, format.CorroborationSpecificKill))
		entry.Extensions = &format.MutantExtensions{Decision: decision(format.BasisJudges), Collateral: true}
	default:
		entry.Validity = judged(t, format.ValidityValid).Corroborated(p.corroboration(mutant, format.CorroborationSpecificKill))
		entry.Extensions = &format.MutantExtensions{Decision: decision(format.BasisJudges)}
	}
	return entry
}

func judged(t testing.TB, value format.Validity) format.Claim[format.Validity] {
	t.Helper()
	claim, err := format.Judged(value, judgeConfidence)
	if err != nil {
		t.Fatal(err)
	}
	return claim
}

func (p Power) corroboration(mutant Mutant, kind format.CorroborationKind) format.Corroboration {
	return format.Corroboration{Kind: kind, Digest: digestOf(p.Ref.String() + mutant.Digest + string(kind))}
}

// Write puts the Statement into the local cache of the project at root, where the
// module reads it, and returns its path.
func (p Power) Write(t testing.TB, root string) string {
	t.Helper()
	statement := p.Statement(t, root)
	decoded, err := format.DecodeAs[format.DiscriminatingPower](statement)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".spec-proof", "cache", "discriminating-power", p.Ref.String(), decoded.Predicate.CacheKey+".json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, statement, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
