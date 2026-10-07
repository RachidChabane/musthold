package execution_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/node"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/execution"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/power"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/bindings"
	"github.com/RachidChabane/spec-proof/discpower/internal/testkit/nodeinstall"
)

const (
	nodeFixturesDirectory = "../../../fixtures/ecosystems"
	nodeSourceFile        = "src/rates.ts"
	nodeSourceRoot        = "src"
	nodeModule            = nodeSourceFile + ":"
	nodeLineTotalTarget   = nodeModule + "lineTotal"
	nodeDiscountedTarget  = nodeModule + "discounted"
	nodeShippingTarget    = nodeModule + "shippingFee"
	nodeFlatFee           = "4.5"
	nodeDrawSeed          = 11
)

var (
	nodeLineTotalEdit = nodeEdit("lineTotal", "function lineTotal(unitPrice: number, quantity: number): number {\n  return unitPrice + quantity;\n}")
	nodeBoundaryEdit  = nodeEdit("shippingFee", "function shippingFee(total: number): number {\n  if (total > FREE_SHIPPING_FROM) {\n    return 0;\n  }\n  return SHIPPING_FEE;\n}")
	nodeEquivalent    = nodeEdit("clampPercent", "function clampPercent(percent: number): number {\n  return Math.min(MAX_PERCENT, Math.max(0, percent));\n}")
	nodeBroken        = nodeEdit("lineTotal", "function lineTotal(unitPrice: number, quantity: number): number {\n  return unitPrice *;\n}")
)

func nodeEdit(qualname, source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: nodeSourceFile, Qualname: qualname, NewSource: source}}}
}

func nodeOperations() execution.Operations {
	return execution.Operations{lineTotal: nodeLineTotalTarget, discounted: nodeDiscountedTarget, shippingFee: nodeShippingTarget}
}

func nodeExamples() []execution.OwnedExample {
	return []execution.OwnedExample{
		{Owner: criterionLine, Example: example(1, lineTotalCall(), returnsInteger(productOfFourBy3))},
		{Owner: criterionShipping, Example: example(1, shippingCall(waivedFrom), returnsInteger(shippingWaived))},
	}
}

func returnsDecimal(value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: spec.TypeDecimal, Value: value}
}

// nodeWorld is the real strategy of a Node runner over its installed fixture
// project, with the reference workspace prepared and built once.
type nodeWorld struct {
	strategy  *node.Strategy
	executor  mutantrun.Executor
	reference mutantrun.Reference
	tests     []ecosystem.BoundTest
	sources   []string
}

func openNodeWorld(t *testing.T, name ecosystem.Name, build func(node.Dependencies) *node.Strategy) *nodeWorld {
	t.Helper()
	project := nodeinstall.Project(t, filepath.Join(nodeFixturesDirectory, string(name)))
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := build(node.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	loaded, err := bindings.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	tests := loaded.All()
	executor := mutantrun.Executor{
		Ecosystem:  decoratedEcosystem{Ecosystem: strategy, runner: allPassing{tests: tests}},
		Workspaces: manager,
		Policy:     mutation.Policy{SourceRoots: []string{nodeSourceRoot}, Layout: strategy.Layout()},
	}
	ctx := context.Background()
	reference, err := executor.PrepareReference(ctx, project)
	if err != nil {
		t.Fatalf("prepare the reference: %v", err)
	}
	if build, err := executor.BuildReference(ctx, reference, []string{nodeSourceFile}); err != nil || !build.OK {
		t.Fatalf("build the reference: %+v %v", build, err)
	}
	source, err := os.ReadFile(filepath.Join(project, filepath.FromSlash(nodeSourceFile)))
	if err != nil {
		t.Fatal(err)
	}
	return &nodeWorld{strategy: strategy, executor: executor, reference: reference, tests: tests, sources: []string{string(source)}}
}

func (w *nodeWorld) runner() execution.Runner {
	return execution.Runner{
		Executor: w.executor, Reference: w.reference, Tests: w.tests, SuiteKey: suiteKey,
		Prober: w.strategy.Prober(), Operations: nodeOperations(), Examples: nodeExamples(),
	}
}

func TestExecutionOfANodeProject(t *testing.T) {
	runners := []struct {
		name  ecosystem.Name
		build func(node.Dependencies) *node.Strategy
	}{{ecosystem.Jest, node.NewJest}, {ecosystem.Vitest, node.NewVitest}}
	for _, framework := range runners {
		t.Run(string(framework.name), func(t *testing.T) { executeNodeProject(t, framework.name, framework.build) })
	}
}

func executeNodeProject(t *testing.T, name ecosystem.Name, build func(node.Dependencies) *node.Strategy) {
	w := openNodeWorld(t, name, build)
	ctx := context.Background()

	t.Run("the examples pass on the reference, fail on the mutants that break them and skip unbound operations [verifies SP-1040~1]", func(t *testing.T) {
		examples := append(nodeExamples(),
			execution.OwnedExample{Owner: criterionShipping, Example: example(2, shippingCall(paidShipping), returnsInteger(shippingWaived))},
			execution.OwnedExample{Owner: criterionDiscount, Example: example(1, spec.Call{Operation: unboundOperation}, returnsInteger(shippingWaived))})
		runner := w.runner()
		runner.Examples = examples
		results, err := runner.ReferenceExamples(ctx)
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
			{execution.MutantSpec{ID: "node-killing", Edits: nodeLineTotalEdit}, []spec.Ref{criterionLine}},
			{execution.MutantSpec{ID: "node-boundary", Edits: nodeBoundaryEdit}, []spec.Ref{criterionShipping}},
			{execution.MutantSpec{ID: "node-equivalent", Edits: nodeEquivalent}, nil},
		} {
			got, err := w.runner().Observe(ctx, mutant.spec)
			if err != nil || !got.Built || !slices.Equal(got.Failed, mutant.failed) {
				t.Fatalf("%s: %+v (%v), want failed examples %v", mutant.spec.ID, got, err, mutant.failed)
			}
		}
	})

	t.Run("the search finds a witness at the boundary a mutant moves and none for an equivalent mutant [verifies SP-1040~1]", func(t *testing.T) {
		runner := w.runner()
		draws, reason, err := runner.PrepareDraws(ctx, w.sources, nodeDrawSeed)
		if err != nil || reason != "" || draws == nil {
			t.Fatalf("draws %+v, reason %q, error %v", draws, reason, err)
		}
		if len(draws.Signatures) != len(nodeOperations()) || len(draws.Reference) != len(draws.Calls) {
			t.Fatalf("draws %+v", draws)
		}
		boundary, err := runner.Differential(ctx, execution.MutantSpec{ID: "node-diff-boundary", Edits: nodeBoundaryEdit}, draws, "")
		if err != nil || boundary.Status != power.DiffersAt {
			t.Fatalf("boundary mutant: %+v (%v)", boundary, err)
		}
		atBoundary := slices.ContainsFunc(boundary.Witnesses, func(witness power.Witness) bool {
			return witness.Call.Operation == shippingFee && witness.Call.Args["total"].Equal(integer(waivedFrom)) &&
				witness.Reference.Same(returnsInteger(shippingWaived)) && witness.Mutant.Same(returnsDecimal(nodeFlatFee))
		})
		if !atBoundary {
			t.Fatalf("no witness at the boundary: %+v", boundary.Witnesses)
		}
		same, err := runner.Differential(ctx, execution.MutantSpec{ID: "node-diff-equivalent", Edits: nodeEquivalent}, draws, "")
		if err != nil || same.Status != power.NoDiffFound || len(same.Witnesses) != 0 {
			t.Fatalf("equivalent mutant: %+v (%v)", same, err)
		}
		skipped, err := runner.Differential(ctx, execution.MutantSpec{ID: "node-diff-broken", Edits: nodeBroken}, draws, "")
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
		{"the reference returns what the judge expects and the mutant does not", execution.MutantSpec{ID: "node-confirm", Edits: nodeBoundaryEdit}, shippingCall(waivedFrom), returnsInteger(shippingWaived), power.WitnessConfirmed},
		{"the reference does not return what the judge expects", execution.MutantSpec{ID: "node-refuse-reference", Edits: nodeBoundaryEdit}, shippingCall(waivedFrom), returnsDecimal(nodeFlatFee), power.WitnessRefused},
		{"the mutant returns what the judge expects too", execution.MutantSpec{ID: "node-refuse-mutant", Edits: nodeBoundaryEdit}, shippingCall("100"), returnsInteger(shippingWaived), power.WitnessRefused},
		{"the mutant does not build", execution.MutantSpec{ID: "node-absent-build", Edits: nodeBroken}, lineTotalCall(), returnsInteger(productOfFourBy3), power.WitnessAbsent},
	}
	for _, tt := range confirmations {
		t.Run("a witness is "+string(tt.want)+" when "+tt.name+" [verifies SP-1040~1]", func(t *testing.T) {
			got, err := w.runner().Confirm(ctx, tt.mutant, tt.call, tt.expected)
			if err != nil || got.Check != tt.want || (got.Check == power.WitnessConfirmed) != (got.Digest != "") {
				t.Fatalf("confirmation %+v (%v)", got, err)
			}
		})
	}
}
