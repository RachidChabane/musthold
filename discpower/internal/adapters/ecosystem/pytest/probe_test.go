package pytest_test

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/process"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/spec-proof/discpower/internal/app/ports"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/differential"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/discpower/internal/domain/spec"
)

const (
	fixtureDirectory = "../../../../fixtures/ecosystems/pytest"
	ratesModule      = "parcel.rates:"
)

// liveProject is the fixture project prepared in a real workspace, with the real
// interpreter resolved for it.
type liveProject struct {
	strategy  *pytest.Strategy
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
}

func openLiveProject(t *testing.T) liveProject {
	t.Helper()
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := pytest.New(pytest.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: treesitter.NewLocator()})
	ctx := context.Background()
	prepared, err := manager.Prepare(ctx, filepath.Clean(fixtureDirectory), strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Skipf("no python interpreter: %v", err)
	}
	return liveProject{strategy: strategy, workspace: prepared, toolchain: toolchain}
}

func literal(kind spec.LiteralType, value string) spec.Literal {
	return spec.Literal{Type: kind, Value: value}
}

func call(function string, args map[string]spec.Literal) ports.ProbeCall {
	return ports.ProbeCall{Target: ratesModule + function, Args: args}
}

func TestProberSignatures(t *testing.T) {
	project := openLiveProject(t)
	prober := project.strategy.Prober()
	ctx := context.Background()

	t.Run("the parameters and their hints are learned from defaults and names [verifies SP-159~1]", func(t *testing.T) {
		signatures, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{ratesModule + "discounted", ratesModule + "shipping_fee"})
		if err != nil {
			t.Fatal(err)
		}
		want := []ports.ProbeSignature{
			{Target: ratesModule + "discounted", Params: []differential.Param{{Name: "total", Hint: differential.HintUnknown}, {Name: "percent", Hint: differential.HintUnknown}}},
			{Target: ratesModule + "shipping_fee", Params: []differential.Param{{Name: "total", Hint: differential.HintUnknown}}},
		}
		if !reflect.DeepEqual(signatures, want) {
			t.Fatalf("signatures = %+v, want %+v", signatures, want)
		}
	})
	t.Run("annotations and default values give the hints and variadics are skipped [verifies SP-159~1]", func(t *testing.T) {
		source := "import datetime\nfrom decimal import Decimal\n\n" +
			"def typed(a: int, b: float, c: Decimal, d: str, e: bool, f: datetime.date, g=3, h=2.5, i=\"x\", j=None, *args, **kwargs): pass\n"
		if err := os.WriteFile(filepath.Join(project.workspace.Dir, "typed_module.py"), []byte(source), 0o600); err != nil {
			t.Fatal(err)
		}
		signatures, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{"typed_module:typed"})
		if err != nil {
			t.Fatal(err)
		}
		var hints []differential.Hint
		for _, param := range signatures[0].Params {
			hints = append(hints, param.Hint)
		}
		want := []differential.Hint{
			differential.HintInteger, differential.HintDecimal, differential.HintDecimal, differential.HintString,
			differential.HintBoolean, differential.HintDatetime, differential.HintInteger, differential.HintDecimal,
			differential.HintString, differential.HintUnknown,
		}
		if !reflect.DeepEqual(hints, want) {
			t.Fatalf("hints = %v, want %v", hints, want)
		}
	})
	t.Run("an unknown target fails the whole call and names the target [verifies SP-159~1]", func(t *testing.T) {
		_, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{ratesModule + "discounted", ratesModule + "missing"})
		if err == nil || !strings.Contains(err.Error(), "missing") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("an unimportable module fails the call and names the module [verifies SP-159~1]", func(t *testing.T) {
		_, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{"parcel.absent:f"})
		if err == nil || !strings.Contains(err.Error(), "parcel.absent") {
			t.Fatalf("err = %v", err)
		}
	})
}

func TestProberInvoke(t *testing.T) {
	project := openLiveProject(t)
	prober := project.strategy.Prober()
	ctx := context.Background()

	t.Run("integer arguments give an integer result [verifies SP-160~1]", func(t *testing.T) {
		got := invoke(t, prober, project, call("line_total", map[string]spec.Literal{"unit_price": literal(spec.TypeInteger, "4"), "quantity": literal(spec.TypeInteger, "3")}))
		requireReturns(t, got[0], literal(spec.TypeInteger, "12"))
	})
	t.Run("decimal arguments give a numerically equal result [verifies SP-160~1]", func(t *testing.T) {
		got := invoke(t, prober, project, call("discounted", map[string]spec.Literal{"total": literal(spec.TypeDecimal, "19.99"), "percent": literal(spec.TypeInteger, "10")}))
		requireReturns(t, got[0], literal(spec.TypeDecimal, "17.99"))
	})
	t.Run("a raised exception is observed by its class name [verifies SP-160~1]", func(t *testing.T) {
		got := invoke(t, prober, project, call("discounted", map[string]spec.Literal{"total": literal(spec.TypeInteger, "80"), "percent": literal(spec.TypeString, "ten")}))
		if got[0].Kind != spec.Raises || got[0].Value != "TypeError" {
			t.Fatalf("observation = %+v", got[0])
		}
	})
	t.Run("a failing call does not hide the others [verifies SP-160~1]", func(t *testing.T) {
		got := invoke(t, prober, project,
			call("shipping_fee", map[string]spec.Literal{"total": literal(spec.TypeInteger, "50")}),
			call("shipping_fee", map[string]spec.Literal{"total": literal(spec.TypeString, "x")}),
			call("shipping_fee", map[string]spec.Literal{"total": literal(spec.TypeInteger, "10")}))
		requireReturns(t, got[0], literal(spec.TypeInteger, "0"))
		if got[1].Kind != spec.Raises || got[1].Value != "TypeError" {
			t.Fatalf("middle observation = %+v", got[1])
		}
		requireReturns(t, got[2], literal(spec.TypeDecimal, "4.5"))
	})
	t.Run("an unimportable module is a raise of the import error for each call [verifies SP-160~1]", func(t *testing.T) {
		got, err := prober.Invoke(ctx, project.workspace, project.toolchain, []ports.ProbeCall{{Target: "parcel.absent:f"}, {Target: "parcel.absent:g"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, observation := range got {
			if observation.Kind != spec.Raises || observation.Value != "ModuleNotFoundError" {
				t.Fatalf("observation = %+v", observation)
			}
		}
	})
}

func invoke(t *testing.T, prober ports.Prober, project liveProject, calls ...ports.ProbeCall) []spec.Observation {
	t.Helper()
	got, err := prober.Invoke(context.Background(), project.workspace, project.toolchain, calls)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(calls) {
		t.Fatalf("%d observations for %d calls", len(got), len(calls))
	}
	return got
}

func requireReturns(t *testing.T, observation spec.Observation, want spec.Literal) {
	t.Helper()
	got := spec.Literal{Type: observation.Type, Value: observation.Value}
	if observation.Kind != spec.Returns || !got.Equal(want) {
		t.Fatalf("observation = %+v, want a return of %+v", observation, want)
	}
}
