package pytest_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

const (
	witnessImports    = "from parcel.rates import shipping_fee"
	freeWitnessName   = "check_free_from_fifty"
	freeWitnessCode   = "def check_free_from_fifty():\n    assert shipping_fee(50) == 0\n"
	feeWitnessName    = "check_fee_below_fifty"
	feeWitnessCode    = "def check_fee_below_fifty():\n    assert shipping_fee(49.99) == 4.5\n"
	shippingQualname  = "shipping_fee"
	alwaysChargedBody = "def shipping_fee(total):\n    return SHIPPING_FEE"
	reformattedBody   = "def shipping_fee(total):\n\n    if total >= FREE_SHIPPING_FROM:\n        return 0\n\n    return SHIPPING_FEE"
	shippingSpec      = "WHEN an order total reaches 50 THE SYSTEM SHALL waive the shipping_fee."
)

var shippingCriterion = spec.Ref{ID: "PAR-3", Revision: 1}

func newStrategy() *pytest.Strategy {
	return pytest.New(pytest.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
}

func shippingSet() witness.Set {
	return witness.Set{Criterion: shippingCriterion, Imports: witnessImports, Witnesses: []witness.Witness{
		witness.New(shippingCriterion, freeWitnessName, "waive the shipping fee", witness.KindExample, freeWitnessCode),
		witness.New(shippingCriterion, feeWitnessName, "reaches 50", witness.KindExample, feeWitnessCode),
	}}
}

func TestWitnessBrief(t *testing.T) {
	project := os.DirFS(filepath.Clean(fixtureDirectory))
	rates, err := os.ReadFile(filepath.Join(fixtureDirectory, ratesFile))
	if err != nil {
		t.Fatal(err)
	}
	sources := []mutation.SourceFile{{Path: ratesFile, Content: rates}}

	t.Run("the brief lists the headers of the definitions the spec mentions, never a body [verifies SP-1979~1]", func(t *testing.T) {
		brief, err := newStrategy().Witnesses().Brief(project, sources, shippingSpec)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(brief.Interface, []string{"parcel.rates: def shipping_fee(total)"}) {
			t.Errorf("interface %q", brief.Interface)
		}
		for _, entry := range append(slices.Clone(brief.Interface), brief.Helpers...) {
			if strings.Contains(entry, "return") {
				t.Errorf("a body leaked: %q", entry)
			}
		}
		if brief.Language == "" || brief.Conventions == "" {
			t.Error("no language or conventions")
		}
	})

	t.Run("a spec that mentions no definition gets every public one [verifies SP-1979~1]", func(t *testing.T) {
		brief, err := newStrategy().Witnesses().Brief(project, sources, "nothing named here")
		if err != nil {
			t.Fatal(err)
		}
		if len(brief.Interface) != 4 {
			t.Errorf("interface %q", brief.Interface)
		}
	})

	t.Run("a set is laid out beside the first test module, each witness renamed and named as JUnit names it [verifies SP-1979~1]", func(t *testing.T) {
		set := shippingSet()
		file, err := newStrategy().Witnesses().Layout(project, set)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(file.Path, "tests/test_musthold_") || !strings.HasSuffix(file.Path, ".py") {
			t.Fatalf("path %q", file.Path)
		}
		content := string(file.Content)
		if !strings.HasPrefix(content, witnessImports) || strings.Contains(content, "def "+freeWitnessName) {
			t.Errorf("content %q", content)
		}
		for _, w := range set.Witnesses {
			test, known := file.Tests[w.ID]
			if !known || test.Classname != strings.ReplaceAll(strings.TrimSuffix(file.Path, ".py"), "/", ".") || !strings.Contains(content, "def "+test.Name+"(") {
				t.Errorf("test %+v of %s", test, w.Name)
			}
			if line := strings.Split(content, "\n")[test.Line-1]; !strings.Contains(line, test.Name) {
				t.Errorf("line %d is %q", test.Line, line)
			}
		}
	})

	t.Run("a witness that does not declare its function is left out [verifies SP-1979~1]", func(t *testing.T) {
		set := shippingSet()
		set.Witnesses[0].Code = "assert True\n"
		file, err := newStrategy().Witnesses().Layout(project, set)
		if err != nil {
			t.Fatal(err)
		}
		if _, known := file.Tests[set.Witnesses[0].ID]; known || len(file.Tests) != 1 {
			t.Errorf("tests %+v", file.Tests)
		}
	})
}

// liveRunner prepares the fixture as a reference and returns a runner of its
// witnesses.
func liveRunner(t *testing.T) execution.Runner {
	t.Helper()
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := newStrategy()
	executor := mutantrun.Executor{Ecosystem: strategy, Workspaces: manager, Policy: mutation.Policy{SourceRoots: []string{"."}, Layout: strategy.Layout()}}
	reference, err := executor.PrepareReference(context.Background(), filepath.Clean(fixtureDirectory))
	if err != nil {
		t.Skipf("no python interpreter: %v", err)
	}
	built, err := executor.BuildReference(context.Background(), reference, []string{ratesFile})
	if err != nil || !built.OK {
		t.Fatalf("the reference does not build: %v %s", err, built.Diagnostics)
	}
	return execution.Runner{
		Executor: executor, Reference: reference, Compiler: strategy.Compiler(),
		Witnesses: execution.Witnesses{Host: strategy.Witnesses(), Project: os.DirFS(filepath.Clean(fixtureDirectory))},
	}
}

func shippingEdit(source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: ratesFile, Qualname: shippingQualname, Target: mutation.TargetDefinition, NewSource: source}}}
}

func TestWitnessRuns(t *testing.T) {
	if testing.Short() {
		t.Skip("runs pytest")
	}
	runner := liveRunner(t)
	ctx := context.Background()
	set := shippingSet()

	t.Run("a laid-out witness passes the reference three times out of three [verifies SP-1981~1]", func(t *testing.T) {
		runs, err := runner.ReferenceWitnesses(ctx, set)
		if err != nil {
			t.Fatal(err)
		}
		for _, w := range set.Witnesses {
			if witness.ReadingOf(runs[w.ID]) != witness.ReadingPasses {
				t.Errorf("%s ran %v", w.Name, runs[w.ID])
			}
		}
	})

	t.Run("a mutant that breaks what the witness checks fails it, reproduced [verifies SP-1981~1]", func(t *testing.T) {
		outcomes, err := runner.EditWitnesses(ctx, "always-charged", shippingEdit(alwaysChargedBody), []witness.Set{set})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[set.Witnesses[0].ID] != witness.OutcomeFailed || outcomes[set.Witnesses[1].ID] != witness.OutcomePassed {
			t.Errorf("outcomes %v", outcomes)
		}
	})
}

func TestCompiledForms(t *testing.T) {
	if testing.Short() {
		t.Skip("runs the interpreter")
	}
	runner := liveRunner(t)
	ctx := context.Background()
	identity := func(t *testing.T, id, source string) power.Identity {
		t.Helper()
		edits := shippingEdit(source)
		got, err := runner.Identity(ctx, execution.MutantSpec{ID: id, Edits: edits})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("a change of layout keeps the compiled form [verifies SP-1974~1]", func(t *testing.T) {
		if got := identity(t, "reformatted", reformattedBody); got != power.IdentityIdentical {
			t.Errorf("identity %q", got)
		}
	})

	t.Run("a change of behaviour changes the compiled form [verifies SP-1974~1]", func(t *testing.T) {
		if got := identity(t, "always-charged", alwaysChargedBody); got != power.IdentityDiffers {
			t.Errorf("identity %q", got)
		}
	})
}
