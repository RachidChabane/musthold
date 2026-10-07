package testkit

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/completeness/internal/domain/criterion"
	"github.com/RachidChabane/spec-proof/format"
)

const (
	powerModule      = "spec-proof-dp"
	alignmentModule  = "spec-proof-alignment"
	moduleVersion    = "0.1.0"
	harnessName      = "fixture-host"
	harnessVersion   = "1"
	judgeConfidence  = 0.9
	decisionLimit    = 0.7
	statementName    = "statement"
	policyName       = "decision-policy"
	templateName     = "judge.validity"
	alignedTemplate  = "judge.alignment"
	powerCacheFolder = "discriminating-power"
	alignCacheFolder = "alignment"
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

func sortedInputs(inputs []format.Input) []format.Input {
	slices.SortFunc(inputs, func(a, b format.Input) int {
		if byRole := strings.Compare(string(a.Role), string(b.Role)); byRole != 0 {
			return byRole
		}
		return strings.Compare(a.Name, b.Name)
	})
	return inputs
}

func fileInputs(t testing.TB, root string, role format.InputRole, files []string) []format.Input {
	inputs := make([]format.Input, len(files))
	for index, file := range files {
		inputs[index] = format.Input{Role: role, Name: file, Digest: format.Digest{SHA256: FileDigest(t, root, file)}}
	}
	return inputs
}

func inlineExecutor(harness, prompt string) format.Executor {
	return format.Executor{
		ExecutorProfile: format.ExecutorProfile{
			Channel: format.ChannelHostInline, Harness: format.Harness{Name: harness, Version: harnessVersion},
			Isolation: format.IsolationInline, Model: format.ExecutorModel{Source: format.ModelSourceUnknown},
		},
		PromptDigest: digestOf(prompt), Blind: true,
	}
}

// Power describes a discriminating-power Statement of one criterion: the examples
// it ran, the test and code files it saw and the mutants no criterion owns.
type Power struct {
	Ref    criterion.Ref
	Digest string
	// Examples are the indexes of the examples that were run.
	Examples  []int
	TestFiles []string
	CodeFiles []string
	// Unattributed is the number of unattributed mutants the Statement lists.
	Unattributed int
	// StatementDigest overrides the digest the Statement states, to build a stale one.
	StatementDigest string
}

// Statement builds the canonical bytes of the Statement for the project at root.
func (p Power) Statement(t testing.TB, root string) []byte {
	t.Helper()
	digest := p.Digest
	if p.StatementDigest != "" {
		digest = p.StatementDigest
	}
	inputs := []format.Input{
		{Role: format.RoleSpec, Name: statementName, Digest: format.Digest{SHA256: digest}},
		{Role: format.RoleDecisionPolicy, Name: policyName, Digest: format.Digest{SHA256: digestOf("policy")}},
		{Role: format.RolePromptTemplate, Name: templateName, Digest: format.Digest{SHA256: digestOf("template")}},
	}
	inputs = append(inputs, fileInputs(t, root, format.RoleTestSource, p.TestFiles)...)
	inputs = append(inputs, fileInputs(t, root, format.RoleCodeTree, p.CodeFiles)...)
	sortedInputs(inputs)
	subject := []format.Subject{format.RequirementSubject(p.Ref.ID, p.Ref.Revision, digest)}
	cacheKey, err := format.CacheKey(subject, inputs)
	if err != nil {
		t.Fatal(err)
	}
	test := "tests.test_totals::test_" + strings.ToLower(p.Ref.ID)
	var mutants []format.Mutant
	for index := range p.Unattributed {
		result := format.TestResult{Test: test, Outcome: format.Executed(format.OutcomeSurvived)}
		validity, err := format.Judged(format.ValidityNotViolating, judgeConfidence)
		if err != nil {
			t.Fatal(err)
		}
		mutants = append(mutants, format.Mutant{
			Digest: digestOf(p.Ref.String() + "mutant" + strconv.Itoa(index)), Validity: validity, Results: []format.TestResult{result},
			Extensions: &format.MutantExtensions{
				Unattributed: true,
				Decision:     &format.Decision{Basis: format.BasisJudges, Deciders: []string{"judge-1", "judge-2"}, RawConfidence: judgeConfidence, Threshold: decisionLimit},
			},
		})
	}
	slices.SortFunc(mutants, func(a, b format.Mutant) int { return strings.Compare(a.Digest, b.Digest) })
	var examples []format.ExampleRun
	for _, index := range p.Examples {
		examples = append(examples, format.ExampleRun{Index: index, Result: format.Executed(format.ExamplePassed)})
	}
	slices.SortFunc(examples, func(a, b format.ExampleRun) int { return a.Index - b.Index })
	predicate := format.DiscriminatingPower{
		Requirement: format.Requirement{ID: p.Ref.ID, Revision: p.Ref.Revision, StatementCanonicalization: format.StatementCanonicalizationC1},
		Core: format.Core{
			Module: format.Module{Name: powerModule, Version: moduleVersion}, Inputs: inputs, CacheKey: cacheKey, ProducedBy: format.ProducedByLocal,
		},
		Tests:             []string{test},
		CorrectCodePasses: format.Executed(true),
		Mutants:           mutants,
		Executor:          inlineExecutor(harnessName, "generator"),
		Examples:          examples,
	}
	encoded, err := format.NewStatement(subject, predicate).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// Write puts the Statement into the local cache of the project at root, where the
// module reads it.
func (p Power) Write(t testing.TB, root string) {
	t.Helper()
	writeCached(t, root, powerCacheFolder, p.Ref, p.Statement(t, root))
}

// Aligned describes an alignment Statement of one test and one criterion.
type Aligned struct {
	Ref    criterion.Ref
	Digest string
	// Test is the label of the test, `classname.name`.
	Test      string
	Verdict   format.Verdict
	TestFiles []string
	CodeFiles []string
}

// Statement builds the canonical bytes of the Statement for the project at root.
func (a Aligned) Statement(t testing.TB, root string) []byte {
	t.Helper()
	inputs := []format.Input{
		{Role: format.RoleSpec, Name: statementName, Digest: format.Digest{SHA256: a.Digest}},
		{Role: format.RoleDecisionPolicy, Name: policyName, Digest: format.Digest{SHA256: digestOf("policy")}},
		{Role: format.RolePromptTemplate, Name: alignedTemplate, Digest: format.Digest{SHA256: digestOf("template")}},
	}
	inputs = append(inputs, fileInputs(t, root, format.RoleTestSource, a.TestFiles)...)
	inputs = append(inputs, fileInputs(t, root, format.RoleCodeTree, a.CodeFiles)...)
	sortedInputs(inputs)
	subject := []format.Subject{format.RequirementSubject(a.Ref.ID, a.Ref.Revision, a.Digest)}
	cacheKey, err := format.CacheKey(subject, inputs)
	if err != nil {
		t.Fatal(err)
	}
	verdict, err := format.Judged(a.Verdict, judgeConfidence)
	if err != nil {
		t.Fatal(err)
	}
	predicate := format.Alignment{
		Requirement: format.Requirement{ID: a.Ref.ID, Revision: a.Ref.Revision, StatementCanonicalization: format.StatementCanonicalizationC1},
		Core: format.Core{
			Module: format.Module{Name: alignmentModule, Version: moduleVersion}, Inputs: inputs, CacheKey: cacheKey, ProducedBy: format.ProducedByLocal,
		},
		Test:     a.Test,
		Verdict:  verdict,
		Executor: inlineExecutor(harnessName, "judge"),
	}
	encoded, err := format.NewStatement(subject, predicate).Encode()
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// Write puts the Statement into the local cache of the project at root.
func (a Aligned) Write(t testing.TB, root string) {
	t.Helper()
	writeCached(t, root, alignCacheFolder, a.Ref, a.Statement(t, root))
}

// writeCached stores a Statement under `.spec-proof/cache/<module>/<ref>/<cacheKey>.json`.
func writeCached(t testing.TB, root, folder string, ref criterion.Ref, statement []byte) {
	t.Helper()
	decoded, err := format.Decode(statement)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".spec-proof", "cache", folder, ref.String(), decoded.Common().CacheKey+".json")
	if err := os.MkdirAll(filepath.Dir(path), directoryMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, statement, fileMode); err != nil {
		t.Fatal(err)
	}
}

// PythonEvidence is what the other modules wrote for the Python ledger: the
// example probe of the totals criterion with its unattributed mutants, and an
// ALIGNED test of the same criterion that calls the line amount.
func PythonEvidence() (Power, Aligned) {
	code := []string{"src/invoicing/totals.py"}
	tests := []string{"tests/test_totals.py"}
	return Power{Ref: Totals, Digest: TotalsDigest, Examples: []int{0}, TestFiles: tests, CodeFiles: code, Unattributed: 2},
		Aligned{
			Ref: Totals, Digest: TotalsDigest, Test: "tests.test_totals.test_line_amount_multiplies_price_by_quantity",
			Verdict: format.VerdictAligned, TestFiles: tests, CodeFiles: code,
		}
}

// GoEvidence is the same for the Go ledger.
func GoEvidence() (Power, Aligned) {
	code := []string{"totals.go"}
	tests := []string{"totals_test.go"}
	return Power{Ref: Totals, Digest: TotalsDigest, Examples: []int{0}, TestFiles: tests, CodeFiles: code, Unattributed: 2},
		Aligned{
			Ref: Totals, Digest: TotalsDigest, Test: "example.com/ledger.TestLineAmount/multiplies_the_price_by_the_quantity",
			Verdict: format.VerdictAligned, TestFiles: tests, CodeFiles: code,
		}
}
