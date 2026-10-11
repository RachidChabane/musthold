package scan_test

import (
	"context"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	pythonmutation "github.com/RachidChabane/spec-proof/discpower/internal/adapters/mutation/python"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/operatorgen"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/scan"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/vacuity"
	"github.com/RachidChabane/spec-proof/format"
)

// The project the fakes scan: one module of two functions and four tests.
const (
	totals      = "invoice/totals.py"
	discountOp  = "invoice.totals:discount"
	headCommit  = "c0ffee0c0ffee0c0ffee0c0ffee0c0ffee0c0ffe"
	baseCommit  = "ba5eba5eba5eba5eba5eba5eba5eba5eba5eba5e"
	vacuousTest = "tests/test_discount.py::test_discount_accepts_any_percent"
	exactTest   = "tests/test_discount.py::test_discount_takes_the_percent_off"
	feeTest     = "tests/test_shipping.py::test_shipping_is_waived_from_fifty"
	redTest     = "tests/test_shipping.py::test_shipping_of_nothing"
)

// totalsSource is the module; its lines are numbered in the comments of the
// tests that run them.
const totalsSource = `def discount(total: int, percent: int) -> int:
    if percent <= 0:
        return total
    return total - total * percent // 100


def shipping(total: int) -> int:
    if total >= 50:
        return 0
    return 5
`

// fakeTest is a test of the fake suite: the lines of the module it runs and
// the lines whose text it requires, which it fails without.
type fakeTest struct {
	file   string
	lines  []int
	checks []int
	red    bool
}

func fakeTests() map[ecosystem.TestID]fakeTest {
	return map[ecosystem.TestID]fakeTest{
		vacuousTest: {file: "tests/test_discount.py", lines: []int{2, 3}},
		exactTest:   {file: "tests/test_discount.py", lines: []int{2, 4}, checks: []int{2, 4}},
		feeTest:     {file: "tests/test_shipping.py", lines: []int{8, 9, 10}, checks: []int{8, 9, 10}},
		redTest:     {file: "tests/test_shipping.py", lines: []int{8, 10}, red: true},
	}
}

func sourceLine(number int) string { return strings.Split(totalsSource, "\n")[number-1] }

// fakeWorkspaces keeps workspaces in memory.
type fakeWorkspaces struct {
	mutex   sync.Mutex
	files   map[string]map[string][]byte
	counter int
	project fs.FS
}

func (w *fakeWorkspaces) Prepare(_ context.Context, _ string, layout ecosystem.Layout) (ecosystem.Workspace, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	files := map[string][]byte{}
	err := fs.WalkDir(w.project, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		for _, excluded := range layout.ExcludedDirs {
			if strings.HasPrefix(path, excluded+"/") {
				return nil
			}
		}
		files[path], err = fs.ReadFile(w.project, path)
		return err
	})
	w.files = map[string]map[string][]byte{"reference": files}
	return ecosystem.Workspace{Dir: "reference"}, err
}

func (w *fakeWorkspaces) Clone(_ context.Context, reference ecosystem.Workspace, _ ecosystem.Layout) (ecosystem.Workspace, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.counter++
	dir := fmt.Sprintf("mutant-%d", w.counter)
	w.files[dir] = maps.Clone(w.files[reference.Dir])
	return ecosystem.Workspace{Dir: dir}, nil
}

func (w *fakeWorkspaces) ReadFile(workspace ecosystem.Workspace, file string) ([]byte, error) {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	content, found := w.files[workspace.Dir][file]
	if !found {
		return nil, fs.ErrNotExist
	}
	return content, nil
}

func (w *fakeWorkspaces) WriteFile(workspace ecosystem.Workspace, _ ecosystem.Layout, file string, content []byte) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	w.files[workspace.Dir][file] = content
	return nil
}

func (w *fakeWorkspaces) Release(workspace ecosystem.Workspace) error {
	w.mutex.Lock()
	defer w.mutex.Unlock()
	delete(w.files, workspace.Dir)
	return nil
}

func (w *fakeWorkspaces) source(workspace ecosystem.Workspace) string {
	content, _ := w.ReadFile(workspace, totals)
	return string(content)
}

// fakeEcosystem is a pytest-shaped strategy whose build check refuses the
// sources a test names.
type fakeEcosystem struct {
	workspaces *fakeWorkspaces
	broken     func(source string) bool
}

func (fakeEcosystem) Name() ecosystem.Name                     { return ecosystem.Pytest }
func (fakeEcosystem) Detect(fs.FS) (ecosystem.Detection, bool) { return ecosystem.Detection{}, true }
func (fakeEcosystem) Toolchain() ports.ToolchainResolver       { return fakeToolchain{} }
func (fakeEcosystem) Locator() ports.DefinitionLocator         { return treesitter.NewLocator() }
func (fakeEcosystem) Assignments() ports.AssignmentLocator     { return pythonmutation.New() }
func (fakeEcosystem) Selector() ports.TestSelector             { return nil }
func (fakeEcosystem) Runner() ports.SuiteRunner                { return nil }
func (e fakeEcosystem) BuildCheck() ports.BuildCheck           { return fakeBuild(e) }
func (fakeEcosystem) Layout() ecosystem.Layout {
	return ecosystem.Layout{TestFileGlobs: []string{"test_*.py"}}
}
func (fakeToolchain) Resolve(context.Context, ecosystem.Workspace) (ecosystem.Toolchain, error) {
	return ecosystem.Toolchain{Command: []string{"python3"}}, nil
}

type fakeToolchain struct{}

type fakeBuild fakeEcosystem

func (b fakeBuild) Check(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, _ []string) (ecosystem.BuildResult, error) {
	if b.broken != nil && b.broken(b.workspaces.source(workspace)) {
		return ecosystem.BuildResult{Diagnostics: "SyntaxError"}, nil
	}
	return ecosystem.BuildResult{OK: true}, nil
}

// fakeSuite runs the fake tests on the source of a workspace: a test passes
// when the lines it checks are still in the source.
type fakeSuite struct {
	workspaces *fakeWorkspaces
	tests      map[ecosystem.TestID]fakeTest
	mutex      sync.Mutex
	selections [][]ecosystem.TestID
	// coverageTimeout is the bound the coverage recording was given, and
	// coverageOverruns makes the recording outlive it.
	coverageTimeout  time.Duration
	coverageOverruns bool
	// covered lists the tests whose coverage was asked for.
	covered []ecosystem.TestID
}

func (s *fakeSuite) Run(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, request ports.ScanRunRequest) (ports.ScanRuns, error) {
	source := s.workspaces.source(workspace)
	result := ports.ScanRuns{Runs: map[ecosystem.TestID][]vacuity.Run{}}
	tests := request.Tests
	if len(tests) == 0 {
		tests = slices.Sorted(maps.Keys(s.tests))
		for _, id := range tests {
			result.Cases = append(result.Cases, ports.ScanCase{ID: id, File: s.tests[id].file})
		}
	} else {
		s.mutex.Lock()
		s.selections = append(s.selections, slices.Clone(tests))
		s.mutex.Unlock()
	}
	for _, id := range tests {
		test, known := s.tests[id]
		run := vacuity.RunPassed
		switch {
		case !known:
			run = vacuity.RunErrored
		case test.red || slices.ContainsFunc(test.checks, func(line int) bool { return !strings.Contains(source, sourceLine(line)) }):
			run = vacuity.RunFailed
		}
		result.Runs[id] = slices.Repeat([]vacuity.Run{run}, request.Runs)
	}
	return result, nil
}

func (s *fakeSuite) LineCoverage(_ context.Context, _ ecosystem.Workspace, _ ecosystem.Toolchain, request ports.ScanCoverageRequest) (map[ecosystem.TestID][]vacuity.Line, error) {
	s.coverageTimeout = request.Timeout
	for _, test := range request.Tests {
		s.covered = append(s.covered, test.ID)
	}
	if s.coverageOverruns {
		return nil, ports.ErrCoverageTimedOut
	}
	covered := map[ecosystem.TestID][]vacuity.Line{}
	for _, test := range request.Tests {
		for _, line := range s.tests[test.ID].lines {
			covered[test.ID] = append(covered[test.ID], vacuity.Line{File: totals, Line: line})
		}
	}
	return covered, nil
}

func (s *fakeSuite) Operation(_ ecosystem.Workspace, file, qualname string) (string, bool) {
	return discountOp, file == totals && qualname == "discount"
}

func (s *fakeSuite) Repro(test ecosystem.TestID) []string {
	return []string{"python3", "-m", "pytest", string(test)}
}

// fakeProber calls discount: its result is the text of the function the
// workspace holds, so every change of it differs on every input. A flaky
// prober gives a new result on each call of a mutant.
type fakeProber struct {
	workspaces *fakeWorkspaces
	flaky      bool
	mutex      sync.Mutex
	calls      int
}

func (p *fakeProber) Signatures(_ context.Context, _ ecosystem.Workspace, _ ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	var signatures []ports.ProbeSignature
	for _, target := range targets {
		if target == discountOp {
			signatures = append(signatures, ports.ProbeSignature{Target: target, Params: []differential.Param{
				{Name: "total", Hint: differential.HintInteger}, {Name: "percent", Hint: differential.HintInteger},
			}})
		}
	}
	return signatures, nil
}

func (p *fakeProber) Invoke(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()
	p.calls++
	source := p.workspaces.source(workspace)
	body := source[:strings.Index(source, "\n\n\n")]
	observations := make([]spec.Observation, len(calls))
	for index := range calls {
		value := format.SHA256Hex([]byte(body))[:8]
		if p.flaky && workspace.Dir != "reference" {
			value += fmt.Sprint(p.calls)
		}
		observations[index] = spec.Observation{Kind: spec.Returns, Type: spec.TypeString, Value: value}
	}
	return observations, nil
}

// fakeCompiler compiles a definition to its text with `<=` read as `<`, so a
// mutant that only drops the equality compiles to the reference's form.
// A folding compiler compiles every definition to one form.
type fakeCompiler struct {
	workspaces *fakeWorkspaces
	folds      bool
}

func (c fakeCompiler) Forms(_ context.Context, workspace ecosystem.Workspace, _ ecosystem.Toolchain, sites []string) (differential.CompiledForms, error) {
	source := c.workspaces.source(workspace)
	forms := differential.CompiledForms{}
	for _, site := range sites {
		forms[site] = strings.ReplaceAll(source, "<=", "<")
		if c.folds {
			forms[site] = "folded"
		}
	}
	return forms, nil
}

// fakeSource is the git state of the fake project.
type fakeSource struct {
	dirty   bool
	changed map[string][]int
}

func (s fakeSource) Head(context.Context) (string, error) { return headCommit, nil }
func (s fakeSource) Resolve(_ context.Context, revision string) (string, error) {
	return baseCommit, nil
}
func (s fakeSource) Dirty(context.Context) (bool, error) { return s.dirty, nil }
func (s fakeSource) TrackedFiles(context.Context) ([]string, error) {
	return []string{totals, "tests/test_discount.py", "tests/test_shipping.py", ".spec-proof/scan/old/report.md"}, nil
}
func (s fakeSource) ChangedLines(context.Context, string) (map[string][]int, error) {
	return s.changed, nil
}
func (s fakeSource) Name(context.Context) string { return "acme/invoicing" }

// clock advances by a step at each reading.
type clock struct {
	mutex sync.Mutex
	now   time.Time
	step  time.Duration
}

func (c *clock) read() time.Time {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.now = c.now.Add(c.step)
	return c.now
}

// fixture is a scan service over the fakes.
type fixture struct {
	workspaces *fakeWorkspaces
	suite      *fakeSuite
	prober     *fakeProber
	source     fakeSource
	broken     func(string) bool
	compiler   bool
	folds      bool
	clock      *clock
	capacity   ports.CapacityReader
}

func newFixture() *fixture {
	project := fstest.MapFS{
		totals:                           {Data: []byte(totalsSource)},
		"tests/test_discount.py":         {Data: []byte("from invoice.totals import discount\n\ndef test_discount_takes_the_percent_off():\n    assert discount(200, 10) == 180\n")},
		"tests/test_shipping.py":         {Data: []byte("from invoice.totals import shipping\n\ndef test_shipping_is_waived_from_fifty():\n    assert shipping(50) == 0\n")},
		".spec-proof/scan/old/report.md": {Data: []byte("an earlier report\n")},
	}
	workspaces := &fakeWorkspaces{project: project}
	return &fixture{
		workspaces: workspaces,
		suite:      &fakeSuite{workspaces: workspaces, tests: fakeTests()},
		prober:     &fakeProber{workspaces: workspaces},
		clock:      &clock{now: time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC), step: time.Millisecond},
	}
}

func (f *fixture) service(t *testing.T) *scan.Service {
	t.Helper()
	engines := operatorgen.NewRegistry()
	if err := engines.Register(pythonmutation.New(), treesitter.NewLocator()); err != nil {
		t.Fatal(err)
	}
	deps := scan.Deps{
		Ecosystem: fakeEcosystem{workspaces: f.workspaces, broken: f.broken}, Suite: f.suite, Workspaces: f.workspaces,
		Engines: engines, Prober: f.prober, Source: f.source, Project: f.workspaces.project, Clock: f.clock.read,
	}
	if f.compiler {
		deps.Compiler = fakeCompiler{workspaces: f.workspaces, folds: f.folds}
	}
	deps.Capacity = f.capacity
	return scan.New(deps)
}

// serviceWith is the scan service of the fixture on a machine of the given
// capacity.
func (f *fixture) serviceWith(t *testing.T, capacity ports.CapacityReader) *scan.Service {
	t.Helper()
	f.capacity = capacity
	return f.service(t)
}

func request() scan.Request {
	return scan.Request{
		Root: "/work/invoicing", Seed: 7, SampleSize: 40, Mutants: 150, Workers: 2,
		Module: format.Module{Name: "spec-proof-dp", Version: "0.1.0"},
	}
}
