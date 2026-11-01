package accept

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

// IntrospectScript prints, as JSON, whether a module imports, whether it holds
// the named callable, the public functions the module defines and the
// parameters of the callable.
//
//go:embed introspect.py
var IntrospectScript string

// The shape a fixture follows (PREREG §3.1).
const (
	minCriteria         = 4
	maxCriteria         = 6
	MinViolations       = 2
	MinRefactors        = 1
	pytestConfigFile    = "pytest.ini"
	ignoreFile          = ".gitignore"
	TestsDirectory      = "tests"
	SpecsDirectory      = "specs/"
	PackageInitFile     = "__init__.py"
	BindingLanguage     = "python"
	BindingSeparator    = ":"
	emptyResultsFile    = "empty-results.xml"
	emptyResultsContent = "<testsuites/>\n"
	CategoryUncovered   = "uncovered_requirement"
)

var ignoreEntries = []string{"__pycache__/", "test-results/"}

// ShallnotReport is the part of Shallnot's JSON report the checks read.
type ShallnotReport struct {
	Verdict      string `json:"verdict"`
	ExitCode     int    `json:"exit_code"`
	Requirements []struct {
		ID        string `json:"id"`
		Revision  int    `json:"revision"`
		Ref       string `json:"ref"`
		Statement string `json:"statement"`
		Status    string `json:"status"`
	} `json:"requirements"`
	Findings []ShallnotFinding `json:"findings"`
}

// ShallnotFinding is one finding of a Shallnot report.
type ShallnotFinding struct {
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	Blocking      bool   `json:"blocking"`
	RequirementID string `json:"requirement_id"`
	Message       string `json:"message"`
}

// Introspection is what IntrospectScript prints.
type Introspection struct {
	Importable      bool     `json:"importable"`
	Callable        bool     `json:"callable"`
	PublicFunctions []string `json:"public_functions"`
	Parameters      []struct {
		Name string `json:"name"`
		Kind string `json:"kind"`
	} `json:"parameters"`
	Error string `json:"error"`
}

// shape is check A1: the fixture follows PREREG §3.1, the spec parses with
// Shallnot, every example names the task's operation, and the operations binding
// names a callable that exists.
func (r *round) shape(ctx context.Context) (*Result, error) {
	res := newResult(CheckShape)
	for _, p := range r.m.problems {
		res.Fail(records.RoleAuthor, p.target, "%s", p.text)
	}
	r.checkTaskFile(res)
	r.checkFixtureFiles(res)
	r.checkTruthLayout(res)
	if r.m.referenceTree == nil {
		return res, nil
	}
	report, err := r.parseSpec(ctx, res)
	if err != nil || report == nil {
		return res, err
	}
	r.checkSpec(res, report)
	r.checkCitations(res)
	if err := r.checkBinding(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *round) checkTaskFile(res *Result) {
	task := r.m.task
	if err := CheckPrompt(task.Prompt, task.Criterion, task.Spec, task.Source); err != nil {
		res.Fail(records.RoleAuthor, records.TargetTask, "task.json: %v", err)
	}
	module := strings.ReplaceAll(task.Interface.Module, ".", "/") + PythonSuffix
	if module != task.Source {
		res.Fail(records.RoleAuthor, records.TargetTask, "task.json: the source %q is not the file of module %q (%s)", task.Source, task.Interface.Module, module)
	}
	binding, found := task.Operations[task.Interface.Operation]
	want := task.Interface.Module + BindingSeparator + task.Interface.Operation
	if len(task.Operations) != 1 || !found || len(binding) != 1 || binding[BindingLanguage] != want {
		res.Fail(records.RoleAuthor, records.TargetTask,
			"task.json: operations must bind the operation %q alone, as {%q: {%q: %q}}", task.Interface.Operation, task.Interface.Operation, BindingLanguage, want)
	}
	if _, kept := r.m.fixture[task.Spec]; !kept || !strings.HasPrefix(task.Spec, SpecsDirectory) {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the spec %q is not a file under %s of the fixture", task.Spec, SpecsDirectory)
	}
	if _, kept := r.m.fixture[task.Source]; !kept {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the source file %q does not exist in the fixture", task.Source)
	}
}

func (r *round) checkFixtureFiles(res *Result) {
	CheckProjectFiles(res, r.m.fixture)
	r.checkPackage(res)
}

// CheckProjectFiles requires the pytest configuration and the ignore file of
// the shape, one spec file under specs/ and a test file under tests/.
func CheckProjectFiles(res *Result, fixture Tree) {
	ini := IniValues(string(fixture[pytestConfigFile]))
	if ini["testpaths"] != TestsDirectory || ini["pythonpath"] != "." {
		res.Fail(records.RoleAuthor, records.TargetFixture, "%s must hold, under [pytest], testpaths = %s and pythonpath = .", pytestConfigFile, TestsDirectory)
	}
	ignored := strings.Fields(string(fixture[ignoreFile]))
	for _, entry := range ignoreEntries {
		if !slices.Contains(ignored, entry) {
			res.Fail(records.RoleAuthor, records.TargetFixture, "%s must hold the line %s", ignoreFile, entry)
		}
	}
	specs := 0
	testFiles := 0
	for name := range fixture {
		if strings.HasPrefix(name, SpecsDirectory) {
			specs++
		}
		if strings.HasPrefix(name, TestsDirectory+"/") && strings.HasSuffix(name, PythonSuffix) {
			testFiles++
		}
	}
	if specs != 1 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the fixture must hold one spec file under %s, it holds %d files", SpecsDirectory, specs)
	}
	if testFiles == 0 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the fixture holds no test file under %s/", TestsDirectory)
	}
}

// checkPackage requires a package of one module: an __init__.py and the source file.
func (r *round) checkPackage(res *Result) {
	dir := r.m.sourceDir()
	if dir == "." {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the source file %q must lie inside a package directory", r.m.task.Source)
		return
	}
	var modules []string
	for name := range r.m.fixture {
		if path.Dir(name) == dir && strings.HasSuffix(name, PythonSuffix) && path.Base(name) != PackageInitFile {
			modules = append(modules, name)
		}
	}
	if _, kept := r.m.fixture[path.Join(dir, PackageInitFile)]; !kept {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the package %s holds no %s", dir, PackageInitFile)
	}
	if len(modules) != 1 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the package %s must hold one module besides %s, it holds %d", dir, PackageInitFile, len(modules))
	}
}

func (r *round) checkTruthLayout(res *Result) {
	for name := range r.m.reference {
		if _, kept := r.m.fixture[path.Join(r.m.sourceDir(), name)]; !kept {
			res.Fail(records.RoleAuthor, records.TargetReference, "reference/%s does not edit a source file of the package", name)
		}
	}
	if len(r.m.refactors) < MinRefactors {
		res.Fail(records.RoleAuthor, records.TargetVariants, "at least %d refactor is required, there are %d", MinRefactors, len(r.m.refactors))
	}
}

// parseSpec runs Shallnot on the fixture's spec three times and checks that the
// spec parses. It returns the report of the first run.
func (r *round) parseSpec(ctx context.Context, res *Result) (*ShallnotReport, error) {
	return r.ParseSpec(ctx, res, r.m.fixture, r.m.task.Spec)
}

// ParseSpec runs Shallnot on a fixture's spec three times and checks that the
// spec parses. It returns the report of the first run, or nil after failing the
// result.
func (b *Bench) ParseSpec(ctx context.Context, res *Result, fixture Tree, spec string) (*ShallnotReport, error) {
	dir, err := b.TreeDir("fixture", fixture)
	if err != nil {
		return nil, err
	}
	empty := path.Join(b.Stage, emptyResultsFile)
	if err := writeFile(empty, []byte(emptyResultsContent)); err != nil {
		return nil, err
	}
	build := func(int) box.Command {
		return b.Command(dir, ShallnotProgram, "check", "--no-config", "--advisory", "--format", "json", "--specs", spec, "--results", empty)
	}
	var parsed [Runs]*ShallnotReport
	runs, err := b.Repeat(ctx, build, func(run int, result box.Result) string {
		parsed[run] = DecodeShallnot(result)
		return ShallnotSignature(parsed[run], result.ExitCode)
	})
	if err != nil {
		return nil, err
	}
	if !runs.Agreed {
		res.Disagree(Disagreement{Signatures: runs.Signatures}, "the spec", records.TargetFixture, "")
		return nil, nil
	}
	if parsed[0] == nil || runs.Results[0].ExitCode != 0 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the spec does not parse with Shallnot (exit %d): %s", runs.Results[0].ExitCode, firstLine(runs.Results[0].Stderr))
		return nil, nil
	}
	return parsed[0], nil
}

// DecodeShallnot reads the JSON report a Shallnot run printed, or nil.
func DecodeShallnot(result box.Result) *ShallnotReport {
	var report ShallnotReport
	if json.Unmarshal(result.Stdout, &report) != nil {
		return nil
	}
	return &report
}

// ShallnotSignature reduces a Shallnot run to what its three runs must agree on.
func ShallnotSignature(report *ShallnotReport, exit int) string {
	if report == nil {
		return fmt.Sprintf("exit=%d unreadable", exit)
	}
	parts := []string{fmt.Sprintf("exit=%d", exit), report.Verdict}
	for _, requirement := range report.Requirements {
		parts = append(parts, requirement.Ref+" "+requirement.Statement)
	}
	for _, finding := range report.Findings {
		parts = append(parts, fmt.Sprintf("%s|%s|%t", finding.Category, finding.RequirementID, finding.Blocking))
	}
	return strings.Join(parts, "\n")
}

// checkSpec checks the criteria of the parsed spec.
func (r *round) checkSpec(res *Result, report *ShallnotReport) {
	task := r.m.task
	r.spec = r.m.fixture[task.Spec]
	r.examples = ParseExamples(r.spec)
	found := false
	for _, requirement := range report.Requirements {
		r.criteria = append(r.criteria, requirement.ID)
		r.refs[requirement.ID] = requirement.Ref
		if requirement.ID == task.Criterion {
			found = true
		}
		if !IsRequirementStatement(requirement.Statement) {
			res.Fail(records.RoleAuthor, records.TargetFixture, "criterion %s is not one WHEN … THE SYSTEM SHALL … statement", requirement.ID)
		}
		r.checkExamples(res, requirement.ID)
	}
	if n := len(report.Requirements); n < minCriteria || n > maxCriteria {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the spec holds %d criteria, %d to %d are required", n, minCriteria, maxCriteria)
	}
	if !found {
		res.Fail(records.RoleAuthor, records.TargetTask, "the task criterion %s is not a criterion of the spec", task.Criterion)
	}
	for _, finding := range report.Findings {
		if finding.Category != CategoryUncovered {
			res.Fail(records.RoleAuthor, records.TargetFixture, "the spec has a problem: %s: %s", finding.Category, finding.Message)
		}
	}
	for _, criterion := range r.m.violatedCriteria() {
		if !slices.Contains(r.criteria, criterion) {
			res.Fail(records.RoleAuthor, records.TargetVariants, "violations/%s: %s is not a criterion of the spec", criterion, criterion)
		}
	}
	for _, criterion := range r.criteria {
		if n := len(r.m.violationsOf(criterion)); n < MinViolations {
			res.Fail(records.RoleAuthor, records.TargetVariants, "criterion %s has %d violations, at least %d are required", criterion, n, MinViolations)
		}
	}
}

func (r *round) checkExamples(res *Result, criterion string) {
	examples := r.examples[criterion]
	if len(examples) == 0 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "criterion %s has no inline example `operation(argument=value) => result`", criterion)
	}
	for _, example := range examples {
		if example.Operation != r.m.task.Interface.Operation {
			res.Fail(records.RoleAuthor, records.TargetFixture, "an example of %s calls %q, the operation is %q", criterion, example.Operation, r.m.task.Interface.Operation)
		}
		if _, err := SplitArguments(example.Arguments); err != nil {
			res.Fail(records.RoleAuthor, records.TargetFixture, "an example of %s: %v", criterion, err)
		}
	}
}

// checkCitations requires the tests to cite every criterion but the task's.
func (r *round) checkCitations(res *Result) {
	var sources []string
	for name, content := range r.m.fixture {
		if strings.HasPrefix(name, TestsDirectory+"/") && strings.HasSuffix(name, PythonSuffix) {
			sources = append(sources, string(content))
		}
	}
	cited := Citations(sources)
	for _, criterion := range r.criteria {
		reference := r.refs[criterion]
		switch {
		case criterion == r.m.task.Criterion && cited[reference] > 0:
			res.Fail(records.RoleAuthor, records.TargetFixture, "a test of the fixture cites the task criterion %s", reference)
		case criterion != r.m.task.Criterion && cited[reference] == 0:
			res.Fail(records.RoleAuthor, records.TargetFixture, "no test of the fixture cites %s", reference)
		}
	}
}

// checkBinding imports the bound callable in the container and checks the
// operation's signature against the interface and the examples.
func (r *round) checkBinding(ctx context.Context, res *Result) error {
	task := r.m.task
	dir := path.Join(r.Stage, stageTreesDir, "fixture")
	module, operation, _ := strings.Cut(task.Operations[task.Interface.Operation][BindingLanguage], BindingSeparator)
	var seen [Runs]Introspection
	runs, err := r.Repeat(ctx,
		func(int) box.Command {
			return r.Command(dir, PythonProgram, "-c", IntrospectScript, module, operation)
		},
		func(run int, result box.Result) string {
			_ = json.Unmarshal(result.Stdout, &seen[run])
			return string(result.Stdout)
		})
	if err != nil {
		return err
	}
	if !runs.Agreed {
		res.Disagree(Disagreement{Signatures: runs.Signatures}, "the operations binding", records.TargetFixture, "")
		return nil
	}
	found := seen[0]
	switch {
	case !found.Importable:
		res.Fail(records.RoleAuthor, records.TargetFixture, "the module %s does not import: %s", module, found.Error)
	case !found.Callable:
		res.Fail(records.RoleAuthor, records.TargetFixture, "the module %s holds no callable %s", module, operation)
	default:
		r.checkOperation(res, found)
	}
	return nil
}

func (r *round) checkOperation(res *Result, found Introspection) {
	task := r.m.task
	if len(found.PublicFunctions) != 1 || found.PublicFunctions[0] != task.Interface.Operation {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the module must define one public operation %q, it defines %v", task.Interface.Operation, found.PublicFunctions)
	}
	var names []string
	for _, parameter := range found.Parameters {
		if parameter.Kind != "POSITIONAL_OR_KEYWORD" && parameter.Kind != "KEYWORD_ONLY" {
			res.Fail(records.RoleAuthor, records.TargetFixture, "the parameter %s of %s cannot be passed by keyword", parameter.Name, task.Interface.Operation)
		}
		names = append(names, parameter.Name)
	}
	if declared := SignatureParameters(task.Interface.Signature); !slices.Equal(declared, names) {
		res.Fail(records.RoleAuthor, records.TargetTask, "task.json: the signature %q lists %v, the operation takes %v", task.Interface.Signature, declared, names)
	}
	for _, criterion := range r.criteria {
		for _, example := range r.examples[criterion] {
			arguments, err := SplitArguments(example.Arguments)
			if err != nil {
				continue
			}
			for _, argument := range arguments {
				if !slices.Contains(names, argument.Name) {
					res.Fail(records.RoleAuthor, records.TargetFixture, "an example of %s passes %q, which %s does not take", criterion, argument.Name, task.Interface.Operation)
				}
			}
		}
	}
}
