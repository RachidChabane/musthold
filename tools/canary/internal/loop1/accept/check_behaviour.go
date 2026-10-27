package accept

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

const (
	inlineTestFile      = "tests/test_inline_examples.py"
	minTestsPerCriteria = 2
	collectedSeparator  = "::"
)

// pytestArgv is the command that runs a project's own tests.
func pytestArgv(junit string) []string {
	return []string{PythonProgram, "-m", "pytest", "-p", "no:cacheprovider", "-q", "--junitxml=" + junit}
}

// oracleArgv is the command that runs the oracle in a tree (PREREG §4.1).
func oracleArgv(junit string) []string {
	return []string{
		PythonProgram, "-m", "pytest", "-c", oraclePrefix + OracleConfigFile, "--rootdir", OracleSubdir, "--noconftest",
		"-p", "no:cacheprovider", "-q", "--junitxml=" + junit, OracleSubdir,
	}
}

func collectArgv() []string {
	return []string{
		PythonProgram, "-m", "pytest", "-c", oraclePrefix + OracleConfigFile, "--rootdir", OracleSubdir, "--noconftest",
		"-p", "no:cacheprovider", "--collect-only", "-q", OracleSubdir,
	}
}

// oracleTree is a project as the oracle is run in it: without its tests, with
// the oracle added.
func (r *round) oracleTree(project tree) tree {
	return project.without(testsDirectory+"/").with(oraclePrefix, r.m.oracle)
}

// tests runs the project's own tests in a tree. It reports ok=false when the
// runs disagree, after failing the result.
func (r *round) tests(ctx context.Context, res *Result, label string, project tree, authorTarget records.Target) (outcome, bool, error) {
	found, err := r.pytestOutcome(ctx, label, project, pytestArgv, nil)
	return found, r.agreed(res, err, label, authorTarget, false), r.fatal(err)
}

// oracle runs the oracle in a tree. It reports ok=false when the runs disagree.
func (r *round) oracle(ctx context.Context, res *Result, label string, project tree, authorTarget records.Target) (outcome, bool, error) {
	label = "spec-tests-" + label
	found, err := r.pytestOutcome(ctx, label, r.oracleTree(project), oracleArgv, ModulesOfCriteria(r.criteria))
	return found, r.agreed(res, err, label, authorTarget, true), r.fatal(err)
}

func (r *round) agreed(res *Result, err error, label string, authorTarget records.Target, withOracle bool) bool {
	var d disagreement
	if errors.As(err, &d) {
		res.disagree(d, label, authorTarget, withOracle)
		return false
	}
	return err == nil
}

func (r *round) fatal(err error) error {
	var d disagreement
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
	if ok && !found.passesAll() {
		res.fail(records.RoleAuthor, records.TargetFixture, "the tests of the fixture do not pass:\n%s", describeFailures(found.failedCases()))
	}
	if err := r.gate(ctx, res); err != nil {
		return nil, err
	}
	return res, nil
}

func (r *round) gate(ctx context.Context, res *Result) error {
	dir, err := r.treeDir("equipped", r.m.fixture)
	if err != nil {
		return err
	}
	if err := r.a.Equipper.EquipControl(ctx, dir, r.m.task); err != nil {
		return fmt.Errorf("equip the control arm: %w", err)
	}
	var reports [Runs]*shallnotReport
	runs, err := r.repeat(ctx,
		func(int) box.Command { return r.command(dir, ShallnotProgram, "gate", "--format", "json") },
		func(run int, result box.Result) string {
			reports[run] = decodeShallnot(result)
			return shallnotSignature(reports[run], result.ExitCode)
		})
	if err != nil {
		return err
	}
	if !runs.agreed {
		res.disagree(disagreement{signatures: runs.signatures}, "the traceability run", records.TargetFixture, false)
		return nil
	}
	if blocked := blockingFindings(reports[0]); !onlyMissingTest(blocked, r.m.task.Criterion) {
		res.fail(records.RoleAuthor, records.TargetFixture,
			"the traceability run of the project must flag the missing test of %s and nothing else; it flagged: %s", r.m.task.Criterion, describeBlocks(reports[0], blocked))
	}
	return nil
}

func blockingFindings(report *shallnotReport) []shallnotFinding {
	if report == nil {
		return nil
	}
	var blocking []shallnotFinding
	for _, finding := range report.Findings {
		if finding.Blocking {
			blocking = append(blocking, finding)
		}
	}
	return blocking
}

func onlyMissingTest(blocking []shallnotFinding, criterion string) bool {
	return len(blocking) == 1 && blocking[0].Category == categoryUncovered && blocking[0].RequirementID == criterion
}

func describeBlocks(report *shallnotReport, blocking []shallnotFinding) string {
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
	if ok && !tests.passesAll() {
		res.fail(records.RoleAuthor, records.TargetReference, "the reference does not pass the tests of the fixture:\n%s", describeFailures(tests.failedCases()))
	}
	found, ok, err := r.oracle(ctx, res, "reference", r.m.referenceTree, records.TargetReference)
	if err != nil {
		return nil, err
	}
	if ok && !found.passesAll() {
		r.failOracle(res, found, records.TargetReference, "the reference", "an implementation of every criterion")
	}
	return res, nil
}

// failOracle reports oracle tests that fail where they should pass, to the
// author with their messages and to the test writer by name alone.
func (r *round) failOracle(res *Result, found outcome, target records.Target, where, implementation string) {
	if found.everythingFails() || len(found.failedCases()) == 0 {
		res.fail(records.RoleAuthor, target, "%s ran no spec test to the end (timeout, crash or collection failure)", where)
		res.fail(records.RoleOracle, records.TargetOracle, "the tests do not run to the end against %s", implementation)
		return
	}
	res.fail(records.RoleAuthor, target, "%s fails spec tests:\n%s", where, describeFailures(found.failedCases()))
	res.fail(records.RoleOracle, records.TargetOracle, "these tests fail against %s:\n%s", implementation, strings.Join(sortedCaseIDs(found.failedCases()), "\n"))
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
	if !found.failsCriterion(criterion) {
		res.fail(records.RoleAuthor, records.TargetFixture, "no spec test of %s fails on the fixture: its source already behaves as the criterion states", criterion)
		res.fail(records.RoleOracle, records.TargetOracle, "no test of %s fails against an implementation that lacks the behaviour it states", criterion)
	}
	if outside := found.failingOutside(criterion); len(outside) > 0 {
		res.fail(records.RoleAuthor, records.TargetFixture, "the fixture fails spec tests of other criteria:\n%s", describeFailures(outside))
		res.fail(records.RoleOracle, records.TargetOracle, "these tests fail against an implementation of every criterion but %s:\n%s", criterion, strings.Join(sortedCaseIDs(outside), "\n"))
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
		if !found.failsCriterion(v.Criterion) {
			res.fail(records.RoleAuthor, records.TargetVariants, "%s fails no spec test of %s", v.Label(), v.Criterion)
			res.fail(records.RoleOracle, records.TargetOracle, "no test of %s fails against an implementation that violates it", v.Criterion)
		}
		if outside := found.failingOutside(v.Criterion); len(outside) > 0 {
			res.fail(records.RoleAuthor, records.TargetVariants, "%s fails spec tests of other criteria:\n%s", v.Label(), describeFailures(outside))
			res.fail(records.RoleOracle, records.TargetOracle, "these tests fail against an implementation that violates only %s:\n%s", v.Criterion, strings.Join(sortedCaseIDs(outside), "\n"))
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
		if ok && !found.passesAll() {
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
	inline, err := inlineExamplesTest(task.Interface, r.examples[task.Criterion])
	if err != nil {
		return nil, err
	}
	witness, err := r.firstPassing(ctx, res, r.m.violationsOf(task.Criterion), func(v variant) (string, tree) {
		project := r.m.violationTree[v.Label()].without(testsDirectory + "/")
		project[inlineTestFile] = inline
		return "inline-" + v.Label(), project
	})
	if err != nil {
		return nil, err
	}
	if res.Agreed && witness == "" {
		res.fail(records.RoleAuthor, records.TargetVariants, "no violation of %s passes a test made of the inline examples of %s alone", task.Criterion, task.Criterion)
	}
	witness, err = r.firstPassing(ctx, res, r.m.violationsExcept(task.Criterion), func(v variant) (string, tree) {
		return "tests-" + v.Label(), r.m.violationTree[v.Label()]
	})
	if err != nil {
		return nil, err
	}
	if res.Agreed && witness == "" {
		res.fail(records.RoleAuthor, records.TargetVariants, "no violation of an implemented criterion passes the tests of the fixture")
	}
	return res, nil
}

// firstPassing runs the project's tests on each variant, in order, and returns
// the label of the first on which they pass.
func (r *round) firstPassing(ctx context.Context, res *Result, variants []variant, project func(variant) (string, tree)) (string, error) {
	for _, v := range variants {
		label, files := project(v)
		found, ok, err := r.tests(ctx, res, label, files, records.TargetVariants)
		if err != nil {
			return "", err
		}
		if ok && found.passesAll() {
			return v.Label(), nil
		}
	}
	return "", nil
}

// inlineExamplesTest is a test file that asserts each inline example of a
// criterion and nothing else.
func inlineExamplesTest(iface study.Interface, examples []Example) ([]byte, error) {
	if len(examples) == 0 {
		return nil, fmt.Errorf("the task criterion has no inline example")
	}
	var source strings.Builder
	fmt.Fprintf(&source, "from %s import %s\n", iface.Module, iface.Operation)
	for index, example := range examples {
		fmt.Fprintf(&source, "\n\ndef test_example_%d():\n    assert %s(%s) == (%s)\n", index+1, iface.Operation, example.Arguments, example.Result)
	}
	return []byte(source.String()), nil
}

// collection is check A9: the oracle collects at least two tests per criterion
// and skips none.
func (r *round) collection(ctx context.Context) (*Result, error) {
	res := newResult(CheckCollection)
	label := "collect"
	dir, err := r.treeDir(label, r.oracleTree(r.m.referenceTree))
	if err != nil {
		return nil, err
	}
	var listed [Runs][]string
	runs, err := r.repeat(ctx,
		func(int) box.Command { return r.command(dir, collectArgv()...) },
		func(run int, result box.Result) string {
			listed[run] = collectedNodes(result.Stdout)
			return fmt.Sprintf("exit=%d timeout=%t\n%s", result.ExitCode, result.TimedOut, strings.Join(listed[run], "\n"))
		})
	if err != nil {
		return nil, err
	}
	if !runs.agreed {
		res.fail(records.RoleOracle, records.TargetOracle, "the three collections of the tests do not give the same list")
		res.Agreed = false
		res.Output += disagreement{signatures: runs.signatures}.Error()
		return res, nil
	}
	if runs.results[0].ExitCode != 0 || runs.results[0].TimedOut {
		res.fail(records.RoleOracle, records.TargetOracle, "the tests cannot be collected (exit %d):\n%s", runs.results[0].ExitCode, collectionErrors(runs.results[0].Stdout))
		return res, nil
	}
	r.countCollected(res, listed[0])
	reference, ok, err := r.oracle(ctx, res, "reference", r.m.referenceTree, records.TargetReference)
	if err != nil {
		return nil, err
	}
	if skipped := reference.skipped(); ok && len(skipped) > 0 {
		res.fail(records.RoleOracle, records.TargetOracle, "these tests are skipped:\n%s", strings.Join(sortedCaseIDs(skipped), "\n"))
	}
	return res, nil
}

func (r *round) countCollected(res *Result, nodes []string) {
	perFile := map[string]int{}
	for _, node := range nodes {
		file, _, _ := strings.Cut(node, collectedSeparator)
		perFile[file[strings.LastIndex(file, "/")+1:]]++
	}
	for _, criterion := range r.criteria {
		if n := perFile[OracleFileName(criterion)]; n < minTestsPerCriteria {
			res.fail(records.RoleOracle, records.TargetOracle, "%s collects %d tests, at least %d are required for %s", OracleFileName(criterion), n, minTestsPerCriteria, criterion)
		}
	}
}

// collectedNodes lists the test node IDs `pytest --collect-only -q` printed.
func collectedNodes(stdout []byte) []string {
	var nodes []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if line = strings.TrimSpace(line); strings.Contains(line, collectedSeparator) && !strings.ContainsAny(line, " \t") {
			nodes = append(nodes, line)
		}
	}
	sort.Strings(nodes)
	return nodes
}

// collectionErrors keeps the lines of a collection report that name an error.
func collectionErrors(stdout []byte) string {
	var kept []string
	for _, line := range strings.Split(string(stdout), "\n") {
		if strings.HasPrefix(line, "ERROR ") {
			kept = append(kept, truncate(line))
		}
	}
	return strings.Join(kept, "\n")
}
