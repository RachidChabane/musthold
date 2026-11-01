package accept

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
)

const (
	// InlineTestFile is where a test made of inline examples lies in a project.
	InlineTestFile       = "tests/test_inline_examples.py"
	MinTestsPerCriterion = 2
	collectedSeparator   = "::"
)

// PytestArgv is the command that runs a project's own tests.
func PytestArgv(junit string) []string {
	return []string{PythonProgram, "-m", "pytest", "-p", "no:cacheprovider", "-q", "--junitxml=" + junit}
}

// OracleArgv is the command that runs the oracle in a tree (PREREG §4.1).
func OracleArgv(junit string) []string {
	return []string{
		PythonProgram, "-m", "pytest", "-c", oraclePrefix + OracleConfigFile, "--rootdir", OracleSubdir, "--noconftest",
		"-p", "no:cacheprovider", "-q", "--junitxml=" + junit, OracleSubdir,
	}
}

// CollectArgv is the command that lists the oracle's tests in a tree.
func CollectArgv() []string {
	return []string{
		PythonProgram, "-m", "pytest", "-c", oraclePrefix + OracleConfigFile, "--rootdir", OracleSubdir, "--noconftest",
		"-p", "no:cacheprovider", "--collect-only", "-q", OracleSubdir,
	}
}

// OracleTree is a project as an oracle is run in it: without its tests, with
// the oracle's files added under oracle/.
func OracleTree(project, oracle Tree) Tree {
	return project.Without(TestsDirectory+"/").With(oraclePrefix, oracle)
}

// oracleTree is a project as the task's oracle is run in it.
func (r *round) oracleTree(project Tree) Tree { return OracleTree(project, r.m.oracle) }

// Tests runs the project's own tests in a tree. It reports ok=false when the
// runs disagree, after failing the result.
func (b *Bench) Tests(ctx context.Context, res *Result, label string, project Tree, authorTarget records.Target) (Outcome, bool, error) {
	found, err := b.PytestOutcome(ctx, label, project, PytestArgv, nil)
	return found, agreed(res, err, label, authorTarget, ""), fatal(err)
}

// SpecTests is an oracle as a bench runs it: its files, the criteria its test
// files are named after, and the role that writes it.
type SpecTests struct {
	Files    Tree
	Criteria []string
	Role     records.Role
}

// Oracle runs an oracle in a tree. It reports ok=false when the runs disagree,
// after failing the result for the author's target and the oracle's role.
func (b *Bench) Oracle(ctx context.Context, res *Result, label string, project Tree, spec SpecTests, authorTarget records.Target) (Outcome, bool, error) {
	label = "spec-tests-" + label
	found, err := b.PytestOutcome(ctx, label, OracleTree(project, spec.Files), OracleArgv, ModulesOfCriteria(spec.Criteria))
	return found, agreed(res, err, label, authorTarget, spec.Role), fatal(err)
}

func (r *round) tests(ctx context.Context, res *Result, label string, project Tree, authorTarget records.Target) (Outcome, bool, error) {
	return r.Tests(ctx, res, label, project, authorTarget)
}

// oracle runs the task's oracle in a tree.
func (r *round) oracle(ctx context.Context, res *Result, label string, project Tree, authorTarget records.Target) (Outcome, bool, error) {
	return r.Oracle(ctx, res, label, project, SpecTests{Files: r.m.oracle, Criteria: r.criteria, Role: records.RoleOracle}, authorTarget)
}

func agreed(res *Result, err error, label string, authorTarget records.Target, oracleRole records.Role) bool {
	var d Disagreement
	if errors.As(err, &d) {
		res.Disagree(d, label, authorTarget, oracleRole)
		return false
	}
	return err == nil
}

func fatal(err error) error {
	var d Disagreement
	if errors.As(err, &d) {
		return nil
	}
	return err
}

// fixture is check A2: the fixture's tests pass, and the traceability gate of the
// control arm blocks on the task criterion's missing test and on nothing else.
func (r *round) fixture(ctx context.Context) (*Result, error) {
	res := newResult(CheckFixture)
	found, ok, err := r.tests(ctx, res, "fixture-tests", r.m.fixture, records.TargetFixture)
	if err != nil {
		return nil, err
	}
	if ok && !found.PassesAll() {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the tests of the fixture do not pass:\n%s", DescribeFailures(found.FailedCases()))
	}
	if err := r.gate(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *round) gate(ctx context.Context, res *Result) error {
	dir, err := r.TreeDir("equipped", r.m.fixture)
	if err != nil {
		return err
	}
	project, err := r.a.Equipper.EquipControl(ctx, dir, r.m.task)
	if err != nil {
		return fmt.Errorf("equip the control arm: %w", err)
	}
	var reports [Runs]*ShallnotReport
	runs, err := r.Repeat(ctx,
		func(int) box.Command { return r.Command(project, ShallnotProgram, "gate", "--format", "json") },
		func(run int, result box.Result) string {
			reports[run] = DecodeShallnot(result)
			return ShallnotSignature(reports[run], result.ExitCode)
		})
	if err != nil {
		return err
	}
	if !runs.Agreed {
		res.Disagree(Disagreement{Signatures: runs.Signatures}, "the traceability run", records.TargetFixture, "")
		return nil
	}
	if blocked := blockingFindings(reports[0]); !onlyMissingTest(blocked, r.m.task.Criterion) {
		res.Fail(records.RoleAuthor, records.TargetFixture,
			"the traceability run of the project must flag the missing test of %s and nothing else; it flagged: %s", r.m.task.Criterion, describeBlocks(reports[0], blocked))
	}
	return nil
}

func blockingFindings(report *ShallnotReport) []ShallnotFinding {
	if report == nil {
		return nil
	}
	var blocking []ShallnotFinding
	for _, finding := range report.Findings {
		if finding.Blocking {
			blocking = append(blocking, finding)
		}
	}
	return blocking
}

func onlyMissingTest(blocking []ShallnotFinding, criterion string) bool {
	return len(blocking) == 1 && blocking[0].Category == CategoryUncovered && blocking[0].RequirementID == criterion
}

func describeBlocks(report *ShallnotReport, blocking []ShallnotFinding) string {
	if report == nil {
		return "no readable report"
	}
	if len(blocking) == 0 {
		return "nothing (verdict " + report.Verdict + ")"
	}
	var parts []string
	for _, finding := range blocking {
		parts = append(parts, finding.Category+" "+finding.RequirementID)
	}
	return strings.Join(parts, ", ")
}

// reference is check A3: the reference tree passes the fixture's tests and every
// oracle test.
func (r *round) reference(ctx context.Context) (*Result, error) {
	res := newResult(CheckReference)
	tests, ok, err := r.tests(ctx, res, "reference-tests", r.m.referenceTree, records.TargetReference)
	if err != nil {
		return nil, err
	}
	if ok && !tests.PassesAll() {
		res.Fail(records.RoleAuthor, records.TargetReference, "the reference does not pass the tests of the fixture:\n%s", DescribeFailures(tests.FailedCases()))
	}
	found, ok, err := r.oracle(ctx, res, "reference", r.m.referenceTree, records.TargetReference)
	if err != nil {
		return nil, err
	}
	if ok && !found.PassesAll() {
		r.failOracle(res, found, records.TargetReference, "the reference", "an implementation of every criterion")
	}
	return res, nil
}

// failOracle reports oracle tests that fail where they should pass, to the
// author with their messages and to the test writer by name alone.
func (r *round) failOracle(res *Result, found Outcome, target records.Target, where, implementation string) {
	if found.EverythingFails() || len(found.FailedCases()) == 0 {
		res.Fail(records.RoleAuthor, target, "%s ran no spec test to the end (timeout, crash or collection failure)", where)
		res.Fail(records.RoleOracle, records.TargetOracle, "the tests do not run to the end against %s", implementation)
		return
	}
	res.Fail(records.RoleAuthor, target, "%s fails spec tests:\n%s", where, DescribeFailures(found.FailedCases()))
	res.Fail(records.RoleOracle, records.TargetOracle, "these tests fail against %s:\n%s", implementation, strings.Join(SortedCaseIDs(found.FailedCases()), "\n"))
}

// base is check A4: the fixture fails an oracle test of the task criterion and
// passes every oracle test of every other criterion.
func (r *round) base(ctx context.Context) (*Result, error) {
	res := newResult(CheckBase)
	criterion := r.m.task.Criterion
	found, ok, err := r.oracle(ctx, res, "fixture", r.m.fixture, records.TargetFixture)
	if err != nil || !ok {
		return res, err
	}
	if !found.FailsCriterion(criterion) {
		res.Fail(records.RoleAuthor, records.TargetFixture, "no spec test of %s fails on the fixture: its source already behaves as the criterion states", criterion)
		res.Fail(records.RoleOracle, records.TargetOracle, "no test of %s fails against an implementation that lacks the behaviour it states", criterion)
	}
	if outside := found.FailingOutside(criterion); len(outside) > 0 {
		res.Fail(records.RoleAuthor, records.TargetFixture, "the fixture fails spec tests of other criteria:\n%s", DescribeFailures(outside))
		res.Fail(records.RoleOracle, records.TargetOracle, "these tests fail against an implementation of every criterion but %s:\n%s", criterion, strings.Join(SortedCaseIDs(outside), "\n"))
	}
	return res, nil
}

// diagonal is check A5: each violation fails an oracle test of its own criterion,
// and every oracle test of the other criteria passes on it.
func (r *round) diagonal(ctx context.Context) (*Result, error) {
	res := newResult(CheckDiagonal)
	for _, v := range r.m.violations {
		found, ok, err := r.oracle(ctx, res, v.Label(), r.m.violationTree[v.Label()], records.TargetVariants)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		if !found.FailsCriterion(v.Criterion) {
			res.Fail(records.RoleAuthor, records.TargetVariants, "%s fails no spec test of %s", v.Label(), v.Criterion)
			res.Fail(records.RoleOracle, records.TargetOracle, "no test of %s fails against an implementation that violates it", v.Criterion)
		}
		if outside := found.FailingOutside(v.Criterion); len(outside) > 0 {
			res.Fail(records.RoleAuthor, records.TargetVariants, "%s fails spec tests of other criteria:\n%s", v.Label(), DescribeFailures(outside))
			res.Fail(records.RoleOracle, records.TargetOracle, "these tests fail against an implementation that violates only %s:\n%s", v.Criterion, strings.Join(SortedCaseIDs(outside), "\n"))
		}
	}
	return res, nil
}

// refactorsCheck is check A6: each refactor passes every oracle test.
func (r *round) refactorsCheck(ctx context.Context) (*Result, error) {
	res := newResult(CheckRefactors)
	for _, refactor := range r.m.refactors {
		found, ok, err := r.oracle(ctx, res, refactor.Label(), r.m.refactorTree[refactor.Label()], records.TargetVariants)
		if err != nil {
			return nil, err
		}
		if ok && !found.PassesAll() {
			r.failOracle(res, found, records.TargetVariants, refactor.Label(), "an implementation that keeps every criterion")
		}
	}
	return res, nil
}

// room is check A7: some violation of the task criterion passes a test made of
// the criterion's inline examples alone, and some violation of an implemented
// criterion passes the fixture's tests.
func (r *round) room(ctx context.Context) (*Result, error) {
	res := newResult(CheckRoom)
	task := r.m.task
	inline, err := InlineExamplesTest(map[string]string{task.Interface.Operation: task.Interface.Module}, r.examples[task.Criterion])
	if err != nil {
		return nil, err
	}
	witness, err := r.firstPassing(ctx, res, r.m.violationsOf(task.Criterion), func(v Variant) (string, Tree) {
		project := r.m.violationTree[v.Label()].Without(TestsDirectory + "/")
		project[InlineTestFile] = inline
		return "inline-" + v.Label(), project
	})
	if err != nil {
		return nil, err
	}
	if res.Agreed && witness == "" {
		res.Fail(records.RoleAuthor, records.TargetVariants, "no violation of %s passes a test made of the inline examples of %s alone", task.Criterion, task.Criterion)
	}
	witness, err = r.firstPassing(ctx, res, r.m.violationsExcept(task.Criterion), func(v Variant) (string, Tree) {
		return "tests-" + v.Label(), r.m.violationTree[v.Label()]
	})
	if err != nil {
		return nil, err
	}
	if res.Agreed && witness == "" {
		res.Fail(records.RoleAuthor, records.TargetVariants, "no violation of an implemented criterion passes the tests of the fixture")
	}
	return res, nil
}

// firstPassing runs the project's tests on each variant, in order, and returns
// the label of the first on which they pass.
func (r *round) firstPassing(ctx context.Context, res *Result, variants []Variant, project func(Variant) (string, Tree)) (string, error) {
	for _, v := range variants {
		label, files := project(v)
		found, ok, err := r.tests(ctx, res, label, files, records.TargetVariants)
		if err != nil {
			return "", err
		}
		if ok && found.PassesAll() {
			return v.Label(), nil
		}
	}
	return "", nil
}

// InlineExamplesTest is a test file that asserts each inline example of a
// criterion and nothing else, importing each operation the examples call from
// its module.
func InlineExamplesTest(modules map[string]string, examples []Example) ([]byte, error) {
	if len(examples) == 0 {
		return nil, fmt.Errorf("the task criterion has no inline example")
	}
	called := map[string]string{}
	for _, example := range examples {
		module, found := modules[example.Operation]
		if !found {
			return nil, fmt.Errorf("an example calls %q, which no module provides", example.Operation)
		}
		called[example.Operation] = module
	}
	var source strings.Builder
	for _, operation := range sortedKeys(called) {
		fmt.Fprintf(&source, "from %s import %s\n", called[operation], operation)
	}
	for index, example := range examples {
		fmt.Fprintf(&source, "\n\ndef test_example_%d():\n    assert %s(%s) == (%s)\n", index+1, example.Operation, example.Arguments, example.Result)
	}
	return []byte(source.String()), nil
}

// collection is check A9 on the task's oracle.
func (r *round) collection(ctx context.Context) (*Result, error) {
	res := newResult(CheckCollection)
	spec := SpecTests{Files: r.m.oracle, Criteria: r.criteria, Role: records.RoleOracle}
	if err := r.Collection(ctx, res, r.m.referenceTree, spec); err != nil {
		return nil, err
	}
	return res, nil
}

// Collection fails the result unless the oracle, run in the reference tree,
// collects at least MinTestsPerCriterion tests for each of its criteria and
// skips none. Its findings go to the oracle's role.
func (b *Bench) Collection(ctx context.Context, res *Result, reference Tree, spec SpecTests) error {
	label := "collect"
	dir, err := b.TreeDir(label, OracleTree(reference, spec.Files))
	if err != nil {
		return err
	}
	var listed [Runs][]string
	runs, err := b.Repeat(ctx,
		func(int) box.Command { return b.Command(dir, CollectArgv()...) },
		func(run int, result box.Result) string {
			listed[run] = CollectedNodes(result.Stdout)
			return fmt.Sprintf("exit=%d timeout=%t\n%s", result.ExitCode, result.TimedOut, strings.Join(listed[run], "\n"))
		})
	if err != nil {
		return err
	}
	if !runs.Agreed {
		res.Fail(spec.Role, records.TargetOracle, "the three collections of the tests do not give the same list")
		res.Agreed = false
		res.Output += Disagreement{Signatures: runs.Signatures}.Error()
		return nil
	}
	if runs.Results[0].ExitCode != 0 || runs.Results[0].TimedOut {
		res.Fail(spec.Role, records.TargetOracle, "the tests cannot be collected (exit %d):\n%s", runs.Results[0].ExitCode, CollectionErrors(runs.Results[0].Stdout))
		return nil
	}
	countCollected(res, listed[0], spec)
	found, ok, err := b.Oracle(ctx, res, "reference", reference, spec, records.TargetReference)
	if err != nil {
		return err
	}
	if skipped := found.Skipped(); ok && len(skipped) > 0 {
		res.Fail(spec.Role, records.TargetOracle, "these tests are skipped:\n%s", strings.Join(SortedCaseIDs(skipped), "\n"))
	}
	return nil
}

func countCollected(res *Result, nodes []string, spec SpecTests) {
	perFile := map[string]int{}
	for _, node := range nodes {
		file, _, _ := strings.Cut(node, collectedSeparator)
		perFile[file[strings.LastIndex(file, "/")+1:]]++
	}
	for _, criterion := range spec.Criteria {
		if n := perFile[OracleFileName(criterion)]; n < MinTestsPerCriterion {
			res.Fail(spec.Role, records.TargetOracle, "%s collects %d tests, at least %d are required for %s", OracleFileName(criterion), n, MinTestsPerCriterion, criterion)
		}
	}
}

// CollectedNodes lists the test node IDs `pytest --collect-only -q` printed.
func CollectedNodes(stdout []byte) []string {
	var nodes []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if line = strings.TrimSpace(line); strings.Contains(line, collectedSeparator) && !strings.ContainsAny(line, " \t") {
			nodes = append(nodes, line)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// CollectionErrors keeps the lines of a collection report that name an error.
func CollectionErrors(stdout []byte) string {
	var kept []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if strings.HasPrefix(line, "ERROR ") {
			kept = append(kept, Truncate(line))
		}
	}
	return strings.Join(kept, "\n")
}
