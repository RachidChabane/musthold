// Package config is the configuration of the agent-loop runner: the tasks it
// gives an agent, the arms it equips their projects with, the repetitions and the
// seed that order the runs, the model and the caps of each session. It is pure:
// it parses bytes and computes the runs, and reads nothing.
package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"regexp"
	"slices"
)

// Arm is what a project is equipped with before the session starts.
type Arm string

// The arms the runner knows.
const (
	// ArmTraceability equips the project with Shallnot alone: the gate checks that
	// each criterion is cited by a test that ran and passed.
	ArmTraceability Arm = "traceability"
	// ArmFourProofs adds the discriminating-power, alignment and completeness
	// modules to Shallnot.
	ArmFourProofs Arm = "four-proofs"
)

// Arms lists every arm, in a fixed order.
func Arms() []Arm { return []Arm{ArmTraceability, ArmFourProofs} }

// WithModules reports whether the arm installs the proof modules.
func (a Arm) WithModules() bool { return a == ArmFourProofs }

// Task is one piece of work given to an agent: the project it starts from, what
// it is asked, and how the operations its spec's examples name bind to code.
type Task struct {
	// Name identifies the task in run directories and records.
	Name string `json:"name"`
	// Fixture is the project directory, relative to the repository root.
	Fixture string `json:"fixture"`
	// Prompt is what the session is asked, verbatim.
	Prompt string `json:"prompt"`
	// Operations binds each operation of the spec's examples to a target per
	// language, as the `operations` block of `.spec-proof/config.yaml` reads.
	Operations map[string]map[string]string `json:"operations"`
}

// Caps bound one session. The runner ends a session that reaches one.
type Caps struct {
	// AssistantMessages bounds the assistant messages of the session, its
	// subagents' included.
	AssistantMessages int `json:"assistant_messages"`
	// Seconds bounds the wall-clock time of the session.
	Seconds int `json:"seconds"`
	// BudgetUSD is what the CLI may spend, as its --max-budget-usd reads it.
	BudgetUSD string `json:"budget_usd"`
}

// Config is the whole configuration of a runner invocation.
type Config struct {
	Tasks []Task `json:"tasks"`
	Arms  []Arm  `json:"arms"`
	// Repetitions is how many times each task runs in each arm.
	Repetitions int `json:"repetitions"`
	// Seed orders the runs.
	Seed uint64 `json:"seed"`
	// Model is the model every session runs on.
	Model string `json:"model"`
	Caps  Caps   `json:"caps"`
	// AllowedTools are the permission rules of the session, in Claude Code's
	// `permissions.allow` syntax.
	AllowedTools []string `json:"allowed_tools"`
}

// Run is one session the configuration asks for.
type Run struct {
	Task Task
	Arm  Arm
	// Repetition counts from 1.
	Repetition int
}

// Name is the run's identity in directories and records.
func (r Run) Name() string { return fmt.Sprintf("%s-%s-%d", r.Task.Name, r.Arm, r.Repetition) }

// Errors of a configuration that cannot be used.
var (
	ErrMalformed = errors.New("the configuration is not a JSON document of the expected shape")
	ErrInvalid   = errors.New("the configuration is invalid")
)

var (
	namePattern   = regexp.MustCompile(`^[a-z][a-z0-9-]{0,39}$`)
	budgetPattern = regexp.MustCompile(`^[0-9]+\.[0-9]{2}$`)
)

// Parse reads a configuration and checks every member a run depends on. Unknown
// members are refused, so that a misspelt cap never goes unnoticed.
func Parse(data []byte) (Config, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var parsed Config
	if err := decoder.Decode(&parsed); err != nil {
		return Config{}, fmt.Errorf("%w: %v", ErrMalformed, err)
	}
	if decoder.More() {
		return Config{}, fmt.Errorf("%w: more than one document", ErrMalformed)
	}
	if problems := parsed.problems(); len(problems) > 0 {
		return Config{}, fmt.Errorf("%w: %s", ErrInvalid, joined(problems))
	}
	return parsed, nil
}

func (c Config) problems() []string {
	var found []string
	add := func(format string, args ...any) { found = append(found, fmt.Sprintf(format, args...)) }
	if len(c.Tasks) == 0 {
		add("tasks: at least one task is required")
	}
	seen := map[string]bool{}
	for index, task := range c.Tasks {
		found = append(found, task.problems(index)...)
		if seen[task.Name] {
			add("tasks[%d].name: %q names two tasks", index, task.Name)
		}
		seen[task.Name] = true
	}
	if len(c.Arms) == 0 {
		add("arms: at least one arm is required")
	}
	for index, arm := range c.Arms {
		if !slices.Contains(Arms(), arm) {
			add("arms[%d]: %q is not one of %v", index, arm, Arms())
		}
	}
	if c.Repetitions < 1 {
		add("repetitions: must be at least 1")
	}
	if c.Model == "" {
		add("model: required")
	}
	if c.Caps.AssistantMessages < 1 {
		add("caps.assistant_messages: must be at least 1")
	}
	if c.Caps.Seconds < 1 {
		add("caps.seconds: must be at least 1")
	}
	if !budgetPattern.MatchString(c.Caps.BudgetUSD) {
		add("caps.budget_usd: %q is not an amount with two decimals such as 12.50", c.Caps.BudgetUSD)
	}
	for index, rule := range c.AllowedTools {
		if rule == "" {
			add("allowed_tools[%d]: empty rule", index)
		}
	}
	return found
}

func (t Task) problems(index int) []string {
	var found []string
	add := func(format string, args ...any) {
		found = append(found, fmt.Sprintf("tasks[%d].", index)+fmt.Sprintf(format, args...))
	}
	if !namePattern.MatchString(t.Name) {
		add("name: %q must be lowercase letters, digits and hyphens, starting with a letter", t.Name)
	}
	if t.Fixture == "" {
		add("fixture: required")
	}
	if t.Prompt == "" {
		add("prompt: required")
	}
	for operation, targets := range t.Operations {
		if len(targets) == 0 {
			add("operations.%s: at least one language target is required", operation)
		}
	}
	return found
}

func joined(problems []string) string {
	var out bytes.Buffer
	for index, problem := range problems {
		if index > 0 {
			out.WriteString("; ")
		}
		out.WriteString(problem)
	}
	return out.String()
}

// Runs lists every run the configuration asks for: each task in each arm, as
// many times as the repetitions say, in an order the seed shuffles. The same
// configuration always gives the same order.
func (c Config) Runs() []Run {
	var runs []Run
	for _, task := range c.Tasks {
		for _, arm := range c.Arms {
			for repetition := 1; repetition <= c.Repetitions; repetition++ {
				runs = append(runs, Run{Task: task, Arm: arm, Repetition: repetition})
			}
		}
	}
	shuffle := rand.New(rand.NewPCG(c.Seed, c.Seed))
	shuffle.Shuffle(len(runs), func(i, j int) { runs[i], runs[j] = runs[j], runs[i] })
	return runs
}
