package task

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/RachidChabane/musthold/tools/canary/internal/demoloop/config"
	l1accept "github.com/RachidChabane/musthold/tools/canary/internal/loop1/accept"
)

// Tier is how hard a task is built: how many levers it must declare.
type Tier string

// The tiers.
const (
	TierHard Tier = "hard"
	TierMax  Tier = "max"
)

// Tiers lists the tiers.
func Tiers() []Tier { return []Tier{TierHard, TierMax} }

// Kind says whether a task criterion is new to the spec or revises one the
// source implements.
type Kind string

// The kinds of task criterion.
const (
	KindNew     Kind = "new"
	KindRevised Kind = "revised"
)

// Criterion is a task criterion: its ID, without its revision, and its kind.
type Criterion struct {
	ID   string `json:"id"`
	Kind Kind   `json:"kind"`
}

// Operation is one operation of the interface the oracles are told about.
type Operation struct {
	// Module is the dotted module path, such as parcels.rates.
	Module string `json:"module"`
	// Operation is the public operation's name.
	Operation string `json:"operation"`
	// Signature is the operation's Python signature.
	Signature string `json:"signature"`
}

// Truth is where a task's truth files lie, relative to the repository root.
// Its first four members are loop-1's.
type Truth struct {
	Reference    string `json:"reference"`
	Violations   string `json:"violations"`
	Refactors    string `json:"refactors"`
	Oracle       string `json:"oracle"`
	SecondOracle string `json:"second_oracle"`
}

// Task is the description of one task of the loop-2 study.
type Task struct {
	Name string `json:"name"`
	Tier Tier   `json:"tier"`
	// Fixture is the project the session starts from, relative to the
	// repository root.
	Fixture string `json:"fixture"`
	// Spec is the spec file, relative to the fixture.
	Spec string `json:"spec"`
	// Package is the package directory, relative to the fixture: the files of
	// the reference and of each variant lie relative to it.
	Package string `json:"package"`
	// Criteria are the two task criteria.
	Criteria []Criterion `json:"criteria"`
	// Prompt is what the session is asked, verbatim.
	Prompt string `json:"prompt"`
	// Operations binds each operation of the spec's examples to its target, as
	// `.musthold/config.yaml` reads it.
	Operations map[string]map[string]string `json:"operations"`
	Interface  []Operation                  `json:"interface"`
	Truth      Truth                        `json:"truth"`
}

// The shape of a task's description.
const (
	TaskCriteria  = 2
	MinOperations = 3
	MaxOperations = 6
)

// Errors of a task description that cannot be used.
var (
	ErrMalformedTask = errors.New("the task description is not a JSON document of the expected shape")
	ErrInvalidTask   = errors.New("the task description is invalid")
)

var (
	taskNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	criterionPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)
	dottedName       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
	plainName        = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// ParseTask reads a task description and checks every member the verbs depend
// on, naming each member at fault. Unknown members are refused.
func ParseTask(data []byte) (Task, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var t Task
	if err := decoder.Decode(&t); err != nil {
		return Task{}, fmt.Errorf("%w: %v", ErrMalformedTask, err)
	}
	if decoder.More() {
		return Task{}, fmt.Errorf("%w: more than one document", ErrMalformedTask)
	}
	if problems := t.problems(); len(problems) > 0 {
		return Task{}, fmt.Errorf("%w: %s", ErrInvalidTask, strings.Join(problems, "; "))
	}
	return t, nil
}

func (t Task) problems() []string {
	var found []string
	add := func(format string, args ...any) { found = append(found, fmt.Sprintf(format, args...)) }
	if !taskNamePattern.MatchString(t.Name) {
		add("name: %q must be lowercase letters, digits and hyphens, starting with a letter", t.Name)
	}
	if !slices.Contains(Tiers(), t.Tier) {
		add("tier: %q is not one of %v", t.Tier, Tiers())
	}
	for member, value := range map[string]string{
		"fixture": t.Fixture, "spec": t.Spec, "package": t.Package,
		"truth.reference": t.Truth.Reference, "truth.violations": t.Truth.Violations, "truth.refactors": t.Truth.Refactors,
		"truth.oracle": t.Truth.Oracle, "truth.second_oracle": t.Truth.SecondOracle,
	} {
		if !relative(value) {
			add("%s: %q must be a clean relative path", member, value)
		}
	}
	found = append(found, t.criteriaProblems()...)
	if t.Prompt == "" {
		add("prompt: required")
	}
	found = append(found, t.interfaceProblems()...)
	slices.Sort(found)
	return found
}

func (t Task) criteriaProblems() []string {
	var found []string
	if len(t.Criteria) != TaskCriteria {
		return []string{fmt.Sprintf("criteria: exactly %d task criteria are required, there are %d", TaskCriteria, len(t.Criteria))}
	}
	news, revised := 0, 0
	for index, criterion := range t.Criteria {
		if !criterionPattern.MatchString(criterion.ID) {
			found = append(found, fmt.Sprintf("criteria[%d].id: %q is not an ID such as RATE-4", index, criterion.ID))
		}
		switch criterion.Kind {
		case KindNew:
			news++
		case KindRevised:
			revised++
		default:
			found = append(found, fmt.Sprintf("criteria[%d].kind: %q is neither %q nor %q", index, criterion.Kind, KindNew, KindRevised))
		}
	}
	if t.Criteria[0].ID == t.Criteria[1].ID {
		found = append(found, fmt.Sprintf("criteria: %s is named twice", t.Criteria[0].ID))
	}
	if revised > 1 {
		found = append(found, "criteria: at most one task criterion is revised, the other is new")
	}
	return found
}

func (t Task) interfaceProblems() []string {
	var found []string
	if n := len(t.Interface); n < MinOperations || n > MaxOperations {
		found = append(found, fmt.Sprintf("interface: %d to %d operations are required, there are %d", MinOperations, MaxOperations, n))
	}
	if len(t.Operations) != len(t.Interface) {
		found = append(found, fmt.Sprintf("operations: %d bindings for %d operations of the interface", len(t.Operations), len(t.Interface)))
	}
	for index, operation := range t.Interface {
		if !dottedName.MatchString(operation.Module) {
			found = append(found, fmt.Sprintf("interface[%d].module: %q is not a dotted module path", index, operation.Module))
		}
		if !plainName.MatchString(operation.Operation) {
			found = append(found, fmt.Sprintf("interface[%d].operation: %q is not a name", index, operation.Operation))
		}
		if !strings.HasPrefix(operation.Signature, operation.Operation+"(") {
			found = append(found, fmt.Sprintf("interface[%d].signature: %q does not start with the operation's name and its arguments", index, operation.Signature))
		}
		binding := t.Operations[operation.Operation]
		want := operation.Module + l1accept.BindingSeparator + operation.Operation
		if len(binding) != 1 || binding[l1accept.BindingLanguage] != want {
			found = append(found, fmt.Sprintf("operations.%s: must be {%q: %q}", operation.Operation, l1accept.BindingLanguage, want))
		}
	}
	return found
}

// relative reports whether a path is clean, relative and inside its root.
func relative(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}

// IDs are the task criteria's IDs, in order.
func (t Task) IDs() []string {
	ids := make([]string, 0, len(t.Criteria))
	for _, criterion := range t.Criteria {
		ids = append(ids, criterion.ID)
	}
	return ids
}

// IsTaskCriterion reports whether a criterion is one of the task's.
func (t Task) IsTaskCriterion(id string) bool { return slices.Contains(t.IDs(), id) }

// Revised returns the revised task criterion, when the task has one.
func (t Task) Revised() (Criterion, bool) {
	at := slices.IndexFunc(t.Criteria, func(c Criterion) bool { return c.Kind == KindRevised })
	if at < 0 {
		return Criterion{}, false
	}
	return t.Criteria[at], true
}

// New returns the new task criteria, in order.
func (t Task) New() []Criterion {
	var found []Criterion
	for _, criterion := range t.Criteria {
		if criterion.Kind == KindNew {
			found = append(found, criterion)
		}
	}
	return found
}

// Modules maps each operation of the interface to its module.
func (t Task) Modules() map[string]string {
	modules := make(map[string]string, len(t.Interface))
	for _, operation := range t.Interface {
		modules[operation.Operation] = operation.Module
	}
	return modules
}

// ConfigTask is the task's entry in the run configuration.
func (t Task) ConfigTask() config.Task {
	return config.Task{Name: t.Name, Fixture: t.Fixture, Prompt: t.Prompt, Operations: t.Operations}
}
