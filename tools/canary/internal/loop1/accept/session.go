package accept

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/construct/records"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// variant is one edit of the reference source: a violation of one criterion or a
// refactor. Its files lie relative to the directory of the source file.
type variant struct {
	// Criterion is the violated criterion; empty for a refactor.
	Criterion string
	Number    string
	Edits     tree
}

// Label names the variant in findings and in stage directories.
func (v variant) Label() string {
	if v.Criterion == "" {
		return "refactor-" + v.Number
	}
	return "violation-" + v.Criterion + "-" + v.Number
}

// material is a task as it lies on disk, read once per acceptance round. What
// cannot be read is reported as problems and left empty.
type material struct {
	repo       string
	task       study.Task
	fixture    tree
	reference  tree
	violations []variant
	refactors  []variant
	oracle     tree
	problems   []problem

	// Trees derived from the material; nil when their inputs are unusable.
	referenceTree tree
	violationTree map[string]tree
	refactorTree  map[string]tree
}

// problem is something the material lacks, with the part of the task it is about.
type problem struct {
	target records.Target
	text   string
}

func (m *material) problem(target records.Target, format string, args ...any) {
	m.problems = append(m.problems, problem{target: target, text: fmt.Sprintf(format, args...)})
}

// sourceDir is the directory every truth source file is relative to.
func (m *material) sourceDir() string { return m.task.TruthBase() }

// errTaskUnreadable is returned when the task's own description cannot be used.
var errTaskUnreadable = errors.New("the task description cannot be read")

// loadMaterial reads a task's description and every file the checks use.
func loadMaterial(repo, name string) (*material, error) {
	data, err := os.ReadFile(filepath.Join(repo, filepath.FromSlash(study.TaskDir(name)), study.TaskFile))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskUnreadable, err)
	}
	task, err := study.ParseTask(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", errTaskUnreadable, err)
	}
	m := &material{repo: repo, task: task}
	m.read("fixture", records.TargetFixture, task.Fixture, &m.fixture)
	m.read("reference", records.TargetReference, task.Truth.Reference, &m.reference)
	m.read("oracle", records.TargetOracle, task.Truth.Oracle, &m.oracle)
	m.readVariants()
	m.derive()
	return m, nil
}

func (m *material) read(what string, target records.Target, relative string, into *tree) {
	files, err := readTree(filepath.Join(m.repo, filepath.FromSlash(relative)))
	if err != nil {
		m.problem(target, "%s: %v", what, err)
		files = tree{}
	}
	*into = files
}

func (m *material) readVariants() {
	var violations, refactors tree
	m.read("violations", records.TargetVariants, m.task.Truth.Violations, &violations)
	m.read("refactors", records.TargetVariants, m.task.Truth.Refactors, &refactors)
	grouped := map[string]tree{}
	for _, name := range sortedKeys(violations) {
		parts := strings.SplitN(name, "/", 3)
		if len(parts) != 3 {
			m.problem(records.TargetVariants, "violations/%s is not under violations/<criterion>/<k>/", name)
			continue
		}
		key := parts[0] + "/" + parts[1]
		if grouped[key] == nil {
			grouped[key] = tree{}
		}
		grouped[key][parts[2]] = violations[name]
	}
	for _, key := range sortedKeys(grouped) {
		criterion, number, _ := strings.Cut(key, "/")
		m.violations = append(m.violations, variant{Criterion: criterion, Number: number, Edits: grouped[key]})
	}
	grouped = map[string]tree{}
	for _, name := range sortedKeys(refactors) {
		number, rest, found := strings.Cut(name, "/")
		if !found {
			m.problem(records.TargetVariants, "refactors/%s is not under refactors/<k>/", name)
			continue
		}
		if grouped[number] == nil {
			grouped[number] = tree{}
		}
		grouped[number][rest] = refactors[name]
	}
	for _, number := range sortedKeys(grouped) {
		m.refactors = append(m.refactors, variant{Number: number, Edits: grouped[number]})
	}
}

// derive builds the reference tree and the tree of each violation and refactor.
func (m *material) derive() {
	if len(m.reference) == 0 {
		m.problem(records.TargetReference, "reference: holds no file")
		return
	}
	reference, err := m.fixture.overlay(m.reference, m.sourceDir())
	if err != nil {
		m.problem(records.TargetReference, "reference: %v", err)
		return
	}
	m.referenceTree = reference
	m.violationTree = map[string]tree{}
	for _, v := range m.violations {
		m.derived(v, m.violationTree)
	}
	m.refactorTree = map[string]tree{}
	for _, r := range m.refactors {
		m.derived(r, m.refactorTree)
	}
}

func (m *material) derived(v variant, into map[string]tree) {
	edited, err := m.referenceTree.overlay(v.Edits, m.sourceDir())
	if err != nil {
		m.problem(records.TargetVariants, "%s: %v", v.Label(), err)
		return
	}
	into[v.Label()] = edited
}

// violationsOf lists the violations of one criterion, in order.
func (m *material) violationsOf(criterion string) []variant {
	var found []variant
	for _, v := range m.violations {
		if v.Criterion == criterion {
			found = append(found, v)
		}
	}
	return found
}

// violationsExcept lists the violations of every criterion but one.
func (m *material) violationsExcept(criterion string) []variant {
	var found []variant
	for _, v := range m.violations {
		if v.Criterion != criterion {
			found = append(found, v)
		}
	}
	return found
}

// violatedCriteria lists the criteria that have a violation directory, in order.
func (m *material) violatedCriteria() []string {
	seen := map[string]bool{}
	for _, v := range m.violations {
		seen[v.Criterion] = true
	}
	return sortedKeys(seen)
}
