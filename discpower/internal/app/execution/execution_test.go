package execution_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	pythonmutation "github.com/RachidChabane/musthold/discpower/internal/adapters/mutation/python"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/coverage"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation/footprint"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/bindings"
	"github.com/RachidChabane/musthold/format"
)

const (
	fixtureDirectory = "../../../fixtures/ecosystems/pytest"
	sourceFile       = "parcel/rates.py"
	sourceRoot       = "parcel"
	pythonCommand    = "python3"
	pytestImport     = "import pytest"
	suiteKey         = "suite-key"
	drawSeed         = 7
	ratesModule      = "parcel.rates:"
	lineTotalTarget  = ratesModule + "line_total"
	shippingTarget   = ratesModule + "shipping_fee"
	discountedTarget = ratesModule + "discounted"
	lineTotal        = "line_total"
	shippingFee      = "shipping_fee"
	discounted       = "discounted"
	unboundOperation = "quote"
	productOfFourBy3 = "12"
	sumOfFourAnd3    = "7"
	shippingWaived   = "0"
	waivedFrom       = "50"
	paidShipping     = "10"
	minimumWitnesses = 1
	rerunCount       = 2
	singleRun        = 1
	skipNoSignature  = "no_signature"
)

var (
	criterionLine     = spec.Ref{ID: "PAR-1", Revision: 1}
	criterionDiscount = spec.Ref{ID: "PAR-2", Revision: 1}
	criterionShipping = spec.Ref{ID: "PAR-3", Revision: 1}

	killingEdit = edit(lineTotal, "def line_total(unit_price, quantity):\n    return unit_price + quantity")
	equivalent  = edit("clamp_percent", "def clamp_percent(percent):\n    return min(MAX_PERCENT, max(0, percent))")
	broken      = edit(lineTotal, "def line_total(unit_price, quantity):\n    return unit_price *")
)

func edit(qualname, source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: sourceFile, Qualname: qualname, NewSource: source}}}
}

func moduleAddition() mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: sourceFile, Qualname: "EXTRA", Target: mutation.TargetModuleAddition, NewSource: "EXTRA = 0"}}}
}

func integer(value string) spec.Literal { return spec.Literal{Type: spec.TypeInteger, Value: value} }

func returnsInteger(value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: spec.TypeInteger, Value: value}
}

func lineTotalCall() spec.Call {
	return spec.Call{Operation: lineTotal, Args: map[string]spec.Literal{"unit_price": integer("4"), "quantity": integer("3")}}
}

func shippingCall(total string) spec.Call {
	return spec.Call{Operation: shippingFee, Args: map[string]spec.Literal{"total": integer(total)}}
}

func example(index int, call spec.Call, expected spec.Observation) spec.Example {
	return spec.Example{Index: index, Call: call, Expect: expected}
}

// decoratedEcosystem is the real strategy with another suite runner.
type decoratedEcosystem struct {
	ports.Ecosystem
	runner ports.SuiteRunner
}

func (d decoratedEcosystem) Runner() ports.SuiteRunner { return d.runner }

// countingRunner delegates to a runner and records the selections it was given.
type countingRunner struct {
	inner      ports.SuiteRunner
	selections []ecosystem.Selection
}

func (c *countingRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	c.selections = append(c.selections, selection)
	return c.inner.Run(ctx, workspace, toolchain, selection)
}

// failingRunner reports every test it is asked about as failed, whatever the code.
type failingRunner struct {
	tests      []ecosystem.BoundTest
	selections []ecosystem.Selection
}

func (f *failingRunner) Run(_ context.Context, _ ecosystem.Workspace, _ ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	f.selections = append(f.selections, selection)
	outcomes := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for _, test := range f.tests {
		outcomes[test.ID] = ecosystem.TestFailed
	}
	return ecosystem.RunResult{Outcomes: outcomes}, nil
}

// failOnceRunner runs the real runner the first time and reports every test as
// passed afterwards: a kill that does not repeat.
type failOnceRunner struct {
	inner ports.SuiteRunner
	calls int
}

func (f *failOnceRunner) Run(ctx context.Context, workspace ecosystem.Workspace, toolchain ecosystem.Toolchain, selection ecosystem.Selection) (ecosystem.RunResult, error) {
	f.calls++
	result, err := f.inner.Run(ctx, workspace, toolchain, selection)
	if f.calls == 1 || err != nil {
		return result, err
	}
	passed := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for test := range result.Outcomes {
		passed[test] = ecosystem.TestPassed
	}
	return ecosystem.RunResult{Outcomes: passed}, nil
}

// stubProber answers every call with one observation and counts its use.
type stubProber struct {
	observation spec.Observation
	invokes     int
}

func (s *stubProber) Signatures(context.Context, ecosystem.Workspace, ecosystem.Toolchain, []string) ([]ports.ProbeSignature, error) {
	return nil, nil
}

func (s *stubProber) Invoke(_ context.Context, _ ecosystem.Workspace, _ ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	s.invokes++
	observed := make([]spec.Observation, len(calls))
	for index := range calls {
		observed[index] = s.observation
	}
	return observed, nil
}

// countingMemo is an in-memory ports.ExecutionMemo that counts its use.
type countingMemo struct {
	values map[string][]byte
	puts   int
}

func newCountingMemo() *countingMemo { return &countingMemo{values: map[string][]byte{}} }

func (m *countingMemo) Get(key string) ([]byte, bool, error) {
	value, found := m.values[key]
	return value, found, nil
}

func (m *countingMemo) Put(key string, value []byte) error {
	m.puts++
	m.values[key] = value
	return nil
}

// world is the real pytest strategy over the fixture project, with the reference
// workspace prepared and built once for every case.
type world struct {
	strategy  *pytest.Strategy
	executor  mutantrun.Executor
	reference mutantrun.Reference
	bindings  bindings.Bindings
	tests     []ecosystem.BoundTest
	sources   []string
}

func requirePython(t *testing.T) {
	t.Helper()
	if testing.Short() {
		t.Skip("toolchain integration tests are skipped in short mode")
	}
	if err := exec.Command(pythonCommand, "-c", pytestImport).Run(); err != nil {
		t.Skipf("%s with pytest is not usable here: %v", pythonCommand, err)
	}
}

func openWorld(t *testing.T) *world {
	t.Helper()
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := pytest.New(pytest.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator(), Assignments: pythonmutation.New()})
	executor := mutantrun.Executor{
		Ecosystem: strategy, Workspaces: manager,
		Policy: mutation.Policy{SourceRoots: []string{sourceRoot}, Layout: strategy.Layout()},
	}
	ctx := context.Background()
	reference, err := executor.PrepareReference(ctx, filepath.Clean(fixtureDirectory))
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	if build, err := executor.BuildReference(ctx, reference, []string{sourceFile}); err != nil || !build.OK {
		t.Fatalf("build the reference: %+v %v", build, err)
	}
	loaded, err := bindings.Load(ecosystem.Pytest)
	if err != nil {
		t.Fatal(err)
	}
	source, err := os.ReadFile(filepath.Join(fixtureDirectory, sourceFile))
	if err != nil {
		t.Fatal(err)
	}
	return &world{strategy: strategy, executor: executor, reference: reference, bindings: loaded, tests: loaded.All(), sources: []string{string(source)}}
}

func (w *world) id(ref spec.Ref) ecosystem.TestID { return w.bindings[ref.String()][0].ID }

func (w *world) idsOf(ref spec.Ref) []ecosystem.TestID {
	var ids []ecosystem.TestID
	for _, test := range w.bindings[ref.String()] {
		ids = append(ids, test.ID)
	}
	return ids
}

// runner builds the execution runner over the world and applies the options.
func (w *world) runner(options ...func(*execution.Runner)) execution.Runner {
	runner := execution.Runner{Executor: w.executor, Reference: w.reference, Tests: w.tests, SuiteKey: suiteKey}
	for _, option := range options {
		option(&runner)
	}
	return runner
}

func (w *world) withSuite(suite ports.SuiteRunner) func(*execution.Runner) {
	return func(r *execution.Runner) {
		r.Executor.Ecosystem = decoratedEcosystem{Ecosystem: w.strategy, runner: suite}
	}
}

func withProber(prober ports.Prober) func(*execution.Runner) {
	return func(r *execution.Runner) { r.Prober = prober }
}

func withOperations(operations execution.Operations) func(*execution.Runner) {
	return func(r *execution.Runner) { r.Operations = operations }
}

func withExamples(examples ...execution.OwnedExample) func(*execution.Runner) {
	return func(r *execution.Runner) { r.Examples = examples }
}

func withCoverage(units coverage.Map) func(*execution.Runner) {
	return func(r *execution.Runner) { r.Coverage = units }
}

func withMemo(memo ports.ExecutionMemo) func(*execution.Runner) {
	return func(r *execution.Runner) { r.Memo = memo }
}

func boundOperations() execution.Operations {
	return execution.Operations{lineTotal: lineTotalTarget, shippingFee: shippingTarget, discounted: discountedTarget}
}

func passingExamples() []execution.OwnedExample {
	return []execution.OwnedExample{
		{Owner: criterionLine, Example: example(1, lineTotalCall(), returnsInteger(productOfFourBy3))},
		{Owner: criterionShipping, Example: example(1, shippingCall(waivedFrom), returnsInteger(shippingWaived))},
	}
}

func unit(qualname string) footprint.Ref { return footprint.Ref{File: sourceFile, Qualname: qualname} }

// coverageOf records which units the tests of each criterion executed.
func (w *world) coverageOf() coverage.Map {
	units := coverage.Map{}
	for ref, executed := range map[spec.Ref][]footprint.Ref{
		criterionLine:     {unit(lineTotal)},
		criterionDiscount: {unit(discounted), unit("clamp_percent")},
		criterionShipping: {unit(shippingFee)},
	} {
		for _, id := range w.idsOf(ref) {
			units[id] = executed
		}
	}
	return units
}

func observe(t *testing.T, runner execution.Runner, mutant execution.MutantSpec) execution.Execution {
	t.Helper()
	got, err := runner.Observe(context.Background(), mutant)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func TestExecution(t *testing.T) {
	requirePython(t)
	w := openWorld(t)
	ctx := context.Background()

	t.Run("a killing mutant is killed by the tests of its criterion, rerun once, and fails the example of that criterion [verifies SP-175~1]", func(t *testing.T) {
		runner := &countingRunner{inner: w.strategy.Runner()}
		got := observe(t, w.runner(w.withSuite(runner), withProber(w.strategy.Prober()), withOperations(boundOperations()), withExamples(passingExamples()...)), execution.MutantSpec{ID: "killing", Edits: killingEdit})
		if !got.Built || got.Outcomes[string(w.id(criterionLine))] != format.OutcomeKilled {
			t.Fatalf("execution %+v", got)
		}
		for _, ref := range []spec.Ref{criterionDiscount, criterionShipping} {
			for _, id := range w.idsOf(ref) {
				if got.Outcomes[string(id)] != format.OutcomeSurvived {
					t.Errorf("%s is %s, want survived", id, got.Outcomes[string(id)])
				}
			}
		}
		if len(runner.selections) != rerunCount {
			t.Fatalf("%d suite runs, want %d (the run and the rerun of the kill)", len(runner.selections), rerunCount)
		}
		if !slices.Equal(got.Failed, []spec.Ref{criterionLine}) {
			t.Fatalf("failed examples %v", got.Failed)
		}
	})

	t.Run("an equivalent mutant survives every test without a rerun and fails no example [verifies SP-175~1]", func(t *testing.T) {
		runner := &countingRunner{inner: w.strategy.Runner()}
		got := observe(t, w.runner(w.withSuite(runner), withProber(w.strategy.Prober()), withOperations(boundOperations()), withExamples(passingExamples()...)), execution.MutantSpec{ID: "equivalent", Edits: equivalent})
		for test, outcome := range got.Outcomes {
			if outcome != format.OutcomeSurvived {
				t.Errorf("%s is %s", test, outcome)
			}
		}
		if !got.Built || len(got.Outcomes) != len(w.tests) || len(got.Failed) != 0 || len(runner.selections) != singleRun {
			t.Fatalf("execution %+v after %d runs", got, len(runner.selections))
		}
	})

	t.Run("a mutant that does not build is reported unbuilt with no outcome [verifies SP-175~1]", func(t *testing.T) {
		got := observe(t, w.runner(), execution.MutantSpec{ID: "broken", Edits: broken})
		if got.Built || got.Diagnostics == "" || len(got.Outcomes) != 0 {
			t.Fatalf("execution %+v", got)
		}
	})

	t.Run("a kill that does not repeat on the rerun counts as survived [verifies SP-175~1]", func(t *testing.T) {
		runner := &failOnceRunner{inner: w.strategy.Runner()}
		got := observe(t, w.runner(w.withSuite(runner)), execution.MutantSpec{ID: "flaky", Edits: killingEdit})
		if runner.calls != rerunCount {
			t.Fatalf("%d suite runs", runner.calls)
		}
		for test, outcome := range got.Outcomes {
			if outcome != format.OutcomeSurvived {
				t.Errorf("%s is %s: a kill that did not repeat must not count", test, outcome)
			}
		}
	})

	selections := []struct {
		name     string
		mutant   execution.MutantSpec
		coverage func(*world) coverage.Map
		want     func(*world) []ecosystem.TestID
	}{
		{"a definition edit runs only the tests that executed it",
			execution.MutantSpec{ID: "covered", Edits: killingEdit}, (*world).coverageOf,
			func(w *world) []ecosystem.TestID { return w.idsOf(criterionLine) }},
		{"a nested definition of the edited one counts as executed",
			execution.MutantSpec{ID: "nested", Edits: equivalent}, func(w *world) coverage.Map {
				units := coverage.Map{}
				for _, id := range w.idsOf(criterionDiscount) {
					units[id] = []footprint.Ref{unit("clamp_percent.inner")}
				}
				for _, id := range w.idsOf(criterionLine) {
					units[id] = []footprint.Ref{unit(lineTotal)}
				}
				return units
			}, func(w *world) []ecosystem.TestID { return w.idsOf(criterionDiscount) }},
		{"a sentinel runs every test", execution.MutantSpec{ID: "sentinel", Edits: killingEdit, FullSuite: true, Sentinel: true}, (*world).coverageOf, (*world).allIDs},
		{"a module-level edit runs every test", execution.MutantSpec{ID: "module", Edits: moduleAddition()}, (*world).coverageOf, (*world).allIDs},
		{"an unknown coverage runs every test", execution.MutantSpec{ID: "unknown", Edits: killingEdit}, func(*world) coverage.Map { return nil }, (*world).allIDs},
		{"an edit no test executed runs every test", execution.MutantSpec{ID: "uncovered", Edits: equivalent}, func(w *world) coverage.Map {
			units := coverage.Map{}
			for _, id := range w.idsOf(criterionLine) {
				units[id] = []footprint.Ref{unit(lineTotal)}
			}
			return units
		}, (*world).allIDs},
	}
	for _, tt := range selections {
		t.Run(tt.name+" and reports the others as survived [verifies SP-175~1]", func(t *testing.T) {
			suite := &failingRunner{tests: w.tests}
			got := observe(t, w.runner(w.withSuite(suite), withCoverage(tt.coverage(w))), tt.mutant)
			want := tt.want(w)
			if len(suite.selections) != rerunCount {
				t.Fatalf("%d suite runs", len(suite.selections))
			}
			if selected := suite.selections[0].Tests; !sameTests(selected, want) {
				t.Fatalf("selected %v, want %v", selected, want)
			}
			for _, test := range w.tests {
				expected := format.OutcomeSurvived
				if slices.Contains(want, test.ID) {
					expected = format.OutcomeKilled
				}
				if got.Outcomes[string(test.ID)] != expected {
					t.Errorf("%s is %s, want %s", test.ID, got.Outcomes[string(test.ID)], expected)
				}
			}
			if full := len(want) == len(w.allIDs()); full != (len(suite.selections[0].Args) == 0) {
				t.Errorf("a full run selects no runner argument, a partial one does: args %v", suite.selections[0].Args)
			}
		})
	}

	t.Run("a sentinel runs no example [verifies SP-175~1]", func(t *testing.T) {
		prober := &stubProber{observation: returnsInteger(sumOfFourAnd3)}
		runner := w.runner(w.withSuite(&failingRunner{tests: w.tests}), withProber(prober), withOperations(boundOperations()), withExamples(passingExamples()...))
		got := observe(t, runner, execution.MutantSpec{ID: "sentinel-examples", Edits: killingEdit, FullSuite: true, Sentinel: true})
		if prober.invokes != 0 || len(got.Failed) != 0 {
			t.Fatalf("examples ran for a sentinel: %d invokes, failed %v", prober.invokes, got.Failed)
		}
		other := observe(t, runner, execution.MutantSpec{ID: "regular-examples", Edits: killingEdit, FullSuite: true})
		if prober.invokes != 1 || !slices.Equal(other.Failed, []spec.Ref{criterionLine, criterionShipping}) {
			t.Fatalf("a regular mutant: %d invokes, failed %v", prober.invokes, other.Failed)
		}
	})

	t.Run("the memoized result of a mutant is returned without executing anything [verifies SP-175~1]", func(t *testing.T) {
		suite := &failingRunner{tests: w.tests}
		memo := newCountingMemo()
		runner := w.runner(w.withSuite(suite), withMemo(memo))
		mutant := execution.MutantSpec{ID: "memoized", Edits: killingEdit}
		first := observe(t, runner, mutant)
		runs := len(suite.selections)
		second := observe(t, runner, mutant)
		if len(suite.selections) != runs || memo.puts != 1 {
			t.Fatalf("the second observation ran the suite: %d then %d runs, %d puts", runs, len(suite.selections), memo.puts)
		}
		if !slices.Equal(outcomeList(first), outcomeList(second)) || second.Built != first.Built {
			t.Fatalf("memoized %+v differs from %+v", second, first)
		}
		observe(t, runner, execution.MutantSpec{ID: "another", Edits: killingEdit})
		if len(suite.selections) == runs {
			t.Fatal("another mutant was served from the memo")
		}
		other := w.runner(w.withSuite(suite), withMemo(memo))
		other.SuiteKey = "another-suite"
		before := len(suite.selections)
		observe(t, other, mutant)
		if len(suite.selections) == before {
			t.Fatal("another suite key was served from the memo")
		}
	})

	t.Run("reference examples report which ones pass on the reference and skip unbound operations [verifies SP-176~1]", func(t *testing.T) {
		examples := append(passingExamples(),
			execution.OwnedExample{Owner: criterionShipping, Example: example(2, shippingCall(paidShipping), returnsInteger(shippingWaived))},
			execution.OwnedExample{Owner: criterionDiscount, Example: example(1, spec.Call{Operation: unboundOperation}, returnsInteger(shippingWaived))},
		)
		results, err := w.runner(withProber(w.strategy.Prober()), withOperations(boundOperations()), withExamples(examples...)).ReferenceExamples(ctx)
		if err != nil {
			t.Fatal(err)
		}
		want := []execution.ExampleResult{
			{Owner: criterionLine, Index: 1, Passed: true},
			{Owner: criterionShipping, Index: 1, Passed: true},
			{Owner: criterionShipping, Index: 2, Passed: false},
		}
		if !slices.Equal(results, want) {
			t.Fatalf("results %+v, want %+v", results, want)
		}
	})

	t.Run("the draws of a search run on the reference and a killing mutant differs where an equivalent one never does [verifies SP-176~1]", func(t *testing.T) {
		operations := execution.Operations{lineTotal: lineTotalTarget, discounted: discountedTarget}
		runner := w.runner(withProber(w.strategy.Prober()), withOperations(operations), withExamples(passingExamples()...))
		draws, reason, err := runner.PrepareDraws(ctx, w.sources, drawSeed)
		if err != nil || reason != "" || draws == nil {
			t.Fatalf("draws %+v, reason %q, error %v", draws, reason, err)
		}
		if len(draws.Calls) == 0 || len(draws.Reference) != len(draws.Calls) || len(draws.Signatures) != len(operations) || draws.Digest == "" {
			t.Fatalf("draws %+v", draws)
		}

		killing, err := runner.Differential(ctx, execution.MutantSpec{ID: "diff-killing", Edits: killingEdit}, draws, "")
		if err != nil || killing.Status != power.DiffersAt || len(killing.Witnesses) < minimumWitnesses {
			t.Fatalf("killing mutant: %+v (%v)", killing, err)
		}
		for _, witness := range killing.Witnesses {
			if witness.Call.Operation != lineTotal || witness.Reference.Same(witness.Mutant) {
				t.Fatalf("witness %+v", witness)
			}
		}
		same, err := runner.Differential(ctx, execution.MutantSpec{ID: "diff-equivalent", Edits: equivalent}, draws, "")
		if err != nil || same.Status != power.NoDiffFound || len(same.Witnesses) != 0 {
			t.Fatalf("equivalent mutant: %+v (%v)", same, err)
		}
	})

	t.Run("a mutant that does not build is a skipped search [verifies SP-176~1]", func(t *testing.T) {
		draws := &execution.Draws{Calls: []spec.Call{lineTotalCall()}, Reference: []spec.Observation{returnsInteger(productOfFourBy3)}, Digest: "digest"}
		runner := w.runner(withProber(&stubProber{observation: returnsInteger(productOfFourBy3)}), withOperations(boundOperations()))
		got, err := runner.Differential(ctx, execution.MutantSpec{ID: "diff-broken", Edits: broken}, draws, "")
		if err != nil || got.Status != power.Skipped || got.SkipReason != execution.SkipDoesNotBuild {
			t.Fatalf("differential %+v (%v)", got, err)
		}
	})

	skips := []struct {
		name    string
		options []func(*execution.Runner)
		reason  string
	}{
		{"an ecosystem that cannot probe", nil, execution.SkipNoProber},
		{"no bound operation", []func(*execution.Runner){withProber(&stubProber{})}, execution.SkipNoOperation},
		{"no signature the prober knows", []func(*execution.Runner){withProber(&stubProber{}), withOperations(boundOperations())}, skipNoSignature},
	}
	for _, tt := range skips {
		t.Run("no draws are prepared for "+tt.name+" and the differential is skipped with the reason [verifies SP-176~1]", func(t *testing.T) {
			runner := w.runner(tt.options...)
			draws, reason, err := runner.PrepareDraws(ctx, w.sources, drawSeed)
			if err != nil || draws != nil || reason != tt.reason {
				t.Fatalf("draws %+v, reason %q, error %v", draws, reason, err)
			}
			got, err := runner.Differential(ctx, execution.MutantSpec{ID: "skipped", Edits: killingEdit}, draws, reason)
			if err != nil || got.Status != power.Skipped || got.SkipReason != tt.reason {
				t.Fatalf("differential %+v (%v)", got, err)
			}
		})
	}

	t.Run("the differential of a mutant is memoized [verifies SP-176~1]", func(t *testing.T) {
		prober := &stubProber{observation: returnsInteger(sumOfFourAnd3)}
		memo := newCountingMemo()
		runner := w.runner(withProber(prober), withOperations(boundOperations()), withMemo(memo))
		draws := &execution.Draws{Calls: []spec.Call{lineTotalCall()}, Reference: []spec.Observation{returnsInteger(productOfFourBy3)}, Digest: "digest"}
		mutant := execution.MutantSpec{ID: "memoized-search", Edits: killingEdit}
		for range rerunCount {
			if got, err := runner.Differential(ctx, mutant, draws, ""); err != nil || got.Status != power.DiffersAt {
				t.Fatalf("differential %+v (%v)", got, err)
			}
		}
		if prober.invokes != singleRun {
			t.Fatalf("the differential was executed %d times", prober.invokes)
		}
	})

	confirmations := []struct {
		name     string
		mutant   execution.MutantSpec
		call     spec.Call
		expected spec.Expectation
		options  func(*world) []func(*execution.Runner)
		want     power.WitnessCheck
	}{
		{"the reference returns what the judge expects and the mutant does not", execution.MutantSpec{ID: "confirm", Edits: killingEdit}, lineTotalCall(), returnsInteger(productOfFourBy3), realProber, power.WitnessConfirmed},
		{"the reference does not return what the judge expects", execution.MutantSpec{ID: "refuse-reference", Edits: killingEdit}, lineTotalCall(), returnsInteger(sumOfFourAnd3), realProber, power.WitnessRefused},
		{"the mutant returns what the judge expects too", execution.MutantSpec{ID: "refuse-mutant", Edits: equivalent}, lineTotalCall(), returnsInteger(productOfFourBy3), realProber, power.WitnessRefused},
		{"the operation is not bound", execution.MutantSpec{ID: "absent-operation", Edits: killingEdit}, spec.Call{Operation: unboundOperation}, returnsInteger(productOfFourBy3), realProber, power.WitnessAbsent},
		{"the ecosystem cannot probe", execution.MutantSpec{ID: "absent-prober", Edits: killingEdit}, lineTotalCall(), returnsInteger(productOfFourBy3), func(*world) []func(*execution.Runner) { return nil }, power.WitnessAbsent},
		{"the mutant does not build", execution.MutantSpec{ID: "absent-build", Edits: broken}, lineTotalCall(), returnsInteger(productOfFourBy3), realProber, power.WitnessAbsent},
	}
	for _, tt := range confirmations {
		t.Run("a witness is "+string(tt.want)+" when "+tt.name+" [verifies SP-176~1]", func(t *testing.T) {
			got, err := w.runner(tt.options(w)...).Confirm(ctx, tt.mutant, tt.call, tt.expected)
			if err != nil || got.Check != tt.want {
				t.Fatalf("confirmation %+v (%v)", got, err)
			}
			if (got.Check == power.WitnessConfirmed) != (got.Digest != "") {
				t.Fatalf("digest %q for %s", got.Digest, got.Check)
			}
		})
	}

	t.Run("a confirmed witness has a stable digest and is memoized [verifies SP-176~1]", func(t *testing.T) {
		prober := &stubProber{observation: returnsInteger(sumOfFourAnd3)}
		reference := &stubReference{prober: prober, expected: returnsInteger(productOfFourBy3)}
		memo := newCountingMemo()
		runner := w.runner(withProber(reference), withOperations(boundOperations()), withMemo(memo))
		mutant := execution.MutantSpec{ID: "memoized-witness", Edits: killingEdit}
		first, err := runner.Confirm(ctx, mutant, lineTotalCall(), returnsInteger(productOfFourBy3))
		if err != nil || first.Check != power.WitnessConfirmed {
			t.Fatalf("confirmation %+v (%v)", first, err)
		}
		invokes := reference.invokes
		second, err := runner.Confirm(ctx, mutant, lineTotalCall(), returnsInteger(productOfFourBy3))
		if err != nil || second != first || reference.invokes != invokes {
			t.Fatalf("second confirmation %+v executed %d more times (%v)", second, reference.invokes-invokes, err)
		}
	})
}

func realProber(w *world) []func(*execution.Runner) {
	return []func(*execution.Runner){withProber(w.strategy.Prober()), withOperations(boundOperations())}
}

// stubReference answers the reference workspace with the expected observation
// and every other workspace with the prober's own.
type stubReference struct {
	prober   *stubProber
	expected spec.Observation
	invokes  int
}

func (s *stubReference) Signatures(ctx context.Context, w ecosystem.Workspace, t ecosystem.Toolchain, targets []string) ([]ports.ProbeSignature, error) {
	return s.prober.Signatures(ctx, w, t, targets)
}

func (s *stubReference) Invoke(ctx context.Context, w ecosystem.Workspace, t ecosystem.Toolchain, calls []ports.ProbeCall) ([]spec.Observation, error) {
	s.invokes++
	observed, err := s.prober.Invoke(ctx, w, t, calls)
	if s.invokes == 1 {
		for index := range observed {
			observed[index] = s.expected
		}
	}
	return observed, err
}

func (w *world) allIDs() []ecosystem.TestID {
	ids := make([]ecosystem.TestID, len(w.tests))
	for index, test := range w.tests {
		ids[index] = test.ID
	}
	return ids
}

func sameTests(got, want []ecosystem.TestID) bool {
	left, right := slices.Clone(got), slices.Clone(want)
	slices.Sort(left)
	slices.Sort(right)
	return slices.Equal(left, right)
}

func outcomeList(e execution.Execution) []format.Outcome {
	keys := make([]string, 0, len(e.Outcomes))
	for key := range e.Outcomes {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	outcomes := make([]format.Outcome, len(keys))
	for index, key := range keys {
		outcomes[index] = e.Outcomes[key]
	}
	return outcomes
}
