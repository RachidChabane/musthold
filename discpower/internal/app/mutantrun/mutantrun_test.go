package mutantrun_test

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/format"
)

const (
	sourceFile = "app.py"
	testFile   = "test_app.py"
	original   = "def f():\n    return 1"
	mutated    = "def f():\n    return 2"
	broken     = "def f():\n    BROKEN"

	brokenDiagnostic = "NameError"
)

var boundTest = ecosystem.NewBoundTest("test_app", "test_f", testFile, 1)

// memoryWorkspaces keeps workspaces in memory.
type memoryWorkspaces struct {
	files    map[string]map[string][]byte
	released []string
	next     int
}

func (m *memoryWorkspaces) Prepare(_ context.Context, _ string, _ ecosystem.Layout) (ecosystem.Workspace, error) {
	m.files = map[string]map[string][]byte{"reference": {sourceFile: []byte(original), testFile: []byte("def test_f():\n    assert f() == 1")}}
	return ecosystem.Workspace{Dir: "reference"}, nil
}

func (m *memoryWorkspaces) Clone(_ context.Context, reference ecosystem.Workspace, _ ecosystem.Layout) (ecosystem.Workspace, error) {
	m.next++
	dir := "mutant" + string(rune('0'+m.next))
	m.files[dir] = map[string][]byte{}
	for name, content := range m.files[reference.Dir] {
		m.files[dir][name] = append([]byte{}, content...)
	}
	return ecosystem.Workspace{Dir: dir}, nil
}

func (m *memoryWorkspaces) ReadFile(w ecosystem.Workspace, file string) ([]byte, error) {
	content, ok := m.files[w.Dir][file]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return content, nil
}

func (m *memoryWorkspaces) WriteFile(w ecosystem.Workspace, _ ecosystem.Layout, file string, content []byte) error {
	m.files[w.Dir][file] = content
	return nil
}

func (m *memoryWorkspaces) Release(w ecosystem.Workspace) error {
	m.released = append(m.released, w.Dir)
	return nil
}

// bodyLength is the length of a definition: its header line and the indented
// lines that follow.
func bodyLength(text string) int {
	length := 0
	for n, line := range strings.SplitAfter(text, "\n") {
		if n > 0 && !strings.HasPrefix(line, "    ") {
			break
		}
		length += len(strings.TrimRight(line, "\n"))
		if n > 0 {
			length++
		}
	}
	return length
}

type fakeLocator struct{}

func (fakeLocator) Locate(file string, source []byte) ([]ecosystem.Definition, error) {
	text := string(source)
	if strings.Contains(text, "SYNTAX") {
		return nil, errors.New("syntax error")
	}
	var definitions []ecosystem.Definition
	for _, name := range []string{"f", "test_f"} {
		if i := strings.Index(text, "def "+name+"("); i >= 0 {
			end := bodyLength(text[i:])
			definitions = append(definitions, ecosystem.Definition{Qualname: name, Span: ecosystem.Span{Start: i, End: i + end}, StartLine: 1, EndLine: 2})
		}
	}
	return definitions, nil
}

// fakeEcosystem is a Strategy whose build check and runner behave by the
// content of the spliced file.
type fakeEcosystem struct {
	workspaces  *memoryWorkspaces
	selections  []ecosystem.Selection
	assignments ports.AssignmentLocator
	// reported is what the build check reports on the reference.
	reported []string
	// baselines counts the times the build check recorded a baseline.
	baselines int
}

// takenName is the module assignment a fake assignment locator reports.
const takenName = "TAKEN"

type fakeAssignments struct{}

func (fakeAssignments) LocateAssignments(string, []byte) ([]ecosystem.Definition, error) {
	return []ecosystem.Definition{{Qualname: takenName, Kind: ecosystem.KindAssignment}}, nil
}

func (f *fakeEcosystem) Name() ecosystem.Name { return ecosystem.Pytest }
func (f *fakeEcosystem) Detect(fs.FS) (ecosystem.Detection, bool) {
	return ecosystem.Detection{}, false
}
func (f *fakeEcosystem) Layout() ecosystem.Layout {
	return ecosystem.Layout{TestFileGlobs: []string{"test_*.py"}}
}
func (f *fakeEcosystem) Toolchain() ports.ToolchainResolver   { return toolchain{} }
func (f *fakeEcosystem) Locator() ports.DefinitionLocator     { return fakeLocator{} }
func (f *fakeEcosystem) Assignments() ports.AssignmentLocator { return f.assignments }
func (f *fakeEcosystem) BuildCheck() ports.BuildCheck         { return buildCheck{f} }
func (f *fakeEcosystem) Selector() ports.TestSelector         { return f }
func (f *fakeEcosystem) Runner() ports.SuiteRunner            { return f }

func (f *fakeEcosystem) Select(bound []ecosystem.BoundTest, _ map[string][]ecosystem.Definition) (ecosystem.Selection, error) {
	selection := ecosystem.Selection{Args: []string{"bound"}}
	for _, test := range bound {
		selection.Tests = append(selection.Tests, test.ID)
	}
	return selection, nil
}

func (f *fakeEcosystem) Run(_ context.Context, w ecosystem.Workspace, _ ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	f.selections = append(f.selections, selection)
	outcome := ecosystem.TestPassed
	if strings.Contains(string(f.workspaces.files[w.Dir][sourceFile]), "return 2") {
		outcome = ecosystem.TestFailed
	}
	return ecosystem.RunResult{Outcomes: map[ecosystem.TestID]ecosystem.TestOutcome{boundTest.ID: outcome}}, nil
}

type toolchain struct{}

func (toolchain) Resolve(context.Context, ecosystem.Workspace) (ecosystem.Toolchain, error) {
	return ecosystem.Toolchain{Command: []string{"py"}}, nil
}

type buildCheck struct{ f *fakeEcosystem }

func (b buildCheck) Baseline(context.Context, ecosystem.Workspace, ecosystem.Toolchain) (ecosystem.BuildBaseline, error) {
	b.f.baselines++
	return ecosystem.NewBuildBaseline(b.f.reported), nil
}

func (b buildCheck) Check(_ context.Context, w ecosystem.Workspace, _ ecosystem.Toolchain, _ []string, baseline ecosystem.BuildBaseline) (ecosystem.BuildResult, error) {
	if strings.Contains(string(b.f.workspaces.files[w.Dir][sourceFile]), "BROKEN") && len(baseline.Introduced([]string{brokenDiagnostic})) > 0 {
		return ecosystem.BuildResult{Diagnostics: brokenDiagnostic}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}

func setup(t *testing.T) (mutantrun.Executor, mutantrun.Reference, *fakeEcosystem, *memoryWorkspaces) {
	t.Helper()
	return setupReporting(t, nil)
}

// setupReporting is setup for a reference whose build check reports diagnostics.
func setupReporting(t *testing.T, reported []string) (mutantrun.Executor, mutantrun.Reference, *fakeEcosystem, *memoryWorkspaces) {
	t.Helper()
	workspaces := &memoryWorkspaces{}
	eco := &fakeEcosystem{workspaces: workspaces, reported: reported}
	executor := mutantrun.Executor{
		Ecosystem: eco, Workspaces: workspaces,
		Policy: mutation.Policy{SourceRoots: []string{"."}, Layout: eco.Layout()},
	}
	reference, err := executor.PrepareReference(context.Background(), "project")
	if err != nil {
		t.Fatal(err)
	}
	return executor, reference, eco, workspaces
}

func request(source string) mutantrun.Request {
	return mutantrun.Request{
		Edits: mutation.EditSet{Edits: []mutation.Edit{{File: sourceFile, Qualname: "f", NewSource: source}}},
		Bound: []ecosystem.BoundTest{boundTest},
	}
}

func TestExecute(t *testing.T) {
	ctx := context.Background()

	t.Run("a mutant that fails a bound test is killed and its workspace released [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, _, workspaces := setup(t)
		result, err := executor.Execute(ctx, reference, request(mutated))
		if err != nil || result.Outcome != format.OutcomeKilled {
			t.Fatalf("%+v %v", result, err)
		}
		if len(workspaces.released) != 1 || string(workspaces.files["reference"][sourceFile]) != original {
			t.Fatalf("released %v, reference %q", workspaces.released, workspaces.files["reference"][sourceFile])
		}
	})

	t.Run("a mutant every bound test survives is survived [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, _, _ := setup(t)
		result, err := executor.Execute(ctx, reference, request(original))
		if err != nil || result.Outcome != format.OutcomeSurvived {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a failing build check is the outcome error with its diagnostics [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, eco, _ := setup(t)
		result, err := executor.Execute(ctx, reference, request(broken))
		if err != nil || result.Outcome != format.OutcomeError || result.Diagnostics != brokenDiagnostic || len(eco.selections) != 0 {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a build check the reference already fails is not held against a mutant [verifies SP-2440~1]", func(t *testing.T) {
		executor, reference, eco, _ := setupReporting(t, []string{brokenDiagnostic})
		result, err := executor.Execute(ctx, reference, request(broken))
		if err != nil || result.Outcome == format.OutcomeError || len(eco.selections) != 1 {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("the build baseline is recorded once for every mutant run on the reference [verifies SP-2440~1]", func(t *testing.T) {
		executor, reference, eco, _ := setupReporting(t, []string{brokenDiagnostic})
		for _, source := range []string{mutated, broken, original} {
			if _, err := executor.Execute(ctx, reference, request(source)); err != nil {
				t.Fatal(err)
			}
		}
		if eco.baselines != 1 {
			t.Fatalf("baselines recorded = %d", eco.baselines)
		}
	})

	t.Run("a splice that no longer parses is the outcome error [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, _, _ := setup(t)
		result, err := executor.Execute(ctx, reference, request("def f():\n    SYNTAX"))
		if err != nil || result.Outcome != format.OutcomeError || result.Diagnostics == "" {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("a module addition is spliced and a module assignment needs the strategy's assignment locator [verifies SP-84~1]", func(t *testing.T) {
		addition := mutation.Edit{File: sourceFile, Qualname: "EXTRA", Target: mutation.TargetModuleAddition, NewSource: "EXTRA = 0"}
		assignment := mutation.Edit{File: sourceFile, Qualname: takenName, Target: mutation.TargetModuleAssignment, NewSource: takenName + " = 1"}
		executor, reference, _, workspaces := setup(t)
		result, err := executor.Execute(ctx, reference, mutantrun.Request{Edits: mutation.EditSet{Edits: []mutation.Edit{addition}}, Bound: []ecosystem.BoundTest{boundTest}})
		if err != nil || result.Outcome != format.OutcomeSurvived || !strings.Contains(string(workspaces.files["mutant1"][sourceFile]), "EXTRA = 0") {
			t.Fatalf("%+v %v", result, err)
		}
		result, err = executor.Execute(ctx, reference, mutantrun.Request{Edits: mutation.EditSet{Edits: []mutation.Edit{assignment}}, Bound: []ecosystem.BoundTest{boundTest}})
		if err != nil || result.Outcome != format.OutcomeError || !strings.Contains(result.Diagnostics, mutation.ErrUnsupportedTarget.Error()) {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("an addition of a name the assignment locator reports is the outcome error [verifies SP-84~1]", func(t *testing.T) {
		executor, reference, eco, _ := setup(t)
		eco.assignments = fakeAssignments{}
		edit := mutation.Edit{File: sourceFile, Qualname: takenName, Target: mutation.TargetModuleAddition, NewSource: takenName + " = 0"}
		result, err := executor.Execute(ctx, reference, mutantrun.Request{Edits: mutation.EditSet{Edits: []mutation.Edit{edit}}, Bound: []ecosystem.BoundTest{boundTest}})
		if err != nil || result.Outcome != format.OutcomeError || !strings.Contains(result.Diagnostics, mutation.ErrNameTaken.Error()) {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("an unknown qualname is the outcome error [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, _, _ := setup(t)
		req := request(mutated)
		req.Edits.Edits[0].Qualname = "missing"
		result, err := executor.Execute(ctx, reference, req)
		if err != nil || result.Outcome != format.OutcomeError {
			t.Fatalf("%+v %v", result, err)
		}
	})

	t.Run("an edit in a test file is refused before any clone [verifies SP-62~1]", func(t *testing.T) {
		executor, reference, _, workspaces := setup(t)
		req := request(mutated)
		req.Edits.Edits[0].File = testFile
		if _, err := executor.Execute(ctx, reference, req); !mutation.IsRefusal(err) || workspaces.next != 0 {
			t.Fatalf("err = %v, clones = %d", err, workspaces.next)
		}
	})

	t.Run("a full-suite run selects nothing yet reads the bound tests [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, eco, _ := setup(t)
		req := request(mutated)
		req.FullSuite = true
		result, err := executor.Execute(ctx, reference, req)
		if err != nil || result.Outcome != format.OutcomeKilled || len(eco.selections[0].Args) != 0 {
			t.Fatalf("%+v %v selections=%v", result, err, eco.selections)
		}
	})
}

func TestReference(t *testing.T) {
	t.Run("the reference runs unmutated for correctCodePasses [verifies SP-82~1]", func(t *testing.T) {
		executor, reference, eco, _ := setup(t)
		build, err := executor.BuildReference(context.Background(), reference, []string{sourceFile})
		if err != nil || !build.OK {
			t.Fatalf("%+v %v", build, err)
		}
		run, err := executor.RunReference(context.Background(), reference, []ecosystem.BoundTest{boundTest}, false)
		if err != nil || run.Outcomes[boundTest.ID] != ecosystem.TestPassed || len(eco.selections) != 1 {
			t.Fatalf("%+v %v", run, err)
		}
	})
}
