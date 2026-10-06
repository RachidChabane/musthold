package ecosystemcontract_test

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/RachidChabane/spec-proof/internal/adapters/ecosystem/golang"
	"github.com/RachidChabane/spec-proof/internal/adapters/ecosystem/pytest"
	"github.com/RachidChabane/spec-proof/internal/adapters/sourceparse/goparser"
	"github.com/RachidChabane/spec-proof/internal/adapters/sourceparse/treesitter"
	"github.com/RachidChabane/spec-proof/internal/app/ports"
	"github.com/RachidChabane/spec-proof/internal/domain/ecosystem"
	"github.com/RachidChabane/spec-proof/internal/domain/mutation"
	"github.com/RachidChabane/spec-proof/internal/testkit/bindings"
	"github.com/RachidChabane/spec-proof/internal/testkit/ecosystemcontract"
)

const (
	fixturesRoot  = "../../../fixtures/ecosystems"
	killing       = "PAR-1~1"
	discount      = "PAR-2~1"
	shipping      = "PAR-3~1"
	pytestSource  = "parcel/rates.py"
	goSource      = "rates.go"
	pytestProbe   = "import pytest"
	pythonCommand = "python3"
)

func edit(file, qualname, source string) mutation.EditSet {
	return mutation.EditSet{Edits: []mutation.Edit{{File: file, Qualname: qualname, NewSource: source}}}
}

func loadBindings(t *testing.T, name ecosystem.Name) bindings.Bindings {
	t.Helper()
	loaded, err := bindings.Load(name)
	if err != nil {
		t.Fatal(err)
	}
	return loaded
}

func requireTool(t *testing.T, name string, args ...string) {
	t.Helper()
	if testing.Short() {
		t.Skip("toolchain integration tests are skipped in short mode")
	}
	if err := exec.Command(name, args...).Run(); err != nil {
		t.Skipf("%s is not usable here: %v", name, err)
	}
}

func pytestSubject(t *testing.T) ecosystemcontract.Subject {
	return ecosystemcontract.Subject{
		Name: "pytest",
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return pytest.New(pytest.Dependencies{Process: a.Process, Reader: a.Reader, Locator: treesitter.NewLocator()})
		},
		FixtureDir:    filepath.Join(fixturesRoot, "pytest"),
		Sources:       []string{pytestSource},
		SourceRoots:   []string{"parcel"},
		BuildOutputs:  "parcel/__pycache__/*.pyc",
		Bindings:      loadBindings(t, ecosystem.Pytest),
		KillingRef:    killing,
		EquivalentRef: discount,
		UnaffectedRef: shipping,
		DisplayRef:    discount,
		UnboundTests:  []ecosystem.TestID{ecosystem.NewTestID("tests.test_rates", "test_line_total_of_no_units_is_zero")},
		Killing:       edit(pytestSource, "line_total", "def line_total(unit_price, quantity):\n    return unit_price + quantity"),
		Equivalent:    edit(pytestSource, "clamp_percent", "def clamp_percent(percent):\n    return min(MAX_PERCENT, max(0, percent))"),
		Broken:        edit(pytestSource, "line_total", "def line_total(unit_price, quantity):\n    return unit_price *"),
		DisplayKilling: edit(pytestSource, "discounted",
			"def discounted(total, percent):\n    kept = MAX_PERCENT + clamp_percent(percent)\n    return round(total * kept / MAX_PERCENT, 2)"),
	}
}

func goSubject(t *testing.T) ecosystemcontract.Subject {
	return ecosystemcontract.Subject{
		Name: "go",
		Build: func(a ecosystemcontract.Adapters) ports.Ecosystem {
			return golang.New(golang.Dependencies{Process: a.Process, Reader: a.Reader, Locator: goparser.NewLocator()})
		},
		FixtureDir:    filepath.Join(fixturesRoot, "go"),
		Sources:       []string{goSource},
		SourceRoots:   []string{"."},
		Bindings:      loadBindings(t, ecosystem.GoTesting),
		KillingRef:    killing,
		EquivalentRef: discount,
		UnaffectedRef: shipping,
		DisplayRef:    discount,
		UnboundTests:  []ecosystem.TestID{ecosystem.NewTestID("example.com/parcel", "TestLineTotal/is_zero_for_no_units")},
		Killing:       edit(goSource, "parcel.LineTotal", "func LineTotal(unitPrice, quantity int) int {\n\treturn unitPrice + quantity\n}"),
		Equivalent:    edit(goSource, "parcel.ClampPercent", "func ClampPercent(percent int) int {\n\treturn min(MaxPercent, max(0, percent))\n}"),
		Broken:        edit(goSource, "parcel.LineTotal", "func LineTotal(unitPrice, quantity int) int {\n\treturn unitPrice *\n}"),
		DisplayKilling: edit(goSource, "parcel.Discounted",
			"func Discounted(total, percent int) int {\n\tkept := MaxPercent + ClampPercent(percent)\n\treturn total * kept / MaxPercent\n}"),
	}
}

// TestEcosystemContract runs the contract of every strategy, one after the
// other: the cases spawn real toolchains and must not overlap.
func TestEcosystemContract(t *testing.T) {
	t.Run("pytest", func(t *testing.T) {
		requireTool(t, pythonCommand, "-c", pytestProbe)
		ecosystemcontract.Run(t, pytestSubject(t))
	})
	t.Run("go", func(t *testing.T) {
		requireTool(t, "go", "version")
		ecosystemcontract.Run(t, goSubject(t))
	})
}
