package golang_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/RachidChabane/musthold/discpower/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/junitxml"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/process"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/musthold/discpower/internal/adapters/workspace"
	"github.com/RachidChabane/musthold/discpower/internal/app/ports"
	"github.com/RachidChabane/musthold/discpower/internal/domain/differential"
	"github.com/RachidChabane/musthold/discpower/internal/domain/ecosystem"
	"github.com/RachidChabane/musthold/discpower/internal/domain/spec"
)

const (
	parcelFixture = "../../../../fixtures/ecosystems/go"
	parcelPackage = "example.com/parcel:"
)

// liveProject is a project prepared in a real workspace, with the real go command.
type liveProject struct {
	strategy  *golang.Strategy
	workspace ecosystem.Workspace
	toolchain ecosystem.Toolchain
	manager   *workspace.Manager
}

// openProject copies the directory into a workspace, or the Go fixture when the
// directory is empty.
func openProject(t *testing.T, directory string) liveProject {
	t.Helper()
	if testing.Short() {
		t.Skip("the Go prober builds programs with the go command")
	}
	if directory == "" {
		directory = parcelFixture
	}
	manager, err := workspace.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = manager.Close() })
	strategy := golang.New(golang.Dependencies{Process: process.New(), Reader: junitxml.NewReader(), Locator: goparser.NewLocator()})
	ctx := context.Background()
	prepared, err := manager.Prepare(ctx, filepath.Clean(directory), strategy.Layout())
	if err != nil {
		t.Fatal(err)
	}
	toolchain, err := strategy.Toolchain().Resolve(ctx, prepared)
	if err != nil {
		t.Skipf("no go command: %v", err)
	}
	return liveProject{strategy: strategy, workspace: prepared, toolchain: toolchain, manager: manager}
}

func literal(kind spec.LiteralType, value string) spec.Literal {
	return spec.Literal{Type: kind, Value: value}
}

func integer(value string) spec.Literal { return literal(spec.TypeInteger, value) }

func (p liveProject) invoke(t *testing.T, calls ...ports.ProbeCall) []spec.Observation {
	t.Helper()
	got, err := p.strategy.Prober().Invoke(context.Background(), p.workspace, p.toolchain, calls)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(calls) {
		t.Fatalf("%d observations for %d calls", len(got), len(calls))
	}
	return got
}

func returnsOf(kind spec.LiteralType, value string) spec.Observation {
	return spec.Observation{Kind: spec.Returns, Type: kind, Value: value}
}

func raisesOf(name string) spec.Observation { return spec.Observation{Kind: spec.Raises, Value: name} }

func requireObservations(t *testing.T, got []spec.Observation, want ...spec.Observation) {
	t.Helper()
	for index := range want {
		if !got[index].Same(want[index]) {
			t.Errorf("observation %d = %+v, want %+v", index, got[index], want[index])
		}
	}
}

func TestProberSignatures(t *testing.T) {
	project := openProject(t, "")
	prober := project.strategy.Prober()
	ctx := context.Background()

	t.Run("the parameters of a function are named as the spec writes them and typed from the package [verifies SP-950~1]", func(t *testing.T) {
		got, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{parcelPackage + "LineTotal", parcelPackage + "ShippingFee"})
		if err != nil {
			t.Fatal(err)
		}
		want := []ports.ProbeSignature{
			{Target: parcelPackage + "LineTotal", Params: []differential.Param{{Name: "unit_price", Hint: differential.HintInteger}, {Name: "quantity", Hint: differential.HintInteger}}},
			{Target: parcelPackage + "ShippingFee", Params: []differential.Param{{Name: "total", Hint: differential.HintInteger}}},
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("signatures = %+v, want %+v", got, want)
		}
	})

	refusals := []struct {
		name   string
		target string
		want   error
	}{
		{"an absent function", parcelPackage + "Missing", golang.ErrUnsupportedTarget},
		{"an unexported function", parcelPackage + "clampPercent", golang.ErrInvalidTarget},
		{"a constant", parcelPackage + "FlatFee", golang.ErrUnsupportedTarget},
		{"a target without a package", "LineTotal", golang.ErrInvalidTarget},
		{"an absent package", "example.com/parcel/absent:LineTotal", golang.ErrProbeFailed},
	}
	for _, tt := range refusals {
		t.Run("a target naming "+tt.name+" is an error [verifies SP-950~1]", func(t *testing.T) {
			_, err := prober.Signatures(ctx, project.workspace, project.toolchain, []string{tt.target})
			if err == nil || !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func replaceOnce(t *testing.T, source, from, to string) string {
	t.Helper()
	if !strings.Contains(source, from) {
		t.Fatalf("the source has no %q", from)
	}
	return strings.Replace(source, from, to, 1)
}

func TestProberInvoke(t *testing.T) {
	project := openProject(t, "")

	t.Run("integer arguments give an integer result and a call does not hide the others [verifies SP-951~1]", func(t *testing.T) {
		got := project.invoke(t,
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": integer("4"), "quantity": integer("3")}},
			ports.ProbeCall{Target: parcelPackage + "ShippingFee", Args: map[string]spec.Literal{"total": integer("5000")}},
			ports.ProbeCall{Target: parcelPackage + "ShippingFee", Args: map[string]spec.Literal{"total": integer("4999")}},
			ports.ProbeCall{Target: parcelPackage + "Discounted", Args: map[string]spec.Literal{"total": integer("20000"), "percent": integer("10")}})
		requireObservations(t, got, returnsOf(spec.TypeInteger, "12"), returnsOf(spec.TypeInteger, "0"), returnsOf(spec.TypeInteger, "450"), returnsOf(spec.TypeInteger, "18000"))
	})

	t.Run("an argument is found under any spelling of its name [verifies SP-951~1]", func(t *testing.T) {
		for _, names := range [][2]string{{"unitPrice", "quantity"}, {"UnitPrice", "Quantity"}, {"unit_price", "quantity"}} {
			got := project.invoke(t, ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{names[0]: integer("4"), names[1]: integer("3")}})
			requireObservations(t, got, returnsOf(spec.TypeInteger, "12"))
		}
	})

	t.Run("a call that cannot be made raises an argument error [verifies SP-951~1]", func(t *testing.T) {
		got := project.invoke(t,
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": integer("4")}},
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": integer("4"), "quantity": integer("3"), "extra": integer("1")}},
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": literal(spec.TypeString, "x"), "quantity": integer("3")}},
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": literal(spec.TypeNull, "null"), "quantity": integer("3")}},
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": literal(spec.TypeDecimal, "1.5"), "quantity": integer("3")}},
			ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": integer("99999999999999999999"), "quantity": integer("3")}})
		for index, observation := range got {
			if observation.Kind != spec.Raises || observation.Value != "ArgumentError" {
				t.Errorf("observation %d = %+v", index, observation)
			}
		}
	})

	t.Run("a call to nothing is an error of the whole request [verifies SP-951~1]", func(t *testing.T) {
		_, err := project.strategy.Prober().Invoke(context.Background(), project.workspace, project.toolchain, []ports.ProbeCall{{Target: parcelPackage + "Missing"}})
		if err == nil {
			t.Fatal("no error")
		}
	})

	t.Run("no call asks for nothing and builds nothing [verifies SP-951~1]", func(t *testing.T) {
		got, err := project.strategy.Prober().Invoke(context.Background(), project.workspace, project.toolchain, nil)
		if err != nil || len(got) != 0 {
			t.Fatalf("%v %v", got, err)
		}
	})
}

func TestProberSeesTheWorkspaceItIsGiven(t *testing.T) {
	project := openProject(t, "")
	call := ports.ProbeCall{Target: parcelPackage + "LineTotal", Args: map[string]spec.Literal{"unit_price": integer("4"), "quantity": integer("3")}}

	t.Run("the code of the workspace is what runs and the workspace is left as it was [verifies SP-953~1]", func(t *testing.T) {
		before, err := os.ReadDir(project.workspace.Dir)
		if err != nil {
			t.Fatal(err)
		}
		requireObservations(t, project.invoke(t, call), returnsOf(spec.TypeInteger, "12"))
		source, err := project.manager.ReadFile(project.workspace, "rates.go")
		if err != nil {
			t.Fatal(err)
		}
		mutated := replaceOnce(t, string(source), "unitPrice * quantity", "unitPrice + quantity")
		if err := project.manager.WriteFile(project.workspace, project.strategy.Layout(), "rates.go", []byte(mutated)); err != nil {
			t.Fatal(err)
		}
		requireObservations(t, project.invoke(t, call), returnsOf(spec.TypeInteger, "7"))
		after, err := os.ReadDir(project.workspace.Dir)
		if err != nil || len(after) != len(before) {
			t.Fatalf("the workspace holds %d entries, it held %d (%v)", len(after), len(before), err)
		}
	})
}
