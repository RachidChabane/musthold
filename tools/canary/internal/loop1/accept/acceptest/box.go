// Package acceptest provides a simulated container for the tests of the
// acceptance checks and of construction. It stands in for the box port: it reads
// the files of the tree a command runs in and answers as Python, pytest and
// Shallnot would, from markers that the test sources hold in comments.
//
// Markers in a source file outside tests/ and oracle/:
//
//	# BREAKS: X-1          the code violates criterion X-1; oracle tests of X-1 fail
//	# TESTS-MISS: X-1      the project's own tests do not notice that violation
//	# INLINE-MISS: X-1     a test made of the inline examples does not notice it
//	# NONDETERMINISTIC     the second run of any pytest command differs
//
// Markers in an oracle file: `# SYNTAX_ERROR` makes its collection fail and
// `# SKIPPED` makes its tests skip.
package acceptest

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/box"
	"github.com/RachidChabane/musthold/tools/canary/internal/loop1/study"
)

// Box is the simulated container.
type Box struct {
	t    testing.TB
	root string

	mu       sync.Mutex
	commands []box.Command
}

// NewBox builds a box that refuses any command whose directory or mount lies
// outside the work directory.
func NewBox(t testing.TB, workRoot string) *Box { return &Box{t: t, root: workRoot} }

// Commands lists the commands run so far.
func (b *Box) Commands() []box.Command {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]box.Command(nil), b.commands...)
}

// Run answers a command.
func (b *Box) Run(_ context.Context, command box.Command) (box.Result, error) {
	b.t.Helper()
	b.mu.Lock()
	b.commands = append(b.commands, command)
	b.mu.Unlock()
	b.checkMounts(command)
	switch {
	case isPytest(command.Argv):
		return b.pytest(command), nil
	case len(command.Argv) > 1 && strings.HasSuffix(command.Argv[0], "shallnot") && command.Argv[1] == "check":
		return b.shallnotCheck(command), nil
	case len(command.Argv) > 1 && strings.HasSuffix(command.Argv[0], "shallnot") && command.Argv[1] == "gate":
		return b.shallnotGate(command), nil
	case len(command.Argv) > 3 && command.Argv[1] == "-c":
		return b.introspect(command), nil
	}
	b.t.Errorf("the simulated box does not know the command %v", command.Argv)
	return box.Result{ExitCode: 127}, nil
}

func (b *Box) checkMounts(command box.Command) {
	b.t.Helper()
	within := func(location string) bool {
		relative, err := filepath.Rel(b.root, location)
		return err == nil && !strings.HasPrefix(relative, "..")
	}
	if !within(command.Dir) {
		b.t.Errorf("the command runs in %s, outside the work directory %s", command.Dir, b.root)
	}
	if len(command.Mounts) == 0 {
		b.t.Errorf("the command mounts nothing")
	}
	for _, mount := range command.Mounts {
		if !within(mount.Path) {
			b.t.Errorf("the command mounts %s, outside the work directory %s", mount.Path, b.root)
		}
	}
}

func isPytest(argv []string) bool {
	return len(argv) > 2 && argv[1] == "-m" && argv[2] == "pytest"
}

// source is what the markers of a tree say.
type source struct {
	breaks, testsMiss, inlineMiss map[string]bool
	nondeterministic              bool
}

func readSource(dir string) source {
	found := source{breaks: map[string]bool{}, testsMiss: map[string]bool{}, inlineMiss: map[string]bool{}}
	_ = filepath.WalkDir(dir, func(location string, entry os.DirEntry, err error) error {
		relative, _ := filepath.Rel(dir, location)
		if err != nil || entry.IsDir() || !strings.HasSuffix(location, ".py") ||
			strings.HasPrefix(relative, "tests/") || strings.HasPrefix(relative, "oracle/") {
			return nil
		}
		data, _ := os.ReadFile(location)
		for _, match := range markerPattern.FindAllStringSubmatch(string(data), -1) {
			switch match[1] {
			case "BREAKS":
				found.breaks[match[2]] = true
			case "TESTS-MISS":
				found.testsMiss[match[2]] = true
			case "INLINE-MISS":
				found.inlineMiss[match[2]] = true
			}
		}
		found.nondeterministic = found.nondeterministic || strings.Contains(string(data), "# NONDETERMINISTIC")
		return nil
	})
	return found
}

var (
	markerPattern   = regexp.MustCompile(`# (BREAKS|TESTS-MISS|INLINE-MISS): ([A-Z0-9-]+)`)
	testDefPattern  = regexp.MustCompile(`(?m)^def (test_\w+)\(`)
	citePattern     = regexp.MustCompile(`verifies\("([A-Z0-9-]+)~[0-9]+"\)`)
	declaration     = regexp.MustCompile(`^\s*[-*]\s+\*\*([A-Z][A-Z0-9]*-[0-9]+)~([0-9]+)\*\*:\s*(.*)$`)
	runNumberSuffix = regexp.MustCompile(`run-([0-9]+)\.xml$`)
)

type testCase struct{ module, name, criterion, status, message string }

func (b *Box) pytest(command box.Command) box.Result {
	collectOnly := false
	junit := ""
	oracle := false
	for index, arg := range command.Argv {
		collectOnly = collectOnly || arg == "--collect-only"
		oracle = oracle || (arg == "-c" && index+1 < len(command.Argv) && strings.HasPrefix(command.Argv[index+1], "oracle/"))
		if path, found := strings.CutPrefix(arg, "--junitxml="); found {
			junit = path
		}
	}
	if oracle {
		return b.oracleRun(command, collectOnly, junit)
	}
	return b.projectRun(command, junit)
}

func (b *Box) oracleRun(command box.Command, collectOnly bool, junit string) box.Result {
	files, _ := filepath.Glob(filepath.Join(command.Dir, "oracle", "test_*.py"))
	sort.Strings(files)
	state := readSource(command.Dir)
	var cases []testCase
	var stdout strings.Builder
	for _, file := range files {
		data, _ := os.ReadFile(file)
		module := strings.TrimSuffix(filepath.Base(file), ".py")
		if strings.Contains(string(data), "# SYNTAX_ERROR") {
			if collectOnly {
				fmt.Fprintf(&stdout, "ERROR %s.py - SyntaxError: invalid syntax\n", module)
			}
			cases = append(cases, testCase{module: "", name: module, status: "error", message: "SyntaxError"})
			continue
		}
		criterion := strings.ToUpper(strings.ReplaceAll(strings.TrimPrefix(module, "test_"), "_", "-"))
		for _, match := range testDefPattern.FindAllStringSubmatch(string(data), -1) {
			status := "passed"
			if state.breaks[criterion] {
				status = "failed"
			}
			if strings.Contains(string(data), "# SKIPPED") {
				status = "skipped"
			}
			cases = append(cases, testCase{module: module, name: match[1], criterion: criterion, status: status, message: "assert failed"})
			if collectOnly {
				fmt.Fprintf(&stdout, "%s.py::%s\n", module, match[1])
			}
		}
	}
	if collectOnly {
		if strings.Contains(stdout.String(), "ERROR") {
			return box.Result{ExitCode: 2, Stdout: []byte(stdout.String())}
		}
		fmt.Fprintf(&stdout, "\n%d tests collected in 0.01s\n", len(cases))
		return box.Result{Stdout: []byte(stdout.String())}
	}
	return b.writeJUnit(junit, cases, state)
}

func (b *Box) projectRun(command box.Command, junit string) box.Result {
	state := readSource(command.Dir)
	var cases []testCase
	files, _ := filepath.Glob(filepath.Join(command.Dir, "tests", "test_*.py"))
	sort.Strings(files)
	for _, file := range files {
		data, _ := os.ReadFile(file)
		module := "tests." + strings.TrimSuffix(filepath.Base(file), ".py")
		for _, block := range strings.Split(string(data), "\n\n\n") {
			for _, def := range testDefPattern.FindAllStringSubmatch(block, -1) {
				cases = append(cases, projectCase(module, def[1], block, state))
			}
		}
	}
	return b.writeJUnit(junit, cases, state)
}

func projectCase(module, name, block string, state source) testCase {
	c := testCase{module: module, name: name, status: "passed", message: "assert failed"}
	if strings.HasSuffix(module, "test_inline_examples") {
		for criterion := range state.breaks {
			if !state.inlineMiss[criterion] {
				c.status = "failed"
			}
		}
		return c
	}
	for _, cited := range citePattern.FindAllStringSubmatch(block, -1) {
		if state.breaks[cited[1]] && !state.testsMiss[cited[1]] {
			c.status = "failed"
		}
	}
	return c
}

func (b *Box) writeJUnit(junit string, cases []testCase, state source) box.Result {
	if state.nondeterministic {
		if match := runNumberSuffix.FindStringSubmatch(junit); match != nil && match[1] == "2" && len(cases) > 0 {
			cases[0].status = "failed"
		}
	}
	var xml strings.Builder
	xml.WriteString("<testsuites><testsuite>")
	failed := false
	for _, c := range cases {
		fmt.Fprintf(&xml, `<testcase classname=%q name=%q>`, c.module, c.name)
		switch c.status {
		case "failed":
			failed = true
			fmt.Fprintf(&xml, `<failure message=%q/>`, c.message)
		case "error":
			failed = true
			fmt.Fprintf(&xml, `<error message=%q/>`, c.message)
		case "skipped":
			xml.WriteString(`<skipped/>`)
		}
		xml.WriteString("</testcase>")
	}
	xml.WriteString("</testsuite></testsuites>")
	if err := os.MkdirAll(filepath.Dir(junit), 0o755); err != nil {
		b.t.Fatal(err)
	}
	if err := os.WriteFile(junit, []byte(xml.String()), 0o644); err != nil {
		b.t.Fatal(err)
	}
	if failed {
		return box.Result{ExitCode: 1}
	}
	return box.Result{}
}

type criterion struct{ id, ref, statement string }

// readSpec lists the criteria of a spec as Shallnot reports them.
func readSpec(path string) []criterion {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var found []criterion
	for _, line := range strings.Split(string(data), "\n") {
		if match := declaration.FindStringSubmatch(line); match != nil {
			found = append(found, criterion{id: match[1], ref: match[1] + "~" + match[2], statement: strings.TrimSpace(match[3])})
		} else if len(found) > 0 && strings.TrimSpace(line) != "" && !strings.HasPrefix(strings.TrimSpace(line), "#") {
			found[len(found)-1].statement += " " + strings.TrimSpace(line)
		}
	}
	return found
}

func (b *Box) shallnotCheck(command box.Command) box.Result {
	spec := ""
	for index, arg := range command.Argv {
		if arg == "--specs" && index+1 < len(command.Argv) {
			spec = command.Argv[index+1]
		}
	}
	report := map[string]any{"verdict": "fail", "exit_code": 1}
	var requirements, findings []map[string]any
	for _, c := range readSpec(filepath.Join(command.Dir, spec)) {
		requirements = append(requirements, map[string]any{"id": c.id, "ref": c.ref, "statement": c.statement, "status": "active"})
		findings = append(findings, map[string]any{"category": "uncovered_requirement", "requirement_id": c.id, "blocking": false})
	}
	report["requirements"], report["findings"] = requirements, findings
	data, _ := json.Marshal(report)
	return box.Result{Stdout: data}
}

func (b *Box) shallnotGate(command box.Command) box.Result {
	specs, _ := filepath.Glob(filepath.Join(command.Dir, "specs", "*.md"))
	cited := map[string]bool{}
	tests, _ := filepath.Glob(filepath.Join(command.Dir, "tests", "*.py"))
	for _, file := range tests {
		data, _ := os.ReadFile(file)
		for _, match := range citePattern.FindAllStringSubmatch(string(data), -1) {
			cited[match[1]] = true
		}
	}
	var findings []map[string]any
	for _, spec := range specs {
		for _, c := range readSpec(spec) {
			if !cited[c.id] {
				findings = append(findings, map[string]any{"category": "uncovered_requirement", "requirement_id": c.id, "blocking": true})
			}
		}
	}
	report := map[string]any{"verdict": "pass", "exit_code": 0, "findings": findings}
	exit := 0
	if len(findings) > 0 {
		report["verdict"], report["exit_code"], exit = "fail", 1, 1
	}
	data, _ := json.Marshal(report)
	return box.Result{ExitCode: exit, Stdout: data}
}

var functionPattern = regexp.MustCompile(`(?m)^def ([A-Za-z_]\w*)\(([^)]*)\)`)

func (b *Box) introspect(command box.Command) box.Result {
	module, operation := command.Argv[3], command.Argv[4]
	data, err := os.ReadFile(filepath.Join(command.Dir, strings.ReplaceAll(module, ".", "/")+".py"))
	report := map[string]any{"importable": err == nil, "callable": false, "public_functions": []string{}, "parameters": []map[string]string{}, "error": ""}
	if err != nil {
		report["error"] = "ModuleNotFoundError: No module named '" + module + "'"
	}
	var public []string
	for _, match := range functionPattern.FindAllStringSubmatch(string(data), -1) {
		if !strings.HasPrefix(match[1], "_") {
			public = append(public, match[1])
		}
		if match[1] == operation {
			report["callable"] = true
			var parameters []map[string]string
			for _, name := range strings.Split(match[2], ",") {
				if name = strings.TrimSpace(name); name != "" {
					parameters = append(parameters, map[string]string{"name": name, "kind": "POSITIONAL_OR_KEYWORD"})
				}
			}
			report["parameters"] = parameters
		}
	}
	if public != nil {
		report["public_functions"] = public
	}
	out, _ := json.Marshal(report)
	return box.Result{Stdout: out}
}

// Equipper is a stand-in for the equipping of the control arm: it copies the
// fixture's files to a project beside them and records each project.
type Equipper struct {
	mu       sync.Mutex
	Equipped []string
}

// ProjectSuffix names the project the stand-in equips beside a fixture's files.
const ProjectSuffix = "-equipped"

// EquipControl copies the directory to its project and records the project.
func (e *Equipper) EquipControl(_ context.Context, dir string, _ study.Task) (string, error) {
	project := dir + ProjectSuffix
	if err := os.RemoveAll(project); err != nil {
		return "", err
	}
	if err := os.CopyFS(project, os.DirFS(dir)); err != nil {
		return "", err
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Equipped = append(e.Equipped, project)
	return project, nil
}
