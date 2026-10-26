package golang_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/execution"
	"github.com/RachidChabane/musthold/discpower/internal/app/mutantrun"
	"github.com/RachidChabane/musthold/discpower/internal/domain/mutation"
	"github.com/RachidChabane/musthold/discpower/internal/domain/power"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
	"github.com/RachidChabane/musthold/discpower/internal/domain/witness"
)

const (
	goShippingSpec     = "WHEN an order total reaches FreeShippingFrom THE SYSTEM SHALL make ShippingFee zero."
	goRatesFile        = "rates.go"
	goShippingQualname = "parcel.ShippingFee"
	goImports          = "\"testing\""
	goFreeName         = "TestFreeFromThreshold"
	goFreeCode         = "func TestFreeFromThreshold(t *testing.T) {\n\tif got := ShippingFee(5000); got != 0 {\n\t\tt.Fatalf(\"ShippingFee(5000) = %d\", got)\n\t}\n}"
	goFeeName          = "TestFeeBelowThreshold"
	goFeeCode          = "func TestFeeBelowThreshold(t *testing.T) {\n\tif got := ShippingFee(4999); got != FlatFee {\n\t\tt.Fatalf(\"ShippingFee(4999) = %d\", got)\n\t}\n}"
	goHelperCode       = "func TestWithHelper(t *testing.T) {}\n\nfunc helper() {}"
	goAlwaysCharged    = "func ShippingFee(total int) int {\n\treturn FlatFee\n}"
	goReformatted      = "func ShippingFee(total int) int {\n\n\tif total >= FreeShippingFrom {\n\t\treturn 0\n\t}\n\n\treturn FlatFee\n}"
)

var goShippingCriterion = spec.Ref{ID: "PAR-3", Revision: 1}

func goStrategy() *golang.Strategy {
	return golang.New(golang.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: goparser.NewLocator()})
}

func goShippingSet() witness.Set {
	return witness.Set{Criterion: goShippingCriterion, Imports: goImports, Witnesses: []witness.Witness{
		witness.New(goShippingCriterion, goFreeName, "make ShippingFee zero", witness.KindExample, goFreeCode),
		witness.New(goShippingCriterion, goFeeName, "reaches FreeShippingFrom", witness.KindExample, goFeeCode),
	}}
}

func TestGoWitnessBrief(t *testing.T) {
	project := os.DirFS(filepath.Clean(parcelFixture))

	t.Run("the brief lists the exported declarations the spec mentions without bodies, and names the package [verifies SP-1980~1]", func(t *testing.T) {
		strategy := goStrategy()
		brief, err := strategy.Witnesses().Brief(project, nil, goShippingSpec)
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"example.com/parcel: const FreeShippingFrom", "example.com/parcel: func ShippingFee(total int) int"}
		if !slices.Equal(brief.Interface, want) {
			t.Errorf("interface %q", brief.Interface)
		}
		if !strings.Contains(brief.Conventions, "package parcel") {
			t.Errorf("conventions %q", brief.Conventions)
		}
	})

	t.Run("a set is laid out as an internal test file, each witness renamed and named as gotestsum names it [verifies SP-1980~1]", func(t *testing.T) {
		strategy := goStrategy()
		if _, err := strategy.Witnesses().Brief(project, nil, goShippingSpec); err != nil {
			t.Fatal(err)
		}
		set := goShippingSet()
		file, err := strategy.Witnesses().Layout(project, set)
		if err != nil {
			t.Fatal(err)
		}
		content := string(file.Content)
		if !strings.HasPrefix(file.Path, "zz_musthold_") || !strings.HasSuffix(file.Path, "_test.go") || !strings.HasPrefix(content, "package parcel\n") {
			t.Fatalf("path %q content %q", file.Path, content)
		}
		for _, w := range set.Witnesses {
			test, known := file.Tests[w.ID]
			if !known || test.Classname != "example.com/parcel" || !strings.HasPrefix(test.Name, "Test_") || !strings.Contains(content, "func "+test.Name+"(") {
				t.Errorf("test %+v of %s", test, w.Name)
			}
		}
	})

	t.Run("a witness that declares more than its function is left out [verifies SP-1980~1]", func(t *testing.T) {
		set := goShippingSet()
		set.Witnesses = append(set.Witnesses, witness.New(goShippingCriterion, "TestWithHelper", "helper", witness.KindExample, goHelperCode))
		file, err := goStrategy().Witnesses().Layout(project, set)
		if err != nil {
			t.Fatal(err)
		}
		if _, known := file.Tests[set.Witnesses[2].ID]; known || strings.Contains(string(file.Content), "func helper") {
			t.Errorf("the witness with a helper was laid out")
		}
	})
}

func goLiveRunner(t *testing.T) execution.Runner {
	t.Helper()
	if testing.Short() {
		t.Skip("builds and runs Go tests")
	}
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := goStrategy()
	project := os.DirFS(filepath.Clean(parcelFixture))
	if _, err := strategy.Witnesses().Brief(project, nil, goShippingSpec); err != nil {
		t.Fatal(err)
	}
	executor := mutantrun.Executor{Ecosystem: strategy, Workspaces: manager, Policy: mutation.Policy{SourceRoots: []string{"."}, Layout: strategy.Layout()}}
	ctx := context.Background()
	reference, err := executor.PrepareReference(ctx, filepath.Clean(parcelFixture))
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	built, err := executor.BuildReference(ctx, reference, []string{goRatesFile})
	if err != nil || !built.OK {
		t.Fatalf("the reference does not build: %v %s", err, built.Diagnostics)
	}
	return execution.Runner{
		Executor: executor, Reference: reference, Compiler: strategy.Compiler(),
		Witnesses: execution.Witnesses{Host: strategy.Witnesses(), Project: project},
	}
}

func goShippingEdit(source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: goRatesFile, Qualname: goShippingQualname, Target: mutation.TargetDefinition, NewSource: source}}}
}

func TestGoWitnessRuns(t *testing.T) {
	runner := goLiveRunner(t)
	ctx := context.Background()
	set := goShippingSet()

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
		outcomes, err := runner.EditWitnesses(ctx, "always-charged", goShippingEdit(goAlwaysCharged), []witness.Set{set})
		if err != nil {
			t.Fatal(err)
		}
		if outcomes[set.Witnesses[0].ID] != witness.OutcomeFailed || outcomes[set.Witnesses[1].ID] != witness.OutcomePassed {
			t.Errorf("outcomes %v", outcomes)
		}
	})
}

func TestGoCompiledForms(t *testing.T) {
	runner := goLiveRunner(t)
	ctx := context.Background()
	identity := func(t *testing.T, id, source string) power.Identity {
		t.Helper()
		got, err := runner.Identity(ctx, execution.MutantSpec{ID: id, Edits: goShippingEdit(source)})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}

	t.Run("a change of layout keeps the compiled form [verifies SP-1975~1]", func(t *testing.T) {
		if got := identity(t, "reformatted", goReformatted); got != power.IdentityIdentical {
			t.Errorf("identity %q", got)
		}
	})

	t.Run("a change of behaviour changes the compiled form [verifies SP-1975~1]", func(t *testing.T) {
		if got := identity(t, "always-charged", goAlwaysCharged); got != power.IdentityDiffers {
			t.Errorf("identity %q", got)
		}
	})
}
