package execution_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/testkit/bindings"
)

const (
	goFixtureDirectory = "../../../fixtures/ecosystems/go"
	goSourceFile       = "rates.go"
	goPackage          = "example.com/parcel:"
	goLineTotalTarget  = goPackage + "LineTotal"
	goDiscountedTarget = goPackage + "Discounted"
	goShippingTarget   = goPackage + "ShippingFee"
	goFreeShippingFrom = "5000"
	goFlatFee          = "450"
	goDrawSeed         = 11
)

var (
	goLineTotalEdit = goEdit("parcel.LineTotal", "func LineTotal(unitPrice, quantity int) int {\n\treturn unitPrice + quantity\n}")
	goBoundaryEdit  = goEdit("parcel.ShippingFee", "func ShippingFee(total int) int {\n\tif total > FreeShippingFrom {\n\t\treturn 0\n\t}\n\treturn FlatFee\n}")
	goEquivalent    = goEdit("parcel.ClampPercent", "func ClampPercent(percent int) int {\n\treturn min(MaxPercent, max(0, percent))\n}")
	goBroken        = goEdit("parcel.LineTotal", "func LineTotal(unitPrice, quantity int) int {\n\treturn unitPrice *\n}")
)

func goEdit(qualname, source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: goSourceFile, Qualname: qualname, NewSource: source}}}
}

func goOperations() execution.Operations {
	return execution.Operations{lineTotal: goLineTotalTarget, discounted: goDiscountedTarget, shippingFee: goShippingTarget}
}

func goShippingCall(total string) spec.Call {
	return spec.Call{Operation: shippingFee, Args: map[string]spec.Literal{"total": integer(total)}}
}

func goExamples() []execution.OwnedExample {
	return []execution.OwnedExample{
		{Owner: criterionLine, Example: example(1, lineTotalCall(), returnsInteger(productOfFourBy3))},
		{Owner: criterionShipping, Example: example(1, goShippingCall(goFreeShippingFrom), returnsInteger(shippingWaived))},
	}
}

// allPassing reports every test as passed, whatever the code: the cases below
// look at the examples and the probes, not at the suite.
type allPassing struct{ tests []ecosystem.BoundTest }

func (a allPassing) Run(context.Context, ecosystem.Workspace, ecosystem.Toolchain, ecosystem.Selection) (ecosystem.RunResult, error) {
	outcomes := map[ecosystem.TestID]ecosystem.TestOutcome{}
	for _, test := range a.tests {
		outcomes[test.ID] = ecosystem.TestPassed
	}
	return ecosystem.RunResult{Outcomes: outcomes}, nil
}

// goWorld is the real Go strategy over the fixture project, with the reference
// workspace prepared and built once.
type goWorld struct {
	strategy  *golang.Strategy
	executor  mutantrun.Executor
	reference mutantrun.Reference
	tests     []ecosystem.BoundTest
	sources   []string
}

func openGoWorld(t *testing.T) *goWorld {
	t.Helper()
	if testing.Short() {
		t.Skip("toolchain integration tests are skipped in short mode")
	}
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := golang.New(golang.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: goparser.NewLocator()})
	loaded, err := bindings.Load(ecosystem.GoTesting)
	if err != nil {
		t.Fatal(err)
	}
	tests := loaded.All()
	executor := mutantrun.Executor{
		Ecosystem:  decoratedEcosystem{Ecosystem: strategy, runner: allPassing{tests: tests}},
		Workspaces: manager,
		Policy:     mutation.Policy{SourceRoots: []string{"."}, Layout: strategy.Layout()},
	}
	ctx := context.Background()
	reference, err := executor.PrepareReference(ctx, filepath.Clean(goFixtureDirectory))
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	if build, err := executor.BuildReference(ctx, reference, []string{goSourceFile}); err != nil || !build.OK {
		t.Fatalf("build the reference: %+v %v", build, err)
	}
	source, err := os.ReadFile(filepath.Join(goFixtureDirectory, goSourceFile))
	if err != nil {
		t.Fatal(err)
	}
	return &goWorld{strategy: strategy, executor: executor, reference: reference, tests: tests, sources: []string{string(source)}}
}

func (w *goWorld) runner(options ...func(*execution.Runner)) execution.Runner {
	runner := execution.Runner{
		Executor: w.executor, Reference: w.reference, Tests: w.tests, SuiteKey: suiteKey,
		Prober: w.strategy.Prober(), Operations: goOperations(), Examples: goExamples(),
	}
	for _, option := range options {
		option(&runner)
	}
	return runner
}

func TestExecutionOfAGoProject(t *testing.T) {
	w := openGoWorld(t)
	ctx := context.Background()

	t.Run("the examples pass on the reference, fail on the mutants that break them and skip unbound operations [verifies SP-956~1]", func(t *testing.T) {
		examples := append(goExamples(),
			execution.OwnedExample{Owner: criterionShipping, Example: example(2, goShippingCall(paidShipping), returnsInteger(shippingWaived))},
			execution.OwnedExample{Owner: criterionDiscount, Example: example(1, spec.Call{Operation: unboundOperation}, returnsInteger(shippingWaived))})
		results, err := w.runner(func(r *execution.Runner) { r.Examples = examples }).ReferenceExamples(ctx)
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
		for _, mutant := range []struct {
			spec   execution.MutantSpec
			failed []spec.Ref
		}{
			{execution.MutantSpec{ID: "go-killing", Edits: goLineTotalEdit}, []spec.Ref{criterionLine}},
			{execution.MutantSpec{ID: "go-boundary", Edits: goBoundaryEdit}, []spec.Ref{criterionShipping}},
			{execution.MutantSpec{ID: "go-equivalent", Edits: goEquivalent}, nil},
		} {
			got, err := w.runner().Observe(ctx, mutant.spec)
			if err != nil || !got.Built || !slices.Equal(got.Failed, mutant.failed) {
				t.Fatalf("%s: %+v (%v), want failed examples %v", mutant.spec.ID, got, err, mutant.failed)
			}
		}
	})

	t.Run("the search finds a witness at the boundary a mutant moves and none for an equivalent mutant [verifies SP-956~1]", func(t *testing.T) {
		runner := w.runner()
		draws, reason, err := runner.PrepareDraws(ctx, w.sources, goDrawSeed)
		if err != nil || reason != "" || draws == nil {
			t.Fatalf("draws %+v, reason %q, error %v", draws, reason, err)
		}
		if len(draws.Signatures) != len(goOperations()) || len(draws.Reference) != len(draws.Calls) {
			t.Fatalf("draws %+v", draws)
		}
		boundary, err := runner.Differential(ctx, execution.MutantSpec{ID: "go-diff-boundary", Edits: goBoundaryEdit}, draws, "")
		if err != nil || boundary.Status != power.DiffersAt {
			t.Fatalf("boundary mutant: %+v (%v)", boundary, err)
		}
		atBoundary := slices.ContainsFunc(boundary.Witnesses, func(witness power.Witness) bool {
			return witness.Call.Operation == shippingFee && witness.Call.Args["total"] == integer(goFreeShippingFrom) &&
				witness.Reference.Same(returnsInteger(shippingWaived)) && witness.Mutant.Same(returnsInteger(goFlatFee))
		})
		if !atBoundary {
			t.Fatalf("no witness at the boundary: %+v", boundary.Witnesses)
		}
		same, err := runner.Differential(ctx, execution.MutantSpec{ID: "go-diff-equivalent", Edits: goEquivalent}, draws, "")
		if err != nil || same.Status != power.NoDiffFound || len(same.Witnesses) != 0 {
			t.Fatalf("equivalent mutant: %+v (%v)", same, err)
		}
		skipped, err := runner.Differential(ctx, execution.MutantSpec{ID: "go-diff-broken", Edits: goBroken}, draws, "")
		if err != nil || skipped.Status != power.Skipped || skipped.SkipReason != execution.SkipDoesNotBuild {
			t.Fatalf("broken mutant: %+v (%v)", skipped, err)
		}
	})

	confirmations := []struct {
		name     string
		mutant   execution.MutantSpec
		call     spec.Call
		expected spec.Expectation
		want     power.WitnessCheck
	}{
		{"the reference returns what the judge expects and the mutant does not", execution.MutantSpec{ID: "go-confirm", Edits: goBoundaryEdit}, goShippingCall(goFreeShippingFrom), returnsInteger(shippingWaived), power.WitnessConfirmed},
		{"the reference does not return what the judge expects", execution.MutantSpec{ID: "go-refuse-reference", Edits: goBoundaryEdit}, goShippingCall(goFreeShippingFrom), returnsInteger(goFlatFee), power.WitnessRefused},
		{"the mutant returns what the judge expects too", execution.MutantSpec{ID: "go-refuse-mutant", Edits: goBoundaryEdit}, goShippingCall("100"), returnsInteger(goFlatFee), power.WitnessRefused},
		{"the mutant does not build", execution.MutantSpec{ID: "go-absent-build", Edits: goBroken}, lineTotalCall(), returnsInteger(productOfFourBy3), power.WitnessAbsent},
	}
	for _, tt := range confirmations {
		t.Run("a witness is "+string(tt.want)+" when "+tt.name+" [verifies SP-956~1]", func(t *testing.T) {
			got, err := w.runner().Confirm(ctx, tt.mutant, tt.call, tt.expected)
			if err != nil || got.Check != tt.want || (got.Check == power.WitnessConfirmed) != (got.Digest != "") {
				t.Fatalf("confirmation %+v (%v)", got, err)
			}
		})
	}
}
