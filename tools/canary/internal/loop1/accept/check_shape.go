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

// introspectScript prints, as JSON, whether a module imports, whether it holds
// the named callable, the public functions the module defines and the
// parameters of the callable.
//
//go:embed introspect.py
var introspectScript string

// The shape a fixture follows (PREREG §3.1).
const (
	minCriteria         = 4
	maxCriteria         = 6
	minViolations       = 2
	minRefactors        = 1
	pytestConfigFile    = "pytest.ini"
	ignoreFile          = ".gitignore"
	testsDirectory      = "tests"
	specsDirectory      = "specs/"
	packageInitFile     = "__init__.py"
	bindingLanguage     = "python"
	bindingSeparator    = ":"
	emptyResultsFile    = "empty-results.xml"
	emptyResultsContent = "<testsuites/>\n"
	categoryUncovered   = "uncovered_requirement"
)

var ignoreEntries = []string{"__pycache__/", "test-results/"}

type shallnotReport struct {
	Verdict      string `json:"verdict"`
	ExitCode     int    `json:"exit_code"`
	Requirements []struct {
		ID        string `json:"id"`
		Revision  int    `json:"revision"`
		Ref       string `json:"ref"`
		Statement string `json:"statement"`
		Status    string `json:"status"`
	} `json:"requirements"`
	Findings []shallnotFinding `json:"findings"`
}

type shallnotFinding struct {
	Category      string `json:"category"`
	Severity      string `json:"severity"`
	Blocking      bool   `json:"blocking"`
	RequirementID string `json:"requirement_id"`
	Message       string `json:"message"`
}

type introspection struct {
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
		res.fail(records.RoleAuthor, p.target, "%s", p.text)
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
		res.fail(records.RoleAuthor, records.TargetTask, "task.json: %v", err)
	}
	module := strings.ReplaceAll(task.Interface.Module, ".", "/") + pythonSuffix
	if module != task.Source {
		res.fail(records.RoleAuthor, records.TargetTask, "task.json: the source %q is not the file of module %q (%s)", task.Source, task.Interface.Module, module)
	}
	binding, found := task.Operations[task.Interface.Operation]
	want := task.Interface.Module + bindingSeparator + task.Interface.Operation
	if len(task.Operations) != 1 || !found || len(binding) != 1 || binding[bindingLanguage] != want {
		res.fail(records.RoleAuthor, records.TargetTask,
			"task.json: operations must bind the operation %q alone, as {%q: {%q: %q}}", task.Interface.Operation, task.Interface.Operation, bindingLanguage, want)
	}
	if _, kept := r.m.fixture[task.Spec]; !kept || !strings.HasPrefix(task.Spec, specsDirectory) {
		res.fail(records.RoleAuthor, records.TargetFixture, "the spec %q is not a file under %s of the fixture", task.Spec, specsDirectory)
	}
	if _, kept := r.m.fixture[task.Source]; !kept {
		res.fail(records.RoleAuthor, records.TargetFixture, "the source file %q does not exist in the fixture", task.Source)
	}
}

func (r *round) checkFixtureFiles(res *Result) {
	fixture := r.m.fixture
	ini := IniValues(string(fixture[pytestConfigFile]))
	if ini["testpaths"] != testsDirectory || ini["pythonpath"] != "." {
		res.fail(records.RoleAuthor, records.TargetFixture, "%s must hold, under [pytest], testpaths = %s and pythonpath = .", pytestConfigFile, testsDirectory)
	}
	ignored := strings.Fields(string(fixture[ignoreFile]))
	for _, entry := range ignoreEntries {
		if !slices.Contains(ignored, entry) {
			res.fail(records.RoleAuthor, records.TargetFixture, "%s must hold the line %s", ignoreFile, entry)
		}
	}
	specs := 0
	testFiles := 0
	for name := range fixture {
		if strings.HasPrefix(name, specsDirectory) {
			specs++
		}
		if strings.HasPrefix(name, testsDirectory+"/") && strings.HasSuffix(name, pythonSuffix) {
			testFiles++
		}
	}
	if specs != 1 {
		res.fail(records.RoleAuthor, records.TargetFixture, "the fixture must hold one spec file under %s, it holds %d files", specsDirectory, specs)
	}
	if testFiles == 0 {
		res.fail(records.RoleAuthor, records.TargetFixture, "the fixture holds no test file under %s/", testsDirectory)
	}
	r.checkPackage(res)
}

// checkPackage requires a package of one module: an __init__.py and the source file.
func (r *round) checkPackage(res *Result) {
	dir := r.m.sourceDir()
	if dir == "." {
		res.fail(records.RoleAuthor, records.TargetFixture, "the source file %q must lie inside a package directory", r.m.task.Source)
		return
	}
	var modules []string
	for name := range r.m.fixture {
		if path.Dir(name) == dir && strings.HasSuffix(name, pythonSuffix) && path.Base(name) != packageInitFile {
			modules = append(modules, name)
		}
	}
	if _, kept := r.m.fixture[path.Join(dir, packageInitFile)]; !kept {
		res.fail(records.RoleAuthor, records.TargetFixture, "the package %s holds no %s", dir, packageInitFile)
	}
	if len(modules) != 1 {
		res.fail(records.RoleAuthor, records.TargetFixture, "the package %s must hold one module besides %s, it holds %d", dir, packageInitFile, len(modules))
	}
}

func (r *round) checkTruthLayout(res *Result) {
	for name := range r.m.reference {
		if _, kept := r.m.fixture[path.Join(r.m.sourceDir(), name)]; !kept {
			res.fail(records.RoleAuthor, records.TargetReference, "reference/%s does not edit a source file of the package", name)
		}
	}
	if len(r.m.refactors) < minRefactors {
		res.fail(records.RoleAuthor, records.TargetVariants, "at least %d refactor is required, there are %d", minRefactors, len(r.m.refactors))
	}
}

// parseSpec runs Shallnot on the fixture's spec three times and checks that the
// spec parses. It returns the report of the first run.
func (r *round) parseSpec(ctx context.Context, res *Result) (*shallnotReport, error) {
	dir, err := r.treeDir("fixture", r.m.fixture)
	if err != nil {
		return nil, err
	}
	empty := path.Join(r.stage, emptyResultsFile)
	if err := writeFile(empty, []byte(emptyResultsContent)); err != nil {
		return nil, err
	}
	build := func(int) box.Command {
		return r.command(dir, ShallnotProgram, "check", "--no-config", "--advisory", "--format", "json", "--specs", r.m.task.Spec, "--results", empty)
	}
	var parsed [Runs]*shallnotReport
	runs, err := r.repeat(ctx, build, func(run int, result box.Result) string {
		parsed[run] = decodeShallnot(result)
		return shallnotSignature(parsed[run], result.ExitCode)
	})
	if err != nil {
		return nil, err
	}
	if !runs.agreed {
		res.disagree(disagreement{signatures: runs.signatures}, "the spec", records.TargetFixture, false)
		return nil, nil
	}
	if parsed[0] == nil || runs.results[0].ExitCode != 0 {
		res.fail(records.RoleAuthor, records.TargetFixture, "the spec does not parse with Shallnot (exit %d): %s", runs.results[0].ExitCode, firstLine(runs.results[0].Stderr))
		return nil, nil
	}
	return parsed[0], nil
}

func decodeShallnot(result box.Result) *shallnotReport {
	var report shallnotReport
	if json.Unmarshal(result.Stdout, &report) != nil {
		return nil
	}
	return &report
}

func shallnotSignature(report *shallnotReport, exit int) string {
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
func (r *round) checkSpec(res *Result, report *shallnotReport) {
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
			res.fail(records.RoleAuthor, records.TargetFixture, "criterion %s is not one WHEN … THE SYSTEM SHALL … statement", requirement.ID)
		}
		r.checkExamples(res, requirement.ID)
	}
	if n := len(report.Requirements); n < minCriteria || n > maxCriteria {
		res.fail(records.RoleAuthor, records.TargetFixture, "the spec holds %d criteria, %d to %d are required", n, minCriteria, maxCriteria)
	}
	if !found {
		res.fail(records.RoleAuthor, records.TargetTask, "the task criterion %s is not a criterion of the spec", task.Criterion)
	}
	for _, finding := range report.Findings {
		if finding.Category != categoryUncovered {
			res.fail(records.RoleAuthor, records.TargetFixture, "the spec has a problem: %s: %s", finding.Category, finding.Message)
		}
	}
	for _, criterion := range r.m.violatedCriteria() {
		if !slices.Contains(r.criteria, criterion) {
			res.fail(records.RoleAuthor, records.TargetVariants, "violations/%s: %s is not a criterion of the spec", criterion, criterion)
		}
	}
	for _, criterion := range r.criteria {
		if n := len(r.m.violationsOf(criterion)); n < minViolations {
			res.fail(records.RoleAuthor, records.TargetVariants, "criterion %s has %d violations, at least %d are required", criterion, n, minViolations)
		}
	}
}

func (r *round) checkExamples(res *Result, criterion string) {
	examples := r.examples[criterion]
	if len(examples) == 0 {
		res.fail(records.RoleAuthor, records.TargetFixture, "criterion %s has no inline example `operation(argument=value) => result`", criterion)
	}
	for _, example := range examples {
		if example.Operation != r.m.task.Interface.Operation {
			res.fail(records.RoleAuthor, records.TargetFixture, "an example of %s calls %q, the operation is %q", criterion, example.Operation, r.m.task.Interface.Operation)
		}
		if _, err := SplitArguments(example.Arguments); err != nil {
			res.fail(records.RoleAuthor, records.TargetFixture, "an example of %s: %v", criterion, err)
		}
	}
}

// checkCitations requires the tests to cite every criterion but the task's.
func (r *round) checkCitations(res *Result) {
	var sources []string
	for name, content := range r.m.fixture {
		if strings.HasPrefix(name, testsDirectory+"/") && strings.HasSuffix(name, pythonSuffix) {
			sources = append(sources, string(content))
		}
	}
	cited := Citations(sources)
	for _, criterion := range r.criteria {
		reference := r.refs[criterion]
		switch {
		case criterion == r.m.task.Criterion && cited[reference] > 0:
			res.fail(records.RoleAuthor, records.TargetFixture, "a test of the fixture cites the task criterion %s", reference)
		case criterion != r.m.task.Criterion && cited[reference] == 0:
			res.fail(records.RoleAuthor, records.TargetFixture, "no test of the fixture cites %s", reference)
		}
	}
}

// checkBinding imports the bound callable in the container and checks the
// operation's signature against the interface and the examples.
func (r *round) checkBinding(ctx context.Context, res *Result) error {
	task := r.m.task
	dir := path.Join(r.stage, stageTreesDir, "fixture")
	module, operation, _ := strings.Cut(task.Operations[task.Interface.Operation][bindingLanguage], bindingSeparator)
	var seen [Runs]introspection
	runs, err := r.repeat(ctx,
		func(int) box.Command {
			return r.command(dir, PythonProgram, "-c", introspectScript, module, operation)
		},
		func(run int, result box.Result) string {
			_ = json.Unmarshal(result.Stdout, &seen[run])
			return string(result.Stdout)
		})
	if err != nil {
		return err
	}
	if !runs.agreed {
		res.disagree(disagreement{signatures: runs.signatures}, "the operations binding", records.TargetFixture, false)
		return nil
	}
	found := seen[0]
	switch {
	case !found.Importable:
		res.fail(records.RoleAuthor, records.TargetFixture, "the module %s does not import: %s", module, found.Error)
	case !found.Callable:
		res.fail(records.RoleAuthor, records.TargetFixture, "the module %s holds no callable %s", module, operation)
	default:
		r.checkOperation(res, found)
	}
	return nil
}

func (r *round) checkOperation(res *Result, found introspection) {
	task := r.m.task
	if len(found.PublicFunctions) != 1 || found.PublicFunctions[0] != task.Interface.Operation {
		res.fail(records.RoleAuthor, records.TargetFixture, "the module must define one public operation %q, it defines %v", task.Interface.Operation, found.PublicFunctions)
	}
	var names []string
	for _, parameter := range found.Parameters {
		if parameter.Kind != "POSITIONAL_OR_KEYWORD" && parameter.Kind != "KEYWORD_ONLY" {
			res.fail(records.RoleAuthor, records.TargetFixture, "the parameter %s of %s cannot be passed by keyword", parameter.Name, task.Interface.Operation)
		}
		names = append(names, parameter.Name)
	}
	if declared := SignatureParameters(task.Interface.Signature); !slices.Equal(declared, names) {
		res.fail(records.RoleAuthor, records.TargetTask, "task.json: the signature %q lists %v, the operation takes %v", task.Interface.Signature, declared, names)
	}
	for _, criterion := range r.criteria {
		for _, example := range r.examples[criterion] {
			arguments, err := SplitArguments(example.Arguments)
			if err != nil {
				continue
			}
			for _, argument := range arguments {
				if !slices.Contains(names, argument.Name) {
					res.fail(records.RoleAuthor, records.TargetFixture, "an example of %s passes %q, which %s does not take", criterion, argument.Name, task.Interface.Operation)
				}
			}
		}
	}
}
