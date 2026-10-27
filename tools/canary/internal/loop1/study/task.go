// Package study holds the shapes that the verbs of the loop-1 study exchange: a
// task's description, the outcomes of a run, the deviation log, the records the
// analysis reads and the layout of the work directory. It is pure: it parses and
// formats bytes and reads nothing.
package study

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

// TaskFile is the name of a task's description inside its directory.
const TaskFile = "task.json"

// Truth is where a task's truth files lie, each path relative to the repository
// root. Only construct writes them; no run reads them.
type Truth struct {
	// Reference holds the source files of the fixture with the task criterion
	// implemented.
	Reference string `json:"reference"`
	// Violations holds one directory per criterion, each holding numbered
	// directories of single-criterion violations.
	Violations string `json:"violations"`
	// Refactors holds numbered directories of edits that keep every criterion.
	Refactors string `json:"refactors"`
	// Oracle holds one test file per criterion and its own pytest.ini.
	Oracle string `json:"oracle"`
}

// Interface is what the oracle writer is told about the code under test: the
// module path and the operation's signature.
type Interface struct {
	// Module is the dotted module path, such as boxoffice.prices.
	Module string `json:"module"`
	// Operation is the public operation's name.
	Operation string `json:"operation"`
	// Signature is the operation's Python signature, keyword arguments included.
	Signature string `json:"signature"`
}

// Task is the description of one task of the study.
type Task struct {
	// Name identifies the task in run names and records.
	Name string `json:"name"`
	// Fixture is the project the session starts from, relative to the
	// repository root.
	Fixture string `json:"fixture"`
	// Spec is the spec file, relative to the fixture.
	Spec string `json:"spec"`
	// Source is the source file the task criterion is implemented in, relative to
	// the fixture.
	Source string `json:"source"`
	// Criterion is the task criterion's ID, without its revision.
	Criterion string `json:"criterion"`
	// Prompt is what the session is asked, verbatim.
	Prompt string `json:"prompt"`
	// Operations binds each operation of the spec's examples to a target per
	// language, as `.musthold/config.yaml` reads it.
	Operations map[string]map[string]string `json:"operations"`
	Interface  Interface                    `json:"interface"`
	Truth      Truth                        `json:"truth"`
}

// Errors of a task description that cannot be used.
var (
	ErrMalformedTask = errors.New("the task description is not a JSON document of the expected shape")
	ErrInvalidTask   = errors.New("the task description is invalid")
)

var (
	taskNamePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	criterionPattern = regexp.MustCompile(`^[A-Z][A-Z0-9]*-[0-9]+$`)
	identifier       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
)

// ParseTask reads a task description and checks every member the verbs depend
// on, naming each member at fault. Unknown members are refused.
func ParseTask(data []byte) (Task, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var task Task
	if err := decoder.Decode(&task); err != nil {
		return Task{}, fmt.Errorf("%w: %v", ErrMalformedTask, err)
	}
	if decoder.More() {
		return Task{}, fmt.Errorf("%w: more than one document", ErrMalformedTask)
	}
	if problems := task.problems(); len(problems) > 0 {
		return Task{}, fmt.Errorf("%w: %s", ErrInvalidTask, strings.Join(problems, "; "))
	}
	return task, nil
}

func (t Task) problems() []string {
	var found []string
	add := func(format string, args ...any) { found = append(found, fmt.Sprintf(format, args...)) }
	if !taskNamePattern.MatchString(t.Name) {
		add("name: %q must be lowercase letters, digits and hyphens, starting with a letter", t.Name)
	}
	for member, value := range map[string]string{
		"fixture": t.Fixture, "spec": t.Spec, "source": t.Source,
		"truth.reference": t.Truth.Reference, "truth.violations": t.Truth.Violations,
		"truth.refactors": t.Truth.Refactors, "truth.oracle": t.Truth.Oracle,
	} {
		if !relative(value) {
			add("%s: %q must be a clean relative path", member, value)
		}
	}
	if !criterionPattern.MatchString(t.Criterion) {
		add("criterion: %q is not an ID such as BOX-4", t.Criterion)
	}
	if t.Prompt == "" {
		add("prompt: required")
	}
	if len(t.Operations) == 0 {
		add("operations: at least one operation is required")
	}
	for operation, targets := range t.Operations {
		if len(targets) == 0 {
			add("operations.%s: at least one language target is required", operation)
		}
	}
	if !identifier.MatchString(t.Interface.Module) {
		add("interface.module: %q is not a dotted module path", t.Interface.Module)
	}
	if !identifier.MatchString(t.Interface.Operation) || strings.Contains(t.Interface.Operation, ".") {
		add("interface.operation: %q is not a name", t.Interface.Operation)
	}
	if !strings.HasPrefix(t.Interface.Signature, t.Interface.Operation+"(") {
		add("interface.signature: %q does not start with the operation's name and its arguments", t.Interface.Signature)
	}
	slices.Sort(found)
	return found
}

// TruthBase is the directory of the fixture, slash-separated, that the files of
// the reference, of each violation and of each refactor are relative to: the
// directory of the source file.
func (t Task) TruthBase() string { return path.Dir(t.Source) }

// relative reports whether a path is clean, relative and inside its root.
func relative(p string) bool {
	return p != "" && !path.IsAbs(p) && path.Clean(p) == p && p != ".." && !strings.HasPrefix(p, "../")
}
