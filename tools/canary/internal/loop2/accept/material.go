package accept

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"

	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop2/task"
)

type tree = l1accept.Tree

// material is a task as it lies on disk, read once per acceptance round. What
// cannot be read is reported as problems and left empty.
type material struct {
	task         task.Task
	levers       task.Levers
	leversErr    error
	fixture      tree
	reference    tree
	oracle       tree
	secondOracle tree
	violations   []l1accept.Variant
	refactors    []l1accept.Variant
	citations    []byte
	provenance   task.Provenance
	problems     []problem

	// Trees derived from the material; nil when their inputs are unusable.
	referenceTree tree
	violationTree map[string]tree
	refactorTree  map[string]tree
}

// problem is something the material lacks, with the part it is about.
type problem struct {
	role   records.Role
	target records.Target
	text   string
}

func (m *material) problem(role records.Role, target records.Target, format string, args ...any) {
	m.problems = append(m.problems, problem{role: role, target: target, text: fmt.Sprintf(format, args...)})
}

// errTaskUnreadable is returned when the task's own description cannot be used.
var errTaskUnreadable = errors.New("the task description cannot be read")

// loadMaterial reads a task's description and every file the checks use.
func loadMaterial(repo, name string) (*material, error) {
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(task.TaskPath(name, task.TaskFile))))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskUnreadable, err)
	}
	t, err := task.ParseTask(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskUnreadable, err)
	}
	m := &material{task: t}
	read := func(what string, role records.Role, target records.Target, relative string) tree {
		files, err := l1accept.ReadTree(filepath.Join(repo, filepath.FromSlash(relative)))
		if err != nil {
			m.problem(role, target, "%s: %v", what, err)
			return tree{}
		}
		return files
	}
	m.fixture = read("fixture", records.RoleAuthor, records.TargetFixture, t.Fixture)
	m.reference = read("reference", records.RoleAuthor, records.TargetReference, t.Truth.Reference)
	m.oracle = read("oracle", records.RoleOracle, records.TargetOracle, t.Truth.Oracle)
	m.secondOracle = read("second oracle", task.RoleSecondOracle, records.TargetOracle, t.Truth.SecondOracle)
	violations := read("violations", task.RoleVariants, records.TargetVariants, t.Truth.Violations)
	refactors := read("refactors", task.RoleVariants, records.TargetVariants, t.Truth.Refactors)
	var problems []string
	m.violations, m.refactors, problems = l1accept.GroupVariants(violations, refactors)
	for _, text := range problems {
		m.problem(task.RoleVariants, records.TargetVariants, "%s", text)
	}
	m.readTaskFiles(repo)
	m.derive()
	return m, nil
}

// readTaskFiles reads levers.json, citations.json and provenance.json.
func (m *material) readTaskFiles(repo string) {
	at := func(file string) string {
		return filepath.Join(repo, filepath.FromSlash(task.TaskPath(m.task.Name, file)))
	}
	if data, err := os.ReadFile(at(task.LeversFile)); err != nil {
		m.leversErr = err
	} else {
		m.levers, m.leversErr = task.ParseLevers(data)
	}
	m.citations, _ = os.ReadFile(at(task.CitationsFile))
	if data, err := os.ReadFile(at(task.ProvenanceFile)); err == nil {
		m.provenance, _ = task.ParseProvenance(data)
	}
}

// derive builds the reference tree and the tree of each violation and refactor.
func (m *material) derive() {
	if len(m.reference) == 0 {
		m.problem(records.RoleAuthor, records.TargetReference, "reference: holds no file")
		return
	}
	reference, err := m.fixture.Overlay(m.reference, m.task.Package)
	if err != nil {
		m.problem(records.RoleAuthor, records.TargetReference, "reference: %v", err)
		return
	}
	m.referenceTree = reference
	m.violationTree, m.refactorTree = map[string]tree{}, map[string]tree{}
	for _, v := range m.violations {
		m.deriveVariant(v, m.violationTree)
	}
	for _, r := range m.refactors {
		m.deriveVariant(r, m.refactorTree)
	}
}

func (m *material) deriveVariant(v l1accept.Variant, into map[string]tree) {
	edited, err := m.referenceTree.Overlay(v.Edits, m.task.Package)
	if err != nil {
		m.problem(task.RoleVariants, records.TargetVariants, "%s: %v", v.Label(), err)
		return
	}
	into[v.Label()] = edited
}

// violationsOf lists the violations of one criterion, in order.
func (m *material) violationsOf(criterion string) []l1accept.Variant {
	var found []l1accept.Variant
	for _, v := range m.violations {
		if v.Criterion == criterion {
			found = append(found, v)
		}
	}
	return found
}

// specText is the fixture's spec.
func (m *material) specText() []byte { return m.fixture[m.task.Spec] }

// packageFile is a file of the fixture's package.
func (m *material) packageFile(name string) string { return path.Join(m.task.Package, name) }
